package aiagent

import (
	"log"
	"path/filepath"
	"time"
)

// Config holds all configuration for the AI agent subsystem.
type Config struct {
	Enabled         bool
	WebhookURL      string
	DebounceSeconds int
	MaxBufferSize   int
	StateDir        string
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		WebhookURL:      "",
		DebounceSeconds: 300,
		MaxBufferSize:   20,
		StateDir:        "",
	}
}

// Agent is the top-level AI agent subsystem.
type Agent struct {
	cfg       Config
	Logger    *log.Logger
	AILog     *AILogger
	Buffer    *Buffer
	Queue     *Queue
	Webhook   *Webhook
	Detectors *Detectors
	API       *AIAPI
}

// New creates and initializes the AI agent subsystem.
// If cfg.Enabled is false, returns nil (no-op).
func New(cfg Config, globalLogger *log.Logger) *Agent {
	if !cfg.Enabled {
		return nil
	}

	if cfg.StateDir == "" {
		cfg.StateDir = "."
	}

	aiLog, err := NewAILogger(filepath.Join(cfg.StateDir, "logs"))
	if err != nil {
		globalLogger.Printf("[AIAgent] WARNING: failed to create AI logger: %v", err)
		aiLog = &AILogger{}
	}

	buffer := NewBuffer(BufferConfig{
		FlushTimeout: time.Duration(cfg.DebounceSeconds) * time.Second,
		MaxSize:      cfg.MaxBufferSize,
	})

	queuePath := filepath.Join(cfg.StateDir, "STATE", "ai-agent-queue.json")
	queue := NewQueue(queuePath)

	webhookCfg := DefaultWebhookConfig()
	webhookCfg.URL = cfg.WebhookURL
	webhook := NewWebhook(webhookCfg, globalLogger)

	detectors := NewDetectors(DefaultDetectorConfig(), buffer, globalLogger, aiLog)
	api := NewAIAPI(detectors, buffer, queue, globalLogger)

	agent := &Agent{
		cfg:       cfg,
		Logger:    globalLogger,
		AILog:     aiLog,
		Buffer:    buffer,
		Queue:     queue,
		Webhook:   webhook,
		Detectors: detectors,
		API:       api,
	}

	// Wire buffer flush → queue + webhook
	buffer.OnFlush(func(batch IssueBatch) {
		if err := batch.Validate(); err != nil {
			aiLog.Error("agent", "invalid batch from buffer", F("error", err.Error()))
			return
		}
		if err := queue.Enqueue(batch); err != nil {
			aiLog.Error("agent", "failed to enqueue batch", F("error", err.Error()))
			return
		}
		if err := webhook.Send(batch); err != nil {
			aiLog.Error("agent", "webhook push failed, batch queued for retry", F("error", err.Error()))
		}
	})

	return agent
}

// Start starts all subsystems.
func (a *Agent) Start() {
	if a == nil {
		return
	}
	a.API.Register()
	a.Detectors.Start()
	a.Logger.Printf("[AIAgent] started (webhook: %s, debounce: %ds)", a.cfg.WebhookURL, a.cfg.DebounceSeconds)
}

// Stop stops all subsystems.
func (a *Agent) Stop() {
	if a == nil {
		return
	}
	a.Detectors.Stop()
	a.Buffer.Stop()
	a.AILog.Close()
	a.Logger.Printf("[AIAgent] stopped")
}
