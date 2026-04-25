package aiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// AIAPI registers all /api/ai/* HTTP handlers.
type AIAPI struct {
	detectors *Detectors
	buffer    *Buffer
	queue     *Queue
	logger    *log.Logger
}

// NewAIAPI creates the AI API handler.
func NewAIAPI(detectors *Detectors, buffer *Buffer, queue *Queue, logger *log.Logger) *AIAPI {
	return &AIAPI{
		detectors: detectors,
		buffer:    buffer,
		queue:     queue,
		logger:    logger,
	}
}

// Register registers all /api/ai/* handlers on the default mux.
func (a *AIAPI) Register() {
	http.HandleFunc("/api/ai/torrent-state", a.handleTorrentState)
	http.HandleFunc("/api/ai/active-playback", a.handleActivePlayback)
	http.HandleFunc("/api/ai/fuse-health", a.handleFuseHealth)
	http.HandleFunc("/api/ai/replace-torrent", a.handleReplaceTorrent)
	http.HandleFunc("/api/ai/remove-torrent", a.handleRemoveTorrent)
	http.HandleFunc("/api/ai/add-torrent", a.handleAddTorrent)
	http.HandleFunc("/api/ai/config", a.handleConfig)
	http.HandleFunc("/api/ai/recent-logs", a.handleRecentLogs)
	http.HandleFunc("/api/ai/queue-status", a.handleQueueStatus)
	http.HandleFunc("/api/ai/favorites-check", a.handleFavoritesCheck)
	a.logger.Printf("[AIAgent] /api/ai/* endpoints registered")
}

func (a *AIAPI) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (a *AIAPI) writeError(w http.ResponseWriter, status int, errorType string, details string, schemaHint string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error":         errorType,
		"details":       details,
		"schema_hint":   schemaHint,
		"retry_allowed": status == 400,
	})
}

// --- GET /api/ai/torrent-state?id=X ---
func (a *AIAPI) handleTorrentState(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		a.writeError(w, 400, "validation_failed", "Query parameter 'id' is required",
			`GET /api/ai/torrent-state?id=<torrent_id>`)
		return
	}
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// --- GET /api/ai/active-playback ---
func (a *AIAPI) handleActivePlayback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

// --- GET /api/ai/fuse-health ---
func (a *AIAPI) handleFuseHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.writeJSON(w, 200, map[string]any{
		"status": "ok",
		"note":   "fuse health check — detailed metrics available at /metrics",
	})
}

// --- POST /api/ai/replace-torrent ---
func (a *AIAPI) handleReplaceTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TorrentID string `json:"torrent_id"`
		NewMagnet string `json:"new_magnet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, 400, "validation_failed", "Invalid JSON body",
			`{"torrent_id": "string", "new_magnet": "magnet:?xt=..."}`)
		return
	}
	if req.TorrentID == "" {
		a.writeError(w, 400, "validation_failed", "Field 'torrent_id' is required but was empty",
			`{"torrent_id": "string", "new_magnet": "magnet:?xt=..."}`)
		return
	}
	if req.NewMagnet == "" {
		a.writeError(w, 400, "validation_failed", "Field 'new_magnet' is required but was empty",
			`{"torrent_id": "string", "new_magnet": "magnet:?xt=..."}`)
		return
	}
	removeResp, err := http.Post("http://localhost:8090/torrents", "application/json",
		bytes.NewReader(mustJSON(map[string]any{"action": "rem", "hash": req.TorrentID})))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm remove failed: %v", err), "")
		return
	}
	removeResp.Body.Close()

	addResp, err := http.Post("http://localhost:8090/torrents", "application/json",
		bytes.NewReader(mustJSON(map[string]any{"action": "add", "link": req.NewMagnet, "save": true})))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm add failed: %v", err), "")
		return
	}
	defer addResp.Body.Close()
	body, _ := io.ReadAll(addResp.Body)
	a.writeJSON(w, 200, map[string]any{
		"status": "replaced",
		"old_id": req.TorrentID,
		"result": string(body),
	})
}

// --- POST /api/ai/remove-torrent ---
func (a *AIAPI) handleRemoveTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TorrentID string `json:"torrent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, 400, "validation_failed", "Invalid JSON body", `{"torrent_id": "string"}`)
		return
	}
	if req.TorrentID == "" {
		a.writeError(w, 400, "validation_failed", "Field 'torrent_id' is required", `{"torrent_id": "string"}`)
		return
	}
	resp, err := http.Post("http://localhost:8090/torrents", "application/json",
		bytes.NewReader(mustJSON(map[string]any{"action": "rem", "hash": req.TorrentID})))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()
	a.writeJSON(w, 200, map[string]any{"status": "removed", "id": req.TorrentID})
}

// --- POST /api/ai/add-torrent ---
func (a *AIAPI) handleAddTorrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Magnet string `json:"magnet"`
		Title  string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, 400, "validation_failed", "Invalid JSON body",
			`{"magnet": "magnet:?xt=...", "title": "string"}`)
		return
	}
	if req.Magnet == "" {
		a.writeError(w, 400, "validation_failed", "Field 'magnet' is required",
			`{"magnet": "magnet:?xt=...", "title": "string"}`)
		return
	}
	resp, err := http.Post("http://localhost:8090/torrents", "application/json",
		bytes.NewReader(mustJSON(map[string]any{"action": "add", "link": req.Magnet, "title": req.Title, "save": true})))
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(body)
}

// --- GET/PUT /api/ai/config ---
func (a *AIAPI) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		a.writeJSON(w, 200, map[string]any{
			"note": "config endpoint — full config available in config.json",
		})
		return
	}
	if r.Method == "PUT" {
		a.writeError(w, 400, "not_implemented", "set_config not yet implemented", "")
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// --- GET /api/ai/recent-logs ---
func (a *AIAPI) handleRecentLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	lines := 50
	if n := r.URL.Query().Get("lines"); n != "" {
		fmt.Sscanf(n, "%d", &lines)
	}
	data, err := os.ReadFile("logs/gostream.log")
	if err != nil {
		a.writeError(w, 500, "file_error", fmt.Sprintf("cannot read log: %v", err), "")
		return
	}
	allLines := strings.Split(string(data), "\n")
	if len(allLines) > lines {
		allLines = allLines[len(allLines)-lines:]
	}
	a.writeJSON(w, 200, map[string]any{"lines": allLines, "count": len(allLines)})
}

// --- GET /api/ai/queue-status ---
func (a *AIAPI) handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := a.queue.Status()
	a.writeJSON(w, 200, status)
}

// --- GET /api/ai/favorites-check ---
func (a *AIAPI) handleFavoritesCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		a.writeError(w, 502, "upstream_error", fmt.Sprintf("GoStorm unreachable: %v", err), "")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	a.writeJSON(w, 200, map[string]any{
		"torrents": json.RawMessage(body),
		"note":     "favorites check requires TMDB integration — implemented in Phase 2",
	})
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
