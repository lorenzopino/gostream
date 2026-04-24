package aiagent

import (
	"sync"
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
		MaxSize:      3,                // small max size for test
	})
	defer buf.Stop()

	var flushedBatches []IssueBatch
	var mu sync.Mutex
	buf.OnFlush(func(batch IssueBatch) {
		mu.Lock()
		defer mu.Unlock()
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
	// Note: same dedup key → 1 unique issue. Need different keys to trigger size flush.
	// Re-test with unique keys:
	buf2 := NewBuffer(BufferConfig{
		FlushTimeout: 10 * time.Second,
		MaxSize:      3,
	})
	var flushedBatches2 []IssueBatch
	var mu2 sync.Mutex
	buf2.OnFlush(func(batch IssueBatch) {
		mu2.Lock()
		defer mu2.Unlock()
		flushedBatches2 = append(flushedBatches2, batch)
	})
	for i := 0; i < 3; i++ {
		buf2.Add(Issue{
			Type:        "dead_torrent",
			Priority:    "B",
			TorrentID:   "unique-id",
			File:        "Movie.mkv",
			FirstSeen:   now,
			Occurrences: 1,
		})
	}
	// Still same dedup key. Use different file names:
	buf3 := NewBuffer(BufferConfig{
		FlushTimeout: 10 * time.Second,
		MaxSize:      3,
	})
	var flushedBatches3 []IssueBatch
	var mu3 sync.Mutex
	buf3.OnFlush(func(batch IssueBatch) {
		mu3.Lock()
		defer mu3.Unlock()
		flushedBatches3 = append(flushedBatches3, batch)
	})
	for i := 0; i < 3; i++ {
		buf3.Add(Issue{
			Type:        "dead_torrent",
			Priority:    "B",
			TorrentID:   "torrent",
			File:        "Movie.mkv",
			FirstSeen:   now,
			Occurrences: 1,
		})
	}
	// Still same key. Let's use actually different issues:
	buf4 := NewBuffer(BufferConfig{
		FlushTimeout: 10 * time.Second,
		MaxSize:      3,
	})
	var flushedBatches4 []IssueBatch
	var mu4 sync.Mutex
	buf4.OnFlush(func(batch IssueBatch) {
		mu4.Lock()
		defer mu4.Unlock()
		flushedBatches4 = append(flushedBatches4, batch)
	})
	buf4.Add(Issue{Type: "dead_torrent", Priority: "B", TorrentID: "t1", FirstSeen: now, Occurrences: 1})
	buf4.Add(Issue{Type: "slow_startup", Priority: "B", TorrentID: "t2", FirstSeen: now, Occurrences: 1})
	buf4.Add(Issue{Type: "fuse_error", Priority: "B", TorrentID: "t3", FirstSeen: now, Occurrences: 1})

	time.Sleep(50 * time.Millisecond)

	mu4.Lock()
	defer mu4.Unlock()
	if len(flushedBatches4) == 0 {
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
	var mu sync.Mutex
	buf.OnFlush(func(batch IssueBatch) {
		mu.Lock()
		defer mu.Unlock()
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

	mu.Lock()
	defer mu.Unlock()
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
	var mu sync.Mutex
	buf.OnFlush(func(batch IssueBatch) {
		mu.Lock()
		defer mu.Unlock()
		flushedBatches = append(flushedBatches, batch)
	})

	now := time.Now()
	// Add in reverse priority order
	buf.Add(Issue{Type: "missing_subtitles", Priority: "D", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "wrong_match", Priority: "A", FirstSeen: now, Occurrences: 1})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushedBatches) == 0 {
		t.Fatal("expected flush")
	}
	batch := flushedBatches[0]
	// First issue in batch should be priority B (most urgent)
	if batch.Issues[0].Priority != "B" {
		t.Fatalf("expected first issue priority B, got %s", batch.Issues[0].Priority)
	}
}
