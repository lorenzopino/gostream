package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Cache TTL for search results.
const searchCacheTTL = 5 * time.Minute

// searchCacheKey is the composite key for cached search results.
type searchCacheKey struct {
	imdbID      string
	contentType string
	title       string
	categories  string // joined categories for cache key
}

// searchCacheEntry holds cached search results with expiration.
type searchCacheEntry struct {
	streams   []Stream
	expiresAt time.Time
}

// Client queries the Prowlarr API and returns results in Stremio/Torrentio format.
// Thread-safe: all methods are safe for concurrent use.
type Client struct {
	cfg        ConfigProwlarr
	httpClient *http.Client
	searchURL  string

	// V320: TTL cache for search results — avoids repeated Prowlarr queries for same IMDB ID.
	cacheMu sync.RWMutex
	cache   map[searchCacheKey]searchCacheEntry
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
		cache:     make(map[searchCacheKey]searchCacheEntry, 64),
	}
}

// FetchTorrents queries Prowlarr and returns Stremio-format streams.
// contentType is "movie" or "series". title is the show/movie name (used for TV UHD searches).
// categories is a slice of Newznab category IDs to search. If empty, defaults are used.
// Returns an empty slice (never nil) if disabled or on error.
func (c *Client) FetchTorrents(imdbID, contentType, title string, categories []string) []Stream {
	if c == nil {
		return []Stream{}
	}

	// V320: Check cache first.
	catKey := strings.Join(categories, ",")
	key := searchCacheKey{
		imdbID:      imdbID,
		contentType: contentType,
		title:       title,
		categories:  catKey,
	}

	c.cacheMu.RLock()
	if entry, ok := c.cache[key]; ok && time.Now().Before(entry.expiresAt) {
		result := make([]Stream, len(entry.streams))
		copy(result, entry.streams)
		c.cacheMu.RUnlock()
		return result
	}
	c.cacheMu.RUnlock()

	// Cache miss — fetch from Prowlarr.
	results := c.fetchFromProwlarr(imdbID, contentType, title, categories)
	streams := c.mapToStremioFormat(results)

	// Store in cache.
	c.cacheMu.Lock()
	c.cache[key] = searchCacheEntry{
		streams:   streams,
		expiresAt: time.Now().Add(searchCacheTTL),
	}
	// Evict old entries (simple: keep max 128 entries).
	if len(c.cache) > 128 {
		now := time.Now()
		for k, v := range c.cache {
			if now.After(v.expiresAt) {
				delete(c.cache, k)
			}
		}
		// If still over limit, delete oldest (approximate).
		if len(c.cache) > 128 {
			for k := range c.cache {
				delete(c.cache, k)
				if len(c.cache) <= 100 {
					break
				}
			}
		}
	}
	c.cacheMu.Unlock()

	return streams
}

// DefaultMovieCategories returns the default categories for movie search.
func DefaultMovieCategories() []string {
	return []string{"2000"} // All movies
}

// SizeFirstMovieCategories returns categories for size-first movie search.
// Uses the broader "Movies" (2000) category to search all subcategories at once,
// then relies on size ceiling filtering to exclude large files.
func SizeFirstMovieCategories() []string {
	return []string{"2000"} // All movies (SD, HD, UHD, WEB-DL, x265, etc.)
}

// fetchFromProwlarr executes the API queries and merges results by infoHash.
func (c *Client) fetchFromProwlarr(imdbID, contentType, title string, categories []string) []ProwlarrResult {
	prowlarrType := "movie"
	if contentType == "series" {
		prowlarrType = "tvsearch"
	}

	baseParams := map[string]string{
		"apikey":     c.cfg.APIKey,
		"type":       prowlarrType,
		"indexerIds": "-2",
	}

	// Default categories if none specified
	if len(categories) == 0 {
		if contentType == "series" {
			categories = []string{"5000"} // TV All
		} else {
			categories = DefaultMovieCategories()
		}
	}

	type result struct {
		items []ProwlarrResult
		idx   int
	}

	var queries []map[string]string
	if contentType == "series" {
		// TV: query by IMDB ID
		queries = append(queries, mergeParams(baseParams, map[string]string{
			"query":      imdbID,
			"categories": categories[0],
		}))
		// Also query by title for indexers that don't map IMDB IDs (YTS, etc.)
		if title != "" {
			cat := categories[0]
			if len(categories) > 1 {
				cat = categories[1]
			}
			queries = append(queries, mergeParams(baseParams, map[string]string{
				"query":      title,
				"categories": cat,
			}))
		}
	} else {
		// For movies: query each category in parallel with IMDB ID
		for _, cat := range categories {
			queries = append(queries, mergeParams(baseParams, map[string]string{
				"query":      imdbID,
				"categories": cat,
			}))
		}
		// Also query by title for indexers that don't map IMDB IDs (YTS, etc.)
		if title != "" {
			for _, cat := range categories {
				queries = append(queries, mergeParams(baseParams, map[string]string{
					"query":      title,
					"categories": cat,
				}))
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := make(chan result, len(queries))
	for i, params := range queries {
		i, params := i, params
		go func() {
			ch <- result{items: c.queryCtxWithRetry(ctx, params), idx: i}
		}()
	}

	// V320: Inline dedup during collection — avoid double allocation.
	seen := make(map[string]bool, 64)
	var merged []ProwlarrResult
	for range queries {
		r := <-ch
		for _, item := range r.items {
			key := strings.ToLower(item.InfoHash)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, item)
		}
	}
	return merged
}

// queryCtxWithRetry executes a single Prowlarr API GET request with exponential backoff retry.
func (c *Client) queryCtxWithRetry(ctx context.Context, params map[string]string) []ProwlarrResult {
	const maxRetries = 2
	baseDelay := 500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		results := c.queryCtx(ctx, params)
		if results != nil || ctx.Err() != nil {
			return results
		}
		if attempt < maxRetries {
			delay := baseDelay * (1 << attempt) // 500ms, 1s, 2s
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil
			}
		}
	}
	_ = lastErr // logged by queryCtx
	return nil
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
			SizeGB:   sizeGB,
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
