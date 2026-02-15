package broker

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Broker coordinates the message flow between producers and consumers
type Broker struct {
	topicManager *TopicManager
	mu           sync.RWMutex
}

func NewBroker(dataDir string) (*Broker, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create topic manager
	topicManager := &TopicManager{
		baseDir: dataDir,
		topics:  make(map[string]*Topic),
	}

	// Create broker
	broker := &Broker{
		topicManager: topicManager,
	}

	// Load existing topics
	if err := broker.loadTopics(); err != nil {
		return nil, fmt.Errorf("failed to load topics: %w", err)
	}

	return broker, nil
}

// loadTopics loads existing topics from disk
func (b *Broker) loadTopics() error {
	// Read topic directories
	entries, err := os.ReadDir(b.topicManager.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read data directory: %w", err)
	}

	// Process each topic directory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		topicName := entry.Name()
		topicDir := filepath.Join(b.topicManager.baseDir, topicName)

		// Find partition files
		partitionFiles, err := filepath.Glob(filepath.Join(topicDir, "partition-*.log"))
		if err != nil {
			return fmt.Errorf("failed to glob partition files: %w", err)
		}

		// Create topic with the correct number of partitions
		numPartitions := len(partitionFiles)
		if numPartitions > 0 {
			_, err := b.topicManager.CreateTopic(topicName, numPartitions)
			if err != nil {
				return fmt.Errorf("failed to load topic %s: %w", topicName, err)
			}
		}
	}

	return nil
}

// CreateTopic creates a new topic
func (b *Broker) CreateTopic(name string, numPartitions int) error {
	_, err := b.topicManager.CreateTopic(name, numPartitions)
	return err
}

// GetTopic gets a topic by name
func (b *Broker) GetTopic(name string) (*Topic, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	topic, exists := b.topicManager.topics[name]
	if !exists {
		return nil, fmt.Errorf("topic %s does not exist", name)
	}

	return topic, nil
}

// ListTopics lists all topics
func (b *Broker) ListTopics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	topics := make([]string, 0, len(b.topicManager.topics))
	for topicName := range b.topicManager.topics {
		topics = append(topics, topicName)
	}

	return topics
}

func serializeMessage(msg *Message) ([]byte, error) {
	// Format matches your existing writeMessage:
	// [size(8)][keySize(4)][timestamp(8)][key][value]

	keyLen := len(msg.Key)
	valueLen := len(msg.Value)

	// Header size: keySize(4) + timestamp(8) = 12 bytes
	headerSize := 12
	totalSize := uint64(headerSize + keyLen + valueLen)

	// Allocate buffer: size prefix + header + key + value
	buf := make([]byte, 8+totalSize)
	offset := 0

	// Write total size (8 bytes)
	binary.BigEndian.PutUint64(buf[offset:], totalSize)
	offset += 8

	// Write key size (4 bytes)
	binary.BigEndian.PutUint32(buf[offset:], uint32(keyLen))
	offset += 4

	// Write timestamp (8 bytes)
	binary.BigEndian.PutUint64(buf[offset:], uint64(msg.Timestamp.UnixNano()))
	offset += 8

	// Write key
	if keyLen > 0 {
		copy(buf[offset:], msg.Key)
		offset += keyLen
	}

	// Write value
	copy(buf[offset:], msg.Value)

	return buf, nil
}
