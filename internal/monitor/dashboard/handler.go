package dashboard

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gostream/internal/monitor/collector"
)

//go:embed dashboard.html
var dashboardHTML []byte

// Handler serves the dashboard and API endpoints.
type Handler struct {
	collector *collector.Collector
	logsDir   string
}

// New creates a dashboard handler.
func New(c *collector.Collector, logsDir string) *Handler {
	return &Handler{collector: c, logsDir: logsDir}
}

// Dashboard serves the HTML page.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}

// Health serves the /api/health JSON endpoint.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.collector.Status())
}

// Torrents serves the /api/torrents JSON endpoint.
func (h *Handler) Torrents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	t := h.collector.Torrents()
	if t == nil {
		t = []collector.TorrentInfo{}
	}
	json.NewEncoder(w).Encode(t)
}

// SpeedHistory serves the /api/speed-history JSON endpoint.
func (h *Handler) SpeedHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s := h.collector.SpeedHistory()
	if s == nil {
		s = []collector.SpeedPoint{}
	}
	json.NewEncoder(w).Encode(s)
}

// Logs serves the /api/logs endpoint (tail of log files).
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	allowed := map[string]string{
		"gostream":       "gostream.log",
		"movies-sync":    "movies-sync.log",
		"tv-sync":        "tv-sync.log",
		"watchlist-sync": "watchlist-sync.log",
	}

	file := r.URL.Query().Get("file")
	if file == "" {
		file = "gostream"
	}
	logName, ok := allowed[file]
	if !ok {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "invalid log file"})
		return
	}

	lines := 80
	if v := r.URL.Query().Get("lines"); v != "" {
		if n := atoi(v); n > 0 && n <= 200 {
			lines = n
		}
	}

	logPath := filepath.Join(h.logsDir, logName)
	result := tailFile(logPath, lines)
	json.NewEncoder(w).Encode(map[string]interface{}{"lines": result})
}

func tailFile(path string, n int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{}
	}
	// For large files, only use last 128KB
	if len(data) > 128*1024 {
		data = data[len(data)-128*1024:]
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}
