package aiagent

import (
	"sort"
	"sync"
	"time"
)

// BufferConfig holds configuration for the IssueBuffer.
type BufferConfig struct {
	FlushTimeout time.Duration // max time to wait before flushing
	MaxSize      int           // max issues before forced flush
}

// Buffer accumulates issues with dedup, debounce, and priority ordering.
type Buffer struct {
	cfg     BufferConfig
	mu      sync.Mutex
	issues  map[string]Issue // keyed by DedupKey()
	order   []string         // insertion order for stable iteration
	onFlush func(IssueBatch) // callback when flushed
	stopCh  chan struct{}
	timer   *time.Timer
}

// NewBuffer creates a new issue buffer.
func NewBuffer(cfg BufferConfig) *Buffer {
	b := &Buffer{
		cfg:    cfg,
		issues: make(map[string]Issue),
		stopCh: make(chan struct{}),
	}
	b.resetTimer()
	return b
}

// OnFlush sets the callback fired when the buffer flushes.
func (b *Buffer) OnFlush(fn func(IssueBatch)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onFlush = fn
}

// Add pushes an issue into the buffer, deduplicating if needed.
func (b *Buffer) Add(issue Issue) {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := issue.DedupKey()
	if existing, ok := b.issues[key]; ok {
		// Dedup: merge occurrences, keep earliest FirstSeen
		existing.Occurrences++
		if issue.FirstSeen.Before(existing.FirstSeen) {
			existing.FirstSeen = issue.FirstSeen
		}
		b.issues[key] = existing
	} else {
		b.issues[key] = issue
		b.order = append(b.order, key)
	}

	// Check size trigger
	if len(b.issues) >= b.cfg.MaxSize {
		b.flushLocked()
		return
	}

	b.resetTimerLocked()
}

// Len returns the number of unique issues in the buffer.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.issues)
}

// Stop stops the buffer's flush timer.
func (b *Buffer) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.stopCh:
		return // already stopped
	default:
		close(b.stopCh)
	}
	if b.timer != nil {
		b.timer.Stop()
	}
}

func (b *Buffer) resetTimer() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.resetTimerLocked()
}

func (b *Buffer) resetTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()
	}
	b.timer = time.AfterFunc(b.cfg.FlushTimeout, func() {
		b.mu.Lock()
		select {
		case <-b.stopCh:
			b.mu.Unlock()
			return
		default:
		}
		b.flushLocked()
		b.mu.Unlock()
	})
}

func (b *Buffer) flushLocked() {
	if len(b.issues) == 0 {
		return
	}

	// Collect and sort by priority
	var list []Issue
	for _, key := range b.order {
		if issue, ok := b.issues[key]; ok {
			list = append(list, issue)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		return PriorityRank(list[i].Priority) < PriorityRank(list[j].Priority)
	})

	batch := IssueBatch{
		ID:      "batch-" + time.Now().Format("20060102-150405"),
		Issues:  list,
		Created: time.Now(),
		Source:  "realtime",
	}

	if b.onFlush != nil {
		// Unlock before calling onFlush to avoid deadlock
		b.mu.Unlock()
		b.onFlush(batch)
		b.mu.Lock()
	}

	// Clear
	b.issues = make(map[string]Issue)
	b.order = b.order[:0]
}
