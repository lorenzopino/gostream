package aiagent

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"
)

// AILogger writes structured JSON entries to the AI agent log file.
type AILogger struct {
	logger *log.Logger
	file   *os.File
}

// AILogEntry is the structured format for AI agent log entries.
type AILogEntry struct {
	Timestamp time.Time      `json:"ts"`
	Level     string         `json:"level"`
	Detector  string         `json:"detector"`
	Issue     string         `json:"issue,omitempty"`
	TorrentID string         `json:"torrent_id,omitempty"`
	File      string         `json:"file,omitempty"`
	IMDBID    string         `json:"imdb_id,omitempty"`
	Seeders   *int           `json:"seeders,omitempty"`
	Peers     *int           `json:"peers,omitempty"`
	AgeSecs   *int           `json:"age_seconds,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	Action    string         `json:"action_needed,omitempty"`
	Message   string         `json:"message,omitempty"`
}

// NewAILogger creates a logger that writes to both stdout and the AI log file.
func NewAILogger(logDir string) (*AILogger, error) {
	if logDir == "" {
		logDir = "logs"
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	filePath := logDir + "/gostream-ai.log"
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &AILogger{
		logger: log.New(io.MultiWriter(os.Stdout, file), "[AIAgent] ", log.LstdFlags),
		file:   file,
	}, nil
}

// Info logs an info-level entry.
func (l *AILogger) Info(detector, msg string, fields ...AILogField) {
	l.write("info", detector, msg, fields)
}

// Warn logs a warning-level entry.
func (l *AILogger) Warn(detector, msg string, fields ...AILogField) {
	l.write("warn", detector, msg, fields)
}

// Error logs an error-level entry.
func (l *AILogger) Error(detector, msg string, fields ...AILogField) {
	l.write("error", detector, msg, fields)
}

// AILogField is a key-value pair for structured log entries.
type AILogField struct {
	Key   string
	Value any
}

// F is a convenience function for creating log fields.
func F(key string, value any) AILogField {
	return AILogField{Key: key, Value: value}
}

func (l *AILogger) write(level, detector, msg string, fields []AILogField) {
	entry := AILogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Detector:  detector,
		Message:   msg,
	}
	for _, f := range fields {
		switch f.Key {
		case "issue":
			if v, ok := f.Value.(string); ok {
				entry.Issue = v
			}
		case "torrent_id":
			if v, ok := f.Value.(string); ok {
				entry.TorrentID = v
			}
		case "file":
			if v, ok := f.Value.(string); ok {
				entry.File = v
			}
		case "imdb_id":
			if v, ok := f.Value.(string); ok {
				entry.IMDBID = v
			}
		case "seeders":
			if v, ok := f.Value.(int); ok {
				entry.Seeders = &v
			}
		case "peers":
			if v, ok := f.Value.(int); ok {
				entry.Peers = &v
			}
		case "age_seconds":
			if v, ok := f.Value.(int); ok {
				entry.AgeSecs = &v
			}
		case "action_needed":
			if v, ok := f.Value.(string); ok {
				entry.Action = v
			}
		case "details":
			if v, ok := f.Value.(map[string]any); ok {
				entry.Details = v
			}
		default:
			if entry.Details == nil {
				entry.Details = make(map[string]any)
			}
			entry.Details[f.Key] = f.Value
		}
	}

	// Write human-readable line
	l.logger.Printf("[%s] %s: %s", level, detector, msg)

	// Write JSON entry to file
	data, err := json.Marshal(entry)
	if err == nil {
		_, _ = l.file.Write(data)
		_, _ = l.file.Write([]byte("\n"))
	}
}

// Close closes the underlying file.
func (l *AILogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
