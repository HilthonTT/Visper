package broker

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"log"
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

// RecoverPartition recovers a partition after a crash
func RecoverPartition(topicDir string, partitionID int) (*Partition, error) {
	filePath := filepath.Join(topicDir, fmt.Sprintf("partition-%d.log", partitionID))

	// Open the file
	file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open partition file: %w", err)
	}

	// Scan the file to find the valid end
	var offset int64 = 0
	reader := bufio.NewReader(file)

	for {
		// Remember current position
		currentOffset := offset

		// Read message size
		sizeBytes := make([]byte, 8)
		_, err := io.ReadFull(reader, sizeBytes)
		if err != nil {
			if err == io.EOF {
				// Reached the end of file, file is valid
				break
			}
			if err == io.ErrUnexpectedEOF {
				// Partial write detected, truncate file here
				log.Printf("Partial write detected at offset %d, truncating", currentOffset)
				file.Truncate(currentOffset)
				offset = currentOffset
				break
			}
			return nil, fmt.Errorf("failed to read message size: %w", err)
		}

		// Parse message size
		messageSize := binary.BigEndian.Uint64(sizeBytes)

		// Skip message content
		if _, err := io.CopyN(io.Discard, reader, int64(messageSize)); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				// Partial write detected, truncate file here
				log.Printf("Partial message detected at offset %d, truncating", currentOffset)
				file.Truncate(currentOffset)
				offset = currentOffset
				break
			}
			return nil, fmt.Errorf("failed to skip message: %w", err)
		}

		// Update offset
		offset = currentOffset + 8 + int64(messageSize)
	}

	// Seek to the end
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to end: %w", err)
	}

	// Create partition
	partition := &Partition{
		id:        partitionID,
		file:      file,
		offset:    offset,
		syncEvery: 50 * time.Millisecond,
	}

	// Start background sync goroutine
	go partition.syncLoop()

	return partition, nil
}

// Message format with checksum
// ┌────────┬────────┬─────────┬─────────────────┬──────────┐
// │ Length │ Header │ Checksum│ Message Payload │ Checksum │
// │ (8B)   │ (var)  │  (4B)   │      (var)      │   (4B)   │
// └────────┴────────┴─────────┴─────────────────┴──────────┘

// writeMessageWithIntegrity writes a message with checksum
func (p *Partition) WriteMessageWithIntegrity(msg *Message) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Record current offset
	currentOffset := p.offset

	// Serialize message header and payload
	headerBytes, payloadBytes, err := serializeMessageParts(msg)
	if err != nil {
		return -1, err
	}

	// Calculate checksums
	headerChecksum := crc32.ChecksumIEEE(headerBytes)
	payloadChecksum := crc32.ChecksumIEEE(payloadBytes)

	// Calculate total message size
	totalSize := uint64(len(headerBytes) + 4 + len(payloadBytes) + 4) // header + header checksum + payload + payload checksum

	// Write length prefix
	sizeBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sizeBytes, totalSize)
	if _, err := p.file.Write(sizeBytes); err != nil {
		return -1, fmt.Errorf("failed to write message size: %w", err)
	}

	// Write header
	if _, err := p.file.Write(headerBytes); err != nil {
		return -1, fmt.Errorf("failed to write message header: %w", err)
	}

	// Write header checksum
	checksumBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(checksumBytes, headerChecksum)
	if _, err := p.file.Write(checksumBytes); err != nil {
		return -1, fmt.Errorf("failed to write header checksum: %w", err)
	}

	// Write payload
	if _, err := p.file.Write(payloadBytes); err != nil {
		return -1, fmt.Errorf("failed to write message payload: %w", err)
	}

	// Write payload checksum
	binary.BigEndian.PutUint32(checksumBytes, payloadChecksum)
	if _, err := p.file.Write(checksumBytes); err != nil {
		return -1, fmt.Errorf("failed to write payload checksum: %w", err)
	}

	// Update partition offset
	p.offset += int64(8 + totalSize)

	// Schedule sync if needed
	if time.Since(p.lastSync) >= p.syncEvery {
		p.file.Sync()
		p.lastSync = time.Now()
	}

	return currentOffset, nil
}

// readMessageWithIntegrity reads a message with integrity checking
func (p *Partition) ReadMessageWithIntegrity(offset int64) (*Message, int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Seek to the offset
	if _, err := p.file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, fmt.Errorf("failed to seek to offset: %w", err)
	}

	// Read message size
	sizeBytes := make([]byte, 8)
	if _, err := io.ReadFull(p.file, sizeBytes); err != nil {
		return nil, offset, fmt.Errorf("failed to read message size: %w", err)
	}
	totalSize := binary.BigEndian.Uint64(sizeBytes)

	// Read the full message
	msgBytes := make([]byte, totalSize)
	if _, err := io.ReadFull(p.file, msgBytes); err != nil {
		return nil, offset, fmt.Errorf("failed to read message: %w", err)
	}

	// Extract header length (first 4 bytes of message)
	headerLength := binary.BigEndian.Uint32(msgBytes[0:4])

	// Extract parts
	headerBytes := msgBytes[0:headerLength]
	headerChecksumBytes := msgBytes[headerLength : headerLength+4]
	payloadBytes := msgBytes[headerLength+4 : len(msgBytes)-4]
	payloadChecksumBytes := msgBytes[len(msgBytes)-4:]

	// Verify checksums
	headerChecksum := binary.BigEndian.Uint32(headerChecksumBytes)
	if calculatedHeaderChecksum := crc32.ChecksumIEEE(headerBytes); calculatedHeaderChecksum != headerChecksum {
		return nil, offset, fmt.Errorf("header checksum mismatch")
	}

	payloadChecksum := binary.BigEndian.Uint32(payloadChecksumBytes)
	if calculatedPayloadChecksum := crc32.ChecksumIEEE(payloadBytes); calculatedPayloadChecksum != payloadChecksum {
		return nil, offset, fmt.Errorf("payload checksum mismatch")
	}

	// Parse message
	msg, err := deserializeMessage(headerBytes, payloadBytes)
	if err != nil {
		return nil, offset, fmt.Errorf("failed to deserialize message: %w", err)
	}

	// Calculate next offset
	nextOffset := offset + int64(8) + int64(totalSize)

	return msg, nextOffset, nil
}
