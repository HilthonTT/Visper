package broker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrTopicExists is returned by CreateTopic for a topic that already exists.
var ErrTopicExists = errors.New("topic already exists")

// TopicManager handles topic and partition creation and access
type TopicManager struct {
	baseDir string
	topics  map[string]*Topic
	mu      sync.RWMutex
}

// Topic represents a named channel for messages. Its partitions are fixed at
// creation time and never modified afterwards.
type Topic struct {
	name       string
	dir        string
	partitions []*Partition
}

// Partition represents an ordered, immutable sequence of messages
type Partition struct {
	topic     *Topic
	id        int
	file      *os.File
	mu        sync.Mutex
	offset    int64
	dirty     bool // written since the last fsync
	lastSync  time.Time
	syncEvery time.Duration

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// CreateTopic creates a new topic with the specified number of partitions
func (tm *TopicManager) CreateTopic(name string, numPartitions int) (*Topic, error) {
	if numPartitions <= 0 {
		return nil, fmt.Errorf("topic %s: invalid number of partitions %d", name, numPartitions)
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.topics[name]; exists {
		return nil, fmt.Errorf("topic %s: %w", name, ErrTopicExists)
	}

	topicDir := filepath.Join(tm.baseDir, name)
	if err := os.MkdirAll(topicDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create topic directory: %w", err)
	}

	topic := &Topic{
		name:       name,
		dir:        topicDir,
		partitions: make([]*Partition, numPartitions),
	}

	for i := range numPartitions {
		partition, err := createPartition(topic, i, topicDir)
		if err != nil {
			for _, p := range topic.partitions[:i] {
				_ = p.close()
			}
			return nil, fmt.Errorf("failed to create partition %d: %w", i, err)
		}
		topic.partitions[i] = partition
	}

	tm.topics[name] = topic
	return topic, nil
}

// GetTopic returns the topic with the given name.
func (tm *TopicManager) GetTopic(name string) (*Topic, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	topic, exists := tm.topics[name]
	if !exists {
		return nil, fmt.Errorf("topic %s does not exist", name)
	}

	return topic, nil
}

// createPartition creates or opens a partition file
func createPartition(topic *Topic, id int, topicDir string) (*Partition, error) {
	filePath := filepath.Join(topicDir, fmt.Sprintf("partition-%d.log", id))
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open partition file: %w", err)
	}

	// Get current file size as initial offset
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to stat partition file: %w", err)
	}

	partition := &Partition{
		topic:     topic,
		id:        id,
		file:      file,
		offset:    info.Size(),
		syncEvery: 50 * time.Millisecond,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}

	go partition.syncLoop()

	return partition, nil
}

// syncLoop periodically flushes written data to disk. It only fsyncs when
// something was written since the last sync.
func (p *Partition) syncLoop() {
	defer close(p.done)

	ticker := time.NewTicker(p.syncEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			if p.dirty {
				_ = p.syncLocked()
			}
			p.mu.Unlock()
		case <-p.stop:
			return
		}
	}
}

// syncLocked fsyncs the partition file. p.mu must be held.
func (p *Partition) syncLocked() error {
	if err := p.file.Sync(); err != nil {
		return err
	}
	p.dirty = false
	p.lastSync = time.Now()
	return nil
}

// sync fsyncs the partition file.
func (p *Partition) sync() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.syncLocked()
}

// close stops the sync loop, flushes and closes the partition file.
func (p *Partition) close() error {
	var err error
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done

		p.mu.Lock()
		defer p.mu.Unlock()

		err = errors.Join(p.file.Sync(), p.file.Close())
	})
	return err
}

// endOffset returns the offset at which the next message will be written.
func (p *Partition) endOffset() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.offset
}
