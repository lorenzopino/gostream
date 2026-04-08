package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"gostream/internal/catalog"
)

const baseURL = "https://api.themoviedb.org/3"

// EndpointConfig holds a list of discovery endpoints (mirrors config.TMDBEndpointGroup).
type EndpointConfig struct {
	Endpoints []Endpoint
}

// Endpoint defines a single TMDB discovery query (mirrors config.TMDBEndpoint).
type Endpoint struct {
	Name                     string
	Enabled                  bool
	EndpointType             string // "discover" | "trending" | "list"
	Language                 *string
	SortBy                   *string
	Pages                    *int
	VoteAverageGte           *float64
	VoteCountGte             *int
	WithGenres               *string
	WithoutGenres            *string
	WithKeywords             *string
	WithoutKeywords          *string
	WithOriginalLanguage     *string
	WithOriginCountry        *string
	WithRuntimeGte           *int
	WithRuntimeLte           *int
	WatchRegion              *string
	IncludeAdult             *bool
	PrimaryReleaseDateGte    *string
	PrimaryReleaseDateLte    *string
	PrimaryReleaseYear       *int
	WithReleaseType          *string
	Region                   *string
	IncludeVideo             *bool
	FirstAirDateGte          *string
	FirstAirDateLte          *string
	FirstAirDateYear         *int
	WithStatus               *string
	WithType                 *string
	WithNetworks             *string
	IncludeNullFirstAirDates *bool
	EndpointURL              *string
	TimeWindow               *string
}

// Client is a TMDB API client with rate limiting.
type Client struct {
	http    *http.Client
	limiter *rate.Limiter
	apiKey  string
}

// NewClient creates a TMDB client with 4 req/s rate limit (40/10s official).
func NewClient(apiKey string) *Client {
	return &Client{
		http:    catalog.NewClient(30 * time.Second),
		limiter: rate.NewLimiter(rate.Every(250*time.Millisecond), 1),
		apiKey:  apiKey,
	}
}

// parseRelativeDate parses relative date strings like "-6months", "+12months", "2024-01-15".
func parseRelativeDate(s string) string {
	if s == "" {
		return ""
	}
	// Check if it's an absolute date
	if _, err := time.Parse("2006-01-02", s); err == nil {
		return s
	}
	// Parse relative: "-6months", "+1y", "-30d", etc.
	now := time.Now()
	if n, unit := parseRelativeUnit(s); n != 0 {
		var d time.Duration
		switch unit {
		case "months", "month":
			d = time.Duration(n*30) * 24 * time.Hour
		case "years", "year", "y":
			d = time.Duration(n*365) * 24 * time.Hour
		case "days", "day", "d":
			d = time.Duration(n) * 24 * time.Hour
		default:
			return s
		}
		return now.Add(d).Format("2006-01-02")
	}
	return s
}

func parseRelativeUnit(s string) (int, string) {
	s = strings.TrimSpace(s)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	} else if len(s) > 0 && s[0] == '+' {
		s = s[1:]
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, ""
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, ""
	}
	if neg {
		n = -n
	}
	return n, s[i:]
}

func pagesOrDefault(p *int, def int) int {
	if p != nil && *p > 0 {
		return *p
	}
	return def
}

// Movie is a minimal movie entry from TMDB discover/search.
type Movie struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	Language      string `json:"original_language"`
}

// DiscoverResponse is the paginated TMDB response.
type DiscoverResponse struct {
	Page    int     `json:"page"`
	Results []Movie `json:"results"`
}

// DiscoverMovies returns movies from TMDB discover endpoint.
func (c *Client) DiscoverMovies(ctx context.Context, lang string, dateGTE, dateLTE string, pages int) ([]Movie, error) {
	var all []Movie
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/discover/movie?api_key=%s&primary_release_date.gte=%s&primary_release_date.lte=%s&with_original_language=%s&sort_by=popularity.desc&include_adult=false&include_video=false&page=%d",
			baseURL, c.apiKey, dateGTE, dateLTE, lang, page)

		movies, err := c.fetchDiscoverPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, movies...)
	}
	return all, nil
}

// DiscoverMoviesByRegion returns movies from a region-specific endpoint (now_playing, popular).
func (c *Client) DiscoverMoviesByRegion(ctx context.Context, endpoint, region string, pages int) ([]Movie, error) {
	var all []Movie
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s%s?api_key=%s&language=en-US&region=%s&page=%d",
			baseURL, endpoint, c.apiKey, region, page)

		movies, err := c.fetchDiscoverPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, movies...)
	}
	return all, nil
}

// TrendingMovies returns trending movies for the week.
func (c *Client) TrendingMovies(ctx context.Context, pages int) ([]Movie, error) {
	var all []Movie
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/trending/movie/week?api_key=%s&page=%d",
			baseURL, c.apiKey, page)

		movies, err := c.fetchDiscoverPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, movies...)
	}
	return all, nil
}

// DiscoverMoviesFromConfig executes all configured movie discovery endpoints.
func (c *Client) DiscoverMoviesFromConfig(ctx context.Context, cfg EndpointConfig) ([]Movie, error) {
	var all []Movie
	seen := make(map[int]bool)

	for _, ep := range cfg.Endpoints {
		if !ep.Enabled {
			continue
		}
		var movies []Movie
		var err error
		switch ep.EndpointType {
		case "discover":
			movies, err = c.discoverMovieFromEndpoint(ctx, ep)
		case "trending":
			tw := "week"
			if ep.TimeWindow != nil {
				tw = *ep.TimeWindow
			}
			movies, err = c.trendingMoviesWithParams(ctx, tw, pagesOrDefault(ep.Pages, 1))
		case "list":
			region := ""
			if ep.Region != nil {
				region = *ep.Region
			}
			url := ep.EndpointURL
			if url == nil {
				continue
			}
			movies, err = c.DiscoverMoviesByRegion(ctx, *url, region, pagesOrDefault(ep.Pages, 1))
		default:
			continue
		}
		if err != nil {
			continue
		}
		for _, m := range movies {
			if !seen[m.ID] {
				seen[m.ID] = true
				all = append(all, m)
			}
		}
	}
	return all, nil
}

func (c *Client) discoverMovieFromEndpoint(ctx context.Context, ep Endpoint) ([]Movie, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if ep.Language != nil {
		q.Set("language", *ep.Language)
	}
	if ep.SortBy != nil {
		q.Set("sort_by", *ep.SortBy)
	}
	if ep.IncludeAdult != nil {
		q.Set("include_adult", fmt.Sprintf("%t", *ep.IncludeAdult))
	}
	if ep.IncludeVideo != nil {
		q.Set("include_video", fmt.Sprintf("%t", *ep.IncludeVideo))
	}
	if ep.VoteAverageGte != nil {
		q.Set("vote_average.gte", fmt.Sprintf("%.1f", *ep.VoteAverageGte))
	}
	if ep.VoteCountGte != nil {
		q.Set("vote_count.gte", fmt.Sprintf("%d", *ep.VoteCountGte))
	}
	if ep.WithGenres != nil {
		q.Set("with_genres", *ep.WithGenres)
	}
	if ep.WithoutGenres != nil {
		q.Set("without_genres", *ep.WithoutGenres)
	}
	if ep.WithKeywords != nil {
		q.Set("with_keywords", *ep.WithKeywords)
	}
	if ep.WithoutKeywords != nil {
		q.Set("without_keywords", *ep.WithoutKeywords)
	}
	if ep.WithOriginalLanguage != nil {
		q.Set("with_original_language", *ep.WithOriginalLanguage)
	}
	if ep.WithOriginCountry != nil {
		q.Set("with_origin_country", *ep.WithOriginCountry)
	}
	if ep.PrimaryReleaseYear != nil {
		q.Set("primary_release_year", fmt.Sprintf("%d", *ep.PrimaryReleaseYear))
	}
	if ep.WithRuntimeGte != nil {
		q.Set("with_runtime.gte", fmt.Sprintf("%d", *ep.WithRuntimeGte))
	}
	if ep.WithRuntimeLte != nil {
		q.Set("with_runtime.lte", fmt.Sprintf("%d", *ep.WithRuntimeLte))
	}
	if ep.WithReleaseType != nil {
		q.Set("with_release_type", *ep.WithReleaseType)
	}
	if ep.Region != nil {
		q.Set("region", *ep.Region)
	}
	if ep.WatchRegion != nil {
		q.Set("watch_region", *ep.WatchRegion)
	}
	if ep.PrimaryReleaseDateGte != nil {
		q.Set("primary_release_date.gte", parseRelativeDate(*ep.PrimaryReleaseDateGte))
	}
	if ep.PrimaryReleaseDateLte != nil {
		q.Set("primary_release_date.lte", parseRelativeDate(*ep.PrimaryReleaseDateLte))
	}

	var all []Movie
	pages := pagesOrDefault(ep.Pages, 1)
	for page := 1; page <= pages; page++ {
		q.Set("page", fmt.Sprintf("%d", page))
		urlStr := fmt.Sprintf("%s/discover/movie?%s", baseURL, q.Encode())
		movies, err := c.fetchDiscoverPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, movies...)
	}
	return all, nil
}

func (c *Client) trendingMoviesWithParams(ctx context.Context, timeWindow string, pages int) ([]Movie, error) {
	var all []Movie
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/trending/movie/%s?api_key=%s&page=%d", baseURL, timeWindow, c.apiKey, page)
		movies, err := c.fetchDiscoverPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, movies...)
	}
	return all, nil
}

// ExternalIDs returns the IMDB ID for a TMDB movie.
func (c *Client) ExternalIDs(ctx context.Context, tmdbID int) (string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", err
	}

	urlStr := fmt.Sprintf("%s/movie/%d/external_ids?api_key=%s", baseURL, tmdbID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.IMDBID, nil
}

// TVExternalIDs returns the IMDB ID for a TMDB TV show.
func (c *Client) TVExternalIDs(ctx context.Context, tmdbID int) (string, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return "", err
	}

	urlStr := fmt.Sprintf("%s/tv/%d/external_ids?api_key=%s", baseURL, tmdbID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		IMDBID string `json:"imdb_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.IMDBID, nil
}

// MovieDetails returns full movie details including watch providers.
func (c *Client) MovieDetails(ctx context.Context, tmdbID int) (*MovieDetail, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	urlStr := fmt.Sprintf("%s/movie/%d?api_key=%s&append_to_response=watch/providers", baseURL, tmdbID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var detail MovieDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

// TVDetails returns full TV show details.
func (c *Client) TVDetails(ctx context.Context, tmdbID int) (*TVDetail, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	urlStr := fmt.Sprintf("%s/tv/%d?api_key=%s&append_to_response=watch/providers", baseURL, tmdbID, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var detail TVDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

// DiscoverTV returns TV shows from TMDB discover.
func (c *Client) DiscoverTV(ctx context.Context, lang string, dateGTE, dateLTE string, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/discover/tv?api_key=%s&first_air_date.gte=%s&first_air_date.lte=%s&with_original_language=%s&sort_by=popularity.desc&page=%d",
			baseURL, c.apiKey, dateGTE, dateLTE, lang, page)

		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

// TVOnTheAir returns TV shows currently on the air.
func (c *Client) TVOnTheAir(ctx context.Context, pages int) ([]TVShow, error) {
	return c.fetchTVListEndpoint(ctx, "/tv/on_the_air", pages)
}

// TVAiringToday returns TV shows airing today.
func (c *Client) TVAiringToday(ctx context.Context, pages int) ([]TVShow, error) {
	return c.fetchTVListEndpoint(ctx, "/tv/airing_today", pages)
}

// TVPopular returns popular TV shows.
func (c *Client) TVPopular(ctx context.Context, pages int) ([]TVShow, error) {
	return c.fetchTVListEndpoint(ctx, "/tv/popular", pages)
}

// TVTrending returns trending TV shows for the week.
func (c *Client) TVTrending(ctx context.Context, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/trending/tv/week?api_key=%s&page=%d",
			baseURL, c.apiKey, page)

		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

// DiscoverTVFromConfig executes all configured TV discovery endpoints.
func (c *Client) DiscoverTVFromConfig(ctx context.Context, cfg EndpointConfig) ([]TVShow, error) {
	var all []TVShow
	seen := make(map[int]bool)

	for _, ep := range cfg.Endpoints {
		if !ep.Enabled {
			continue
		}
		var shows []TVShow
		var err error
		switch ep.EndpointType {
		case "discover":
			shows, err = c.discoverTVFromEndpoint(ctx, ep)
		case "trending":
			tw := "week"
			if ep.TimeWindow != nil {
				tw = *ep.TimeWindow
			}
			shows, err = c.trendingTVWithParams(ctx, tw, pagesOrDefault(ep.Pages, 1))
		case "list":
			url := ep.EndpointURL
			if url == nil {
				continue
			}
			shows, err = c.fetchTVListPage(ctx, *url, pagesOrDefault(ep.Pages, 1))
		default:
			continue
		}
		if err != nil {
			continue
		}
		for _, s := range shows {
			if !seen[s.ID] {
				seen[s.ID] = true
				all = append(all, s)
			}
		}
	}
	return all, nil
}

func (c *Client) discoverTVFromEndpoint(ctx context.Context, ep Endpoint) ([]TVShow, error) {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	if ep.Language != nil {
		q.Set("language", *ep.Language)
	}
	if ep.SortBy != nil {
		q.Set("sort_by", *ep.SortBy)
	}
	if ep.IncludeAdult != nil {
		q.Set("include_adult", fmt.Sprintf("%t", *ep.IncludeAdult))
	}
	if ep.IncludeNullFirstAirDates != nil {
		q.Set("include_null_first_air_dates", fmt.Sprintf("%t", *ep.IncludeNullFirstAirDates))
	}
	if ep.VoteAverageGte != nil {
		q.Set("vote_average.gte", fmt.Sprintf("%.1f", *ep.VoteAverageGte))
	}
	if ep.VoteCountGte != nil {
		q.Set("vote_count.gte", fmt.Sprintf("%d", *ep.VoteCountGte))
	}
	if ep.WithGenres != nil {
		q.Set("with_genres", *ep.WithGenres)
	}
	if ep.WithoutGenres != nil {
		q.Set("without_genres", *ep.WithoutGenres)
	}
	if ep.WithKeywords != nil {
		q.Set("with_keywords", *ep.WithKeywords)
	}
	if ep.WithoutKeywords != nil {
		q.Set("without_keywords", *ep.WithoutKeywords)
	}
	if ep.WithOriginalLanguage != nil {
		q.Set("with_original_language", *ep.WithOriginalLanguage)
	}
	if ep.WithOriginCountry != nil {
		q.Set("with_origin_country", *ep.WithOriginCountry)
	}
	if ep.WithRuntimeGte != nil {
		q.Set("with_runtime.gte", fmt.Sprintf("%d", *ep.WithRuntimeGte))
	}
	if ep.WithRuntimeLte != nil {
		q.Set("with_runtime.lte", fmt.Sprintf("%d", *ep.WithRuntimeLte))
	}
	if ep.WithStatus != nil {
		q.Set("with_status", *ep.WithStatus)
	}
	if ep.WithType != nil {
		q.Set("with_type", *ep.WithType)
	}
	if ep.WithNetworks != nil {
		q.Set("with_networks", *ep.WithNetworks)
	}
	if ep.WatchRegion != nil {
		q.Set("watch_region", *ep.WatchRegion)
	}
	if ep.FirstAirDateGte != nil {
		q.Set("first_air_date.gte", parseRelativeDate(*ep.FirstAirDateGte))
	}
	if ep.FirstAirDateLte != nil {
		q.Set("first_air_date.lte", parseRelativeDate(*ep.FirstAirDateLte))
	}
	if ep.FirstAirDateYear != nil {
		q.Set("first_air_date_year", fmt.Sprintf("%d", *ep.FirstAirDateYear))
	}

	var all []TVShow
	pages := pagesOrDefault(ep.Pages, 1)
	for page := 1; page <= pages; page++ {
		q.Set("page", fmt.Sprintf("%d", page))
		urlStr := fmt.Sprintf("%s/discover/tv?%s", baseURL, q.Encode())
		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

func (c *Client) trendingTVWithParams(ctx context.Context, timeWindow string, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s/trending/tv/%s?api_key=%s&page=%d", baseURL, timeWindow, c.apiKey, page)
		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

func (c *Client) fetchTVListPage(ctx context.Context, endpoint string, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s%s?api_key=%s&page=%d", baseURL, endpoint, c.apiKey, page)
		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

// SearchMovie searches for a movie by query and optional year.
func (c *Client) SearchMovie(ctx context.Context, query, year string) (int, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return 0, err
	}

	urlStr := fmt.Sprintf("%s/search/movie?api_key=%s&query=%s&page=1",
		baseURL, c.apiKey, url.QueryEscape(query))
	if year != "" {
		urlStr += "&year=" + year
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return 0, err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ID int `json:"id"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	if len(result.Results) == 0 {
		return 0, fmt.Errorf("no results for %q", query)
	}

	return result.Results[0].ID, nil
}

func (c *Client) fetchDiscoverPage(ctx context.Context, urlStr string) ([]Movie, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result DiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

func (c *Client) fetchTVPage(ctx context.Context, urlStr string) ([]TVShow, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := catalog.Do(ctx, c.http, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Results []TVShow `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

func (c *Client) fetchTVListEndpoint(ctx context.Context, endpoint string, pages int) ([]TVShow, error) {
	var all []TVShow
	for page := 1; page <= pages; page++ {
		urlStr := fmt.Sprintf("%s%s?api_key=%s&page=%d", baseURL, endpoint, c.apiKey, page)

		shows, err := c.fetchTVPage(ctx, urlStr)
		if err != nil {
			return all, err
		}
		all = append(all, shows...)
	}
	return all, nil
}

// MovieDetail is the full movie details response from TMDB.
type MovieDetail struct {
	ID             int                    `json:"id"`
	Title          string                 `json:"title"`
	ReleaseDate    string                 `json:"release_date"`
	Runtime        int                    `json:"runtime"`
	WatchProviders map[string]interface{} `json:"watch/providers"`
}

// TVShow is a minimal TV show entry from TMDB.
type TVShow struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	OriginalName string `json:"original_name"`
	FirstAirDate string `json:"first_air_date"`
	Language     string `json:"original_language"`
	GenreIDs     []int  `json:"genre_ids"`
}

// TVDetail is the full TV show details response from TMDB.
type TVDetail struct {
	ID               int         `json:"id"`
	Name             string      `json:"name"`
	FirstAirDate     string      `json:"first_air_date"`
	LastAirDate      string      `json:"last_air_date"`
	NextEpisodeToAir interface{} `json:"next_episode_to_air"`
	NumberOfSeasons  int         `json:"number_of_seasons"`
	Seasons          []Season    `json:"seasons"`
	WatchProviders   interface{} `json:"watch/providers"`
}

// Season represents a TV season.
type Season struct {
	SeasonNumber int `json:"season_number"`
	EpisodeCount int `json:"episode_count"`
}

// HasPremiumProvider checks if a movie/show is available on premium streaming in Italy.
func HasPremiumProvider(providers interface{}) bool {
	if providers == nil {
		return false
	}

	pm, ok := providers.(map[string]interface{})
	if !ok {
		return false
	}

	results, ok := pm["results"].(map[string]interface{})
	if !ok {
		return false
	}

	it, ok := results["IT"].(map[string]interface{})
	if !ok {
		return false
	}

	flatrate, ok := it["flatrate"].([]interface{})
	if !ok {
		return false
	}

	premiumIDs := map[int]bool{8: true, 9: true, 337: true, 531: true}
	for _, p := range flatrate {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := pm["provider_id"].(float64); ok && premiumIDs[int(id)] {
			return true
		}
	}

	return false
}
