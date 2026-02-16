package broker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ReplicatedPartition represents a partition with replication
type ReplicatedPartition struct {
	partition *Partition
	replicaID int
	isLeader  bool
	replicas  []*PartitionReplica
	replicaCh chan *Message
	done      chan struct{}
}

// PartitionReplica represents a remote partition replica
type PartitionReplica struct {
	id         int
	client     *ReplicaClient
	lastOffset int64
}

// ReplicaClient handles communication with a replica
type ReplicaClient struct {
	address string
	client  *http.Client
}

// NewReplicatedPartition creates a new replicated partition
func NewReplicatedPartition(partition *Partition, replicaID int, isLeader bool, replicaAddresses []string) *ReplicatedPartition {
	rp := &ReplicatedPartition{
		partition: partition,
		replicaID: replicaID,
		isLeader:  isLeader,
		replicaCh: make(chan *Message, 1000),
		done:      make(chan struct{}),
	}

	// Create replica clients
	for id, address := range replicaAddresses {
		// Skip self
		if id == replicaID {
			continue
		}

		client := &ReplicaClient{
			address: address,
			client:  &http.Client{Timeout: 5 * time.Second},
		}

		replica := &PartitionReplica{
			id:     id,
			client: client,
		}

		rp.replicas = append(rp.replicas, replica)
	}

	// Start replication if leader
	if isLeader {
		go rp.replicationLoop()
	}

	return rp
}

// Write writes a message to the partition and replicates if needed
func (rp *ReplicatedPartition) Write(msg *Message) (int64, error) {
	// Write to local partition
	offset, err := rp.partition.writeMessage(msg)
	if err != nil {
		return -1, err
	}

	// Replicate if leader
	if rp.isLeader {
		rp.replicaCh <- msg
	}

	return offset, nil
}

// replicationLoop sends messages to replicas
func (rp *ReplicatedPartition) replicationLoop() {
	for {
		select {
		case msg := <-rp.replicaCh:
			// Send to all replicas
			for _, replica := range rp.replicas {
				go rp.sendToReplica(replica, msg)
			}
		case <-rp.done:
			return
		}
	}
}

// sendToReplica sends a message to a specific replica
func (rp *ReplicatedPartition) sendToReplica(replica *PartitionReplica, msg *Message) {
	// Serialize message
	msgBytes, err := serializeMessage(msg)
	if err != nil {
		log.Printf("Failed to serialize message for replica %d: %v", replica.id, err)
		return
	}

	// Create request
	url := fmt.Sprintf("http://%s/replicate", replica.client.address)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(msgBytes))
	if err != nil {
		log.Printf("Failed to create request for replica %d: %v", replica.id, err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Replica-ID", strconv.Itoa(rp.replicaID))
	req.Header.Set("X-Topic", rp.partition.topic.name)
	req.Header.Set("X-Partition", strconv.Itoa(rp.partition.id))

	// Send request
	resp, err := replica.client.client.Do(req)
	if err != nil {
		log.Printf("Failed to send message to replica %d: %v", replica.id, err)
		return
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Failed to replicate to %d: %s - %s", replica.id, resp.Status, body)
		return
	}

	// Parse response for new offset
	var result struct {
		Offset int64 `json:"offset"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Failed to parse replica %d response: %v", replica.id, err)
		return
	}

	// Update last offset
	replica.lastOffset = result.Offset
}

// Close stops replication
func (rp *ReplicatedPartition) Close() error {
	close(rp.done)
	return nil
}
