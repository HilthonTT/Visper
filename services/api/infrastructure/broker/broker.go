package broker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Broker coordinates the message flow between producers and consumers
type Broker struct {
	topicManager *TopicManager
}

func NewBroker(dataDir string) (*Broker, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	broker := &Broker{
		topicManager: &TopicManager{
			baseDir: dataDir,
			topics:  make(map[string]*Topic),
		},
	}

	// Load existing topics
	if err := broker.loadTopics(); err != nil {
		_ = broker.Close()
		return nil, fmt.Errorf("failed to load topics: %w", err)
	}

	return broker, nil
}

// loadTopics loads existing topics from disk
func (b *Broker) loadTopics() error {
	entries, err := os.ReadDir(b.topicManager.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read data directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		topicName := entry.Name()
		topicDir := filepath.Join(b.topicManager.baseDir, topicName)

		partitionFiles, err := filepath.Glob(filepath.Join(topicDir, "partition-*.log"))
		if err != nil {
			return fmt.Errorf("failed to glob partition files: %w", err)
		}

		if numPartitions := len(partitionFiles); numPartitions > 0 {
			if _, err := b.topicManager.CreateTopic(topicName, numPartitions); err != nil {
				return fmt.Errorf("failed to load topic %s: %w", topicName, err)
			}
		}
	}

	return nil
}

// CreateTopic creates a new topic. It returns ErrTopicExists if the topic is
// already present (e.g. loaded from disk).
func (b *Broker) CreateTopic(name string, numPartitions int) error {
	_, err := b.topicManager.CreateTopic(name, numPartitions)
	return err
}

// GetTopic gets a topic by name
func (b *Broker) GetTopic(name string) (*Topic, error) {
	return b.topicManager.GetTopic(name)
}

// ListTopics lists all topics in name order.
func (b *Broker) ListTopics() []string {
	b.topicManager.mu.RLock()
	defer b.topicManager.mu.RUnlock()

	topics := make([]string, 0, len(b.topicManager.topics))
	for topicName := range b.topicManager.topics {
		topics = append(topics, topicName)
	}
	sort.Strings(topics)

	return topics
}

// Close stops the background sync goroutines and closes every partition file.
func (b *Broker) Close() error {
	b.topicManager.mu.Lock()
	defer b.topicManager.mu.Unlock()

	var errs []error
	for _, topic := range b.topicManager.topics {
		for _, p := range topic.partitions {
			if err := p.close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}
