package broker

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"time"
)

// AckMode defines the producer acknowledgment levels
type AckMode int

const (
	// AckNone means no acknowledgment is required
	AckNone AckMode = 0

	// AckLeader means the leader must acknowledge
	AckLeader AckMode = 1

	// AckAll means all replicas must acknowledge (not implemented in this version)
	AckAll AckMode = -1
)

// ProduceOptions defines options for producing messages
type ProduceOptions struct {
	AckMode AckMode
	Timeout time.Duration
}

// ProduceResult represents the result of a produce operation
type ProduceResult struct {
	Topic     string
	Partition int
	Offset    int64
	Error     error
}

// Producer publishes messages to topics
type Producer struct {
	broker      *Broker
	acks        int // 0=no ack, 1=leader ack
	partitioner PartitionStrategy
}

// PartitionStrategy determines which partition a message is sent to
type PartitionStrategy func(key []byte, numPartitions int) int

// DefaultPartitioner implements a simple hash-based partitioning strategy
func DefaultPartitioner(key []byte, numPartitions int) int {
	if numPartitions <= 0 {
		return 0
	}
	if len(key) == 0 {
		return rand.IntN(numPartitions)
	}

	h := fnv.New32a()
	h.Write(key)
	return int(h.Sum32() % uint32(numPartitions))
}

// NewProducer creates a new producer
func NewProducer(broker *Broker, acks int) *Producer {
	return &Producer{
		broker:      broker,
		acks:        acks,
		partitioner: DefaultPartitioner,
	}
}

// Produce sends a message to the specified topic
func (p *Producer) Produce(tropicName string, msg *Message) (int, int64, error) {
	// Set message timestamp if not set
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	topic, err := p.broker.GetTopic(tropicName)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to get topic: %w", err)
	}

	// Determine partition
	numPartitions := len(topic.partitions)
	partitionID := p.partitioner(msg.Key, numPartitions)

	// Get partition
	partition := topic.partitions[partitionID]

	// Write message to partition
	offset, err := partition.writeMessage(msg)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to write message: %w", err)
	}

	if p.acks > 0 {
		partition.mu.Lock()
		partition.file.Sync()
		partition.lastSync = time.Now()
		partition.mu.Unlock()
	}

	return partitionID, offset, nil
}

// ProduceAsync sends a message asynchronously
func (p *Producer) ProduceAsync(topicName string, msg *Message, options ProduceOptions) <-chan ProduceResult {
	resultCh := make(chan ProduceResult, 1)

	go func() {
		// Apply timeout
		_, cancel := context.WithTimeout(context.Background(), options.Timeout)
		defer cancel()

		// Get topic
		topic, err := p.broker.GetTopic(topicName)
		if err != nil {
			resultCh <- ProduceResult{
				Topic: topicName,
				Error: fmt.Errorf("failed to get topic: %w", err),
			}
			return
		}

		// Determine partition
		numPartitions := len(topic.partitions)
		partitionID := p.partitioner(msg.Key, numPartitions)

		// Get partition
		partition := topic.partitions[partitionID]

		// Write message to partition
		offset, err := partition.writeMessage(msg)
		if err != nil {
			resultCh <- ProduceResult{
				Topic:     topicName,
				Partition: partitionID,
				Error:     fmt.Errorf("failed to write message: %w", err),
			}
			return
		}

		// Handle acknowledgment
		switch options.AckMode {
		case AckLeader:
			// Sync to disk
			partition.mu.Lock()
			partition.file.Sync()
			partition.lastSync = time.Now()
			partition.mu.Unlock()
		case AckNone:
			// No sync required
		}

		// Send result
		resultCh <- ProduceResult{
			Topic:     topicName,
			Partition: partitionID,
			Offset:    offset,
			Error:     nil,
		}
	}()

	return resultCh
}
