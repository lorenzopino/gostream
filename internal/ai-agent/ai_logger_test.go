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

func TestAILogger_InfoLevel(t *testing.T) {
	dir := t.TempDir()
	l, err := NewAILogger(dir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer l.Close()

	l.Info("startup", "agent initialized")

	data, err := os.ReadFile(dir + "/gostream-ai.log")
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[info]") {
		t.Fatalf("expected '[info]' in log, got: %s", content)
	}
}

func TestAILogger_ErrorLevel(t *testing.T) {
	dir := t.TempDir()
	l, err := NewAILogger(dir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer l.Close()

	l.Error("webhook", "failed to push batch", F("error", "connection refused"))

	data, err := os.ReadFile(dir + "/gostream-ai.log")
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "[error]") {
		t.Fatalf("expected '[error]' in log, got: %s", content)
	}
}

func TestAILogger_CustomFields(t *testing.T) {
	dir := t.TempDir()
	l, err := NewAILogger(dir)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer l.Close()

	l.Warn("test_detector", "custom test",
		F("custom_field", "custom_value"),
	)

	data, err := os.ReadFile(dir + "/gostream-ai.log")
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "custom_field") {
		t.Fatalf("expected 'custom_field' in JSON log entry, got: %s", content)
	}
}

func TestAILogger_DefaultDir(t *testing.T) {
	// Should not panic even with empty dir
	l, err := NewAILogger("")
	if err != nil {
		// Expected if 'logs' dir doesn't exist or permission denied
		t.Logf("NewAILogger('') returned error (expected): %v", err)
		return
	}
	l.Close()
}
