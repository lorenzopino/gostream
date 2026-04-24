package aiagent

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// DetectorConfig holds configuration for all detectors.
type DetectorConfig struct {
	CheckInterval      time.Duration // how often to poll for issues
	LogTailWindow      time.Duration // how far back to scan logs
	MaxErrorsPerSpike  int           // error spike threshold (>N in window = spike)
	SlowStartupMs      int           // threshold for slow startup (ms)
	TimeoutStartupMs   int           // threshold for timeout startup (ms)
	LowSeederThreshold int           // seeder count below which = low seeders
	NoDownloadTimeout  time.Duration // time with 0 KBps = no download
	ReadStallTimeout   time.Duration // per-block read timeout = stall
}

// DefaultDetectorConfig returns sensible defaults.
func DefaultDetectorConfig() DetectorConfig {
	return DetectorConfig{
		CheckInterval:      60 * time.Second,
		LogTailWindow:      5 * time.Minute,
		MaxErrorsPerSpike:  5,
		SlowStartupMs:      15000,
		TimeoutStartupMs:   30000,
		LowSeederThreshold: 3,
		NoDownloadTimeout:  60 * time.Second,
		ReadStallTimeout:   5 * time.Second,
	}
}

// Detectors manages all issue detection goroutines.
type Detectors struct {
	cfg     DetectorConfig
	buffer  *Buffer
	logger  *log.Logger
	aiLog   *AILogger
	stopCh  chan struct{}
	once    sync.Once

	// Internal state for tracking
	torrentStates    map[string]torrentState
	torrentStatesMu  sync.RWMutex
	recentWebhooks   map[string]time.Time // IMDB ID → timestamp
	recentWebhooksMu sync.RWMutex
}

type torrentState struct {
	seeders      int
	peers        int
	downloadKBps float64
	lastChecked  time.Time
}

type logError struct {
	ts  time.Time
	msg string
}

// Detectors creates and starts all detector goroutines.
func NewDetectors(cfg DetectorConfig, buffer *Buffer, logger *log.Logger, aiLog *AILogger) *Detectors {
	return &Detectors{
		cfg:            cfg,
		buffer:         buffer,
		logger:         logger,
		aiLog:          aiLog,
		stopCh:         make(chan struct{}),
		torrentStates:  make(map[string]torrentState),
		recentWebhooks: make(map[string]time.Time),
	}
}

// Start launches all detector goroutines.
func (d *Detectors) Start() {
	go d.torrentHealthLoop()
	go d.logMonitorLoop()
	go d.webhookMatcherLoop()
	d.logger.Printf("[AIAgent] detectors started")
}

// Stop stops all detector goroutines.
func (d *Detectors) Stop() {
	d.once.Do(func() {
		close(d.stopCh)
		d.logger.Printf("[AIAgent] detectors stopped")
	})
}

// RecordWebhookMatch records a successful webhook match for later correlation.
func (d *Detectors) RecordWebhookMatch(imdbID string) {
	d.recentWebhooksMu.Lock()
	defer d.recentWebhooksMu.Unlock()
	d.recentWebhooks[imdbID] = time.Now()
}

// --- Torrent Health Detector ---

func (d *Detectors) torrentHealthLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkTorrentHealth()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Detectors) checkTorrentHealth() {
	torrents, err := d.fetchActiveTorrents()
	if err != nil {
		d.aiLog.Error("torrent_health", "failed to fetch torrents", F("error", err.Error()))
		return
	}

	for _, t := range torrents {
		d.torrentStatesMu.Lock()
		prev, existed := d.torrentStates[t.ID]
		current := torrentState{
			seeders:      t.Stats.Seeders,
			peers:        t.Stats.Peers,
			downloadKBps: float64(t.Stats.DownloadSpeed) / 1024.0,
			lastChecked:  time.Now(),
		}
		d.torrentStates[t.ID] = current
		d.torrentStatesMu.Unlock()

		// Check for dead torrent (0 seeders, existed for a while)
		if t.Stats.Seeders == 0 && existed {
			age := time.Since(prev.lastChecked).Seconds()
			if age > 60 {
				d.aiLog.Warn("torrent_health", "dead torrent detected",
					F("issue", "dead_torrent"),
					F("torrent_id", t.ID),
					F("file", t.Title),
					F("seeders", 0),
					F("age_seconds", int(age)),
					F("action_needed", "replace"),
				)
				d.buffer.Add(Issue{
					Type:        TypeDeadTorrent,
					Priority:    PriorityB,
					TorrentID:   t.ID,
					File:        t.Title,
					FirstSeen:   time.Now(),
					Occurrences: 1,
					Details: map[string]any{
						"seeders":     0,
						"age_seconds": int(age),
					},
				})
			}
		}

		// Check for low seeders
		if t.Stats.Seeders > 0 && t.Stats.Seeders < d.cfg.LowSeederThreshold {
			d.aiLog.Warn("torrent_health", "low seeder count",
				F("issue", "low_seeders"),
				F("torrent_id", t.ID),
				F("file", t.Title),
				F("seeders", t.Stats.Seeders),
			)
			d.buffer.Add(Issue{
				Type:        TypeLowSeeders,
				Priority:    PriorityB,
				TorrentID:   t.ID,
				File:        t.Title,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				Details: map[string]any{
					"seeders": t.Stats.Seeders,
				},
			})
		}

		// Check for no download despite active peers
		if t.Stats.DownloadSpeed == 0 && t.Stats.Peers > 0 && existed {
			sinceLast := time.Since(prev.lastChecked)
			if sinceLast > d.cfg.NoDownloadTimeout {
				d.aiLog.Warn("torrent_health", "no download despite active peers",
					F("issue", "no_download"),
					F("torrent_id", t.ID),
					F("file", t.Title),
				)
				d.buffer.Add(Issue{
					Type:        TypeNoDownload,
					Priority:    PriorityB,
					TorrentID:   t.ID,
					File:        t.Title,
					FirstSeen:   time.Now(),
					Occurrences: 1,
					Details: map[string]any{
						"stale_seconds": int(sinceLast.Seconds()),
					},
				})
			}
		}
	}
}

// --- Log Monitor Detector ---

func (d *Detectors) logMonitorLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	// Track error patterns in a sliding window
	var windowMu sync.Mutex
	window := make([]logError, 0)
	knownPatterns := make(map[string]int)

	for {
		select {
		case <-ticker.C:
			errors := d.scanRecentLogErrors()

			windowMu.Lock()
			now := time.Now()
			for _, e := range errors {
				window = append(window, logError{ts: now, msg: e.msg})
			}

			// Prune old entries
			cutoff := now.Add(-d.cfg.LogTailWindow)
			filtered := make([]logError, 0, len(window))
			for _, e := range window {
				if e.ts.After(cutoff) {
					filtered = append(filtered, e)
				}
			}

			// Count by pattern
			counts := make(map[string]int)
			for _, e := range filtered {
				pattern := normalizeErrorPattern(e.msg)
				counts[pattern]++
			}

			// Check for spikes
			for pattern, count := range counts {
				if count >= d.cfg.MaxErrorsPerSpike && count > knownPatterns[pattern] {
					d.aiLog.Warn("log_monitor", "error spike detected",
						F("issue", "error_spike"),
						F("count", count),
						F("pattern", pattern),
						F("action_needed", "investigate"),
					)
					d.buffer.Add(Issue{
						Type:        TypeErrorSpike,
						Priority:    PriorityB,
						FirstSeen:   time.Now(),
						Occurrences: count,
						LogSnippet:  pattern,
						Details: map[string]any{
							"count":   count,
							"pattern": pattern,
						},
					})
				}
			}
			knownPatterns = counts
			window = filtered
			windowMu.Unlock()

		case <-d.stopCh:
			return
		}
	}
}

func (d *Detectors) scanRecentLogErrors() []logError {
	const logPath = "logs/gostream.log"
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}

	var errors []logError
	errorRe := regexp.MustCompile(`(?i)(error|fail|timeout|panic|dead|stall)`)
	for _, line := range lines {
		if errorRe.MatchString(line) {
			errors = append(errors, logError{msg: line})
		}
	}
	return errors
}

func normalizeErrorPattern(msg string) string {
	normalized := regexp.MustCompile(`[0-9a-f]{8,}`).ReplaceAllString(msg, "<HASH>")
	normalized = regexp.MustCompile(`\d+`).ReplaceAllString(normalized, "<N>")
	return normalized
}

// --- Webhook Matcher Detector ---

func (d *Detectors) webhookMatcherLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkUnconfirmedPlay()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Detectors) checkUnconfirmedPlay() {
	lines := d.scanRecentLogErrors()
	for _, e := range lines {
		if strings.Contains(strings.ToLower(e.msg), "unconfirmed") ||
			strings.Contains(strings.ToLower(e.msg), "no match") {
			imdbRe := regexp.MustCompile(`tt\d+`)
			imdbID := ""
			if m := imdbRe.FindString(e.msg); m != "" {
				imdbID = m
			}

			d.aiLog.Warn("webhook_matcher", "unconfirmed play detected",
				F("issue", "unconfirmed_play"),
				F("imdb_id", imdbID),
				F("log_snippet", e.msg),
				F("action_needed", "verify"),
			)
			d.buffer.Add(Issue{
				Type:        TypeUnconfirmedPlay,
				Priority:    PriorityA,
				IMDBID:      imdbID,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				LogSnippet:  e.msg,
			})
		}
	}
}

// --- GoStorm API Types ---

type goStormTorrent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Stats struct {
		Peers         int     `json:"peers"`
		Seeders       int     `json:"seeders"`
		DownloadSpeed float64 `json:"download_speed"`
	} `json:"stats"`
}

type goStormResponse struct {
	Result []goStormTorrent `json:"result"`
}

func (d *Detectors) fetchActiveTorrents() ([]goStormTorrent, error) {
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result goStormResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	sort.Slice(result.Result, func(i, j int) bool {
		return result.Result[i].ID < result.Result[j].ID
	})

	return result.Result, nil
}
