package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"gostream/internal/catalog/tmdb"
	"gostream/internal/config"
	"gostream/internal/gostorm/settings"
	"gostream/internal/prowlarr"
	"gostream/internal/syncer/engines"
	"gostream/internal/syncer/quality"
	"gostream/internal/gostorm/torr"
)

// DemandJob tracks the state of an on-demand sync request.
type DemandJob struct {
	JobID           string    `json:"job_id"`
	TMDBID          int       `json:"tmdb_id"`
	ShowName        string    `json:"show_name,omitempty"`
	JellyfinItemID  string    `json:"jellyfin_item_id,omitempty"`
	Status          string    `json:"status"` // "started", "downloading", "completed", "failed"
	Progress        float64   `json:"progress,omitempty"`
	DownloadedBytes int64     `json:"downloaded_bytes,omitempty"`
	TotalBytes      int64     `json:"total_bytes,omitempty"`
	EpisodesCreated int       `json:"episodes_created,omitempty"`
	EpisodesSkipped int       `json:"episodes_skipped,omitempty"`
	Error           string    `json:"error,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
}

// DemandTracker manages concurrent demand sync jobs.
type DemandTracker struct {
	mu   sync.RWMutex
	jobs map[string]*DemandJob
}

// NewDemandTracker creates a new tracker.
func NewDemandTracker() *DemandTracker {
	return &DemandTracker{
		jobs: make(map[string]*DemandJob),
	}
}

// Add registers a new job and returns it.
func (t *DemandTracker) Add(job *DemandJob) *DemandJob {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.jobs[job.JobID] = job
	return job
}

// Get returns a copy of the job by ID.
func (t *DemandTracker) Get(id string) *DemandJob {
	t.mu.RLock()
	defer t.mu.RUnlock()
	j, ok := t.jobs[id]
	if !ok {
		return nil
	}
	// Return a copy
	cp := *j
	return &cp
}

// ListActive returns all jobs that are not completed or failed.
func (t *DemandTracker) ListActive() []*DemandJob {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var result []*DemandJob
	for _, j := range t.jobs {
		if j.Status == "started" || j.Status == "running" {
			cp := *j
			result = append(result, &cp)
		}
	}
	return result
}

var demandTracker *DemandTracker

// handleDemandPOST handles POST /api/tv-sync/demand
func handleDemandPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TMDBID         int    `json:"tmdb_id"`
		JellyfinItemID string `json:"jellyfin_item_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.TMDBID <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "tmdb_id is required and must be positive"})
		return
	}

	// Check if already running for this TMDB ID
	activeJobs := demandTracker.ListActive()
	for _, j := range activeJobs {
		if j.TMDBID == req.TMDBID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"job_id":    j.JobID,
				"status":    j.Status,
				"show_name": j.ShowName,
				"message":   "sync already in progress for this series",
			})
			return
		}
	}

	jobID := fmt.Sprintf("demand-%d-%d", req.TMDBID, time.Now().Unix())
	job := &DemandJob{
		JobID:          jobID,
		TMDBID:         req.TMDBID,
		JellyfinItemID: req.JellyfinItemID,
		Status:         "started",
		StartedAt:      time.Now(),
	}
	demandTracker.Add(job)

	// Run sync in background
	go runDemandSync(job)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id":  jobID,
		"status":  "started",
		"message": "sync queued",
	})
}

// handleDemandGET handles GET /api/tv-sync/demand/{job_id}
func handleDemandGET(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job_id from path
	path := strings.TrimPrefix(r.URL.Path, "/api/tv-sync/demand/")
	if path == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	job := demandTracker.Get(path)
	if job == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "job not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// runDemandSync executes the on-demand sync for a single series.
func runDemandSync(job *DemandJob) {
	job.Status = "running"
	logger.Printf("[Demand] sync started: TMDB %d (job=%s)", job.TMDBID, job.JobID)

	defer func() {
		if r := recover(); r != nil {
			job.Status = "failed"
			job.Error = fmt.Sprintf("panic: %v", r)
			logger.Printf("[Demand] panic in job %s: %v", job.JobID, r)
		}
		job.CompletedAt = time.Now()
		logger.Printf("[Demand] job %s completed: status=%s created=%d skipped=%d",
			job.JobID, job.Status, job.EpisodesCreated, job.EpisodesSkipped)

		// Trigger Jellyfin refresh if item ID is set and episodes were created
		if job.Status == "completed" && job.JellyfinItemID != "" {
			triggerJellyfinRefresh(job)
		}
	}()

	syncer := buildDemandSyncer(job)
	if syncer == nil {
		job.Status = "failed"
		job.Error = "failed to create demand syncer"
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := syncer.Run(ctx); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		logger.Printf("[Demand] sync failed: %v", err)
		return
	}

	// Get show name from the syncer's engine (best effort)
	// The engine logs the show name, so we can extract it from logs
	// For now, mark as completed with whatever was created
	job.Status = "completed"
}

// buildDemandSyncer creates a TV syncer for on-demand mode.
func buildDemandSyncer(job *DemandJob) interface{ Run(context.Context) error } {
	// Build TVSyncerConfig for demand mode
	cfg := engines.TVSyncerConfig{
		GoStormURL:   globalConfig.GoStormBaseURL,
		TMDBAPIKey:   globalConfig.TMDBAPIKey,
		TorrentioURL: globalConfig.TorrentioURL,
		PlexURL:      globalConfig.Plex.URL,
		PlexToken:    globalConfig.Plex.Token,
		PlexTVLib:    globalConfig.Plex.TVLibraryID,
		TVDir:        filepath.Join(globalConfig.PhysicalSourcePath, "tv"),
		StateDir:     GetStateDir(),
		LogsDir:      filepath.Join(filepath.Dir(globalConfig.ConfigPath), "logs"),
		ProwlarrCfg:  globalConfig.Prowlarr,
		DB:           stateDB,
		QualityProfile: quality.ResolveTVProfile(globalConfig.Quality),
		TMDBDiscovery:  tmdb.EndpointConfig{}, // No discovery endpoints needed
		Channel: config.TVChannelConfig{
			Enabled:             true,
			Name:                "demand",
			Mode:                "demand",
			TMDBIDs:             []int{job.TMDBID},
			SkipCompleteSeasons: false, // Always try to complete missing episodes
			JellyfinItemID:      job.JellyfinItemID,
		},
	}

	return engines.NewTVSyncer(cfg)
}

// countRegistryEpisodes returns the current episode count by scanning .mkv files on disk.
func countRegistryEpisodes() int {
	tvDir := filepath.Join(globalConfig.PhysicalSourcePath, "tv")
	count := 0
	filepath.Walk(tvDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".mkv") {
			count++
		}
		return nil
	})
	return count
}

// triggerJellyfinRefresh asks Jellyfin to re-scan a specific series.
// This is much faster than a full library scan (2-5s vs minutes).
func triggerJellyfinRefresh(job *DemandJob) {
	if job.JellyfinItemID == "" {
		return
	}

	// Find Jellyfin URL and API key from global config
	jellyfinURL := ""
	jellyfinAPIKey := ""

	// Check if we have Jellyfin config in globalConfig
	if globalConfig.Jellyfin.URL != "" {
		jellyfinURL = globalConfig.Jellyfin.URL
		jellyfinAPIKey = globalConfig.Jellyfin.APIKey
	}

	if jellyfinURL == "" || jellyfinAPIKey == "" {
		logger.Printf("[Demand] Jellyfin refresh skipped: no URL or API key configured")
		return
	}

	// Build the refresh URL
	// POST /Items/{ItemId}/Refresh?Recursive=true&ImageRefreshMode=Default&MetadataRefreshMode=Default&ReplaceAllMetadata=false
	refreshURL := fmt.Sprintf("%s/Items/%s/Refresh?Recursive=true&ImageRefreshMode=Default&MetadataRefreshMode=Default&ReplaceAllMetadata=false",
		jellyfinURL, job.JellyfinItemID)

	req, err := http.NewRequest("POST", refreshURL, nil)
	if err != nil {
		logger.Printf("[Demand] Jellyfin refresh failed: failed to create request: %v", err)
		return
	}
	req.Header.Set("X-Emby-Token", jellyfinAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Printf("[Demand] Jellyfin refresh failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.Printf("[Demand] Jellyfin refresh succeeded for item %s", job.JellyfinItemID)
	} else {
		logger.Printf("[Demand] Jellyfin refresh returned status %d for item %s", resp.StatusCode, job.JellyfinItemID)
	}
}

// --- Movie Download Handlers ---

var movieTracker *DemandTracker

// handleMovieDownloadPOST handles POST /api/movie-cache/download
func handleMovieDownloadPOST(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TMDBID         int    `json:"tmdb_id"`
		JellyfinItemID string `json:"jellyfin_item_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.TMDBID <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "tmdb_id is required"})
		return
	}

	jobID := fmt.Sprintf("movie-%d", req.TMDBID)

	// Check if already downloading
	if existing := movieTracker.Get(jobID); existing != nil {
		if existing.Status == "downloading" || existing.Status == "started" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(existing)
			return
		}
	}

	job := &DemandJob{
		JobID:          jobID,
		TMDBID:         req.TMDBID,
		JellyfinItemID: req.JellyfinItemID,
		Status:         "started",
		StartedAt:      time.Now(),
	}
	movieTracker.Add(job)

	go runMovieDownload(job)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id": jobID,
		"status": "started",
	})
}

// handleMovieDownloadGET handles GET /api/movie-cache/status/{job_id}
func handleMovieDownloadGET(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/movie-cache/status/")
	if path == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	job := movieTracker.Get(path)
	if job == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "job not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// runMovieDownload performs the full movie download in background.
func runMovieDownload(job *DemandJob) {
	job.Status = "downloading"
	logger.Printf("[MovieDownload] started: TMDB %d (job=%s)", job.TMDBID, job.JobID)

	defer func() {
		if r := recover(); r != nil {
			job.Status = "failed"
			job.Error = fmt.Sprintf("panic: %v\n%s", r, string(debug.Stack()))
			logger.Printf("[MovieDownload] PANIC in job %s: %v", job.JobID, r)
			logger.Printf("[MovieDownload] Stack trace:\n%s", string(debug.Stack()))
		} else if job.Status == "failed" {
			logger.Printf("[MovieDownload] job %s FAILED: %s", job.JobID, job.Error)
		}
		job.CompletedAt = time.Now()
		logger.Printf("[MovieDownload] job %s completed: status=%s",
			job.JobID, job.Status)
	}()

	// 1. Resolve TMDB details
	ctx := context.Background()
	tmdbClient := tmdb.NewClient(globalConfig.TMDBAPIKey)
	details, err := tmdbClient.MovieDetails(ctx, job.TMDBID)
	if err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("TMDB lookup failed: %v", err)
		return
	}

	imdbID, err := tmdbClient.ExternalIDs(ctx, job.TMDBID)
	if err != nil || imdbID == "" {
		job.Status = "failed"
		job.Error = "no IMDB ID found"
		return
	}

	// V469: Check if movie already exists as MKV in the library.
	// If it does, extract the torrent hash from the MKV stub and use that
	// instead of searching for a new torrent via Prowlarr.
	existingHash, existingFile := findExistingMovieMKV(imdbID)
	if existingHash != "" {
		logger.Printf("[MovieDownload] movie already present: %s (hash=%s, file=%s)",
			details.Title, existingHash, existingFile)
		// Use the existing torrent hash — no need to search Prowlarr
		job.Status = "downloading"
		hash := existingHash
		t := getOrCreateTorrentByHash(hash)
		if t == nil {
			job.Status = "failed"
			job.Error = fmt.Sprintf("torrent %s not found or info timeout", hash)
			return
		}
		// Torrent already has info (GotInfo called in getOrCreateTorrentByHash)
		// Trigger download of all pieces
		t.Torrent.DownloadAll()
		if settings.BTsets.DisablePreloadSeeding {
			t.SetUploadLimit(0)
			t.SetSeedMode(false)
		}
		logger.Printf("[MovieDownload] waiting for download: TMDB %d, hash=%s", job.TMDBID, hash)
		downloadCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
		defer cancel()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		// ... rest of download loop
		waitForDownloadCompletion(job, t, hash, downloadCtx, ticker)
		return
	}

	// 2. Movie not present — find best torrent via Prowlarr
	pc := prowlarr.NewClient(globalConfig.Prowlarr)
	if pc == nil {
		job.Status = "failed"
		job.Error = "Prowlarr is not enabled"
		return
	}
	streams := pc.FetchTorrents(imdbID, "movie", details.Title, prowlarr.DefaultMovieCategories())
	if len(streams) == 0 {
		job.Status = "failed"
		job.Error = "no torrents found via Prowlarr"
		return
	}

	// 3. Add torrent for pre-download (download only, no seeding)
	bestStream := streams[0]
	magnet := engines.BuildMagnet(bestStream.InfoHash, bestStream.Title, engines.DefaultTrackers())

	// Parse magnet to get TorrentSpec
	mag, err := metainfo.ParseMagnetUri(magnet)
	if err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("parse magnet failed: %v", err)
		return
	}

	spec := &torrent.TorrentSpec{
		InfoBytes:   nil,
		Trackers:    [][]string{mag.Trackers},
		DisplayName: mag.DisplayName,
		InfoHash:    mag.InfoHash,
	}

	// Use in-process AddTorrentForPreDownload which handles priority + seeding
	t, err := torr.AddTorrentForPreDownload(spec, bestStream.Title, "", "", "movie")
	if err != nil {
		job.Status = "failed"
		job.Error = fmt.Sprintf("add torrent failed: %v", err)
		return
	}
	if t == nil {
		job.Status = "failed"
		job.Error = "add torrent returned nil torrent without error"
		return
	}
	hash := t.Hash().HexString()

	// 5. Wait for download to complete (poll torrent stats)
	logger.Printf("[MovieDownload] waiting for download: TMDB %d, hash=%s", job.TMDBID, hash)
	downloadCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// V467: Get torrent reference by hash each iteration to avoid stale pointer.
	// The original *Torrent may become invalid if the torrent is removed/re-added.
	getTorrent := func() *torr.Torrent {
		return torr.GetTorrent(hash)
	}

	// Initial info fetch
	cur := getTorrent()
	totalLength := int64(0)
	totalPieces := 0
	if cur != nil {
		totalLength = cur.Length()
		if info := cur.Info(); info != nil {
			totalPieces = info.NumPieces()
		}
	} else {
		logger.Printf("[MovieDownload] warning: torrent not found after add: %s", hash)
	}

	for {
		select {
		case <-downloadCtx.Done():
			job.Status = "failed"
			job.Error = "download timed out after 2 hours"
			logger.Printf("[MovieDownload] timeout: TMDB %d", job.TMDBID)
			return
		case <-ticker.C:
			// V467: Re-fetch torrent each iteration to avoid stale pointer
			cur := getTorrent()
			if cur == nil {
				logger.Printf("[MovieDownload] torrent lost during download: %s", hash)
				job.Status = "failed"
				job.Error = fmt.Sprintf("torrent lost: hash=%s", hash[:min(8, len(hash))])
				return
			}
			stats := cur.Stats()
			var progress float64
			if totalPieces > 0 {
				progress = float64(stats.PiecesComplete) / float64(totalPieces)
			}
			job.Progress = math.Min(progress, 1.0)

			downloadedBytes := int64(float64(totalLength) * progress)
			job.DownloadedBytes = downloadedBytes
			job.TotalBytes = totalLength

			if totalPieces > 0 && stats.PiecesComplete >= totalPieces {
				// Download complete
				job.Status = "completed"
				job.Progress = 1.0
				logger.Printf("[MovieDownload] completed: TMDB %d, hash=%s, size=%.1fGB",
					job.TMDBID, hash, float64(totalLength)/1024/1024/1024)

				// 6. Trigger Jellyfin refresh
				if job.JellyfinItemID != "" {
					triggerJellyfinRefreshForMovie(job)
				}
				return
			}

			logger.Printf("[MovieDownload] progress: %.1f%% (%d/%d pieces, %.1f/%.1f GB)",
				progress*100,
				stats.PiecesComplete, totalPieces,
				float64(downloadedBytes)/1024/1024/1024,
				float64(totalLength)/1024/1024/1024)
		}
	}
}

// findExistingMovieMKV scans the movies directory for an MKV stub matching the IMDB ID.
// Returns the torrent hash and file path if found.
func findExistingMovieMKV(imdbID string) (hash string, filePath string) {
	moviesDir := globalConfig.PhysicalSourcePath
	if moviesDir == "" {
		return "", ""
	}
	moviesDir = filepath.Join(moviesDir, "movies")
	if _, err := os.Stat(moviesDir); err != nil {
		return "", ""
	}

	reIMDB := regexp.MustCompile(`^tt\d+$`)
	if !reIMDB.MatchString(imdbID) {
		return "", ""
	}

	entries, _ := os.ReadDir(moviesDir)
	for _, e := range entries {
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".mkv") {
			continue
		}
		path := filepath.Join(moviesDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		content := strings.TrimSpace(string(data))
		// MKV stubs are JSON files with "imdb" field
		if strings.HasPrefix(content, "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(content), &obj); err == nil {
				if imdb, ok := obj["imdb"].(string); ok && imdb == imdbID {
					// Try explicit hash field first
					if h, ok := obj["hash"].(string); ok && h != "" {
						return h, path
					}
					// Try extracting from magnet URL
					if magnet, ok := obj["magnet"].(string); ok && magnet != "" {
						if h := extractHashFromURL(magnet); h != "" {
							return h, path
						}
					}
					// Try extracting from stream URL
					if url, ok := obj["url"].(string); ok && url != "" {
						if h := extractHashFromURL(url); h != "" {
							return h, path
						}
					}
				}
			}
		} else {
			// Old format: line-based MKV stub
			lines := strings.Split(content, "\n")
			if len(lines) >= 4 && strings.TrimSpace(lines[3]) == imdbID {
				// Found matching MKV — extract hash from filename
				base := strings.TrimSuffix(e.Name(), ".mkv")
				if idx := strings.LastIndex(base, "_"); idx > 0 {
					hash8 := base[idx+1:]
					// Search all torrents for one ending with this suffix
					for _, tr := range torr.ListTorrent() {
						if tr != nil && strings.HasSuffix(tr.Hash().HexString(), hash8) {
							return tr.Hash().HexString(), path
						}
					}
				}
			}
		}
	}
	return "", ""
}

// extractHashFromURL extracts the torrent hash from a stream URL.
func extractHashFromURL(url string) string {
	// URL format: http://127.0.0.1:8090/stream?link=HASH&index=1&play
	// or: http://127.0.0.1:8090/stream?link=magnet:?xt=urn:btih:HASH&index=0&play
	if idx := strings.Index(url, "link="); idx < 0 {
		return ""
	} else {
		rest := url[idx+5:]
		if amp := strings.Index(rest, "&"); amp > 0 {
			rest = rest[:amp]
		}
		// Check if it's a magnet URL
		if btihIdx := strings.Index(rest, "btih:"); btihIdx >= 0 {
			return rest[btihIdx+5:]
		}
		// Direct hash format
		if len(rest) >= 40 {
			return rest[:40]
		}
		return ""
	}
}

// getOrCreateTorrentByHash finds an existing torrent by hash or adds it if not present.
func getOrCreateTorrentByHash(hash string) *torr.Torrent {
	// Try active torrent first (already loaded in engine)
	t := torr.GetTorrent(hash)
	if t != nil && t.GotInfo() {
		logger.Printf("[MovieDownload] found active torrent in engine: %s", hash[:16])
		return t
	}

	// Torrent is in DB but not loaded — use LoadTorrent to load it from DB
	logger.Printf("[MovieDownload] loading torrent from DB: %s", hash[:16])
	dbTorrent := torr.GetTorrentDB(metainfo.NewHashFromHex(hash))
	if dbTorrent != nil && dbTorrent.TorrentSpec != nil {
		loaded := torr.LoadTorrent(dbTorrent)
		if loaded != nil && loaded.GotInfo() {
			logger.Printf("[MovieDownload] loaded torrent from DB: %s", hash[:16])
			return loaded
		}
	}

	// Not in DB either — find the magnet from the MKV stub and add it
	logger.Printf("[MovieDownload] searching MKV stubs for hash: %s", hash[:16])
	moviesDir := globalConfig.PhysicalSourcePath
	if moviesDir != "" {
		moviesDir = filepath.Join(moviesDir, "movies")
		entries, _ := os.ReadDir(moviesDir)
		for _, e := range entries {
			if !strings.HasSuffix(strings.ToLower(e.Name()), ".mkv") {
				continue
			}
			path := filepath.Join(moviesDir, e.Name())
			data, _ := os.ReadFile(path)
			if len(data) == 0 {
				continue
			}
			content := strings.TrimSpace(string(data))
			if strings.HasPrefix(content, "{") {
				var obj map[string]interface{}
				if err := json.Unmarshal([]byte(content), &obj); err == nil {
					if m, ok := obj["magnet"].(string); ok && m != "" {
						// Check if this magnet matches our hash
						if extractedHash := extractHashFromURL(m); strings.EqualFold(extractedHash, hash) {
							logger.Printf("[MovieDownload] found matching MKV stub: %s", e.Name())
							// Add the torrent to the engine
							spec := &torrent.TorrentSpec{
								InfoHash: metainfo.NewHashFromHex(hash),
							}
							addedTorrent, err := torr.AddTorrentForPreDownload(spec, e.Name(), "", "", "movie")
							if err != nil {
								logger.Printf("[MovieDownload] AddTorrentForPreDownload error: %v", err)
								continue
							}
							if addedTorrent == nil {
								logger.Printf("[MovieDownload] AddTorrentForPreDownload returned nil")
								continue
							}
							logger.Printf("[MovieDownload] re-added torrent from MKV stub: %s, waiting for info...", hash[:16])
							// Wait for GotInfo before returning
							if addedTorrent.GotInfo() {
								return addedTorrent
							}
							logger.Printf("[MovieDownload] GotInfo timeout for %s", hash[:16])
							return nil
						}
					}
				}
			}
		}
	}

	logger.Printf("[MovieDownload] torrent not found for hash %s", hash[:16])
	return nil
}

// waitForDownloadCompletion polls torrent stats until all pieces are downloaded.
func waitForDownloadCompletion(job *DemandJob, t *torr.Torrent, hash string, ctx context.Context, ticker *time.Ticker) {
	totalLength := int64(0)
	totalPieces := 0
	if t != nil {
		totalLength = t.Length()
		if info := t.Info(); info != nil {
			totalPieces = info.NumPieces()
		}
	}

	getTorrent := func() *torr.Torrent {
		return torr.GetTorrent(hash)
	}

	for {
		select {
		case <-ctx.Done():
			job.Status = "failed"
			job.Error = "download timed out after 2 hours"
			logger.Printf("[MovieDownload] timeout: TMDB %d", job.TMDBID)
			return
		case <-ticker.C:
			cur := getTorrent()
			if cur == nil {
				logger.Printf("[MovieDownload] torrent lost during download: %s", hash)
				job.Status = "failed"
				job.Error = fmt.Sprintf("torrent lost: hash=%s", hash[:min(8, len(hash))])
				return
			}
			stats := cur.Stats()
			var progress float64
			if totalPieces > 0 {
				progress = float64(stats.PiecesComplete) / float64(totalPieces)
			}
			job.Progress = math.Min(progress, 1.0)
			downloadedBytes := int64(float64(totalLength) * progress)
			job.DownloadedBytes = downloadedBytes
			job.TotalBytes = totalLength

			if totalPieces > 0 && stats.PiecesComplete >= totalPieces {
				job.Status = "completed"
				job.Progress = 1.0
				logger.Printf("[MovieDownload] completed: TMDB %d, hash=%s, size=%.1fGB",
					job.TMDBID, hash, float64(totalLength)/1024/1024/1024)
				if job.JellyfinItemID != "" {
					triggerJellyfinRefreshForMovie(job)
				}
				return
			}

			logger.Printf("[MovieDownload] progress: %.1f%% (%d/%d pieces, %.1f/%.1f GB)",
				progress*100,
				stats.PiecesComplete, totalPieces,
				float64(downloadedBytes)/1024/1024/1024,
				float64(totalLength)/1024/1024/1024)
		}
	}
}

// triggerJellyfinRefreshForMovie asks Jellyfin to re-scan a specific movie item.
func triggerJellyfinRefreshForMovie(job *DemandJob) {
	if globalConfig.Jellyfin.URL == "" || globalConfig.Jellyfin.APIKey == "" {
		logger.Printf("[MovieDownload] Jellyfin refresh skipped: no config")
		return
	}

	url := fmt.Sprintf("%s/Items/%s/Refresh?Recursive=true",
		globalConfig.Jellyfin.URL, job.JellyfinItemID)
	req, _ := http.NewRequest("POST", url, nil)
	req.Header.Set("X-Emby-Token", globalConfig.Jellyfin.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.Printf("[MovieDownload] Jellyfin refresh failed: %v", err)
		return
	}
	defer resp.Body.Close()
	logger.Printf("[MovieDownload] Jellyfin refresh succeeded for item %s", job.JellyfinItemID)
}
