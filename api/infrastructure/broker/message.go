package broker

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"
)

// Message represents a single message in the system
type Message struct {
	Key       []byte
	Value     []byte
	Timestamp time.Time
}

// MessageHeader contains metadata about a message
type MessageHeader struct {
	KeySize   uint32
	Timestamp int64
}

// writeMessage writes a message to the partition file
func (p *Partition) writeMessage(msg *Message) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Record the current offset before writing
	currentOffset := p.offset

	// Calculate header size and total size
	header := MessageHeader{
		KeySize:   uint32(len(msg.Key)),
		Timestamp: msg.Timestamp.UnixNano(),
	}

	// Serialize header
	headerBytes := make([]byte, 12) // 4 + 8 bytes
	binary.BigEndian.PutUint32(headerBytes[0:4], header.KeySize)
	binary.BigEndian.PutUint64(headerBytes[4:12], uint64(header.Timestamp))

	// Calculate total message size
	totalSize := uint64(len(headerBytes) + len(msg.Key) + len(msg.Value))

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

	// Write key (if present)
	if len(msg.Key) > 0 {
		if _, err := p.file.Write(msg.Key); err != nil {
			return -1, fmt.Errorf("failed to write message key: %w", err)
		}
	}

	// Write value
	if _, err := p.file.Write(msg.Value); err != nil {
		return -1, fmt.Errorf("failed to write message value: %w", err)
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

// readMessage reads a message from the partition file at the specified offset
func (p *Partition) readMessage(offset int64) (*Message, int64, error) {
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

	// Parse header
	keySize := binary.BigEndian.Uint32(msgBytes[0:4])
	timestamp := binary.BigEndian.Uint64(msgBytes[4:12])

	// Extract key and value
	headerSize := 12
	var key []byte
	if keySize > 0 {
		key = msgBytes[headerSize : headerSize+int(keySize)]
	}
	value := msgBytes[headerSize+int(keySize):]

	// Create message
	msg := &Message{
		Key:       key,
		Value:     value,
		Timestamp: time.Unix(0, int64(timestamp)),
	}

	// Calculate next offset
	nextOffset := offset + int64(8) + int64(totalSize)

	return msg, nextOffset, nil
}
