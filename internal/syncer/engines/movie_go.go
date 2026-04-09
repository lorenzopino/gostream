package engines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"gostream/internal/catalog"
	"gostream/internal/catalog/tmdb"
	"gostream/internal/catalog/torrentio"
	"gostream/internal/prowlarr"
	"gostream/internal/syncer/quality"
)

// MovieGoEngine is the pure Go implementation of movie sync.
type MovieGoEngine struct {
	gostorm   *GoStormClient
	tmdb      *tmdb.Client
	torrentio *torrentio.Client
	prowlarr  *prowlarr.Client
	plexURL   string
	plexToken string
	plexLib   int
	moviesDir string
	stateDir  string
	limiter   *rate.Limiter
	logger    *log.Logger

	// Config-driven quality and discovery
	qualityProfile     quality.MovieProfile
	tmdbDiscovery      tmdb.EndpointConfig
	prowlarrCategories []string

	// Negative caches
	noMKVCache     map[string]CacheEntry
	noStreamsCache map[string]CacheEntry
	recheckCache   map[string]CacheEntry
	addFailCache   map[string]CacheEntry
	imdbCache      map[string]IMDBCacheEntry
	noMKVCFile     string
	noStreamsCFile string
	recheckCFile   string
	addFailCFile   string
	imdbCFile      string

	blacklist     BlacklistData
	blacklistFile string
}

// CacheEntry is a generic cache entry with timestamp.
type CacheEntry struct {
	Reason string `json:"reason,omitempty"`
	Title  string `json:"title,omitempty"`
	TS     int64  `json:"ts"`
}

// IMDBCacheEntry caches TMDB→IMDB mapping.
type IMDBCacheEntry struct {
	IMDBID string `json:"imdb_id"`
	Title  string `json:"title"`
	TS     int64  `json:"ts"`
}

// BlacklistData holds blocked hashes and titles.
type BlacklistData struct {
	Hashes map[string]string `json:"hashes,omitempty"`
	Titles []string          `json:"titles,omitempty"`
}

// MovieEngineConfig holds config for the movie engine.
type MovieEngineConfig struct {
	GoStormURL   string
	TMDBAPIKey   string
	TorrentioURL string
	PlexURL      string
	PlexToken    string
	PlexLib      int
	MoviesDir    string
	StateDir     string
	LogsDir      string
	ProwlarrCfg     prowlarr.ConfigProwlarr
	QualityProfile  quality.MovieProfile
	TMDBDiscovery   tmdb.EndpointConfig
	ProwlarrCategories []string
}

// Movie thresholds
const (
	mMovie4KBase         = 1000
	mMovie1080pBase      = 200
	mMovieHDRBonus       = 60
	mMovieDVBonus        = 100
	mMovieAtmosBonus     = 50
	mMovie51Bonus        = 25
	mMovieStereoPenalty  = -50
	mMovieRemuxBonus     = 30
	mMovieITABonus       = 60
	mMovieUnknownPenalty = -5
	mMovieMinSeeders     = 15
	mMovie4KMinGB        = 10
	mMovie4KMaxGB        = 60
	mMovie1080PMinGB     = 4
	mMovie1080PMaxGB     = 20
	mMovieUpgradePct     = 1.1
	mMovieProcessSleep   = 1 * time.Second
	mMovieMetadataWait   = 12
	mMovie4KMetadataWait = 45
	noMKVCacheTTL        = 12 * time.Hour
	noStreamsCacheTTL    = 24 * time.Hour
	recheckCacheTTL      = 48 * time.Hour
	addFailCacheTTL      = 168 * time.Hour
)

var (
	reM4K        = regexp.MustCompile(`(?i)2160p|4[kK]|uhd`)
	reM1080p     = regexp.MustCompile(`(?i)1080p|1080i|fhd`)
	reM720p      = regexp.MustCompile(`(?i)720p|720i`)
	reMHDR       = regexp.MustCompile(`(?i)\bhdr\b|hdr10\+?`)
	reMDV        = regexp.MustCompile(`(?i)\bdv\b|dovi|dolby.?vision`)
	reMAtmos     = regexp.MustCompile(`(?i)atmos`)
	reM51        = regexp.MustCompile(`(?i)5\.1|dts|ddp5|ddp|dd\+|eac3|ac3`)
	reMStereo    = regexp.MustCompile(`(?i)stereo|aac|mp3|2\.0`)
	reMRemux     = regexp.MustCompile(`(?i)\bremux\b`)
	reMITA       = regexp.MustCompile(`(?i)\bita\b|🇮🇹`)
	reMExclLang  = regexp.MustCompile(`🇪🇸|🇫🇷|🇩🇪|🇷🇺|🇨🇳|🇯🇵|🇰🇷|🇹🇭|🇵🇹|🇧🇷`)
	reMGarbage   = regexp.MustCompile(`(?i)camrip|hdcam|hdts|telesync|\bts\b|telecine|\btc\b|\bscr\b|screener|webscreener`)
	reMSeeders   = regexp.MustCompile(`👤\s*(\d+)`)
	reMSize      = regexp.MustCompile(`(?i)💾\s*([\d.]+)\s*(GB|MB)`)
	reMHashURL   = regexp.MustCompile(`link=([a-f0-9]{40})`)
	reMMKVHash8  = regexp.MustCompile(`_([a-f0-9]{8})\.mkv$`)
	reMYear      = regexp.MustCompile(`[._]((?:19|20)\d{2})[._]`)
	reMNonWord   = regexp.MustCompile(`[^a-z0-9]`)
	reMQuality   = regexp.MustCompile(`(?i)(2160p|1080p|720p|4k|uhd)`)
	reMTitleYear = regexp.MustCompile(`(.+?)[._\s]\(?((?:19|20)\d{2})\)?`)
)

// NewMovieGoEngine creates a new Go movie sync engine.
func NewMovieGoEngine(cfg MovieEngineConfig) *MovieGoEngine {
	var prowlarrClient *prowlarr.Client
	if cfg.ProwlarrCfg.Enabled {
		prowlarrClient = prowlarr.NewClient(cfg.ProwlarrCfg)
	}

	logPath := filepath.Join(cfg.LogsDir, "movies-sync.log")
	logFile, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	logger := log.New(io.MultiWriter(os.Stdout, logFile), "[MovieSync] ", log.LstdFlags)

	e := &MovieGoEngine{
		gostorm:   NewGoStormClient(cfg.GoStormURL),
		tmdb:      tmdb.NewClient(cfg.TMDBAPIKey),
		torrentio: torrentio.NewClient(cfg.TorrentioURL, "sort=qualitysize|qualityfilter=480p,720p,scr,cam"),
		prowlarr:  prowlarrClient,
		plexURL:   cfg.PlexURL,
		plexToken: cfg.PlexToken,
		plexLib:   cfg.PlexLib,
		moviesDir: cfg.MoviesDir,
		stateDir:  cfg.StateDir,
		limiter:   rate.NewLimiter(rate.Every(250*time.Millisecond), 1),
		logger:    logger,

		qualityProfile:     cfg.QualityProfile,
		tmdbDiscovery:      cfg.TMDBDiscovery,
		prowlarrCategories: cfg.ProwlarrCategories,

		noMKVCFile:     filepath.Join(cfg.StateDir, "no_mkv_hashes.json"),
		noStreamsCFile: filepath.Join(cfg.StateDir, "movie_no_streams_cache.json"),
		recheckCFile:   filepath.Join(cfg.StateDir, "movie_recheck_cache.json"),
		addFailCFile:   filepath.Join(cfg.StateDir, "movie_add_fail_cache.json"),
		imdbCFile:      filepath.Join(cfg.StateDir, "movie_imdb_cache.json"),
		blacklistFile:  filepath.Join(cfg.StateDir, "blacklist.json"),
	}

	e.noMKVCache = e.loadCache(e.noMKVCFile)
	e.noStreamsCache = e.loadCache(e.noStreamsCFile)
	e.recheckCache = e.loadCache(e.recheckCFile)
	e.addFailCache = e.loadCache(e.addFailCFile)
	e.imdbCache = e.loadIMDBCache(e.imdbCFile)
	e.blacklist = e.loadBlacklist()

	e.pruneExpiredCaches()

	return e
}

func (e *MovieGoEngine) Name() string { return "movies" }

func (e *MovieGoEngine) Run(ctx context.Context) error {
	e.logger.Printf("[MovieSync] Starting discovery...")
	movies, err := e.discoverMovies(ctx)
	if err != nil {
		return fmt.Errorf("discover movies: %w", err)
	}
	e.logger.Printf("[MovieSync] Discovered %d movies", len(movies))

	existingIndex, diskHashes := e.buildExistingMovieIndex()
	e.logger.Printf("[MovieSync] Existing index: %d movies, %d hashes on disk", len(existingIndex), len(diskHashes))

	created := 0
	for i, m := range movies {
		select {
		case <-ctx.Done():
			e.logger.Printf("[MovieSync] Stopped after %d/%d movies (%d created)", i, len(movies), created)
			return ctx.Err()
		default:
		}

		if e.processMovie(ctx, m, existingIndex, diskHashes) {
			created++
		}
		time.Sleep(mMovieProcessSleep)
	}

	e.logger.Printf("[MovieSync] Processing complete: %d created out of %d discovered", created, len(movies))
	e.saveAllCaches()
	e.rehydrateMissingTorrents(ctx)
	e.cleanupOrphanedFiles(ctx)

	if e.plexLib > 0 && e.plexURL != "" && e.plexToken != "" {
		url := fmt.Sprintf("%s/library/sections/%d/refresh?X-Plex-Token=%s", e.plexURL, e.plexLib, e.plexToken)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		client := catalog.NewClient(10 * time.Second)
		resp, err := catalog.Do(context.Background(), client, req)
		if err == nil {
			resp.Body.Close()
		}
	}

	return nil
}

func (e *MovieGoEngine) discoverMovies(ctx context.Context) ([]tmdb.Movie, error) {
	if len(e.tmdbDiscovery.Endpoints) > 0 {
		return e.tmdb.DiscoverMoviesFromConfig(ctx, e.tmdbDiscovery)
	}
	return e.discoverMoviesHardcoded(ctx)
}

func (e *MovieGoEngine) discoverMoviesHardcoded(ctx context.Context) ([]tmdb.Movie, error) {
	cutoff := time.Now().AddDate(0, -6, 0).Format("2006-01-02")
	currentYear := time.Now().Year() + 1
	dateLTE := fmt.Sprintf("%d-12-31", currentYear)

	var all []tmdb.Movie
	seen := make(map[int]bool)

	endpoints := []func(context.Context) ([]tmdb.Movie, error){
		func(ctx context.Context) ([]tmdb.Movie, error) {
			return e.tmdb.DiscoverMovies(ctx, "en", cutoff, dateLTE, 12)
		},
		func(ctx context.Context) ([]tmdb.Movie, error) {
			return e.tmdb.DiscoverMovies(ctx, "it", cutoff, dateLTE, 3)
		},
		func(ctx context.Context) ([]tmdb.Movie, error) {
			return e.tmdb.DiscoverMoviesByRegion(ctx, "/movie/now_playing", "US", 1)
		},
		func(ctx context.Context) ([]tmdb.Movie, error) {
			return e.tmdb.DiscoverMoviesByRegion(ctx, "/movie/now_playing", "GB", 1)
		},
		func(ctx context.Context) ([]tmdb.Movie, error) {
			return e.tmdb.DiscoverMoviesByRegion(ctx, "/movie/popular", "US", 3)
		},
		func(ctx context.Context) ([]tmdb.Movie, error) {
			return e.tmdb.TrendingMovies(ctx, 1)
		},
	}

	for _, fn := range endpoints {
		movies, err := fn(ctx)
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

type movieFile struct {
	path  string
	imdb  string
	score int
}

func (e *MovieGoEngine) buildExistingMovieIndex() (map[string]movieFile, map[string]bool) {
	index := make(map[string]movieFile)
	diskHashes := make(map[string]bool)
	if _, err := os.Stat(e.moviesDir); err != nil {
		return index, diskHashes
	}

	filepath.Walk(e.moviesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(strings.ToLower(path), ".mkv") {
			return nil
		}
		// Collect hash8 from filename (last 8 hex chars before .mkv)
		if m := reMMKVHash8.FindStringSubmatch(info.Name()); len(m) >= 2 {
			diskHashes[strings.ToLower(m[1])] = true
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) > 10240 {
			return nil
		}

		var imdb string
		content := strings.TrimSpace(string(data))

		// Try JSON format first (new Go format)
		if strings.HasPrefix(content, "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(content), &obj); err == nil {
				imdb, _ = obj["imdb"].(string)
			}
		} else {
			// Text format (old Python format): line 4 = IMDB ID
			lines := strings.SplitN(content, "\n", 4)
			if len(lines) >= 4 {
				imdb = strings.TrimSpace(lines[3])
			}
		}

		if imdb == "" {
			return nil
		}
		name := info.Name()
		is4K := reM4K.MatchString(name)
		is1080p := reM1080p.MatchString(name) && !reM720p.MatchString(name)
		is720p := reM720p.MatchString(name) && !is1080p && !is4K
		score := e.calculateMovieScore(name, 0, 0, is4K, is1080p, is720p, 0)
		if existing, ok := index[imdb]; !ok || score > existing.score {
			index[imdb] = movieFile{path: path, imdb: imdb, score: score}
		}
		return nil
	})

	return index, diskHashes
}

func (e *MovieGoEngine) processMovie(ctx context.Context, movie tmdb.Movie, existingIndex map[string]movieFile, diskHashes map[string]bool) bool {
	title := movie.Title
	if title == "" {
		title = movie.OriginalTitle
	}
	if title == "" {
		return false
	}

	// Blacklist check
	if e.isBlacklisted(title) {
		return false
	}

	// Resolve IMDB
	imdbID := e.resolveIMDB(ctx, movie.ID, title)
	if imdbID == "" {
		return false
	}

	// Check negative caches
	if e.isInCache(e.noStreamsCache, imdbID, noStreamsCacheTTL) {
		return false
	}
	if e.isInCache(e.recheckCache, imdbID, recheckCacheTTL) {
		return false
	}
	if e.isInCache(e.addFailCache, imdbID, addFailCacheTTL) {
		return false
	}

	// Get streams
	e.logger.Printf("[MovieSync] Processing: %s (%s)", title, imdbID)
	streams, err := e.getMovieStreams(ctx, imdbID, title)
	if err != nil || len(streams) == 0 {
		e.setCache(e.noStreamsCache, imdbID, CacheEntry{Title: title, TS: time.Now().Unix()})
		return false
	}
	delete(e.noStreamsCache, imdbID)

	// Filter: pick best by priority order + size
	// Log ALL Prowlarr results to debug why some are filtered
	for i, s := range streams {
		if i >= 25 {
			break
		}
		e.logger.Printf("[MovieSync]     Prowlarr #%d: %s", i+1, s.Title)
	}
	candidates := e.filterMovieStreams(streams)
	if len(candidates) == 0 {
		e.setCache(e.recheckCache, imdbID, CacheEntry{Title: title, Reason: "no_valid_stream", TS: time.Now().Unix()})
		return false
	}
	e.logger.Printf("[MovieSync]   %s: %d candidates after filtering (from %d Prowlarr results)", title, len(candidates), len(streams))
	for i, c := range candidates {
		if i >= 10 {
			break
		}
		e.logger.Printf("[MovieSync]     Candidate #%d: %.2fGB seeders=%d score=%d", i+1, c.SizeGB, c.Seeders, c.QualityScore)
	}

	// Check if we already have this movie
	existing := existingIndex[imdbID]
	existingPath := existing.path
	existingScore := existing.score

	// Try candidates
	for _, c := range candidates {
		if existingPath != "" && float64(c.QualityScore) <= float64(existingScore)*mMovieUpgradePct {
			e.setCache(e.recheckCache, imdbID, CacheEntry{Title: title, Reason: "no_better_stream", TS: time.Now().Unix()})
			return false
		}

		if e.isInCache(e.noMKVCache, c.Hash, noMKVCacheTTL) {
			continue
		}

		if diskHashes[c.Hash[len(c.Hash)-8:]] {
			continue
		}

		magnet := BuildMagnet(c.Hash, title, DefaultTrackers())
		e.logger.Printf("[MovieSync]     Trying: %.2fGB hash=%s", c.SizeGB, c.Hash)
		hash, err := e.gostorm.AddTorrent(ctx, magnet, title)
		if err != nil || hash == "" {
			e.logger.Printf("[MovieSync]     AddTorrent failed: err=%v", err)
			e.setCache(e.addFailCache, imdbID, CacheEntry{Title: title, Reason: "add_failed", TS: time.Now().Unix()})
			continue
		}
		delete(e.addFailCache, imdbID)

		maxWait := mMovieMetadataWait
		if c.Is4K {
			maxWait = mMovie4KMetadataWait
		}

		info, err := e.gostorm.GetTorrentInfo(ctx, hash, maxWait)
		if err != nil {
			e.logger.Printf("[MovieSync]     GetTorrentInfo failed for %.2fGB: %v (waited %ds)", c.SizeGB, err, maxWait)
			e.setCache(e.noMKVCache, hash, CacheEntry{Reason: "metadata_timeout", TS: time.Now().Unix()})
			e.gostorm.RemoveTorrent(ctx, hash)
			continue
		}

		videoFiles := e.filterVideoFiles(info.FileStats, c.Is4K, c.Is1080p, c.Is720p)
		if len(videoFiles) == 0 {
			e.logger.Printf("[MovieSync]     No valid video files in %.2fGB torrent (%d files total)", c.SizeGB, len(info.FileStats))
			e.setCache(e.noMKVCache, hash, CacheEntry{Reason: "no_valid_files", TS: time.Now().Unix()})
			e.gostorm.RemoveTorrent(ctx, hash)
			continue
		}

		e.logger.Printf("[MovieSync]     SUCCESS: %.2fGB torrent matched with %d video files", c.SizeGB, len(videoFiles))

		// Take largest
		sort.Slice(videoFiles, func(i, j int) bool {
			return videoFiles[i].Length > videoFiles[j].Length
		})
		bestFile := videoFiles[0]

		// Remove existing if upgrading
		if existingPath != "" {
			e.logger.Printf("[MovieSync] Upgrade: removing %s", filepath.Base(existingPath))
			os.Remove(existingPath)
		}

		filename := e.buildMovieFilename(title, movie.ReleaseDate, c)
		mkvPath := filepath.Join(e.moviesDir, filename)
		streamURL := fmt.Sprintf("%s/stream?link=%s&index=%d&play", e.gostorm.baseURL, hash, bestFile.ID)

		if e.createMKV(mkvPath, streamURL, bestFile.Length, magnet, imdbID) {
			res := "4K"
			if !c.Is4K {
				res = "1080p"
			}
			e.logger.Printf("[MovieSync] Created: %s (%s, %.1fGB, score:%d)", filename, res, float64(bestFile.Length)/1024/1024/1024, c.QualityScore)
			e.setCache(e.recheckCache, imdbID, CacheEntry{Title: title, Reason: "processed", TS: time.Now().Unix()})
			return true
		}

		e.gostorm.RemoveTorrent(ctx, hash)
	}

	e.setCache(e.recheckCache, imdbID, CacheEntry{Title: title, Reason: "no_better_stream", TS: time.Now().Unix()})
	return false
}

type MovieStream struct {
	Title        string
	Hash         string
	Is4K         bool
	Is1080p      bool
	Is720p       bool
	QualityScore int
	Seeders      int
	SizeGB       float64
}

func (e *MovieGoEngine) getMovieStreams(ctx context.Context, imdbID, title string) ([]prowlarr.Stream, error) {
	// Prowlarr first
	if e.prowlarr != nil {
		streams := e.prowlarr.FetchTorrents(imdbID, "movie", title, e.prowlarrCategories)
		if len(streams) > 0 {
			return streams, nil
		}
	}

	// Torrentio fallback
	tioStreams, err := e.torrentio.FetchMovieStreams(ctx, imdbID)
	if err != nil {
		return nil, err
	}

	var streams []prowlarr.Stream
	for _, s := range tioStreams {
		streams = append(streams, prowlarr.Stream{
			Name:     s.Name,
			Title:    s.Title,
			InfoHash: s.InfoHash,
		})
	}

	return streams, nil
}

func (e *MovieGoEngine) filterMovieStreams(streams []prowlarr.Stream) []MovieStream {
	prof := e.qualityProfile
	priorityOrder := prof.PriorityOrder
	if len(priorityOrder) == 0 {
		priorityOrder = []string{"4k", "1080p", "720p"}
	}

	// Try each resolution in priority order; first match wins
	for _, res := range priorityOrder {
		var candidates []MovieStream
		for _, s := range streams {
			c := e.classifyMovieStream(s)
			if c == nil {
				continue
			}
			// Match resolution to current priority pass
			match := false
			switch res {
			case "4k":
				match = c.Is4K
			case "1080p":
				match = c.Is1080p && !c.Is4K
			case "720p":
				match = c.Is720p && !c.Is4K && !c.Is1080p
			}
			if !match {
				continue
			}
			candidates = append(candidates, *c)
		}
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].QualityScore > candidates[j].QualityScore
			})
			return candidates
		}
	}

	// 4K fallback if configured (when 4K wasn't in priority_order or nothing matched)
	if prof.Fallback4KMinSeeders != nil {
		var fallback []MovieStream
		for _, s := range streams {
			c := e.classifyMovieStream(s)
			if c != nil && c.Is4K && c.Seeders >= *prof.Fallback4KMinSeeders {
				fallback = append(fallback, *c)
			}
		}
		if len(fallback) > 0 {
			sort.Slice(fallback, func(i, j int) bool {
				return fallback[i].SizeGB < fallback[j].SizeGB
			})
			return fallback
		}
	}

	return nil
}

func (e *MovieGoEngine) classifyMovieStream(s prowlarr.Stream) *MovieStream {
	title := s.Title
	fullText := title + " " + s.Name
	prof := e.qualityProfile

	if reMGarbage.MatchString(fullText) {
		return nil
	}
	if reMExclLang.MatchString(title) {
		return nil
	}
	if e.isBlacklisted(title) {
		return nil
	}
	if _, ok := e.blacklist.Hashes[strings.ToLower(s.InfoHash)]; ok {
		return nil
	}

	seeders := e.extractMovieSeeders(title)
	minSeeders := 15 // fallback default
	if prof.MinSeeders != nil {
		minSeeders = *prof.MinSeeders
	}
	// Only filter by seeders if we have a valid count (some indexers like Pirate Bay don't report it)
	if seeders > 0 && seeders < minSeeders {
		return nil
	}

	is4K := reM4K.MatchString(fullText)
	is1080p := reM1080p.MatchString(fullText) && !reM720p.MatchString(fullText)
	is720p := reM720p.MatchString(fullText) && !is1080p && !is4K

	// Check include flags from profile (nil means use default=true)
	include4K := prof.Include4K == nil || *prof.Include4K
	include1080p := prof.Include1080p == nil || *prof.Include1080p
	include720p := prof.Include720p == nil || *prof.Include720p
	if is4K && !include4K {
		return nil
	}
	if is1080p && !include1080p {
		return nil
	}
	if is720p && !include720p {
		return nil
	}

	if !is4K && !is1080p && !is720p {
		return nil
	}

	sizeGB := s.SizeGB

	// Check size floor/ceiling from profile
	var floorGB, ceilingGB float64
	if is4K {
		floorGB = prof.SizeFloorGB["4k"]
		ceilingGB = prof.SizeCeilingGB["4k"]
	} else if is1080p {
		floorGB = prof.SizeFloorGB["1080p"]
		ceilingGB = prof.SizeCeilingGB["1080p"]
	} else {
		floorGB = prof.SizeFloorGB["720p"]
		ceilingGB = prof.SizeCeilingGB["720p"]
	}

	// Check size floor/ceiling from profile
	if ceilingGB > 0 && sizeGB != 0 && (sizeGB < floorGB || sizeGB > ceilingGB) {
		return nil
	}
	// Reject unknown size only for 4K (unless it's a fallback scenario)
	if ceilingGB > 0 && sizeGB == 0 && is4K && prof.Fallback4KMinSeeders == nil {
		return nil
	}

	score := e.calculateMovieScore(fullText, seeders, sizeGB, is4K, is1080p, is720p, ceilingGB)
	if score <= 0 {
		return nil
	}

	return &MovieStream{
		Title:        title,
		Hash:         strings.ToLower(s.InfoHash),
		Is4K:         is4K,
		Is1080p:      is1080p,
		Is720p:       is720p,
		QualityScore: score,
		Seeders:      seeders,
		SizeGB:       sizeGB,
	}
}

func (e *MovieGoEngine) calculateMovieScore(text string, seeders int, sizeGB float64, is4K, is1080p, is720p bool, ceilingGB float64) int {
	w := e.qualityProfile.ScoreWeights
	score := 0

	// Resolution score
	if is4K && w.Resolution4K != nil {
		score += *w.Resolution4K
	} else if is1080p && w.Resolution1080p != nil {
		score += *w.Resolution1080p
	} else if is720p && w.Resolution720p != nil {
		score += *w.Resolution720p
	}

	// HDR / Dolby Vision
	if reMDV.MatchString(text) && w.DolbyVision != nil {
		score += *w.DolbyVision
	} else if reMHDR.MatchString(text) && w.HDR != nil {
		score += *w.HDR
	}

	// Audio
	if reMAtmos.MatchString(text) && w.Atmos != nil {
		score += *w.Atmos
	} else if reM51.MatchString(text) && w.Audio51 != nil {
		score += *w.Audio51
	} else if reMStereo.MatchString(text) && w.AudioStereo != nil {
		score += *w.AudioStereo
	} else if w.AudioStereo == nil {
		score += 5 // default neutral bonus when not configured
	}

	// Remux (usually veto at -500)
	if reMRemux.MatchString(text) && w.Remux != nil {
		score += *w.Remux
	}

	// Italian language
	if reMITA.MatchString(text) && w.ITA != nil {
		score += *w.ITA
	}

	// Unknown size penalty for 4K
	if sizeGB == 0 && is4K && w.UnknownSizePenalty != nil {
		score += *w.UnknownSizePenalty
	}

	// Size bonus: +N points per GB under ceiling (rewards smaller files)
	if sizeGB > 0 && ceilingGB > 0 && w.SizeBonusPerGBUnder != nil {
		underGB := ceilingGB - sizeGB
		if underGB > 0 {
			score += int(underGB) * (*w.SizeBonusPerGBUnder)
		}
	}

	// Seeder bonus: +5 points per seeder, capped at 500 total (max at 100 seeders)
	if w.SeederBonus != nil && seeders > 0 {
		bonus := seeders * (*w.SeederBonus)
		if bonus > 500 {
			bonus = 500
		}
		score += bonus
	}

	return score
}

func (e *MovieGoEngine) extractMovieSeeders(title string) int {
	m := reMSeeders.FindStringSubmatch(title)
	if len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func (e *MovieGoEngine) extractMovieSizeGB(title string) float64 {
	m := reMSize.FindStringSubmatch(title)
	if len(m) >= 3 {
		v, _ := strconv.ParseFloat(m[1], 64)
		if strings.EqualFold(m[2], "GB") {
			return v
		}
		return v / 1000.0
	}
	return 0
}

func (e *MovieGoEngine) filterVideoFiles(files []FileStat, is4K, is1080p, is720p bool) []FileStat {
	prof := e.qualityProfile
	var valid []FileStat
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		if ext != ".mkv" && ext != ".mp4" && ext != ".avi" && ext != ".mov" && ext != ".m4v" {
			continue
		}
		// Get floor/ceiling from profile (fallback to defaults if not set)
		var minGB, maxGB float64
		if is4K {
			minGB = prof.SizeFloorGB["4k"]
			maxGB = prof.SizeCeilingGB["4k"]
			if minGB == 0 {
				minGB = 1
			}
			if maxGB == 0 {
				maxGB = 60
			}
		} else if is1080p {
			minGB = prof.SizeFloorGB["1080p"]
			maxGB = prof.SizeCeilingGB["1080p"]
			if minGB == 0 {
				minGB = 0.5
			}
			if maxGB == 0 {
				maxGB = 20
			}
		} else if is720p {
			minGB = prof.SizeFloorGB["720p"]
			maxGB = prof.SizeCeilingGB["720p"]
			if minGB == 0 {
				minGB = 0.3
			}
			if maxGB == 0 {
				maxGB = 10
			}
		}
		minSize := int64(minGB * 1024 * 1024 * 1024)
		maxSize := int64(maxGB * 1024 * 1024 * 1024)
		if f.Length >= minSize && f.Length <= maxSize {
			valid = append(valid, f)
		}
	}
	return valid
}

func (e *MovieGoEngine) buildMovieFilename(title, releaseDate string, stream MovieStream) string {
	year := ""
	if len(releaseDate) >= 4 {
		year = releaseDate[:4]
	} else if m := reMTitleYear.FindStringSubmatch(title); len(m) > 2 {
		year = m[2]
	}

	base := e.sanitizeMovieFilename(title)
	if year != "" {
		base = fmt.Sprintf("%s_%s", base, year)
	}

	if stream.Is4K {
		base += "_2160p"
	} else {
		base += "_1080p"
	}

	if reMDV.MatchString(stream.Title) {
		base += "_DV"
	} else if reMHDR.MatchString(stream.Title) {
		base += "_HDR"
	}

	if reMAtmos.MatchString(stream.Title) {
		base += "_Atmos"
	} else if reM51.MatchString(stream.Title) {
		base += "_5.1"
	}

	if reMRemux.MatchString(stream.Title) {
		base += "_REMUX"
	}

	return fmt.Sprintf("%s_%s.mkv", base, stream.Hash[len(stream.Hash)-8:])
}

func (e *MovieGoEngine) sanitizeMovieFilename(s string) string {
	s = regexp.MustCompile(`[^a-zA-Z0-9._-]`).ReplaceAllString(s, "_")
	s = regexp.MustCompile(`_+`).ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

func (e *MovieGoEngine) resolveIMDB(ctx context.Context, tmdbID int, title string) string {
	// Check cache
	if entry, ok := e.imdbCache[strconv.Itoa(tmdbID)]; ok {
		if entry.IMDBID != "" {
			return entry.IMDBID
		}
	}

	imdbID, err := e.tmdb.ExternalIDs(ctx, tmdbID)
	if err != nil || imdbID == "" {
		return ""
	}

	e.imdbCache[strconv.Itoa(tmdbID)] = IMDBCacheEntry{
		IMDBID: imdbID,
		Title:  title,
		TS:     time.Now().Unix(),
	}

	return imdbID
}

func (e *MovieGoEngine) isBlacklisted(title string) bool {
	t := strings.ToLower(title)
	t = reMYear.ReplaceAllString(t, "")
	t = reMQuality.ReplaceAllString(t, "")
	normalized := reMNonWord.ReplaceAllString(t, "")

	for _, bt := range e.blacklist.Titles {
		if bt == normalized {
			return true
		}
	}

	return false
}

func (e *MovieGoEngine) rehydrateMissingTorrents(ctx context.Context) {
	torrents, err := e.gostorm.ListTorrents(ctx)
	if err != nil {
		return
	}
	activeHashes := make(map[string]bool)
	for _, t := range torrents {
		activeHashes[t.Hash] = true
	}

	filepath.Walk(e.moviesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(strings.ToLower(path), ".mkv") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var url, magnet string
		var size float64
		content := strings.TrimSpace(string(data))

		if strings.HasPrefix(content, "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal(data, &obj); err != nil {
				return nil
			}
			url, _ = obj["url"].(string)
			magnet, _ = obj["magnet"].(string)
			size, _ = obj["size"].(float64)
		} else {
			lines := strings.SplitN(content, "\n", 4)
			if len(lines) < 3 {
				return nil
			}
			url = strings.TrimSpace(lines[0])
			magnet = strings.TrimSpace(lines[2])
			if len(lines) > 1 {
				size, _ = strconv.ParseFloat(strings.TrimSpace(lines[1]), 64)
			}
		}

		m := reMHashURL.FindStringSubmatch(url)
		if len(m) < 2 {
			return nil
		}
		hash := m[1]

		if activeHashes[hash] {
			return nil
		}

		if strings.HasPrefix(magnet, "magnet:?") {
			displayTitle := TitleFromFilename(info.Name())
			freshMagnet := BuildMagnet(hash, displayTitle, DefaultTrackers())
			if _, err := e.gostorm.AddTorrent(ctx, freshMagnet, displayTitle); err == nil {
				e.createMKV(path, url, int64(size), freshMagnet, "")
			}
		}

		return nil
	})
}

func (e *MovieGoEngine) cleanupOrphanedFiles(ctx context.Context) {
	torrents, err := e.gostorm.ListTorrents(ctx)
	if err != nil {
		return
	}
	activeHashes := make(map[string]bool)
	for _, t := range torrents {
		activeHashes[t.Hash] = true
	}

	filepath.Walk(e.moviesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(strings.ToLower(path), ".mkv") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var url string
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal(data, &obj); err != nil {
				return nil
			}
			url, _ = obj["url"].(string)
		} else {
			lines := strings.SplitN(content, "\n", 2)
			if len(lines) < 1 {
				return nil
			}
			url = strings.TrimSpace(lines[0])
		}

		m := reMHashURL.FindStringSubmatch(url)
		if len(m) < 2 {
			return nil
		}
		if !activeHashes[m[1]] {
			os.Remove(path)
		}
		return nil
	})
}

// Cache helpers
func (e *MovieGoEngine) loadCache(file string) map[string]CacheEntry {
	data, err := os.ReadFile(file)
	if err != nil {
		return make(map[string]CacheEntry)
	}
	var m map[string]CacheEntry
	json.Unmarshal(data, &m)
	return m
}

func (e *MovieGoEngine) loadIMDBCache(file string) map[string]IMDBCacheEntry {
	data, err := os.ReadFile(file)
	if err != nil {
		return make(map[string]IMDBCacheEntry)
	}
	var m map[string]IMDBCacheEntry
	json.Unmarshal(data, &m)
	return m
}

func (e *MovieGoEngine) loadBlacklist() BlacklistData {
	data, err := os.ReadFile(e.blacklistFile)
	if err != nil {
		return BlacklistData{Hashes: make(map[string]string), Titles: []string{}}
	}
	var bl BlacklistData
	json.Unmarshal(data, &bl)
	if bl.Hashes == nil {
		bl.Hashes = make(map[string]string)
	}
	return bl
}

func (e *MovieGoEngine) isInCache(cache map[string]CacheEntry, key string, ttl time.Duration) bool {
	entry, ok := cache[key]
	if !ok {
		return false
	}
	if time.Since(time.Unix(entry.TS, 0)) > ttl {
		delete(cache, key)
		return false
	}
	return true
}

func (e *MovieGoEngine) setCache(cache map[string]CacheEntry, key string, entry CacheEntry) {
	cache[key] = entry
}

func (e *MovieGoEngine) pruneExpiredCaches() {
	now := time.Now()

	for k, v := range e.noMKVCache {
		if now.Sub(time.Unix(v.TS, 0)) > noMKVCacheTTL {
			delete(e.noMKVCache, k)
		}
	}
	for k, v := range e.noStreamsCache {
		if now.Sub(time.Unix(v.TS, 0)) > noStreamsCacheTTL {
			delete(e.noStreamsCache, k)
		}
	}
	for k, v := range e.recheckCache {
		if now.Sub(time.Unix(v.TS, 0)) > recheckCacheTTL {
			delete(e.recheckCache, k)
		}
	}
	for k, v := range e.addFailCache {
		if now.Sub(time.Unix(v.TS, 0)) > addFailCacheTTL {
			delete(e.addFailCache, k)
		}
	}
}

func (e *MovieGoEngine) saveAllCaches() {
	e.saveCache(e.noMKVCFile, e.noMKVCache)
	e.saveCache(e.noStreamsCFile, e.noStreamsCache)
	e.saveCache(e.recheckCFile, e.recheckCache)
	e.saveCache(e.addFailCFile, e.addFailCache)
	e.saveIMDBCache(e.imdbCFile, e.imdbCache)
}

func (e *MovieGoEngine) saveCache(file string, data interface{}) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	tmp := file + ".tmp"
	os.WriteFile(tmp, jsonData, 0644)
	os.Rename(tmp, file)
}

func (e *MovieGoEngine) saveIMDBCache(file string, data map[string]IMDBCacheEntry) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	tmp := file + ".tmp"
	os.WriteFile(tmp, jsonData, 0644)
	os.Rename(tmp, file)
}

func (e *MovieGoEngine) createMKV(path, streamURL string, fileSize int64, magnet, imdbID string) bool {
	data := map[string]interface{}{
		"url":    streamURL,
		"size":   fileSize,
		"magnet": magnet,
		"imdb":   imdbID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false
	}
	return os.WriteFile(path, jsonData, 0644) == nil
}
