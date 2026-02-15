package broker

import (
	"bytes"
	"time"
)

const (
	DefaultMaxMessages = 1_000
)

type BatchWriter struct {
	partition   *Partition
	buffer      bytes.Buffer
	maxSize     int
	maxMessages int
	count       int
}

// Add a message to the batch
func (bw *BatchWriter) Add(msg *Message) error {
	// Serialize message
	msgBytes, err := serializeMessage(msg)
	if err != nil {
		return err
	}

	// Check if we would exceed batch size
	if bw.buffer.Len()+len(msgBytes) > bw.maxSize || bw.count >= bw.maxMessages {
		if err := bw.Flush(); err != nil {
			return err
		}
	}

	// Add to buffer
	bw.buffer.Write(msgBytes)
	bw.count++

	return nil
}

func (bw *BatchWriter) Flush() error {
	if bw.buffer.Len() == 0 {
		return nil
	}

	bw.partition.mu.Lock()
	defer bw.partition.mu.Unlock()

	// Write buffer to file
	if _, err := bw.partition.file.Write(bw.buffer.Bytes()); err != nil {
		return err
	}

	// Update offset
	bw.partition.offset += int64(bw.buffer.Len())

	// Reset buffer
	bw.buffer.Reset()
	bw.count = 0

	return nil
}

type PartitionWriter struct {
	partition *Partition
	writeCh   chan writeRequest
	done      chan struct{}
}

type writeRequest struct {
	msg      *Message
	resultCh chan writeResult
}

type writeResult struct {
	offset int64
	err    error
}

// NewPartitionWriter creates a new partition writer
func NewPartitionWriter(partition *Partition) *PartitionWriter {
	pw := &PartitionWriter{
		partition: partition,
		writeCh:   make(chan writeRequest, DefaultMaxMessages),
		done:      make(chan struct{}),
	}

	// Start writer goroutine
	go pw.writeLoop()

	return pw
}

// Write sends a message to be written asynchronously
func (pw *PartitionWriter) Write(msg *Message) (int64, error) {
	resultCh := make(chan writeResult, 1)

	// Send write request
	pw.writeCh <- writeRequest{
		msg:      msg,
		resultCh: resultCh,
	}

	result := <-resultCh
	return result.offset, result.err
}

// writeLoop processes write requests in a dedicated goroutine
func (pw *PartitionWriter) writeLoop() {
	bw := &BatchWriter{
		partition:   pw.partition,
		maxSize:     1024 * 1024, // 1MB
		maxMessages: DefaultMaxMessages,
	}

	// Use a ticker to flush periodically
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case req := <-pw.writeCh:
			// Serialize message
			_, err := serializeMessage(req.msg)
			if err != nil {
				req.resultCh <- writeResult{-1, err}
				continue
			}

			// Record current offset
			offset := pw.partition.offset

			// Add to batch
			if err := bw.Add(req.msg); err != nil {
				req.resultCh <- writeResult{-1, err}
				continue
			}

			// Send result
			req.resultCh <- writeResult{offset, nil}

		case <-ticker.C:
			// Flush batch periodically
			bw.Flush()

		case <-pw.done:
			// Flush before exiting
			bw.Flush()
			return
		}
	}
}

// Close stops the writer
func (pw *PartitionWriter) Close() error {
	close(pw.done)
	return nil
}
