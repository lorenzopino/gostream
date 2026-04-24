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
