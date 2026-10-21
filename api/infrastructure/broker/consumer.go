package broker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Consumer reads messages from topics. Committed offsets are persisted per
// consumer group next to the topic data, so a restarted consumer resumes where
// it left off instead of replaying the whole log.
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

	topic, err := c.broker.GetTopic(topicName)
	if err != nil {
		return fmt.Errorf("failed to get topic: %w", err)
	}

	if _, exists := c.offsets[topicName]; !exists {
		c.offsets[topicName] = make(map[int]int64)
	}

	stored, err := c.loadOffsets(topic)
	if err != nil {
		log.Printf("broker: ignoring unreadable offsets for group %s, topic %s: %v", c.groupID, topicName, err)
		stored = nil
	}

	for _, partition := range topic.partitions {
		if _, exists := c.offsets[topicName][partition.id]; exists {
			continue
		}

		offset := stored[partition.id]
		if end := partition.endOffset(); offset < 0 || offset > end {
			// The log was truncated or replaced; do not read past its end.
			offset = end
		}
		c.offsets[topicName][partition.id] = offset
	}

	return nil
}

// Poll returns at most one message per partition of each subscribed topic.
// timeoutMs is currently unused: Poll never blocks.
func (c *Consumer) Poll(timeoutMs int) ([]*ConsumerRecord, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var records []*ConsumerRecord

	for topicName, partitionOffsets := range c.offsets {
		topic, err := c.broker.GetTopic(topicName)
		if err != nil {
			return nil, fmt.Errorf("failed to get topic: %w", err)
		}

		advanced := false
		for partitionID, offset := range partitionOffsets {
			partition := topic.partitions[partitionID]

			if offset >= partition.endOffset() {
				continue
			}

			msg, nextOffset, err := partition.readMessage(offset)
			if err != nil {
				if errors.Is(err, io.EOF) {
					continue
				}
				if errors.Is(err, ErrCorruptRecord) {
					// Record boundaries are unknown past a corrupt record; skip to
					// the end of the partition instead of retrying forever.
					end := partition.endOffset()
					log.Printf("broker: %v; skipping partition %s/%d to offset %d", err, topicName, partitionID, end)
					partitionOffsets[partitionID] = end
					advanced = true
					continue
				}
				return nil, fmt.Errorf("failed to read message: %w", err)
			}

			records = append(records, &ConsumerRecord{
				Topic:     topicName,
				Partition: partitionID,
				Offset:    offset,
				Key:       msg.Key,
				Value:     msg.Value,
				Timestamp: msg.Timestamp,
			})

			partitionOffsets[partitionID] = nextOffset
			advanced = true
		}

		if advanced && c.autoCommit {
			if err := c.commitTopic(topic, partitionOffsets); err != nil {
				log.Printf("broker: failed to commit offsets for group %s, topic %s: %v", c.groupID, topicName, err)
			}
		}
	}

	return records, nil
}

// CommitOffsets commits the current offsets to storage
func (c *Consumer) CommitOffsets() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for topicName, partitionOffsets := range c.offsets {
		topic, err := c.broker.GetTopic(topicName)
		if err != nil {
			return err
		}
		if err := c.commitTopic(topic, partitionOffsets); err != nil {
			return err
		}
	}

	return nil
}

func (c *Consumer) offsetsPath(topic *Topic) string {
	return filepath.Join(topic.dir, "offsets-"+filepath.Base(c.groupID)+".json")
}

// commitTopic atomically persists the offsets of one topic.
func (c *Consumer) commitTopic(topic *Topic, offsets map[int]int64) error {
	encoded := make(map[string]int64, len(offsets))
	for partition, offset := range offsets {
		encoded[strconv.Itoa(partition)] = offset
	}

	data, err := json.Marshal(encoded)
	if err != nil {
		return err
	}

	path := c.offsetsPath(topic)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadOffsets reads the committed offsets of a topic. A missing file yields
// no offsets (start from the beginning).
func (c *Consumer) loadOffsets(topic *Topic) (map[int]int64, error) {
	data, err := os.ReadFile(c.offsetsPath(topic))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var encoded map[string]int64
	if err := json.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}

	offsets := make(map[int]int64, len(encoded))
	for key, offset := range encoded {
		partition, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("invalid partition %q: %w", key, err)
		}
		offsets[partition] = offset
	}

	return offsets, nil
}
