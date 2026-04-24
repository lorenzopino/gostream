package aiagent

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestWebhook_Send_Success(t *testing.T) {
	var receivedBody IssueBatch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected application/json content type")
		}
		if r.Header.Get("X-GoStream-AI") != "true" {
			t.Fatalf("expected X-GoStream-AI header")
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultWebhookConfig()
	cfg.URL = server.URL
	cfg.MaxRetries = 1
	w := NewWebhook(cfg, log.New(os.Stderr, "[test] ", 0))

	batch := IssueBatch{
		ID:      "batch-test",
		Issues:  []Issue{{Type: "dead_torrent", Priority: "B", FirstSeen: time.Now(), Occurrences: 1}},
		Created: time.Now(),
		Source:  "realtime",
	}

	if err := w.Send(batch); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if receivedBody.ID != "batch-test" {
		t.Fatalf("expected batch-test, got %s", receivedBody.ID)
	}
}

func TestWebhook_Send_UnconfiguredURL(t *testing.T) {
	w := NewWebhook(WebhookConfig{}, log.New(os.Stderr, "[test] ", 0))
	err := w.Send(IssueBatch{})
	if err == nil {
		t.Fatal("expected error for unconfigured URL")
	}
}

func TestWebhook_Send_RetryableError(t *testing.T) {
	// Server always returns 503 — should retry
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := DefaultWebhookConfig()
	cfg.URL = server.URL
	cfg.MaxRetries = 2
	cfg.BackoffBase = 10 * time.Millisecond // fast for test
	w := NewWebhook(cfg, log.New(os.Stderr, "[test] ", 0))

	batch := IssueBatch{
		ID:      "batch-retry",
		Issues:  []Issue{{Type: "dead_torrent", Priority: "B", FirstSeen: time.Now(), Occurrences: 1}},
		Created: time.Now(),
		Source:  "realtime",
	}

	err := w.Send(batch)
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if callCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", callCount)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		msg      string
		expected bool
	}{
		{"connection refused", true},
		{"dial tcp: timeout", true},
		{"no such host", true},
		{"EOF", true},
		{"context deadline exceeded", true},
		{"HTTP 400: bad request", false},
		{"some random error", false},
	}
	for _, tt := range tests {
		if got := isRetryable(errors.New(tt.msg)); got != tt.expected {
			t.Errorf("isRetryable(%q) = %v, want %v", tt.msg, got, tt.expected)
		}
	}
}
