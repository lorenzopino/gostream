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
