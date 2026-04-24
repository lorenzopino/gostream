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
	}
}

// Detectors manages all issue detection goroutines.
type Detectors struct {
	cfg    DetectorConfig
	buffer *Buffer
	logger *log.Logger
	aiLog  *AILogger
	stopCh chan struct{}
	once   sync.Once

	// Internal state for tracking
	torrentStates   map[string]torrentState
	torrentStatesMu sync.RWMutex
}

type torrentState struct {
	seeders      int
	peers        int
	downloadKBps float64
	lastChecked  time.Time
}

// NewDetectors creates and initializes all detector goroutines.
func NewDetectors(cfg DetectorConfig, buffer *Buffer, logger *log.Logger, aiLog *AILogger) *Detectors {
	return &Detectors{
		cfg:           cfg,
		buffer:        buffer,
		logger:        logger,
		aiLog:         aiLog,
		stopCh:        make(chan struct{}),
		torrentStates: make(map[string]torrentState),
	}
}

// Start launches all detector goroutines.
func (d *Detectors) Start() {
	go d.torrentHealthLoop()
	go d.logMonitorLoop()
	d.logger.Printf("[AIAgent] detectors started")
}

// Stop stops all detector goroutines.
func (d *Detectors) Stop() {
	d.once.Do(func() {
		close(d.stopCh)
		d.logger.Printf("[AIAgent] detectors stopped")
	})
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
			downloadKBps: t.Stats.DownloadSpeed / 1024.0,
			lastChecked:  time.Now(),
		}
		d.torrentStates[t.ID] = current
		d.torrentStatesMu.Unlock()

		// Check for dead torrent (0 seeders, existed for a while)
		if t.Stats.Seeders == 0 && existed {
			age := time.Since(prev.lastChecked).Seconds()
			if age > 60 {
				d.aiLog.Warn("torrent_health", "dead torrent",
					F("issue", TypeDeadTorrent),
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
			d.aiLog.Warn("torrent_health", "low seeders",
				F("issue", TypeLowSeeders),
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

		// Check for no download (active but 0 KBps)
		if t.Stats.DownloadSpeed == 0 && t.Stats.Seeders > 0 && existed {
			sinceLast := time.Since(prev.lastChecked)
			if sinceLast > d.cfg.NoDownloadTimeout {
				d.aiLog.Warn("torrent_health", "no download despite active peers",
					F("issue", TypeNoDownload),
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
				window = append(window, logError{ts: now, msg: e})
			}

			cutoff := now.Add(-d.cfg.LogTailWindow)
			filtered := make([]logError, 0, len(window))
			for _, e := range window {
				if e.ts.After(cutoff) {
					filtered = append(filtered, e)
				}
			}

			counts := make(map[string]int)
			for _, e := range filtered {
				pattern := normalizeErrorPattern(e.msg)
				counts[pattern]++
			}

			for pattern, count := range counts {
				if count >= d.cfg.MaxErrorsPerSpike && count > knownPatterns[pattern] {
					d.aiLog.Warn("log_monitor", "error spike detected",
						F("issue", TypeErrorSpike),
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
				knownPatterns[pattern] = count
			}

			window = filtered
			windowMu.Unlock()

		case <-d.stopCh:
			return
		}
	}
}

type logError struct {
	ts  time.Time
	msg string
}

func (d *Detectors) scanRecentLogErrors() []string {
	const logPath = "logs/gostream.log"
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}

	var errors []string
	errorRe := regexp.MustCompile(`(?i)(error|fail|timeout|panic|dead|stall)`)
	for _, line := range lines {
		if errorRe.MatchString(line) {
			errors = append(errors, line)
		}
	}
	return errors
}

func normalizeErrorPattern(msg string) string {
	normalized := regexp.MustCompile(`[0-9a-f]{8,}`).ReplaceAllString(msg, "<HASH>")
	normalized = regexp.MustCompile(`\d+`).ReplaceAllString(normalized, "<N>")
	return normalized
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

// --- Placeholder detectors (to be implemented in Phase 2) ---

// webhookMatcherLoop will detect unconfirmed plays and wrong matches.
func (d *Detectors) webhookMatcherLoop() {
	// TODO: Implement in Phase 2 — requires integration with webhook handler
}

// fuseAccessLoop will detect FUSE mount stalls and read errors.
func (d *Detectors) fuseAccessLoop() {
	// TODO: Implement in Phase 2 — requires FUSE instrumentation
}

// subtitleCheckerLoop will check subtitle availability after playback.
func (d *Detectors) subtitleCheckerLoop() {
	// TODO: Implement in Phase 2 — requires Jellyfin API integration
}

// seriesCompletenessLoop will check favorited series against TMDB.
func (d *Detectors) seriesCompletenessLoop() {
	// TODO: Implement in Phase 2 — requires TMDB + Jellyfin integration
}

// favoritesCheckLoop will verify movie pre-download completion.
func (d *Detectors) favoritesCheckLoop() {
	// TODO: Implement in Phase 2 — requires Jellyfin favorites API
}
