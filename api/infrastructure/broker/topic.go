package broker

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TopicManager handles topic and partition creation and access
type TopicManager struct {
	baseDir string
	topics  map[string]*Topic
	mu      sync.RWMutex
}

// Topic represents a named channel for messages
type Topic struct {
	name       string
	partitions []*Partition
	mu         sync.RWMutex
}

// Partition represents an ordered, immutable sequence of messages
type Partition struct {
	topic     *Topic
	id        int
	file      *os.File
	mu        sync.Mutex
	offset    int64
	lastSync  time.Time
	syncEvery time.Duration
}

// CreateTopic creates a new topic with the specified number of partitions
func (tm *TopicManager) CreateTopic(name string, numPartitions int) (*Topic, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if the topic already exists
	if _, exists := tm.topics[name]; exists {
		return nil, fmt.Errorf("topic %s already exists", name)
	}

	// Create the topic directory
	topicDir := filepath.Join(tm.baseDir, name)
	if err := os.MkdirAll(topicDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create topic directory: %w", err)
	}

	// Create topic and its partitions
	topic := &Topic{
		name:       name,
		partitions: make([]*Partition, numPartitions),
	}

	for i := range numPartitions {
		partition, err := createPartition(topic, i, topicDir)
		if err != nil {
			return nil, fmt.Errorf("failed to create partition %d: %w", i, err)
		}
		topic.partitions[i] = partition
	}

	tm.topics[name] = topic
	return topic, nil
}

// createPartition creates a new partition file
func createPartition(topic *Topic, id int, topicDir string) (*Partition, error) {
	// Create or open partition file
	filePath := filepath.Join(topicDir, fmt.Sprintf("partition-%d.log", id))
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open partition file: %w", err)
	}

	// Get current file size as initial offset
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat partition file: %w", err)
	}

	partition := &Partition{
		topic:     topic,
		id:        id,
		file:      file,
		offset:    info.Size(),
		syncEvery: 50 * time.Millisecond,
	}

	// Start background sync goroutine
	go partition.syncLoop()

	return partition, nil
}

// syncLoop periodically syncs partition data to disk
func (p *Partition) syncLoop() {
	ticker := time.NewTicker(p.syncEvery)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.Lock()

		if time.Since(p.lastSync) >= p.syncEvery {
			p.file.Sync()
			p.lastSync = time.Now()
		}

		p.mu.Unlock()
	}
}
