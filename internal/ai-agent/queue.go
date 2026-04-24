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
	PendingBatches    int `json:"pending_batches"`
	ProcessingBatches int `json:"processing_batches"`
	FailedBatches     int `json:"failed_batches"`
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

	var remaining []queueEntry
	var result []IssueBatch

	for _, e := range q.data.Batches {
		if e.Status == "pending" {
			e.Status = "processing"
			result = append(result, e.Batch)
			remaining = append(remaining, e)
		}
	}

	// Sort by priority
	sort.Slice(result, func(i, j int) bool {
		ri := PriorityRank(result[i].Issues[0].Priority)
		rj := PriorityRank(result[j].Issues[0].Priority)
		return ri < rj
	})

	q.data.Batches = remaining
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
