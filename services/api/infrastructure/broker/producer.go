package broker

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"time"
)

// Producer publishes messages to topics
type Producer struct {
	broker      *Broker
	acks        int // 0=no ack, 1=leader ack (fsync before returning)
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
	_, _ = h.Write(key)
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

// Produce sends a message to the specified topic and returns the partition
// and offset it was written to.
func (p *Producer) Produce(topicName string, msg *Message) (int, int64, error) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	topic, err := p.broker.GetTopic(topicName)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to get topic: %w", err)
	}

	partitionID := p.partitioner(msg.Key, len(topic.partitions))
	partition := topic.partitions[partitionID]

	offset, err := partition.writeMessage(msg)
	if err != nil {
		return -1, -1, fmt.Errorf("failed to write message: %w", err)
	}

	if p.acks > 0 {
		if err := partition.sync(); err != nil {
			return partitionID, offset, fmt.Errorf("failed to sync message: %w", err)
		}
	}

	return partitionID, offset, nil
}
