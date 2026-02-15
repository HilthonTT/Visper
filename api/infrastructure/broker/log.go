package broker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LogSegment represents a segment of a partition log
type LogSegment struct {
	file       *os.File
	baseOffset int64
	maxSize    int64
	startTime  time.Time
}

// SegmentedPartition manages multiple log segments
type SegmentedPartition struct {
	topic          *Topic
	id             int
	dir            string
	activeSegment  *LogSegment
	segments       []*LogSegment
	mu             sync.Mutex
	maxSegmentSize int64
	retention      time.Duration
}

// NewSegmentedPartition creates a new segmented partition
func NewSegmentedPartition(topic *Topic, id int, dir string) (*SegmentedPartition, error) {
	sp := &SegmentedPartition{
		topic:          topic,
		id:             id,
		dir:            dir,
		maxSegmentSize: 1024 * 1024 * 1024, // 1GB
		retention:      7 * 24 * time.Hour, // 7 days
	}

	// Create partition directory
	partitionDir := filepath.Join(dir, fmt.Sprintf("partition-%d", id))
	if err := os.MkdirAll(partitionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create partition directory: %w", err)
	}

	// Load existing segments
	if err := sp.loadSegments(); err != nil {
		return nil, fmt.Errorf("failed to load segments: %w", err)
	}

	// Create active segment if none exists
	if len(sp.segments) == 0 {
		if err := sp.createNewSegment(0); err != nil {
			return nil, fmt.Errorf("failed to create initial segment: %w", err)
		}
	} else {
		// Set the last segment as active
		sp.activeSegment = sp.segments[len(sp.segments)-1]
	}

	// Start segment cleaner
	go sp.segmentCleanerLoop()

	return sp, nil
}

// createNewSegment creates a new log segment
func (sp *SegmentedPartition) createNewSegment(baseOffset int64) error {
	// Create segment file
	fileName := fmt.Sprintf("%020d.log", baseOffset)
	filePath := filepath.Join(sp.dir, fileName)

	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create segment file: %w", err)
	}

	// Create segment
	segment := &LogSegment{
		file:       file,
		baseOffset: baseOffset,
		startTime:  time.Now(),
	}

	// Add to segments list
	sp.segments = append(sp.segments, segment)
	sp.activeSegment = segment

	return nil
}

// loadSegments loads existing log segments
func (sp *SegmentedPartition) loadSegments() error {
	// Find segment files
	pattern := filepath.Join(sp.dir, "*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob segment files: %w", err)
	}

	// Sort segment files by base offset
	sort.Strings(matches)

	// Load each segment
	for _, match := range matches {
		// Extract base offset from filename
		baseName := filepath.Base(match)
		baseOffsetStr := strings.TrimSuffix(baseName, ".log")
		baseOffset, err := strconv.ParseInt(baseOffsetStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid segment filename %s: %w", baseName, err)
		}

		// Open segment file
		file, err := os.OpenFile(match, os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open segment file %s: %w", match, err)
		}

		// Get file info
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to stat segment file %s: %w", match, err)
		}

		// Create segment
		segment := &LogSegment{
			file:       file,
			baseOffset: baseOffset,
			startTime:  info.ModTime(),
		}

		// Add to segments list
		sp.segments = append(sp.segments, segment)
	}

	return nil
}

// WriteMessage writes a message to the active segment, rolling if necessary
func (sp *SegmentedPartition) WriteMessage(msg *Message) (int64, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Get file info
	info, err := sp.activeSegment.file.Stat()
	if err != nil {
		return -1, fmt.Errorf("failed to stat segment file: %w", err)
	}

	// Check if segment is full
	if info.Size() >= sp.maxSegmentSize {
		// Roll segment
		newBaseOffset := sp.activeSegment.baseOffset + info.Size()
		if err := sp.createNewSegment(newBaseOffset); err != nil {
			return -1, fmt.Errorf("failed to roll segment: %w", err)
		}
	}

	// Calculate offset within segment
	segmentOffset := info.Size()

	// Calculate global offset
	globalOffset := sp.activeSegment.baseOffset + segmentOffset

	// Serialize message
	msgBytes, err := serializeMessage(msg)
	if err != nil {
		return -1, fmt.Errorf("failed to serialize message: %w", err)
	}

	// Write message to active segment
	if _, err := sp.activeSegment.file.Write(msgBytes); err != nil {
		return -1, fmt.Errorf("failed to write message: %w", err)
	}

	return globalOffset, nil
}

// segmentCleanerLoop periodically cleans up old segments
func (sp *SegmentedPartition) segmentCleanerLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		sp.cleanOldSegments()
	}
}

// cleanOldSegments removes segments older than the retention period
func (sp *SegmentedPartition) cleanOldSegments() {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Can't clean if we only have one segment
	if len(sp.segments) <= 1 {
		return
	}

	cutoffTime := time.Now().Add(-sp.retention)

	// Find segments to remove
	var newSegments []*LogSegment
	for i, segment := range sp.segments {
		// Skip the active segment
		if segment == sp.activeSegment {
			newSegments = append(newSegments, segment)
			continue
		}

		// Skip segments newer than cutoff
		if segment.startTime.After(cutoffTime) {
			newSegments = append(newSegments, segment)
			continue
		}

		// Skip the newest segment that's older than cutoff
		// (we need at least one old segment for historical data)
		if i > 0 && sp.segments[i-1].startTime.Before(cutoffTime) &&
			(i == len(sp.segments)-1 || sp.segments[i+1].startTime.After(cutoffTime)) {
			newSegments = append(newSegments, segment)
			continue
		}

		// Remove this segment
		log.Printf("Removing old segment %s (created at %s)",
			segment.file.Name(), segment.startTime)

		segment.file.Close()
		os.Remove(segment.file.Name())
	}

	sp.segments = newSegments
}
