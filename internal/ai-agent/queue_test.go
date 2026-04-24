package aiagent

import (
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
	// Should be ordered B > A > C > D (B=1, A=2, C=3, D=4)
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

func TestQueue_MarkComplete(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir + "/test-complete.json")

	q.Enqueue(newTestBatch("batch-1", "B"))
	q.Enqueue(newTestBatch("batch-2", "A"))

	if err := q.MarkComplete("batch-1"); err != nil {
		t.Fatalf("mark complete failed: %v", err)
	}

	status := q.Status()
	if status.PendingBatches != 1 {
		t.Fatalf("expected 1 pending after mark complete, got %d", status.PendingBatches)
	}

	batches, err := q.DequeueAll()
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if len(batches) != 1 || batches[0].ID != "batch-2" {
		t.Fatalf("expected only batch-2, got %d batches", len(batches))
	}
}

func TestQueue_MarkFailed(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir + "/test-failed.json")

	q.Enqueue(newTestBatch("batch-fail", "B"))
	q.MarkFailed("batch-fail")

	status := q.Status()
	if status.FailedBatches != 1 {
		t.Fatalf("expected 1 failed, got %d", status.FailedBatches)
	}
	if status.PendingBatches != 0 {
		t.Fatalf("expected 0 pending after mark failed, got %d", status.PendingBatches)
	}
}
