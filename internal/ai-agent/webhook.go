package aiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// WebhookConfig holds the configuration for pushing batches to Hermes.
type WebhookConfig struct {
	URL         string        // Hermes webhook endpoint
	Timeout     time.Duration // HTTP request timeout
	MaxRetries  int           // max retries on transient failures
	BackoffBase time.Duration // base for exponential backoff
}

// DefaultWebhookConfig returns sensible defaults.
func DefaultWebhookConfig() WebhookConfig {
	return WebhookConfig{
		Timeout:     10 * time.Second,
		MaxRetries:  3,
		BackoffBase: 1 * time.Second,
	}
}

// Webhook pushes IssueBatches to Hermes via HTTP POST.
type Webhook struct {
	cfg    WebhookConfig
	client *http.Client
	logger *log.Logger
}

// NewWebhook creates a webhook pusher.
func NewWebhook(cfg WebhookConfig, logger *log.Logger) *Webhook {
	return &Webhook{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// Send posts an IssueBatch to Hermes with retry on transient errors.
// Returns the final error if all retries fail.
func (w *Webhook) Send(batch IssueBatch) error {
	if w.cfg.URL == "" {
		return fmt.Errorf("webhook URL is not configured")
	}

	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < w.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := w.cfg.BackoffBase * time.Duration(1<<uint(attempt-1))
			w.logger.Printf("[Webhook] retry %d/%d after %v", attempt, w.cfg.MaxRetries, backoff)
			time.Sleep(backoff)
		}

		lastErr = w.doPost(data)
		if lastErr == nil {
			w.logger.Printf("[Webhook] batch %s sent successfully", batch.ID)
			return nil
		}

		// Check if error is retryable
		if !isRetryable(lastErr) {
			return fmt.Errorf("non-retryable error: %w", lastErr)
		}
	}

	return fmt.Errorf("webhook send failed after %d attempts: %w", w.cfg.MaxRetries, lastErr)
}

func (w *Webhook) doPost(data []byte) error {
	req, err := http.NewRequest("POST", w.cfg.URL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GoStream-AI", "true")

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		// Server errors are retryable
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	if resp.StatusCode >= 400 {
		// Client errors are NOT retryable
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// isRetryable determines whether an error is transient (retryable) or not.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Network errors
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "context deadline") {
		return true
	}
	// HTTP 5xx server errors
	if strings.Contains(msg, "HTTP 5") {
		return true
	}
	return false
}
