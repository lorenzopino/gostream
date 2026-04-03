package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client queries the Prowlarr API and returns results in Stremio/Torrentio format.
// Thread-safe: all methods are safe for concurrent use.
type Client struct {
	cfg        ConfigProwlarr
	httpClient *http.Client
	searchURL  string
}

// NewClient creates a Prowlarr client from the given configuration.
// Returns nil if Prowlarr is not enabled.
func NewClient(cfg ConfigProwlarr) *Client {
	if !cfg.Enabled {
		return nil
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        10,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		searchURL: cfg.URL + "/api/v1/search",
	}
}

// FetchTorrents queries Prowlarr and returns Stremio-format streams.
// contentType is "movie" or "series". title is the show/movie name (used for TV UHD searches).
// Returns an empty slice (never nil) if disabled or on error.
func (c *Client) FetchTorrents(imdbID, contentType, title string) []Stream {
	if c == nil {
		return []Stream{}
	}
	results := c.fetchFromProwlarr(imdbID, contentType, title)
	return c.mapToStremioFormat(results)
}

// fetchFromProwlarr executes the dual-strategy API query in parallel and merges results by infoHash.
// Movies: HD (2040) + UHD (2045) — parallel
// TV: HD by imdbID (5040) + All by title (5000) — parallel when title is set
func (c *Client) fetchFromProwlarr(imdbID, contentType, title string) []ProwlarrResult {
	prowlarrType := "movie"
	if contentType == "series" {
		prowlarrType = "tvsearch"
	}

	baseParams := map[string]string{
		"apikey":     c.cfg.APIKey,
		"type":       prowlarrType,
		"indexerIds": "-2",
	}

	type result struct {
		items []ProwlarrResult
		idx   int
	}

	var queries []map[string]string
	if contentType == "series" {
		queries = append(queries, mergeParams(baseParams, map[string]string{
			"query":      imdbID,
			"categories": "5040",
		}))
		if title != "" {
			queries = append(queries, mergeParams(baseParams, map[string]string{
				"query":      title,
				"categories": "5000",
			}))
		}
	} else {
		queries = append(queries, mergeParams(baseParams, map[string]string{
			"query":      imdbID,
			"categories": "2040",
		}))
		queries = append(queries, mergeParams(baseParams, map[string]string{
			"query":      imdbID,
			"categories": "2045",
		}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	ch := make(chan result, len(queries))
	for i, params := range queries {
		i, params := i, params
		go func() {
			ch <- result{items: c.queryCtx(ctx, params), idx: i}
		}()
	}

	// Collect results preserving q1-first order for dedup
	collected := make([][]ProwlarrResult, len(queries))
	for range queries {
		r := <-ch
		collected[r.idx] = r.items
	}

	// Merge deduplicating by infoHash (q1 first, then q2)
	var all []ProwlarrResult
	for _, items := range collected {
		all = append(all, items...)
	}
	seen := make(map[string]bool, len(all))
	merged := make([]ProwlarrResult, 0, len(all))
	for _, r := range all {
		key := strings.ToLower(r.InfoHash)
		if key == "" {
			continue
		}
		if !seen[key] {
			seen[key] = true
			merged = append(merged, r)
		}
	}
	return merged
}

// queryCtx executes a single Prowlarr API GET request, respecting context cancellation.
func (c *Client) queryCtx(ctx context.Context, params map[string]string) []ProwlarrResult {
	req, err := http.NewRequestWithContext(ctx, "GET", c.searchURL, nil)
	if err != nil {
		log.Printf("[Prowlarr] Error building request: %v", err)
		return nil
	}

	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[Prowlarr] Error fetching from API: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[Prowlarr] API returned status %d", resp.StatusCode)
		return nil
	}

	var results []ProwlarrResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		log.Printf("[Prowlarr] Error decoding response: %v", err)
		return nil
	}
	return results
}

// mapToStremioFormat converts raw Prowlarr results to Stremio/Torrentio stream format.
// Filters out garbage releases and entries without valid resolution.
func (c *Client) mapToStremioFormat(results []ProwlarrResult) []Stream {
	streams := make([]Stream, 0, len(results))
	for _, res := range results {
		if res.InfoHash == "" {
			continue
		}
		if garbageRe.MatchString(res.Title) {
			continue
		}

		resTag := resolveResolution(res.Quality.Quality.Resolution, res.Title)

		sizeGB := float64(res.Size) / (1024 * 1024 * 1024)
		formattedTitle := fmt.Sprintf("%s\n👤 %d ⬇️ %d\n💾 %.2fGB",
			res.Title, res.Seeders, res.Leechers, sizeGB)

		streams = append(streams, Stream{
			Name:     fmt.Sprintf("Torrentio\n%s", resTag),
			Title:    formattedTitle,
			InfoHash: res.InfoHash,
			BehaviorHints: BehaviorHints{
				BingeGroup: fmt.Sprintf("prowlarr-%s", resTag),
			},
		})
	}
	return streams
}

// resolveResolution determines the resolution tag from the API value or falls back to regex.
func resolveResolution(resVal int, title string) string {
	switch resVal {
	case 2160:
		return "4k"
	case 1080:
		return "1080p"
	case 720:
		return "720p"
	}

	// Fallback: regex on title
	if res4kRe.MatchString(title) {
		return "4k"
	}
	if res1080Re.MatchString(title) {
		return "1080p"
	}
	if res720Re.MatchString(title) {
		return "720p"
	}
	return ""
}

// mergeParams combines two parameter maps. Second map overrides first on conflicts.
func mergeParams(base, extra map[string]string) map[string]string {
	result := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}
