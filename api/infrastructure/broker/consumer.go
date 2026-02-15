package broker

import (
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

// Consumer reads messages from topics
type Consumer struct {
	broker     *Broker
	groupID    string
	offsets    map[string]map[int]int64 // topic -> partition -> offset
	mu         sync.Mutex
	autoCommit bool
}

// ConsumerRecord represents a consumed message with metadata
type ConsumerRecord struct {
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
	Value     []byte
	Timestamp time.Time
}

// NewConsumer creates a new consumer
func NewConsumer(broker *Broker, groupID string) *Consumer {
	return &Consumer{
		broker:     broker,
		groupID:    groupID,
		offsets:    make(map[string]map[int]int64),
		autoCommit: true,
	}
}

// Subscribe subscribes to a topic
func (c *Consumer) Subscribe(topicName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Get topic
	topic, err := c.broker.GetTopic(topicName)
	if err != nil {
		return fmt.Errorf("failed to get topic: %w", err)
	}

	// Initialize offsets for this topic
	if _, exists := c.offsets[topicName]; !exists {
		c.offsets[topicName] = make(map[int]int64)
	}

	// Initialize offsets for each partition
	for _, partition := range topic.partitions {
		// If we already have an offset for this partition, use it
		if _, exists := c.offsets[topicName][partition.id]; exists {
			continue
		}

		// Otherwise, load from offset store or start from beginning
		offset, err := c.loadOffset(topicName, partition.id)
		if err != nil {
			// If no stored offset, start from beginning
			c.offsets[topicName][partition.id] = 0
		} else {
			c.offsets[topicName][partition.id] = offset
		}
	}

	return nil
}

func (c *Consumer) Poll(timeoutMs int) ([]*ConsumerRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var records []*ConsumerRecord

	for topicName, partitionOffsets := range c.offsets {
		topic, err := c.broker.GetTopic(topicName)
		if err != nil {
			return nil, fmt.Errorf("failed to get topic: %w", err)
		}

		for partitionID, offset := range partitionOffsets {
			partition := topic.partitions[partitionID]

			// Check if we've reached the end of the partition
			if offset >= partition.offset {
				continue
			}

			// Read message
			msg, nextOffset, err := partition.readMessage(offset)
			if err != nil {
				// Skip corrupted messages
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					// Update offset to skip this message
					c.offsets[topicName][partitionID] = partition.offset
					continue
				}
				return nil, fmt.Errorf("failed to read message: %w", err)
			}

			// Create consumer record
			record := &ConsumerRecord{
				Topic:     topicName,
				Partition: partitionID,
				Offset:    offset,
				Key:       msg.Key,
				Value:     msg.Value,
				Timestamp: msg.Timestamp,
			}

			records = append(records, record)

			// Update offset
			c.offsets[topicName][partitionID] = nextOffset

			// Auto-commit offset if enabled
			if c.autoCommit {
				c.commitOffset(topicName, partitionID, nextOffset)
			}

			// Only read one message per partition per poll to be fair
			break
		}
	}

	return records, nil
}

// CommitOffsets commits the current offsets to storage
func (c *Consumer) CommitOffsets() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for topicName, partitionOffsets := range c.offsets {
		for partitionID, offset := range partitionOffsets {
			if err := c.commitOffset(topicName, partitionID, offset); err != nil {
				return err
			}
		}
	}

	return nil
}

// commitOffset commits a single offset to storage
func (c *Consumer) commitOffset(topic string, partition int, offset int64) error {
	// In a real implementation, this would persist the offset
	// to a file or database. For simplicity, we're just logging it.
	log.Printf("Committed offset %d for topic %s, partition %d",
		offset, topic, partition)
	return nil
}

// loadOffset loads a committed offset from storage
func (c *Consumer) loadOffset(topic string, partition int) (int64, error) {
	// In a real implementation, this would load the offset from
	// a file or database. For simplicity, we're returning an error.
	return 0, fmt.Errorf("no stored offset")
}
