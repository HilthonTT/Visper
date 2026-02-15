package broker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
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

// MessagePool provides a pool of message objects to reduce allocations
var messagePool = sync.Pool{
	New: func() interface{} {
		return &Message{}
	},
}

// GetMessage gets a message from the pool
func GetMessage() *Message {
	return messagePool.Get().(*Message)
}

// PutMessage returns a message to the pool
func PutMessage(msg *Message) {
	// Clear message fields
	msg.Key = nil
	msg.Value = nil
	msg.Timestamp = time.Time{}

	// Return to pool
	messagePool.Put(msg)
}

// BufferPool provides a pool of byte buffers
var bufferPool = sync.Pool{
	New: func() interface{} {
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

// GetBuffer gets a buffer from the pool
func GetBuffer() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

// PutBuffer returns a buffer to the pool
func PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}

// serializeMessageParts splits message into header and payload for integrity checking
func serializeMessageParts(msg *Message) ([]byte, []byte, error) {
	keyLen := len(msg.Key)
	valueLen := len(msg.Value)

	// Header: [headerLength(4)][keySize(4)][timestamp(8)]
	headerSize := 4 + 4 + 8
	headerBytes := make([]byte, headerSize)
	offset := 0

	// Write header length (for easier parsing during recovery)
	binary.BigEndian.PutUint32(headerBytes[offset:], uint32(headerSize))
	offset += 4

	// Write key size
	binary.BigEndian.PutUint32(headerBytes[offset:], uint32(keyLen))
	offset += 4

	// Write timestamp
	binary.BigEndian.PutUint64(headerBytes[offset:], uint64(msg.Timestamp.UnixNano()))

	// Payload: [key][value]
	payloadBytes := make([]byte, keyLen+valueLen)
	offset = 0

	if keyLen > 0 {
		copy(payloadBytes[offset:], msg.Key)
		offset += keyLen
	}

	copy(payloadBytes[offset:], msg.Value)

	return headerBytes, payloadBytes, nil
}

// deserializeMessage reconstructs a message from header and payload bytes
func deserializeMessage(headerBytes, payloadBytes []byte) (*Message, error) {
	if len(headerBytes) < 16 {
		return nil, fmt.Errorf("header too short: %d bytes", len(headerBytes))
	}

	// Parse header
	// Skip headerLength (first 4 bytes, already used during read)
	keySize := binary.BigEndian.Uint32(headerBytes[4:8])
	timestamp := binary.BigEndian.Uint64(headerBytes[8:16])

	// Validate payload size
	if uint32(len(payloadBytes)) < keySize {
		return nil, fmt.Errorf("payload too short for key: expected at least %d bytes, got %d", keySize, len(payloadBytes))
	}

	// Extract key and value from payload
	var key []byte
	if keySize > 0 {
		key = make([]byte, keySize)
		copy(key, payloadBytes[0:keySize])
	}

	value := make([]byte, len(payloadBytes)-int(keySize))
	copy(value, payloadBytes[keySize:])

	msg := &Message{
		Key:       key,
		Value:     value,
		Timestamp: time.Unix(0, int64(timestamp)),
	}

	return msg, nil
}
