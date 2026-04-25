package aiagent

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// FavoritesCheckAPI handles favorites completeness checking.
type FavoritesCheckAPI struct {
	jellyfinURL string
	jellyfinKey string
	tmdbKey     string
	logger      *log.Logger
}

// NewFavoritesCheckAPI creates the handler.
func NewFavoritesCheckAPI(jellyfinURL, jellyfinKey, tmdbKey string, logger *log.Logger) *FavoritesCheckAPI {
	return &FavoritesCheckAPI{
		jellyfinURL: jellyfinURL,
		jellyfinKey: jellyfinKey,
		tmdbKey:     tmdbKey,
		logger:      logger,
	}
}

// HandleFavoritesCheck is the HTTP handler for /api/ai/favorites-check.
func (f *FavoritesCheckAPI) HandleFavoritesCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reportType := r.URL.Query().Get("type")
	tmdbID := r.URL.Query().Get("tmdb_id")

	if reportType == "movie" && tmdbID != "" {
		f.checkSingleMovie(w, tmdbID)
		return
	}
	if reportType == "tv" && tmdbID != "" {
		f.checkSingleTVSeries(w, tmdbID)
		return
	}

	// Full report
	f.fullReport(w)
}

func (f *FavoritesCheckAPI) fullReport(w http.ResponseWriter) {
	report := FavoritesReport{
		Movies:   []MovieStatus{},
		TVSeries: []TVSeriesStatus{},
	}

	// Get all torrents from GoStorm
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "GoStorm unreachable"})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var gsResp goStormResponse
	json.Unmarshal(body, &gsResp)

	for _, t := range gsResp.Result {
		report.Movies = append(report.Movies, MovieStatus{
			Title:     t.Title,
			TorrentID: t.ID,
			Seeders:   t.Stats.Seeders,
			Status:    "checking",
		})
	}

	// Get TV series from Jellyfin if configured
	if f.jellyfinURL != "" && f.jellyfinKey != "" {
		jfResp, err := http.Get(fmt.Sprintf("%s/Items?IncludeItemTypes=Series&Recursive=true&api_key=%s", f.jellyfinURL, f.jellyfinKey))
		if err == nil {
			defer jfResp.Body.Close()
			var library struct {
				Items []struct {
					ID   string `json:"Id"`
					Name string `json:"Name"`
				} `json:"Items"`
			}
			body, _ := io.ReadAll(jfResp.Body)
			json.Unmarshal(body, &library)
			for _, show := range library.Items {
				report.TVSeries = append(report.TVSeries, TVSeriesStatus{
					Title:  show.Name,
					Status: "checking",
				})
			}
		}
	}

	writeJSON(w, 200, report)
}

func (f *FavoritesCheckAPI) checkSingleMovie(w http.ResponseWriter, tmdbID string) {
	resp, err := http.Get("http://localhost:8090/torrents")
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "GoStorm unreachable"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var gsResp goStormResponse
	json.Unmarshal(body, &gsResp)

	for _, t := range gsResp.Result {
		report := MovieStatus{
			Title:     t.Title,
			TorrentID: t.ID,
			Seeders:   t.Stats.Seeders,
			Status:    "ok",
		}
		if t.Stats.Seeders == 0 {
			report.Status = "no_seeders"
		}
		writeJSON(w, 200, report)
		return
	}

	writeJSON(w, 404, map[string]any{"error": "no matching torrent found"})
}

func (f *FavoritesCheckAPI) checkSingleTVSeries(w http.ResponseWriter, tmdbID string) {
	if f.tmdbKey == "" {
		writeJSON(w, 400, map[string]any{"error": "TMDB API key not configured"})
		return
	}

	tmdbResp, err := http.Get(fmt.Sprintf("https://api.themoviedb.org/3/tv/%s?api_key=%s", tmdbID, f.tmdbKey))
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "TMDB unreachable"})
		return
	}
	defer tmdbResp.Body.Close()

	var tmdbData struct {
		Name             string `json:"name"`
		NumberOfSeasons  int    `json:"number_of_seasons"`
		NumberOfEpisodes int    `json:"number_of_episodes"`
	}
	body, _ := io.ReadAll(tmdbResp.Body)
	json.Unmarshal(body, &tmdbData)

	report := TVSeriesStatus{
		Title:        tmdbData.Name,
		TMDBSeasons:  tmdbData.NumberOfSeasons,
		TMDBEpisodes: tmdbData.NumberOfEpisodes,
		Status:       "checking_jellyfin",
	}

	if f.jellyfinURL != "" && f.jellyfinKey != "" {
		searchResp, err := http.Get(fmt.Sprintf("%s/Items?SearchTerm=%s&IncludeItemTypes=Series&api_key=%s", f.jellyfinURL, tmdbData.Name, f.jellyfinKey))
		if err == nil {
			defer searchResp.Body.Close()
			body, _ := io.ReadAll(searchResp.Body)
			var results struct {
				Items []struct {
					ID   string `json:"Id"`
					Name string `json:"Name"`
				} `json:"Items"`
			}
			json.Unmarshal(body, &results)
			if len(results.Items) > 0 {
				report.Status = "found_in_jellyfin"
			} else {
				report.Status = "not_in_jellyfin"
			}
		}
	}

	writeJSON(w, 200, report)
}

// --- Data Types ---

type FavoritesReport struct {
	Movies   []MovieStatus    `json:"movies"`
	TVSeries []TVSeriesStatus `json:"tv_series"`
}

type MovieStatus struct {
	Title     string `json:"title"`
	TorrentID string `json:"torrent_id,omitempty"`
	Seeders   int    `json:"seeders"`
	Status    string `json:"status"`
}

type TVSeriesStatus struct {
	Title             string `json:"title"`
	TMDBSeasons       int    `json:"tmdb_seasons,omitempty"`
	TMDBEpisodes      int    `json:"tmdb_episodes,omitempty"`
	AvailableEpisodes int    `json:"available_episodes,omitempty"`
	Status            string `json:"status"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
