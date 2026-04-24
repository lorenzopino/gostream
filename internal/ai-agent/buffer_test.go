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
	buf.Add(Issue{Type: "missing_subtitles", Priority: "D", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "dead_torrent", Priority: "B", FirstSeen: now, Occurrences: 1})
	buf.Add(Issue{Type: "wrong_match", Priority: "A", FirstSeen: now, Occurrences: 1})

	time.Sleep(200 * time.Millisecond)

	if len(flushedBatches) == 0 {
		t.Fatal("expected flush")
	}
	batch := flushedBatches[0]
	if batch.Issues[0].Priority != "B" {
		t.Fatalf("expected first issue priority B, got %s", batch.Issues[0].Priority)
	}
}
