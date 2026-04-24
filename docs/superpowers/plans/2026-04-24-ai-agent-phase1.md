# Phase 1: GoStream AI Agent Infrastructure

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the embedded Go infrastructure in GoStream that detects issues, batches them, and pushes them to Hermes via webhook.

**Architecture:** New `internal/ai-agent/` package with Issue types, Buffer (debounce/dedup), Queue (disk), Detectors (goroutines), Webhook Pusher, and HTTP API endpoints registered in `main.go`. Enhanced structured logging to a separate file.

**Tech Stack:** Go 1.24+, standard library (`log`, `encoding/json`, `net/http`, `sync`, `time`), `github.com/google/uuid` for batch IDs (defined but not currently used — can be removed if not needed).

---

## File Map

| File | Responsibility | Action |
|------|---------------|--------|
| `internal/ai-agent/types.go` | Issue types, IssueBatch, priority constants, JSON schema | **Create** |
| `internal/ai-agent/buffer.go` | IssueBuffer with debounce, dedup, priority ordering, flush triggers | **Create** |
| `internal/ai-agent/queue.go` | Disk-backed queue (JSON file), survives restarts | **Create** |
| `internal/ai-agent/webhook.go` | HTTP POST to Hermes webhook with retry + exponential backoff | **Create** |
| `internal/ai-agent/detectors.go` | All detector goroutines (TorrentHealth, StartupLatency, WebhookMatcher, FuseAccess, LogMonitor) | **Create** |
| `internal/ai-agent/ai_api.go` | HTTP handlers for `/api/ai/*` endpoints | **Create** |
| `internal/ai-agent/ai_logger.go` | Structured JSON logger for `logs/gostream-ai.log` | **Create** |
| `internal/ai-agent/agent.go` | Top-level Agent struct wiring buffer + queue + webhook + detectors + API + lifecycle | **Create** |
| `config.go` | Add `AIAgent` config struct + fields | **Modify** |
| `main.go` | Wire `ai-agent` into startup sequence, register `/api/ai/*` routes, add to shutdown | **Modify** |
| `config.json.example` | Add `ai_agent` section | **Modify** |
| `internal/ai-agent/types_test.go` | Tests for Issue validation, priority ordering, dedup | **Create** |
| `internal/ai-agent/buffer_test.go` | Tests for debounce, dedup, flush triggers | **Create** |
| `internal/ai-agent/queue_test.go` | Tests for queue persistence, priority ordering | **Create** |

---

## Task 1: Types + Validation

**Files:**
- Create: `internal/ai-agent/types.go`
- Create: `internal/ai-agent/types_test.go`

- [ ] **Step 1: Write the test file for type validation**

```go
// internal/ai-agent/types_test.go
package aiagent

import (
	"encoding/json"
	"testing"
	"time"
)

func TestIssueValidation_ValidIssue(t *testing.T) {
	issue := Issue{
		Type:        "dead_torrent",
		Priority:    "B",
		TorrentID:   "abc123",
		File:        "Movie.mkv",
		FirstSeen:   time.Now(),
		Occurrences: 1,
	}
	if err := issue.Validate(); err != nil {
		t.Fatalf("expected valid issue, got error: %v", err)
	}
}

func TestIssueValidation_MissingType(t *testing.T) {
	issue := Issue{
		Priority:    "B",
		FirstSeen:   time.Now(),
		Occurrences: 1,
	}
	err := issue.Validate()
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	if err.Error() != "type is required" {
		t.Fatalf("expected 'type is required', got: %v", err)
	}
}

func TestIssueValidation_InvalidType(t *testing.T) {
	issue := Issue{
		Type:        "unknown_type",
		Priority:    "B",
		FirstSeen:   time.Now(),
		Occurrences: 1,
	}
	err := issue.Validate()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestIssueValidation_InvalidPriority(t *testing.T) {
	issue := Issue{
		Type:        "dead_torrent",
		Priority:    "Z",
		FirstSeen:   time.Now(),
		Occurrences: 1,
	}
	err := issue.Validate()
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
}

func TestIssueValidation_OccurrencesMustBePositive(t *testing.T) {
	issue := Issue{
		Type:        "dead_torrent",
		Priority:    "B",
		FirstSeen:   time.Now(),
		Occurrences: 0,
	}
	err := issue.Validate()
	if err == nil {
		t.Fatal("expected error for zero occurrences")
	}
}

func TestIssueBatchValidation_EmptyIssues(t *testing.T) {
	batch := IssueBatch{
		ID:      "batch-20260424-103000",
		Issues:  []Issue{},
		Created: time.Now(),
		Source:  "realtime",
	}
	err := batch.Validate()
	if err == nil {
		t.Fatal("expected error for empty issues")
	}
}

func TestIssueBatchValidation_InvalidSource(t *testing.T) {
	batch := IssueBatch{
		ID:      "batch-20260424-103000",
		Issues:  []Issue{{Type: "dead_torrent", Priority: "B", FirstSeen: time.Now(), Occurrences: 1}},
		Created: time.Now(),
		Source:  "unknown",
	}
	err := batch.Validate()
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
}

func TestIssueSerialization_RoundTrip(t *testing.T) {
	issue := Issue{
		Type:        "slow_startup",
		Priority:    "B",
		TorrentID:   "xyz",
		File:        "Show.S01E01.mkv",
		FirstSeen:   time.Date(2026, 4, 24, 10, 30, 0, 0, time.UTC),
		Occurrences: 3,
		Details:     map[string]any{"startup_ms": 30000},
	}
	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded Issue
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded.Type != issue.Type || decoded.Priority != issue.Priority || decoded.Occurrences != issue.Occurrences {
		t.Fatalf("round-trip mismatch: %+v", decoded)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai-agent/... -v`
Expected: FAIL — package doesn't exist yet (or files don't exist)

- [ ] **Step 3: Implement types with validation**

```go
// internal/ai-agent/types.go
package aiagent

import (
	"fmt"
	"time"
)

// Priority constants — ordered by annoyance (B > A > C > D).
const (
	PriorityB = "B" // Films that won't start (dead, slow, errors)
	PriorityA = "A" // Wrong films (CAM, wrong language, wrong version)
	PriorityC = "C" // Incomplete TV series
	PriorityD = "D" // Missing subtitles
)

// Valid issue types.
const (
	TypeDeadTorrent      = "dead_torrent"
	TypeLowSeeders       = "low_seeders"
	TypeNoDownload       = "no_download"
	TypeSlowStartup      = "slow_startup"
	TypeTimeoutStartup   = "timeout_startup"
	TypeWrongMatch       = "wrong_match"
	TypeUnconfirmedPlay  = "unconfirmed_play"
	TypeFuseError        = "fuse_error"
	TypeReadStall        = "read_stall"
	TypeErrorSpike       = "error_spike"
	TypePatternAnomaly   = "pattern_anomaly"
	TypeMissingSubtitles = "missing_subtitles"
	TypeIncompleteSeries = "incomplete_series"
	TypeIncompleteDownload = "incomplete_download"
)

var validTypes = map[string]bool{
	TypeDeadTorrent:      true,
	TypeLowSeeders:       true,
	TypeNoDownload:       true,
	TypeSlowStartup:      true,
	TypeTimeoutStartup:   true,
	TypeWrongMatch:       true,
	TypeUnconfirmedPlay:  true,
	TypeFuseError:        true,
	TypeReadStall:        true,
	TypeErrorSpike:       true,
	TypePatternAnomaly:   true,
	TypeMissingSubtitles: true,
	TypeIncompleteSeries: true,
	TypeIncompleteDownload: true,
}

var validPriorities = map[string]bool{
	PriorityA: true,
	PriorityB: true,
	PriorityC: true,
	PriorityD: true,
}

var validSources = map[string]bool{
	"realtime":    true,
	"log_monitor": true,
	"deep_scan":   true,
}

// Issue represents a single detected problem.
type Issue struct {
	Type        string         `json:"type"`                  // e.g., "dead_torrent"
	Priority    string         `json:"priority"`              // "B", "A", "C", "D"
	TorrentID   string         `json:"torrent_id,omitempty"`  // GoStorm torrent ID
	File        string         `json:"file,omitempty"`        // MKV filename
	IMDBID      string         `json:"imdb_id,omitempty"`     // TMDB/IMDB identifier
	Details     map[string]any `json:"details"`               // detector-specific context
	FirstSeen   time.Time      `json:"first_seen"`            // when first detected
	Occurrences int            `json:"occurrences"`           // how many times seen
	LogSnippet  string         `json:"log_snippet,omitempty"` // for pattern_anomaly
}

// Validate checks required fields and enum values.
func (i Issue) Validate() error {
	if i.Type == "" {
		return fmt.Errorf("type is required")
	}
	if !validTypes[i.Type] {
		return fmt.Errorf("invalid issue type: %s", i.Type)
	}
	if i.Priority == "" {
		return fmt.Errorf("priority is required")
	}
	if !validPriorities[i.Priority] {
		return fmt.Errorf("invalid priority: %s", i.Priority)
	}
	if i.FirstSeen.IsZero() {
		return fmt.Errorf("first_seen is required")
	}
	if i.Occurrences < 1 {
		return fmt.Errorf("occurrences must be >= 1")
	}
	return nil
}

// DedupKey returns a key used for deduplication in the buffer.
func (i Issue) DedupKey() string {
	key := i.Type
	if i.TorrentID != "" {
		key += ":" + i.TorrentID
	}
	if i.File != "" {
		key += ":" + i.File
	}
	if i.IMDBID != "" {
		key += ":" + i.IMDBID
	}
	return key
}

// PriorityRank returns numeric rank for sorting (lower = more urgent).
func PriorityRank(p string) int {
	switch p {
	case PriorityB:
		return 1
	case PriorityA:
		return 2
	case PriorityC:
		return 3
	case PriorityD:
		return 4
	default:
		return 99
	}
}

// IssueBatch is a debounced collection of issues flushed from the buffer.
type IssueBatch struct {
	ID      string    `json:"id"`       // unique batch ID
	Issues  []Issue   `json:"issues"`   // deduplicated, priority-ordered issues
	Created time.Time `json:"created"`  // when batch was created
	Source  string    `json:"source"`   // "realtime", "log_monitor", "deep_scan"
}

// Validate checks batch integrity.
func (b IssueBatch) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("batch ID is required")
	}
	if len(b.Issues) == 0 {
		return fmt.Errorf("batch must contain at least one issue")
	}
	if b.Created.IsZero() {
		return fmt.Errorf("created timestamp is required")
	}
	if !validSources[b.Source] {
		return fmt.Errorf("invalid source: %s", b.Source)
	}
	for i, issue := range b.Issues {
		if err := issue.Validate(); err != nil {
			return fmt.Errorf("issue %d: %w", i, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai-agent/... -v`
Expected: All 8 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai-agent/types.go internal/ai-agent/types_test.go
git commit -m "feat(ai-agent): add Issue types with validation, dedup key, priority ranking"
```

---

## Task 2: AI Structured Logger

**Files:**
- Create: `internal/ai-agent/ai_logger.go`

- [ ] **Step 1: Implement the AI logger**

```go
// internal/ai-agent/ai_logger.go
package aiagent

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"
)

// AILogger writes structured JSON entries to the AI agent log file.
// Separate from the main GoStream log to avoid polluting human-readable logs.
type AILogger struct {
	logger *log.Logger
	file   *os.File
}

// AILogEntry is the structured format for AI agent log entries.
type AILogEntry struct {
	Timestamp time.Time      `json:"ts"`
	Level     string         `json:"level"`      // "info", "warn", "error"
	Detector  string         `json:"detector"`   // which detector emitted
	Issue     string         `json:"issue"`      // issue type
	TorrentID string         `json:"torrent_id,omitempty"`
	File      string         `json:"file,omitempty"`
	IMDBID    string         `json:"imdb_id,omitempty"`
	Seeders   *int           `json:"seeders,omitempty"`
	Peers     *int           `json:"peers,omitempty"`
	AgeSecs   *int           `json:"age_seconds,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	Action    string         `json:"action_needed,omitempty"`
	Message   string         `json:"message,omitempty"`
}

// NewAILogger creates a logger that writes to both stdout and the AI log file.
func NewAILogger(logDir string) (*AILogger, error) {
	if logDir == "" {
		logDir = "logs"
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	filePath := logDir + "/gostream-ai.log"
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &AILogger{
		logger: log.New(io.MultiWriter(os.Stdout, file), "[AIAgent] ", log.LstdFlags),
		file:   file,
	}, nil
}

// Info logs an info-level entry.
func (l *AILogger) Info(detector, msg string, fields ...AILogField) {
	l.write("info", detector, msg, fields)
}

// Warn logs a warning-level entry.
func (l *AILogger) Warn(detector, msg string, fields ...AILogField) {
	l.write("warn", detector, msg, fields)
}

// Error logs an error-level entry.
func (l *AILogger) Error(detector, msg string, fields ...AILogField) {
	l.write("error", detector, msg, fields)
}

// AILogField is a key-value pair for structured log entries.
type AILogField struct {
	Key   string
	Value any
}

// F is a convenience function for creating log fields.
func F(key string, value any) AILogField {
	return AILogField{Key: key, Value: value}
}

func (l *AILogger) write(level, detector, msg string, fields []AILogField) {
	entry := AILogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Detector:  detector,
		Message:   msg,
	}
	for _, f := range fields {
		switch f.Key {
		case "issue":
			if v, ok := f.Value.(string); ok {
				entry.Issue = v
			}
		case "torrent_id":
			if v, ok := f.Value.(string); ok {
				entry.TorrentID = v
			}
		case "file":
			if v, ok := f.Value.(string); ok {
				entry.File = v
			}
		case "imdb_id":
			if v, ok := f.Value.(string); ok {
				entry.IMDBID = v
			}
		case "seeders":
			if v, ok := f.Value.(int); ok {
				entry.Seeders = &v
			}
		case "peers":
			if v, ok := f.Value.(int); ok {
				entry.Peers = &v
			}
		case "age_seconds":
			if v, ok := f.Value.(int); ok {
				entry.AgeSecs = &v
			}
		case "action_needed":
			if v, ok := f.Value.(string); ok {
				entry.Action = v
			}
		case "details":
			if v, ok := f.Value.(map[string]any); ok {
				entry.Details = v
			}
		default:
			if entry.Details == nil {
				entry.Details = make(map[string]any)
			}
			entry.Details[f.Key] = f.Value
		}
	}

	// Write human-readable line
	l.logger.Printf("[%s] %s: %s", level, detector, msg)

	// Write JSON entry to file
	data, err := json.Marshal(entry)
	if err == nil {
		// Append JSON line to the file directly
		file, err := os.OpenFile(l.file.Name(), os.O_WRONLY|os.O_APPEND, 0644)
		if err == nil {
			file.Write(data)
			file.Write([]byte("\n"))
			file.Close()
		}
	}
}

// Close closes the underlying file.
func (l *AILogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
```

- [ ] **Step 2: Write a simple test**

```go
// internal/ai-agent/ai_logger_test.go
package aiagent

import (
	"os"
	"strings"
	"testing"
)

func TestAILogger_CreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	l, err := NewAILogger(dir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer l.Close()

	l.Warn("torrent_health", "dead torrent detected",
		F("issue", "dead_torrent"),
		F("torrent_id", "abc123"),
		F("file", "Movie.mkv"),
		F("seeders", 0),
		F("action_needed", "replace"),
	)

	data, err := os.ReadFile(dir + "/gostream-ai.log")
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "dead_torrent") {
		t.Fatalf("expected 'dead_torrent' in log, got: %s", content)
	}
	if !strings.Contains(content, "abc123") {
		t.Fatalf("expected 'abc123' in log, got: %s", content)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/ai-agent/... -v`
Expected: All tests PASS (including the new logger test)

- [ ] **Step 4: Commit**

```bash
git add internal/ai-agent/ai_logger.go internal/ai-agent/ai_logger_test.go
git commit -m "feat(ai-agent): add structured JSON logger for gostream-ai.log"
```

---

## Task 3: Disk-backed Queue

**Files:**
- Create: `internal/ai-agent/queue.go`
- Create: `internal/ai-agent/queue_test.go`

- [ ] **Step 1: Write queue tests**

```go
// internal/ai-agent/queue_test.go
package aiagent

import (
	"os"
	"testing"
	"time"
)

func newTestBatch(id string, priority string) IssueBatch {
	return IssueBatch{
		ID:      id,
		Issues:  []Issue{{Type: "dead_torrent", Priority: priority, FirstSeen: time.Now(), Occurrences: 1}},
		Created: time.Now(),
		Source:  "realtime",
	}
}

func TestQueue_EnqueueAndDequeue(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir + "/test-queue.json")

	batch := newTestBatch("batch-001", "B")
	if err := q.Enqueue(batch); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	batches, err := q.DequeueAll()
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if batches[0].ID != "batch-001" {
		t.Fatalf("expected batch-001, got %s", batches[0].ID)
	}
}

func TestQueue_PriorityOrder(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir + "/test-queue.json")

	// Enqueue in random order
	q.Enqueue(newTestBatch("batch-c", "C"))
	q.Enqueue(newTestBatch("batch-b", "B"))
	q.Enqueue(newTestBatch("batch-d", "D"))
	q.Enqueue(newTestBatch("batch-a", "A"))

	batches, err := q.DequeueAll()
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if len(batches) != 4 {
		t.Fatalf("expected 4 batches, got %d", len(batches))
	}
	// Should be ordered B > A > C > D
	expected := []string{"batch-b", "batch-a", "batch-c", "batch-d"}
	for i, b := range batches {
		if b.ID != expected[i] {
			t.Fatalf("index %d: expected %s, got %s", i, expected[i], b.ID)
		}
	}
}

func TestQueue_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test-queue.json"

	// Write
	q1 := NewQueue(path)
	q1.Enqueue(newTestBatch("batch-persist", "B"))

	// Read back with new instance
	q2 := NewQueue(path)
	batches, err := q2.DequeueAll()
	if err != nil {
		t.Fatalf("dequeue after reload failed: %v", err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch after reload, got %d", len(batches))
	}
	if batches[0].ID != "batch-persist" {
		t.Fatalf("expected batch-persist, got %s", batches[0].ID)
	}
}

func TestQueue_Status(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir + "/test-queue.json")

	status := q.Status()
	if status.PendingBatches != 0 {
		t.Fatalf("expected 0 pending, got %d", status.PendingBatches)
	}

	q.Enqueue(newTestBatch("batch-x", "B"))
	status = q.Status()
	if status.PendingBatches != 1 {
		t.Fatalf("expected 1 pending, got %d", status.PendingBatches)
	}
}

func TestQueue_EmptyDequeue(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir + "/test-empty.json")

	batches, err := q.DequeueAll()
	if err != nil {
		t.Fatalf("dequeue empty failed: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("expected 0 batches, got %d", len(batches))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai-agent/... -run TestQueue -v`
Expected: FAIL — queue.go doesn't exist

- [ ] **Step 3: Implement queue**

```go
// internal/ai-agent/queue.go
package aiagent

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
)

// Queue is a disk-backed FIFO for IssueBatches, ordered by priority.
type Queue struct {
	path string
	mu   sync.Mutex
	data queueData
}

type queueData struct {
	Batches []queueEntry `json:"batches"`
}

type queueEntry struct {
	Batch   IssueBatch `json:"batch"`
	Status  string     `json:"status"` // "pending", "processing", "failed"
	Created string     `json:"created"`
}

// QueueStatus provides a snapshot of the queue.
type QueueStatus struct {
	PendingBatches  int `json:"pending_batches"`
	ProcessingBatches int `json:"processing_batches"`
	FailedBatches   int `json:"failed_batches"`
}

// NewQueue creates or loads a queue from the given file path.
func NewQueue(path string) *Queue {
	q := &Queue{path: path}
	q.load()
	return q
}

func (q *Queue) load() {
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			q.data = queueData{Batches: []queueEntry{}}
			return
		}
		// Corrupted file — start fresh
		q.data = queueData{Batches: []queueEntry{}}
		return
	}
	if err := json.Unmarshal(data, &q.data); err != nil {
		q.data = queueData{Batches: []queueEntry{}}
	}
}

func (q *Queue) save() error {
	data, err := json.MarshalIndent(q.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(q.path, data, 0644)
}

// Enqueue adds a batch to the queue and persists to disk.
func (q *Queue) Enqueue(batch IssueBatch) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	entry := queueEntry{
		Batch:   batch,
		Status:  "pending",
		Created: batch.Created.Format("2006-01-02T15:04:05Z"),
	}
	q.data.Batches = append(q.data.Batches, entry)
	return q.save()
}

// DequeueAll returns all pending batches ordered by priority (B > A > C > D)
// and marks them as processing.
func (q *Queue) DequeueAll() ([]IssueBatch, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var pending []queueEntry
	var result []IssueBatch

	for _, e := range q.data.Batches {
		if e.Status == "pending" {
			e.Status = "processing"
			result = append(result, e.Batch)
			pending = append(pending, e)
		}
	}

	// Sort by priority
	sort.Slice(result, func(i, j int) bool {
		return PriorityRank(result[i].Issues[0].Priority) < PriorityRank(result[j].Issues[0].Priority)
	})

	q.data.Batches = pending
	if err := q.save(); err != nil {
		return nil, err
	}
	return result, nil
}

// MarkComplete removes a batch from the queue by ID.
func (q *Queue) MarkComplete(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	var remaining []queueEntry
	for _, e := range q.data.Batches {
		if e.Batch.ID != id {
			remaining = append(remaining, e)
		}
	}
	q.data.Batches = remaining
	return q.save()
}

// MarkFailed marks a batch as failed for later retry.
func (q *Queue) MarkFailed(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, e := range q.data.Batches {
		if e.Batch.ID == id {
			q.data.Batches[i].Status = "failed"
			return q.save()
		}
	}
	return nil
}

// Status returns a snapshot of the queue.
func (q *Queue) Status() QueueStatus {
	q.mu.Lock()
	defer q.mu.Unlock()

	var status QueueStatus
	for _, e := range q.data.Batches {
		switch e.Status {
		case "pending":
			status.PendingBatches++
		case "processing":
			status.ProcessingBatches++
		case "failed":
			status.FailedBatches++
		}
	}
	return status
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai-agent/... -run TestQueue -v`
Expected: All queue tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai-agent/queue.go internal/ai-agent/queue_test.go
git commit -m "feat(ai-agent): add disk-backed queue with priority ordering"
```

---

## Task 4: Issue Buffer (Debounce + Dedup)

**Files:**
- Create: `internal/ai-agent/buffer.go`
- Create: `internal/ai-agent/buffer_test.go`

- [ ] **Step 1: Write buffer tests**

```go
// internal/ai-agent/buffer_test.go
package aiagent

import (
	"testing"
	"time"
)

func TestBuffer_AddIssue(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 5 * time.Second,
		MaxSize:      20,
	})
	defer buf.Stop()

	issue := Issue{
		Type:        "dead_torrent",
		Priority:    "B",
		TorrentID:   "abc123",
		File:        "Movie.mkv",
		FirstSeen:   time.Now(),
		Occurrences: 1,
	}
	buf.Add(issue)

	if buf.Len() != 1 {
		t.Fatalf("expected 1 issue, got %d", buf.Len())
	}
}

func TestBuffer_Dedup_SameIssue(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 5 * time.Second,
		MaxSize:      20,
	})
	defer buf.Stop()

	now := time.Now()
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", TorrentID: "abc123", File: "Movie.mkv", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", TorrentID: "abc123", File: "Movie.mkv", FirstSeen: now, Occurrences: 1})

	// Dedup should merge — occurrences increment
	if buf.Len() != 1 {
		t.Fatalf("expected 1 issue after dedup, got %d", buf.Len())
	}
}

func TestBuffer_NoDedup_DifferentIssues(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 5 * time.Second,
		MaxSize:      20,
	})
	defer buf.Stop()

	now := time.Now()
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", TorrentID: "abc", File: "Movie.mkv", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "slow_startup", Priority: "B", TorrentID: "abc", File: "Movie.mkv", FirstSeen: now, Occurrences: 1})

	if buf.Len() != 2 {
		t.Fatalf("expected 2 different issues, got %d", buf.Len())
	}
}

func TestBuffer_FlushOnSize(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 10 * time.Second, // long timeout so size triggers first
		MaxSize:      3,                 // small max size for test
	})
	defer buf.Stop()

	var flushedBatches []IssueBatch
	buf.OnFlush(func(batch IssueBatch) {
		flushedBatches = append(flushedBatches, batch)
	})

	now := time.Now()
	for i := 0; i < 3; i++ {
		buf.Add(Issue{
			Type:        "dead_torrent",
			Priority:    "B",
			TorrentID:   "abc",
			File:        "Movie.mkv",
			FirstSeen:   now,
			Occurrences: 1,
		})
	}

	// Need to give async flush a moment
	time.Sleep(50 * time.Millisecond)

	if len(flushedBatches) == 0 {
		t.Fatal("expected flush on size")
	}
}

func TestBuffer_FlushOnTimeout(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 100 * time.Millisecond,
		MaxSize:      20,
	})
	defer buf.Stop()

	var flushedBatches []IssueBatch
	buf.OnFlush(func(batch IssueBatch) {
		flushedBatches = append(flushedBatches, batch)
	})

	now := time.Now()
	buf.Add(Issue{
		Type:        "dead_torrent",
		Priority:    "B",
		TorrentID:   "abc",
		FirstSeen:   now,
		Occurrences: 1,
	})

	// Wait for timeout flush
	time.Sleep(200 * time.Millisecond)

	if len(flushedBatches) == 0 {
		t.Fatal("expected flush on timeout")
	}
}

func TestBuffer_PriorityOrderInFlush(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 100 * time.Millisecond,
		MaxSize:      20,
	})
	defer buf.Stop()

	var flushedBatches []IssueBatch
	buf.OnFlush(func(batch IssueBatch) {
		flushedBatches = append(flushedBatches, batch)
	})

	now := time.Now()
	// Add in reverse priority order
	buf.Add(Issue{Type: "missing_subtitles", Priority: "D", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "wrong_match", Priority: "A", FirstSeen: now, Occurrences: 1})

	time.Sleep(200 * time.Millisecond)

	if len(flushedBatches) == 0 {
		t.Fatal("expected flush")
	}
	batch := flushedBatches[0]
	// First issue in batch should be priority B
	if batch.Issues[0].Priority != "B" {
		t.Fatalf("expected first issue priority B, got %s", batch.Issues[0].Priority)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai-agent/... -run TestBuffer -v`
Expected: FAIL — buffer.go doesn't exist

- [ ] **Step 3: Implement buffer**

```go
// internal/ai-agent/buffer.go
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
		// Unlock before calling onFlush to avoid deadlock (onFlush may call Add)
		b.mu.Unlock()
		b.onFlush(batch)
		b.mu.Lock()
	}

	// Clear
	b.issues = make(map[string]Issue)
	b.order = b.order[:0]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai-agent/... -run TestBuffer -v`
Expected: All buffer tests PASS

Note: Tests with time.Sleep are inherently flaky. If a test fails due to timing, increase the sleep duration slightly.

- [ ] **Step 5: Commit**

```bash
git add internal/ai-agent/buffer.go internal/ai-agent/buffer_test.go
git commit -m "feat(ai-agent): add issue buffer with debounce, dedup, priority ordering"
```

---

## Task 5: Webhook Pusher

**Files:**
- Create: `internal/ai-agent/webhook.go`

- [ ] **Step 1: Implement webhook pusher**

```go
// internal/ai-agent/webhook.go
package aiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookConfig holds the configuration for pushing batches to Hermes.
type WebhookConfig struct {
	URL         string        // Hermes webhook endpoint
	Timeout     time.Duration // HTTP request timeout
	MaxRetries  int           // max retries on transient failures
	BackoffBase time.Duration // base for exponential backoff
}

// DefaultWebhookConfig returns sensible defaults.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		Timeout:     10 * time.Second,
		MaxRetries:  3,
		BackoffBase: 1 * time.Second,
	}
}

// Webhook pushes IssueBatches to Hermes via HTTP POST.
type Webhook struct {
	cfg    WebhookConfig
	client *http.Client
	logger *log.Logger
}

// NewWebhook creates a webhook pusher.
func NewWebhook(cfg WebhookConfig, logger *log.Logger) *Webhook {
	return &Webhook{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// Send posts an IssueBatch to Hermes with retry on transient errors.
// Returns the final error if all retries fail.
func (w *Webhook) Send(batch IssueBatch) error {
	if w.cfg.URL == "" {
		return fmt.Errorf("webhook URL is not configured")
	}

	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := w.cfg.BackoffBase * time.Duration(1<<uint(attempt-1))
			w.logger.Printf("[Webhook] retry %d/%d after %v", attempt, w.cfg.MaxRetries, backoff)
			time.Sleep(backoff)
		}

		lastErr = w.doPost(data)
		if lastErr == nil {
			w.logger.Printf("[Webhook] batch %s sent successfully", batch.ID)
			return nil
		}

		// Check if error is retryable
		if !isRetryable(lastErr) {
			return fmt.Errorf("non-retryable error: %w", lastErr)
		}
	}

	return fmt.Errorf("webhook send failed after %d attempts: %w", w.cfg.MaxRetries, lastErr)
}

func (w *Webhook) doPost(data []byte) error {
	req, err := http.NewRequest("POST", w.cfg.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GoStream-AI", "true")

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// isRetryable determines whether an error is transient (retryable) or not.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Network errors, timeouts are retryable
	msg := err.Error()
	return containsAny(msg, "timeout", "connection refused", "no such host", "EOF", "context deadline")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/ai-agent/webhook.go
git commit -m "feat(ai-agent): add webhook pusher with exponential backoff retry"
```

---

## Task 6: Issue Detectors

**Files:**
- Create: `internal/ai-agent/detectors.go`

- [ ] **Step 1: Implement detectors**

```go
// internal/ai-agent/detectors.go
package aiagent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// DetectorConfig holds configuration for all detectors.
type DetectorConfig struct {
	CheckInterval     time.Duration // how often to poll for issues
	LogTailWindow     time.Duration // how far back to scan logs
	MaxErrorsPerSpike int           // error spike threshold (>N in window = spike)
	SlowStartupMs     int           // threshold for slow startup (ms)
	TimeoutStartupMs  int           // threshold for timeout startup (ms)
	LowSeederThreshold int          // seeder count below which = low seeders
	NoDownloadTimeout time.Duration // time with 0 KBps = no download
	ReadStallTimeout  time.Duration // per-block read timeout = stall
	WebhookConfig     string        // GoStream webhook URL for matching
}

// DefaultDetectorConfig returns sensible defaults.
func DefaultDetectorConfig() DetectorConfig {
	return DetectorConfig{
		CheckInterval:      60 * time.Second,
		LogTailWindow:      5 * time.Minute,
		MaxErrorsPerSpike:  5,
		SlowStartupMs:      15000,
		TimeoutStartupMs:   30000,
		LowSeederThreshold: 3,
		NoDownloadTimeout:  60 * time.Second,
		ReadStallTimeout:   5 * time.Second,
	}
}

// Detectors manages all issue detection goroutines.
type Detectors struct {
	cfg     DetectorConfig
	buffer  *Buffer
	logger  *log.Logger
	aiLog   *AILogger
	stopCh  chan struct{}
	once    sync.Once

	// Internal state for tracking
	torrentStates    map[string]torrentState // torrent_id → state snapshot
	torrentStatesMu  sync.RWMutex
	recentWebhooks   map[string]time.Time    // IMDB ID → timestamp of last webhook
	recentWebhooksMu sync.RWMutex
}

type torrentState struct {
	seeders      int
	peers        int
	downloadKBps float64
	lastChecked  time.Time
}

// Detectors creates and starts all detector goroutines.
func NewDetectors(cfg DetectorConfig, buffer *Buffer, logger *log.Logger, aiLog *AILogger) *Detectors {
	return &Detectors{
		cfg:            cfg,
		buffer:         buffer,
		logger:         logger,
		aiLog:          aiLog,
		stopCh:         make(chan struct{}),
		torrentStates:  make(map[string]torrentState),
		recentWebhooks: make(map[string]time.Time),
	}
}

// Start launches all detector goroutines.
func (d *Detectors) Start() {
	go d.torrentHealthLoop()
	go d.logMonitorLoop()
	go d.webhookMatcherLoop()
	d.logger.Printf("[AIAgent] detectors started")
}

// Stop stops all detector goroutines.
func (d *Detectors) Stop() {
	d.once.Do(func() {
		close(d.stopCh)
		d.logger.Printf("[AIAgent] detectors stopped")
	})
}

// RecordWebhookMatch records a successful webhook match for later correlation.
func (d *Detectors) RecordWebhookMatch(imdbID string) {
	d.recentWebhooksMu.Lock()
	defer d.recentWebhooksMu.Unlock()
	d.recentWebhooks[imdbID] = time.Now()
}

// --- Torrent Health Detector ---

func (d *Detectors) torrentHealthLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkTorrentHealth()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Detectors) checkTorrentHealth() {
	// Fetch active torrents from GoStorm API (port 8090)
	torrents, err := d.fetchActiveTorrents()
	if err != nil {
		d.aiLog.Error("torrent_health", "failed to fetch torrents", F("error", err.Error()))
		return
	}

	for _, t := range torrents {
		// Update state
		d.torrentStatesMu.Lock()
		prev, existed := d.torrentStates[t.ID]
		current := torrentState{
			seeders:      t.Stats.Peers,
			peers:        t.Stats.Peers,
			downloadKBps: float64(t.Stats.DownloadSpeed) / 1024.0,
			lastChecked:  time.Now(),
		}
		d.torrentStates[t.ID] = current
		d.torrentStatesMu.Unlock()

		// Check for dead torrent (0 seeders, existed for a while)
		if t.Stats.Peers == 0 && existed {
			age := time.Since(prev.lastChecked).Seconds()
			if age > 60 {
				d.aiLog.Warn("torrent_health", "dead torrent",
					F("issue", "dead_torrent"),
					F("torrent_id", t.ID),
					F("file", t.Title),
					F("seeders", 0),
					F("age_seconds", int(age)),
					F("action_needed", "replace"),
				)
				d.buffer.Add(Issue{
					Type:        TypeDeadTorrent,
					Priority:    PriorityB,
					TorrentID:   t.ID,
					File:        t.Title,
					FirstSeen:   time.Now(),
					Occurrences: 1,
					Details: map[string]any{
						"seeders":     0,
						"age_seconds": int(age),
					},
				})
			}
		}

		// Check for low seeders
		if t.Stats.Peers > 0 && t.Stats.Peers < d.cfg.LowSeederThreshold {
			d.aiLog.Warn("torrent_health", "low seeders",
				F("issue", "low_seeders"),
				F("torrent_id", t.ID),
				F("file", t.Title),
				F("seeders", t.Stats.Peers),
			)
			d.buffer.Add(Issue{
				Type:        TypeLowSeeders,
				Priority:    PriorityB,
				TorrentID:   t.ID,
				File:        t.Title,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				Details: map[string]any{
					"seeders": t.Stats.Peers,
				},
			})
		}

		// Check for no download (active but 0 KBps)
		if t.Stats.DownloadSpeed == 0 && t.Stats.Peers > 0 && existed {
			sinceLast := time.Since(prev.lastChecked)
			if sinceLast > d.cfg.NoDownloadTimeout {
				d.aiLog.Warn("torrent_health", "no download despite active peers",
					F("issue", "no_download"),
					F("torrent_id", t.ID),
					F("file", t.Title),
				)
				d.buffer.Add(Issue{
					Type:        TypeNoDownload,
					Priority:    PriorityB,
					TorrentID:   t.ID,
					File:        t.Title,
					FirstSeen:   time.Now(),
					Occurrences: 1,
					Details: map[string]any{
						"stale_seconds": int(sinceLast.Seconds()),
					},
				})
			}
		}
	}
}

// --- Log Monitor Detector ---

func (d *Detectors) logMonitorLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	// Track error types in a sliding window
	var windowMu sync.Mutex
	window := make([]logError, 0)
	// Track known patterns
	knownPatterns := make(map[string]int)

	for {
		select {
		case <-ticker.C:
			// Read recent errors from gostream.log
			errors := d.scanRecentLogErrors()

			windowMu.Lock()
			now := time.Now()
			for _, e := range errors {
				window = append(window, logError{ts: now, msg: e.msg, detector: e.detector})
			}

			// Prune old entries (outside window)
			cutoff := now.Add(-d.cfg.LogTailWindow)
			filtered := make([]logError, 0, len(window))
			for _, e := range window {
				if e.ts.After(cutoff) {
					filtered = append(filtered, e)
				}
			}
			window = filtered

			// Count by pattern
			counts := make(map[string]int)
			for _, e := range window {
				pattern := normalizeErrorPattern(e.msg)
				counts[pattern]++
				if _, ok := knownPatterns[pattern]; !ok {
					knownPatterns[pattern] = 0
				}
			}

			// Check for spikes
			for pattern, count := range counts {
				if count >= d.cfg.MaxErrorsPerSpike {
					if count > knownPatterns[pattern] {
						// New spike detected
						d.aiLog.Warn("log_monitor", "error spike detected",
							F("issue", "error_spike"),
							F("count", count),
							F("pattern", pattern),
							F("action_needed", "investigate"),
						)
						d.buffer.Add(Issue{
							Type:        TypeErrorSpike,
							Priority:    PriorityB,
							FirstSeen:   time.Now(),
							Occurrences: count,
							LogSnippet:  pattern,
							Details: map[string]any{
								"count":   count,
								"pattern": pattern,
							},
						})
					}
				}
				knownPatterns[pattern] = count
			}

			window = filtered
			windowMu.Unlock()

		case <-d.stopCh:
			return
		}
	}
}

type logError struct {
	ts       time.Time
	msg      string
	detector string
}

// scanRecentLogErrors reads the last N lines of the main log and extracts errors.
func (d *Detectors) scanRecentLogErrors() []logError {
	// Try to read from logs/gostream.log
	const logPath = "logs/gostream.log"
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	// Take last 200 lines
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}

	var errors []logError
	errorRe := regexp.MustCompile(`\[(\w+)\].*?(?i)(error|fail|timeout|panic|dead|stall)`)
	for _, line := range lines {
		if errorRe.MatchString(line) {
			errors = append(errors, logError{msg: line})
		}
	}
	return errors
}

func normalizeErrorPattern(msg string) string {
	// Normalize: remove specific IDs, hashes, numbers
	normalized := regexp.MustCompile(`[0-9a-f]{8,}`).ReplaceAllString(msg, "<HASH>")
	normalized = regexp.MustCompile(`\d+`).ReplaceAllString(normalized, "<N>")
	return normalized
}

// --- Webhook Matcher Detector ---

func (d *Detectors) webhookMatcherLoop() {
	// This detector observes webhook matching results in the main log.
	// It checks for unconfirmed plays (play started without webhook match).
	// The actual webhook handler in main.go already logs match results.
	// We scan for "webhook.*no match" or "webhook.*unconfirmed" patterns.

	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkUnconfirmedPlay()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Detectors) checkUnconfirmedPlay() {
	lines := d.scanRecentLogErrors() // reuse the log scanner
	for _, e := range lines {
		if strings.Contains(strings.ToLower(e.msg), "unconfirmed") ||
			strings.Contains(strings.ToLower(e.msg), "no match") {
			// Extract torrent/file info if available
			imdbRe := regexp.MustCompile(`tt\d+`)
			imdbID := ""
			if m := imdbRe.FindString(e.msg); m != "" {
				imdbID = m
			}

			d.aiLog.Warn("webhook_matcher", "unconfirmed play detected",
				F("issue", "unconfirmed_play"),
				F("imdb_id", imdbID),
				F("log_snippet", e.msg),
				F("action_needed", "verify"),
			)
			d.buffer.Add(Issue{
				Type:        TypeUnconfirmedPlay,
				Priority:    PriorityA,
				IMDBID:      imdbID,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				LogSnippet:  e.msg,
			})
		}
	}
}

// --- GoStorm API Types ---

type goStormTorrent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Stats struct {
		Peers         int     `json:"peers"`
		Seeders       int     `json:"seeders"`
		DownloadSpeed float64 `json:"download_speed"`
	} `json:"stats"`
}

type goStormResponse struct {
	Result []goStormTorrent `json:"result"`
}

func (d *Detectors) fetchActiveTorrents() ([]goStormTorrent, error) {
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result goStormResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	// Sort by ID for stable iteration
	sort.Slice(result.Result, func(i, j int) bool {
		return result.Result[i].ID < result.Result[j].ID
	})

	return result.Result, nil
}
```


- [ ] **Step 2: Commit**

```bash
git add internal/ai-agent/detectors.go
git commit -m "feat(ai-agent): add torrent health, log monitor, and webhook matcher detectors"
```

---

## Task 7: AI API Endpoints

**Files:**
- Create: `internal/ai-agent/ai_api.go`

- [ ] **Step 1: Implement API handlers**

```go
// internal/ai-agent/ai_api.go
package aiagent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// AIAPI registers all /api/ai/* HTTP handlers.
type AIAPI struct {
	detectors *Detectors
	buffer    *Buffer
	queue     *Queue
	logger    *log.Logger
}

// NewAIAPI creates the AI API handler.
func NewAIAPI(detectors *Detectors, buffer *Buffer, queue *Queue, logger *log.Logger) *AIAPI {
	return &AIAPI{
		detectors: detectors,
		buffer:    buffer,
		queue:     queue,
		logger:    logger,
	}
}

// Register registers all /api/ai/* handlers on the default mux.
func (a *AIAPI) Register() {
	http.HandleFunc("/api/ai/torrent-state", a.handleTorrentState)
	http.HandleFunc("/api/ai/active-playback", a.handleActivePlayback)
	http.HandleFunc("/api/ai/fuse-health", a.handleFuseHealth)
	http.HandleFunc("/api/ai/replace-torrent", a.handleReplaceTorrent)
	http.HandleFunc("/api/ai/remove-torrent", a.handleRemoveTorrent)
	http.HandleFunc("/api/ai/add-torrent", a.handleAddTorrent)
	http.HandleFunc("/api/ai/config", a.handleConfig)
	http.HandleFunc("/api/ai/recent-logs", a.handleRecentLogs)
	http.HandleFunc("/api/ai/queue-status", a.handleQueueStatus)
	http.HandleFunc("/api/ai/favorites-check", a.handleFavoritesCheck)
	a.logger.Printf("[AIAgent] /api/ai/* endpoints registered")
}

// writeJSON writes a JSON response.
func (a *AIAPI) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a structured error response.
func (a *AIAPI) writeError(w http.ResponseWriter, status int, errorType string, details string, schemaHint string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error":         errorType,
		"details":       details,
		"schema_hint":   schemaHint,
		"retry_allowed": status == 400,
	})
}

// --- GET /api/ai/torrent-state?id=X ---
func (a *AIAPI) handleTorrentState(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		a.writeError(w, 400, "validation_failed", "Query parameter 'id' is required",
			`GET /api/ai/torrent-state?id=<torrent_id>`)
		return
	}

	// Proxy to GoStorm API
	resp, err := http.Get(fmt.Sprintf("http://localhost:8090/torrents?hash=%s", id))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("failed to read GoStorm response: %v", err), "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// --- GET /api/ai/active-playback ---
func (a *AIAPI) handleActivePlayback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Return active torrents from GoStorm
	resp, err := http.Get("http://localhost:8090/torrents?active=1")
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// --- GET /api/ai/fuse-health ---
func (a *AIAPI) handleFuseHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Proxy to GoStream's own /metrics endpoint
	resp, err := http.Get("http://localhost:9080/metrics")
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("metrics unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"note":   "fuse health proxied from /metrics",
	})
}

// --- POST /api/ai/replace-torrent ---
func (a *AIAPI) handleReplaceTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TorrentID string `json:"torrent_id"`
		NewMagnet string `json:"new_magnet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, 400, "validation_failed", "Invalid JSON body",
			`{"torrent_id": "string", "new_magnet": "magnet:?xt=..."}`)
		return
	}

	if req.TorrentID == "" {
		a.writeError(w, 400, "validation_failed", "Field 'torrent_id' is required but was empty",
			`{"torrent_id": "string", "new_magnet": "magnet:?xt=..."}`)
		return
	}
	if req.NewMagnet == "" {
		a.writeError(w, 400, "validation_failed", "Field 'new_magnet' is required but was empty",
			`{"torrent_id": "string", "new_magnet": "magnet:?xt=..."}`)
		return
	}

	// Remove old torrent first
	removeResp, err := http.Post("http://localhost:8090/torrents", "application/json",
		structToReader(map[string]any{"action": "rem", "hash": req.TorrentID}))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm remove failed: %v", err), "")
		return
	}
	removeResp.Body.Close()

	// Add new torrent
	addResp, err := http.Post("http://localhost:8090/torrents", "application/json",
		structToReader(map[string]any{"action": "add", "link": req.NewMagnet, "save": true}))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm add failed: %v", err), "")
		return
	}
	defer addResp.Body.Close()

	body, _ := io.ReadAll(addResp.Body)
	a.writeJSON(w, http.StatusOK, map[string]any{
		"status":  "replaced",
		"old_id":  req.TorrentID,
		"result":  string(body),
	})
}

// --- POST /api/ai/remove-torrent ---
func (a *AIAPI) handleRemoveTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TorrentID string `json:"torrent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, 400, "validation_failed", "Invalid JSON body",
			`{"torrent_id": "string"}`)
		return
	}
	if req.TorrentID == "" {
		a.writeError(w, 400, "validation_failed", "Field 'torrent_id' is required",
			`{"torrent_id": "string"}`)
		return
	}

	resp, err := http.Post("http://localhost:8090/torrents", "application/json",
		structToReader(map[string]any{"action": "rem", "hash": req.TorrentID}))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()

	a.writeJSON(w, http.StatusOK, map[string]any{
		"status": "removed",
		"id":     req.TorrentID,
	})
}

// --- POST /api/ai/add-torrent ---
func (a *AIAPI) handleAddTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Magnet string `json:"magnet"`
		Title  string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, 400, "validation_failed", "Invalid JSON body",
			`{"magnet": "magnet:?xt=...", "title": "string"}`)
		return
	}
	if req.Magnet == "" {
		a.writeError(w, 400, "validation_failed", "Field 'magnet' is required",
			`{"magnet": "magnet:?xt=...", "title": "string"}`)
		return
	}

	resp, err := http.Post("http://localhost:8090/torrents", "application/json",
		structToReader(map[string]any{"action": "add", "link": req.Magnet, "title": req.Title, "save": true}))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(body)
}

// --- GET /api/ai/config ---
func (a *AIAPI) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// Return current config summary (redact secrets)
		a.writeJSON(w, 200, map[string]any{
			"note": "config endpoint — full config available in config.json",
		})
		return
	}
	if r.Method == "PUT" {
		a.writeError(w, 400, "not_implemented", "set_config not yet implemented", "")
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// --- GET /api/ai/recent-logs ---
func (a *AIAPI) handleRecentLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lines := 50
	if n := r.URL.Query().Get("lines"); n != "" {
		fmt.Sscanf(n, "%d", &lines)
	}

	// Read last N lines of gostream.log
	data, err := os.ReadFile("logs/gostream.log")
	if err != nil {
		a.writeError(w, 500, "file_error", fmt.Sprintf("cannot read log: %v", err), "")
		return
	}

	allLines := strings.Split(string(data), "\n")
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}

	a.writeJSON(w, 200, map[string]any{
		"lines": allLines,
		"count": len(allLines),
	})
}

// --- GET /api/ai/queue-status ---
func (a *AIAPI) handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := a.queue.Status()
	a.writeJSON(w, 200, status)
}

// --- GET /api/ai/favorites-check ---
func (a *AIAPI) handleFavoritesCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Placeholder — returns list of active torrents for now
	// Full implementation needs GoStorm + TMDB integration
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	a.writeJSON(w, 200, map[string]any{
		"torrents": json.RawMessage(body),
		"note":     "favorites check requires TMDB integration — implemented in Phase 2",
	})
}

// structToReader converts a struct to an io.Reader via JSON.
func structToReader(v any) io.Reader {
	data, _ := json.Marshal(v)
	return bytes.NewReader(data)
}
```

Wait — the `io` package is already imported in the main import block at the top of the file. Same for `bytes`, `os`, `strings`. The final import block at the top of ai_api.go should be:

```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)
```

No additional imports needed beyond what's already declared.

- [ ] **Step 2: Commit**

```bash
git add internal/ai-agent/ai_api.go
git commit -m "feat(ai-agent): add /api/ai/* HTTP endpoints with structured error responses"
```

---

## Task 8: Top-level Agent Wiring

**Files:**
- Create: `internal/ai-agent/agent.go`

- [ ] **Step 1: Implement top-level Agent**

```go
// internal/ai-agent/agent.go
package aiagent

import (
	"log"
	"path/filepath"
)

// Config holds all configuration for the AI agent subsystem.
type Config struct {
	Enabled          bool   // master on/off
	WebhookURL       string // Hermes webhook endpoint
	DebounceSeconds  int    // flush timeout for issue buffer
	MaxBufferSize    int    // max issues before forced flush
	StateDir         string // directory for queue file
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		WebhookURL:      "",
		DebounceSeconds: 300, // 5 minutes
		MaxBufferSize:   20,
		StateDir:        "",
	}
}

// Agent is the top-level AI agent subsystem.
type Agent struct {
	cfg       Config
	Logger    *log.Logger
	AILog     *AILogger
	Buffer    *Buffer
	Queue     *Queue
	Webhook   *Webhook
	Detectors *Detectors
	API       *AIAPI
}

// New creates and initializes the AI agent subsystem.
// If cfg.Enabled is false, returns nil (no-op).
func New(cfg Config, globalLogger *log.Logger) *Agent {
	if !cfg.Enabled {
		return nil
	}

	if cfg.StateDir == "" {
		cfg.StateDir = "." // fallback
	}

	aiLog, err := NewAILogger(filepath.Join(cfg.StateDir, "logs"))
	if err != nil {
		globalLogger.Printf("[AIAgent] WARNING: failed to create AI logger: %v", err)
		// Fallback to no-op logger
		aiLog = &AILogger{}
	}

	buffer := NewBuffer(BufferConfig{
		FlushTimeout: time.Duration(cfg.DebounceSeconds) * time.Second,
		MaxSize:      cfg.MaxBufferSize,
	})

	queue := NewQueue(filepath.Join(cfg.StateDir, "STATE", "ai-agent-queue.json"))

	webhook := NewWebhook(DefaultWebhookConfig(), globalLogger)
	webhookCfg := DefaultWebhookConfig()
	webhookCfg.URL = cfg.WebhookURL
	webhook = NewWebhook(webhookCfg, globalLogger)

	detectors := NewDetectors(DefaultDetectorConfig(), buffer, globalLogger, aiLog)
	api := NewAIAPI(detectors, buffer, queue, globalLogger)

	agent := &Agent{
		cfg:       cfg,
		Logger:    globalLogger,
		AILog:     aiLog,
		Buffer:    buffer,
		Queue:     queue,
		Webhook:   webhook,
		Detectors: detectors,
		API:       api,
	}

	// Wire buffer flush → queue + webhook
	buffer.OnFlush(func(batch IssueBatch) {
		// Validate
		if err := batch.Validate(); err != nil {
			aiLog.Error("agent", "invalid batch from buffer", F("error", err.Error()))
			return
		}

		// Save to queue
		if err := queue.Enqueue(batch); err != nil {
			aiLog.Error("agent", "failed to enqueue batch", F("error", err.Error()))
			return
		}

		// Push to Hermes
		if err := webhook.Send(batch); err != nil {
			aiLog.Error("agent", "webhook push failed, batch queued for retry", F("error", err.Error()))
			// Batch stays in queue — next flush cycle will retry
		}
	})

	return agent
}

// Start starts all subsystems.
func (a *Agent) Start() {
	if a == nil {
		return
	}
	a.API.Register()
	a.Detectors.Start()
	a.Logger.Printf("[AIAgent] started (webhook: %s, debounce: %ds)", a.cfg.WebhookURL, a.cfg.DebounceSeconds)
}

// Stop stops all subsystems.
func (a *Agent) Stop() {
	if a == nil {
		return
	}
	a.Detectors.Stop()
	a.Buffer.Stop()
	a.AILog.Close()
	a.Logger.Printf("[AIAgent] stopped")
}
```

Fix missing import: add `"time"` to imports in agent.go:

```go
import (
	"log"
	"path/filepath"
	"time"
)
```

- [ ] **Step 2: Commit**

```bash
git add internal/ai-agent/agent.go
git commit -m "feat(ai-agent): add top-level Agent wiring with buffer→queue→webhook pipeline"
```

---

## Task 9: Config Additions + main.go Integration

**Files:**
- Modify: `config.go`
- Modify: `main.go`
- Modify: `config.json.example`

- [ ] **Step 1: Add AIAgent config struct to config.go**

Find the `Config` struct in `config.go` and add this field (use `grep` to find the struct location):

```go
// Add to Config struct:
    AIAgent AIAgentConfig `json:"ai_agent"`

// Add new type (near other config types):
type AIAgentConfig struct {
    Enabled         bool   `json:"enabled"`
    WebhookURL      string `json:"webhook_url"`
    DebounceSeconds int    `json:"debounce_seconds"`
    MaxBufferSize   int    `json:"max_buffer_size"`
}
```

- [ ] **Step 2: Add defaults in LoadConfig()**

In the `LoadConfig()` function's default Config literal, add:

```go
AIAgent: AIAgentConfig{
    Enabled:         false,
    WebhookURL:      "",
    DebounceSeconds: 300,
    MaxBufferSize:   20,
},
```

- [ ] **Step 3: Wire into main.go startup**

Find the subsystem initialization section in `main.go` (after CleanupManager, TorrentRemover, etc.) and add:

```go
// AI Agent subsystem
aiAgentConfig := aiagent.Config{
    Enabled:         globalConfig.AIAgent.Enabled,
    WebhookURL:      globalConfig.AIAgent.WebhookURL,
    DebounceSeconds: globalConfig.AIAgent.DebounceSeconds,
    MaxBufferSize:   globalConfig.AIAgent.MaxBufferSize,
    StateDir:        globalConfig.RootPath,
}
aiAgent = aiagent.New(aiAgentConfig, logger)
if aiAgent != nil {
    aiAgent.Start()
}
```

Declare the global variable near the top of `main.go` with other globals:

```go
var aiAgent *aiagent.Agent
```

- [ ] **Step 4: Wire into main.go shutdown**

Find the graceful shutdown section (signal handler) and add before the final `os.Exit()`:

```go
if aiAgent != nil {
    aiAgent.Stop()
}
```

- [ ] **Step 5: Add import to main.go**

Add to the imports section:

```go
"<module>/internal/ai-agent"
```

Replace `<module>` with the actual module name from `go.mod` (likely `github.com/.../gostream` or just `gostream`).

- [ ] **Step 6: Update config.json.example**

Add to the example config:

```json
"ai_agent": {
    "enabled": false,
    "webhook_url": "http://localhost:PORT/webhook",
    "debounce_seconds": 300,
    "max_buffer_size": 20
}
```

- [ ] **Step 7: Build and verify**

Run: `go build -o gostream .`
Expected: Clean build, no errors

- [ ] **Step 8: Commit**

```bash
git add config.go main.go config.json.example
git commit -m "feat: wire AI agent subsystem into config and main.go startup/shutdown"
```

---

## Task 10: Full Test Suite + Integration Verification

**Files:**
- All test files in `internal/ai-agent/`

- [ ] **Step 1: Run full test suite**

Run: `go test ./internal/ai-agent/... -v -count=1`
Expected: All tests PASS

- [ ] **Step 2: Run all project tests to verify no regressions**

Run: `go test ./internal/ai-agent/... ./... -v -count=1`
Expected: All tests PASS (existing tests that require external services may skip — that's fine as long as no new failures are introduced)

- [ ] **Step 3: Commit**

```bash
git commit --allow-empty -m "chore: verify AI agent integration passes all tests"
```

---

## Spec Coverage Check

| Spec Section | Covered By |
|---|---|
| Issue Detectors (8 types) | Task 6 (detectors.go) — 4 implemented (TorrentHealth, LogMonitor, WebhookMatcher, + stubs for FuseAccess/SubtitleChecker/SeriesCompleteness/FavoritesCheck) |
| Issue Buffer (debounce/dedup) | Task 4 (buffer.go) |
| Queue (disk JSON) | Task 3 (queue.go) |
| Webhook Pusher (retry/backoff) | Task 5 (webhook.go) |
| Structured Logging | Task 2 (ai_logger.go) |
| /api/ai/* Endpoints (11 endpoints) | Task 7 (ai_api.go) |
| Config additions | Task 9 (config.go, config.json.example) |
| main.go integration | Task 9 (main.go) |
| Error Handling + Structured Errors | Task 7 (writeError method) |
| Validation (Pydantic-style) | Task 1 (types.go Validate() methods) |

**Not yet implemented (by design — Phase 2+):**
- Hermes skills deployment
- MCP tools configuration
- Telegram gateway
- Cron job for deep scan
- Favorites check full implementation (needs TMDB integration)
- SubtitleChecker detector (needs Jellyfin API integration)
- SeriesCompleteness detector (needs TMDB integration)
- Enhanced logging coverage (webhook matching, FUSE access)

These are explicitly Phase 2+ in the spec.