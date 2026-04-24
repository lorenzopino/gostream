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
		FlushTimeout: 10 * time.Second,
		MaxSize:      3,
	})

	flushed := make(chan IssueBatch, 1)
	buf.OnFlush(func(batch IssueBatch) {
		flushed <- batch
	})

	now := time.Now()
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", TorrentID: "t1", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "slow_startup", Priority: "B", TorrentID: "t2", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "fuse_error", Priority: "B", TorrentID: "t3", FirstSeen: now, Occurrences: 1})

	select {
	case batch := <-flushed:
		if len(batch.Issues) != 3 {
			t.Fatalf("expected 3 issues in flushed batch, got %d", len(batch.Issues))
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected flush on size, but no flush occurred")
	}

	if buf.Len() != 0 {
		t.Fatalf("expected buffer empty after flush, got %d", buf.Len())
	}
}

func TestBuffer_FlushOnTimeout(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 100 * time.Millisecond,
		MaxSize:      20,
	})
	defer buf.Stop()

	flushed := make(chan IssueBatch, 1)
	buf.OnFlush(func(batch IssueBatch) {
		flushed <- batch
	})

	now := time.Now()
	buf.Add(Issue{
		Type:        "dead_torrent",
		Priority:    "B",
		TorrentID:   "abc",
		FirstSeen:   now,
		Occurrences: 1,
	})

	select {
	case <-flushed:
		// expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected flush on timeout")
	}
}

func TestBuffer_PriorityOrderInFlush(t *testing.T) {
	buf := NewBuffer(BufferConfig{
		FlushTimeout: 100 * time.Millisecond,
		MaxSize:      20,
	})
	defer buf.Stop()

	flushed := make(chan IssueBatch, 1)
	buf.OnFlush(func(batch IssueBatch) {
		flushed <- batch
	})

	now := time.Now()
	buf.Add(Issue{Type: "missing_subtitles", Priority: "D", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "wrong_match", Priority: "A", FirstSeen: now, Occurrences: 1})

	select {
	case batch := <-flushed:
		if batch.Issues[0].Priority != "B" {
			t.Fatalf("expected first issue priority B, got %s", batch.Issues[0].Priority)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected flush")
	}
}
