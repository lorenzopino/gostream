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
	TypeDeadTorrent        = "dead_torrent"
	TypeLowSeeders         = "low_seeders"
	TypeNoDownload         = "no_download"
	TypeSlowStartup        = "slow_startup"
	TypeTimeoutStartup     = "timeout_startup"
	TypeWrongMatch         = "wrong_match"
	TypeUnconfirmedPlay    = "unconfirmed_play"
	TypeFuseError          = "fuse_error"
	TypeReadStall          = "read_stall"
	TypeErrorSpike         = "error_spike"
	TypePatternAnomaly     = "pattern_anomaly"
	TypeMissingSubtitles   = "missing_subtitles"
	TypeIncompleteSeries   = "incomplete_series"
	TypeIncompleteDownload = "incomplete_download"
)

var validTypes = map[string]bool{
	TypeDeadTorrent:        true,
	TypeLowSeeders:         true,
	TypeNoDownload:         true,
	TypeSlowStartup:        true,
	TypeTimeoutStartup:     true,
	TypeWrongMatch:         true,
	TypeUnconfirmedPlay:    true,
	TypeFuseError:          true,
	TypeReadStall:          true,
	TypeErrorSpike:         true,
	TypePatternAnomaly:     true,
	TypeMissingSubtitles:   true,
	TypeIncompleteSeries:   true,
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
	Type        string         `json:"type"`
	Priority    string         `json:"priority"`
	TorrentID   string         `json:"torrent_id,omitempty"`
	File        string         `json:"file,omitempty"`
	IMDBID      string         `json:"imdb_id,omitempty"`
	Details     map[string]any `json:"details"`
	FirstSeen   time.Time      `json:"first_seen"`
	Occurrences int            `json:"occurrences"`
	LogSnippet  string         `json:"log_snippet,omitempty"`
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
	ID      string    `json:"id"`
	Issues  []Issue   `json:"issues"`
	Created time.Time `json:"created"`
	Source  string    `json:"source"`
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
