package broker

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/golang/snappy"
)

// CompressionType defines the compression algorithm used
type CompressionType int

const (
	// CompressionNone means no compression is used
	CompressionNone CompressionType = 0

	// CompressionGzip uses gzip compression
	CompressionGzip CompressionType = 1

	// CompressionSnappy uses snappy compression
	CompressionSnappy CompressionType = 2
)

// CompressedMessage represents a message with compression metadata
type CompressedMessage struct {
	Message
	CompressionType CompressionType
}

// Compress compresses a message payload
func Compress(msg *Message, compressionType CompressionType) (*CompressedMessage, error) {
	// Create compressed message
	compMsg := &CompressedMessage{
		Message: Message{
			Key:       msg.Key,
			Timestamp: msg.Timestamp,
		},
		CompressionType: compressionType,
	}

	switch compressionType {
	case CompressionNone:
		compMsg.Value = msg.Value
	case CompressionGzip:
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)

		if _, err := writer.Write(msg.Value); err != nil {
			return nil, fmt.Errorf("failed to compress with gzip: %w", err)
		}

		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip writer: %w", err)
		}

		compMsg.Value = buf.Bytes()
	case CompressionSnappy:
		compMsg.Value = snappy.Encode(nil, msg.Value)
	default:
		return nil, fmt.Errorf("unknown compression type: %d", compressionType)
	}

	return compMsg, nil
}

// Decompress decompresses a message payload
func Decompress(compMsg *CompressedMessage) (*Message, error) {
	// Create decompressed message
	msg := &Message{
		Key:       compMsg.Key,
		Timestamp: compMsg.Timestamp,
	}

	// Decompress payload
	switch compMsg.CompressionType {
	case CompressionNone:
		msg.Value = compMsg.Value

	case CompressionGzip:
		reader, err := gzip.NewReader(bytes.NewReader(compMsg.Value))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}

		value, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress with gzip: %w", err)
		}

		if err := reader.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip reader: %w", err)
		}

		msg.Value = value
	case CompressionSnappy:
		value, err := snappy.Decode(nil, compMsg.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress with snappy: %w", err)
		}

		msg.Value = value
	default:
		return nil, fmt.Errorf("unknown compression type: %d", compMsg.CompressionType)
	}

	return msg, nil
}
