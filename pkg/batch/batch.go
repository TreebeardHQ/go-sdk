package batch

import (
	"sync"
	"time"

	"github.com/treebeard/go-sdk/pkg/logger"
)

// Batch manages batching of log entries before sending
type Batch struct {
	mu        sync.Mutex
	entries   []logger.LogEntry
	maxSize   int
	maxAge    time.Duration
	lastFlush time.Time
}

// New creates a new Batch with the specified configuration
func New(maxSize int, maxAge time.Duration) *Batch {
	return &Batch{
		entries:   make([]logger.LogEntry, 0, maxSize),
		maxSize:   maxSize,
		maxAge:    maxAge,
		lastFlush: time.Now(),
	}
}

// Add adds a log entry to the batch and returns whether it should be flushed
func (b *Batch) Add(entry logger.LogEntry) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries = append(b.entries, entry)

	// Check if we should flush based on size or age
	shouldFlush := len(b.entries) >= b.maxSize ||
		time.Since(b.lastFlush) >= b.maxAge

	return shouldFlush
}

// GetEntries returns all entries and clears the batch
func (b *Batch) GetEntries() []logger.LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	entries := make([]logger.LogEntry, len(b.entries))
	copy(entries, b.entries)

	// Clear the batch
	b.entries = b.entries[:0]
	b.lastFlush = time.Now()

	return entries
}

// Size returns the current number of entries in the batch
func (b *Batch) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.entries)
}

// IsEmpty returns whether the batch is empty
func (b *Batch) IsEmpty() bool {
	return b.Size() == 0
}

// ShouldFlush returns whether the batch should be flushed based on size or age
func (b *Batch) ShouldFlush() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.entries) >= b.maxSize ||
		time.Since(b.lastFlush) >= b.maxAge
}