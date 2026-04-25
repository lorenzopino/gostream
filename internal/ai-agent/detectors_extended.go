package aiagent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ExtendedDetectors adds 5 new detector goroutines.
type ExtendedDetectors struct {
	cfg           DetectorConfig
	buffer        *Buffer
	logger        *log.Logger
	aiLog         *AILogger
	stopCh        chan struct{}
	once          sync.Once
	jellyfinURL   string
	jellyfinKey   string
	tmdbKey       string
	webhookEvents map[string]time.Time
	webhookMu     sync.RWMutex
}

// NewExtendedDetectors creates the extended detector suite.
func NewExtendedDetectors(cfg DetectorConfig, buffer *Buffer, logger *log.Logger, aiLog *AILogger, jellyfinURL, jellyfinKey, tmdbKey string) *ExtendedDetectors {
	return &ExtendedDetectors{
		cfg:           cfg,
		buffer:        buffer,
		logger:        logger,
		aiLog:         aiLog,
		stopCh:        make(chan struct{}),
		jellyfinURL:   jellyfinURL,
		jellyfinKey:   jellyfinKey,
		tmdbKey:       tmdbKey,
		webhookEvents: make(map[string]time.Time),
	}
}

// Start launches all extended detector goroutines.
func (d *ExtendedDetectors) Start() {
	go d.startupLatencyLoop()
	go d.webhookMatcherLoop()
	go d.fuseAccessLoop()
	go d.subtitleCheckerLoop()
	go d.seriesCompletenessLoop()
	d.logger.Printf("[AIAgent] extended detectors started")
}

// Stop stops all extended detector goroutines.
func (d *ExtendedDetectors) Stop() {
	d.once.Do(func() {
		close(d.stopCh)
		d.logger.Printf("[AIAgent] extended detectors stopped")
	})
}

// RecordWebhookEvent records a webhook event for correlation.
func (d *ExtendedDetectors) RecordWebhookEvent(imdbID string, eventType string) {
	d.webhookMu.Lock()
	defer d.webhookMu.Unlock()
	d.webhookEvents[imdbID+":"+eventType] = time.Now()
}

// --- Startup Latency Detector ---

func (d *ExtendedDetectors) startupLatencyLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkStartupLatency()
		case <-d.stopCh:
			return
		}
	}
}

func (d *ExtendedDetectors) checkStartupLatency() {
	data, err := os.ReadFile("logs/gostream.log")
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}

	slowRe := regexp.MustCompile(`(?i)(slow|timeout|stall|taking.*long)`)
	for _, line := range lines {
		if slowRe.MatchString(line) && (strings.Contains(line, "startup") || strings.Contains(line, "ready") || strings.Contains(line, "metadata")) {
			d.aiLog.Warn("startup_latency", "slow startup detected",
				F("issue", TypeSlowStartup),
				F("log_snippet", line),
				F("action_needed", "investigate"),
			)
			d.buffer.Add(Issue{
				Type:        TypeSlowStartup,
				Priority:    PriorityB,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				LogSnippet:  line,
			})
		}
	}
}

// --- Webhook Matcher Detector ---

func (d *ExtendedDetectors) webhookMatcherLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkWebhookMatches()
		case <-d.stopCh:
			return
		}
	}
}

func (d *ExtendedDetectors) checkWebhookMatches() {
	data, err := os.ReadFile("logs/gostream.log")
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 300 {
		lines = lines[len(lines)-300:]
	}

	unconfirmedRe := regexp.MustCompile(`(?i)(webhook.*no.?match|unconfirmed|playback.*no.?webhook)`)
	for _, line := range lines {
		if unconfirmedRe.MatchString(line) {
			imdbRe := regexp.MustCompile(`tt\d+`)
			imdbID := ""
			if m := imdbRe.FindString(line); m != "" {
				imdbID = m
			}

			d.aiLog.Warn("webhook_matcher", "unconfirmed play or no match",
				F("issue", TypeUnconfirmedPlay),
				F("imdb_id", imdbID),
				F("log_snippet", line),
				F("action_needed", "verify"),
			)
			d.buffer.Add(Issue{
				Type:        TypeUnconfirmedPlay,
				Priority:    PriorityA,
				IMDBID:      imdbID,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				LogSnippet:  line,
			})
		}
	}
}

// --- FUSE Access Detector ---

func (d *ExtendedDetectors) fuseAccessLoop() {
	ticker := time.NewTicker(d.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkFuseAccess()
		case <-d.stopCh:
			return
		}
	}
}

func (d *ExtendedDetectors) checkFuseAccess() {
	data, err := os.ReadFile("logs/gostream.log")
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}

	fuseErrorRe := regexp.MustCompile(`(?i)(fuse.*error|mount.*stale|device.*not.*configured|input.*output.*error|transport.*endpoint.*not.*connected)`)
	for _, line := range lines {
		if fuseErrorRe.MatchString(line) {
			d.aiLog.Warn("fuse_access", "FUSE mount error detected",
				F("issue", TypeFuseError),
				F("log_snippet", line),
				F("action_needed", "remount"),
			)
			d.buffer.Add(Issue{
				Type:        TypeFuseError,
				Priority:    PriorityB,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				LogSnippet:  line,
			})
		}
	}
}

// --- Subtitle Checker Detector ---

func (d *ExtendedDetectors) subtitleCheckerLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkSubtitles()
		case <-d.stopCh:
			return
		}
	}
}

func (d *ExtendedDetectors) checkSubtitles() {
	if d.jellyfinURL == "" || d.jellyfinKey == "" {
		return
	}

	resp, err := http.Get(fmt.Sprintf("%s/Sessions?api_key=%s", d.jellyfinURL, d.jellyfinKey))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var sessions []struct {
		NowPlayingItem *struct {
			ID          string            `json:"Id"`
			Name        string            `json:"Name"`
			Type        string            `json:"Type"`
			ProviderIDs map[string]string `json:"ProviderIds"`
		} `json:"NowPlayingItem"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	json.Unmarshal(body, &sessions)

	for _, s := range sessions {
		if s.NowPlayingItem == nil {
			continue
		}
		item := s.NowPlayingItem

		hasEnglishSub := d.checkItemSubtitles(item.ID)
		if !hasEnglishSub && item.Type == "Movie" {
			imdbID := ""
			if v, ok := item.ProviderIDs["Imdb"]; ok {
				imdbID = v
			}

			d.aiLog.Warn("subtitle_checker", "missing English subtitles",
				F("issue", TypeMissingSubtitles),
				F("file", item.Name),
				F("imdb_id", imdbID),
				F("action_needed", "download"),
			)
			d.buffer.Add(Issue{
				Type:        TypeMissingSubtitles,
				Priority:    PriorityD,
				File:        item.Name,
				IMDBID:      imdbID,
				FirstSeen:   time.Now(),
				Occurrences: 1,
			})
		}
	}
}

func (d *ExtendedDetectors) checkItemSubtitles(itemID string) bool {
	resp, err := http.Get(fmt.Sprintf("%s/Items/%s?api_key=%s", d.jellyfinURL, itemID, d.jellyfinKey))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var details struct {
		MediaStreams []struct {
			Type     string `json:"Type"`
			Language string `json:"Language"`
		} `json:"MediaStreams"`
	}
	json.Unmarshal(body, &details)

	for _, ms := range details.MediaStreams {
		if ms.Type == "Subtitle" && (strings.EqualFold(ms.Language, "en") || strings.EqualFold(ms.Language, "eng") || strings.EqualFold(ms.Language, "english")) {
			return true
		}
	}
	return false
}

// --- Series Completeness Detector ---

func (d *ExtendedDetectors) seriesCompletenessLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.checkSeriesCompleteness()
		case <-d.stopCh:
			return
		}
	}
}

func (d *ExtendedDetectors) checkSeriesCompleteness() {
	if d.jellyfinURL == "" || d.jellyfinKey == "" || d.tmdbKey == "" {
		return
	}

	resp, err := http.Get(fmt.Sprintf("%s/Items?IncludeItemTypes=Series&Recursive=true&api_key=%s", d.jellyfinURL, d.jellyfinKey))
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var library struct {
		Items []struct {
			ID          string            `json:"Id"`
			Name        string            `json:"Name"`
			ProviderIDs map[string]string `json:"ProviderIds"`
		} `json:"Items"`
	}

	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &library)

	for _, show := range library.Items {
		tmdbID := ""
		if v, ok := show.ProviderIDs["Tmdb"]; ok {
			tmdbID = v
		}
		if tmdbID == "" {
			continue
		}

		tmdbResp, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s", tmdbID, d.tmdbKey))
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(tmdbResp.Body)
		tmdbResp.Body.Close()

		var tmdbData struct {
			NumberOfSeasons  int `json:"number_of_seasons"`
			NumberOfEpisodes int `json:"number_of_episodes"`
		}
		json.Unmarshal(body, &tmdbData)

		jfResp, err := http.Get(fmt.Sprintf("%s/Shows/%s/Seasons?api_key=%s", d.jellyfinURL, show.ID, d.jellyfinKey))
		if err != nil {
			continue
		}

		var jfSeasons struct {
			Items []struct {
				ID string `json:"Id"`
			} `json:"Items"`
		}
		body, _ = io.ReadAll(jfResp.Body)
		jfResp.Body.Close()
		json.Unmarshal(body, &jfSeasons)

		totalJfEpisodes := 0
		for _, season := range jfSeasons.Items {
			epResp, err := http.Get(fmt.Sprintf("%s/Shows/%s/Episodes?SeasonId=%s&api_key=%s", d.jellyfinURL, show.ID, season.ID, d.jellyfinKey))
			if err != nil {
				continue
			}
			var epData struct {
				TotalRecordCount int `json:"TotalRecordCount"`
			}
			body, _ := io.ReadAll(epResp.Body)
			epResp.Body.Close()
			json.Unmarshal(body, &epData)
			totalJfEpisodes += epData.TotalRecordCount
		}

		if totalJfEpisodes < tmdbData.NumberOfEpisodes {
			missing := tmdbData.NumberOfEpisodes - totalJfEpisodes
			d.aiLog.Warn("series_completeness", "incomplete TV series",
				F("issue", TypeIncompleteSeries),
				F("file", show.Name),
				F("details", map[string]any{
					"tmdb_seasons":       tmdbData.NumberOfSeasons,
					"tmdb_episodes":      tmdbData.NumberOfEpisodes,
					"available_episodes": totalJfEpisodes,
					"missing_episodes":   missing,
				}),
				F("action_needed", "download_missing"),
			)
			d.buffer.Add(Issue{
				Type:        TypeIncompleteSeries,
				Priority:    PriorityC,
				File:        show.Name,
				FirstSeen:   time.Now(),
				Occurrences: 1,
				Details: map[string]any{
					"tmdb_seasons":       tmdbData.NumberOfSeasons,
					"tmdb_episodes":      tmdbData.NumberOfEpisodes,
					"available_episodes": totalJfEpisodes,
					"missing_episodes":   missing,
				},
			})
		}
	}
}
