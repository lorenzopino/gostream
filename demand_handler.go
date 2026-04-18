package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DemandJob tracks the state of an on-demand sync request.
type DemandJob struct {
	JobID           string    `json:"job_id"`
	TMDBID          int       `json:"tmdb_id"`
	ShowName        string    `json:"show_name,omitempty"`
	JellyfinItemID  string    `json:"jellyfin_item_id,omitempty"`
	Status          string    `json:"status"` // "started", "running", "completed", "failed"
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
	}()

	// Build a temporary TV syncer with demand mode channel config
	syncer := buildDemandSyncer(job)
	if syncer == nil {
		job.Status = "failed"
		job.Error = "failed to create demand syncer"
		return
	}

	// Run the sync
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Count episodes before
	createdBefore := countRegistryEpisodes()

	if err := syncer.Run(ctx); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
		logger.Printf("[Demand] sync failed: %v", err)
		return
	}

	// Count episodes after
	createdAfter := countRegistryEpisodes()
	job.EpisodesCreated = createdAfter - createdBefore
	if job.EpisodesCreated < 0 {
		job.EpisodesCreated = 0
	}
	job.Status = "completed"

	logger.Printf("[Demand] sync completed: TMDB %d, %d episodes created", job.TMDBID, job.EpisodesCreated)
}

// buildDemandSyncer creates a TV syncer for on-demand mode.
// TODO: Wire up actual TVSyncer creation with demand channel config in Round 2.
func buildDemandSyncer(job *DemandJob) interface{ Run(context.Context) error } {
	// Placeholder — will be replaced with actual TVSyncer creation in the wiring step
	return nil
}

// countRegistryEpisodes returns the current episode count in the sync registry.
// TODO: Implement based on how episode registry is accessed in Round 2.
func countRegistryEpisodes() int {
	// Placeholder — will be implemented based on how episode registry is accessed
	return 0
}
