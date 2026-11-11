package broker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

// On-disk record format:
//
//	[size uint64][keySize uint32][timestamp int64][key][value]
//
// size counts everything after the size field itself.
const (
	sizeFieldLen    = 8
	recordHeaderLen = 4 + 8 // keySize + timestamp

	// maxRecordSize guards against corrupt size fields making the reader
	// allocate absurd amounts of memory.
	maxRecordSize = 64 << 20
)

// ErrCorruptRecord is returned when a record on disk cannot be decoded.
var ErrCorruptRecord = errors.New("corrupt record")

// Message represents a single message in the system
type Message struct {
	Key       []byte
	Value     []byte
	Timestamp time.Time
}

// encodeRecord serialises msg in the on-disk format.
func encodeRecord(msg *Message) []byte {
	bodyLen := recordHeaderLen + len(msg.Key) + len(msg.Value)
	buf := make([]byte, sizeFieldLen+bodyLen)

	binary.BigEndian.PutUint64(buf[0:8], uint64(bodyLen))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(msg.Key)))
	binary.BigEndian.PutUint64(buf[12:20], uint64(msg.Timestamp.UnixNano()))
	n := copy(buf[20:], msg.Key)
	copy(buf[20+n:], msg.Value)

	return buf
}

// decodeRecordBody parses a record body (everything after the size field).
func decodeRecordBody(body []byte) (*Message, error) {
	if len(body) < recordHeaderLen {
		return nil, fmt.Errorf("%w: body too short (%d bytes)", ErrCorruptRecord, len(body))
	}

	keySize := int(binary.BigEndian.Uint32(body[0:4]))
	timestamp := int64(binary.BigEndian.Uint64(body[4:12]))

	if keySize > len(body)-recordHeaderLen {
		return nil, fmt.Errorf("%w: key size %d exceeds record", ErrCorruptRecord, keySize)
	}

	var key []byte
	if keySize > 0 {
		key = body[recordHeaderLen : recordHeaderLen+keySize]
	}

	return &Message{
		Key:       key,
		Value:     body[recordHeaderLen+keySize:],
		Timestamp: time.Unix(0, timestamp),
	}, nil
}

// writeMessage appends a message to the partition file and returns its offset.
func (p *Partition) writeMessage(msg *Message) (int64, error) {
	record := encodeRecord(msg)

	p.mu.Lock()
	defer p.mu.Unlock()

	currentOffset := p.offset

	// A single write keeps the record contiguous even with concurrent readers.
	n, err := p.file.Write(record)
	if err != nil {
		// Keep the in-memory offset in sync with what actually hit the file.
		p.offset += int64(n)
		return -1, fmt.Errorf("failed to write message: %w", err)
	}

	p.offset += int64(len(record))
	p.dirty = true

	if time.Since(p.lastSync) >= p.syncEvery {
		_ = p.syncLocked()
	}

	return currentOffset, nil
}

// readMessage reads the message at offset and returns it with the offset of
// the next message.
func (p *Partition) readMessage(offset int64) (*Message, int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if offset < 0 || offset+sizeFieldLen > p.offset {
		return nil, offset, io.EOF
	}

	sizeBytes := make([]byte, sizeFieldLen)
	if _, err := p.file.ReadAt(sizeBytes, offset); err != nil {
		return nil, offset, fmt.Errorf("failed to read message size: %w", err)
	}

	totalSize := binary.BigEndian.Uint64(sizeBytes)
	if totalSize < recordHeaderLen || totalSize > maxRecordSize ||
		offset+sizeFieldLen+int64(totalSize) > p.offset {
		return nil, offset, fmt.Errorf("%w: invalid size %d at offset %d", ErrCorruptRecord, totalSize, offset)
	}

	body := make([]byte, totalSize)
	if _, err := p.file.ReadAt(body, offset+sizeFieldLen); err != nil {
		return nil, offset, fmt.Errorf("failed to read message: %w", err)
	}

	msg, err := decodeRecordBody(body)
	if err != nil {
		return nil, offset, err
	}

	return msg, offset + sizeFieldLen + int64(totalSize), nil
}
