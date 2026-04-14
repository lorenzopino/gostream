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
	"gostream/internal/metadb"
	"gostream/internal/prowlarr"
	"gostream/internal/syncer/quality"
)

// TVGoEngine is the pure Go implementation of TV sync.
type TVGoEngine struct {
	gostorm   *GoStormClient
	tmdb      *tmdb.Client
	torrentio *torrentio.Client
	prowlarr  *prowlarr.Client
	plexURL   string
	plexToken string
	plexTVLib int
	tvDir     string
	stateDir  string
	limiter   *rate.Limiter
	logger    *log.Logger

	registry     map[string]TVEpisodeEntry
	registryFile string
	db           *metadb.DB // V1.7.1: Optional SQLite backend

	processedThisRun map[string]bool
	stats            TVSyncStats

	blacklist     BlacklistData
	blacklistFile string

	qualityProfile quality.TVProfile
	tmdbDiscovery  tmdb.EndpointConfig
}

// TVEpisodeEntry is a single entry in the TV episode registry.
type TVEpisodeEntry struct {
	QualityScore int    `json:"quality_score"`
	Hash         string `json:"hash"`
	FilePath     string `json:"file_path"`
	Source       string `json:"source"`
	Created      int64  `json:"created"`
}

// TVSyncStats tracks sync statistics.
type TVSyncStats struct {
	Shows           int `json:"shows"`
	EpisodesCreated int `json:"episodes_created"`
	EpisodesSkipped int `json:"episodes_skipped"`
	Upgrades        int `json:"upgrades"`
}

// TVEngineConfig holds config for the TV engine.
type TVEngineConfig struct {
	GoStormURL     string
	TMDBAPIKey     string
	TorrentioURL   string
	PlexURL        string
	PlexToken      string
	PlexTVLib      int
	TVDir          string
	StateDir       string
	LogsDir        string
	ProwlarrCfg    prowlarr.ConfigProwlarr
	QualityProfile quality.TVProfile
	TMDBDiscovery  tmdb.EndpointConfig
}

// TV thresholds
const (
	tv4KBase           = 1000
	tv1080pBase        = 200
	tvHDRBonus         = 100
	tvAtmosBonus       = 50
	tv51Bonus          = 25
	tvITABonus         = 40
	tvFullpackBonus    = 500
	tvMinSeeders4K     = 10
	tvMinSeeders       = 5
	tvMinEpisodeSize   = 214748364   // 0.2GB (200MB)
	tvMaxEpisodeSize   = 32212254720 // 30GB
	tvUpgradeThreshold = 1.2
	tvMinQualitySkip   = 1000
	tvSinglesLimit     = 15
	tvMaxShowAgeDays   = 180
)

var (
	reTV4K        = regexp.MustCompile(`(?i)2160p|4k|uhd`)
	reTV1080p     = regexp.MustCompile(`(?i)1080p`)
	reTV720p      = regexp.MustCompile(`(?i)\b720p\b`)
	reTV480p      = regexp.MustCompile(`(?i)\b480p\b|[^a-z]sd[^a-z]`)
	reTVHDR       = regexp.MustCompile(`(?i)\bhdr\b|hdr10\+?|\bdv\b|dovi|dolby.?vision`)
	reTVAtmos     = regexp.MustCompile(`atmos`)
	reTV51        = regexp.MustCompile(`(?i)5\.1|dd5|ddp5|dts|truehd`)
	reTVITA       = regexp.MustCompile(`(?i)ita|🇮🇹|multi|dual`)
	reTVExclLang  = regexp.MustCompile(`🇪🇸|🇫🇷|🇩🇪|🇷🇺|🇨🇳|🇯🇵|🇰🇷|🇹🇭|🇵🇹|🇧🇷|🇺🇦|🇵🇱|🇳🇱|🇹🇷|🇸🇦|🇮🇳|🇨🇿|🇭🇺|🇷🇴`)
	reTVSeeders   = regexp.MustCompile(`👤\s*(\d+)`)
	reTVSize      = regexp.MustCompile(`(?i)(\d+\.?\d*)\s*(GB|TB)`)
	reTVFullpack  = regexp.MustCompile(`(?i)\b(season|complete|full|pack)\b`)
	reTVRange     = regexp.MustCompile(`(?i)s\d+e\d+\s*-\s*e?\d+`)
	reTVMultiEp   = regexp.MustCompile(`(?i)s\d+e\d+`)
	reTVSeason    = regexp.MustCompile(`\.s\d{2}\.`)
	reTVSeasonP   = regexp.MustCompile(`\ss\d{2}\s*\(`)
	reTVSeasonN   = regexp.MustCompile(`[Ss](\d+)`)
	reTVEpRangeN  = regexp.MustCompile(`(?i)s\d+e(\d+)\s*-\s*e?(\d+)`)
	reTVSeasonR   = regexp.MustCompile(`\bs(\d{1,2})\s*[-–]\s*s(\d{1,2})\b`)
	reTVSeasonW   = regexp.MustCompile(`\bseasons?\s*(\d{1,2})\s*[-–]\s*(\d{1,2})\b`)
	reTVCompleteS = regexp.MustCompile(`(?i)\b(complete\s+series|all\s+seasons|full\s+series)\b`)
	reTVEpNum     = regexp.MustCompile(`[Ss](\d+)[Ee](\d+)`)
	reTV1xEp      = regexp.MustCompile(`(\d+)x(\d+)`)
	reTVFileName  = regexp.MustCompile(`(.+)_S(\d+)E(\d+)_([a-f0-9]{8})\.mkv$`)
	reTVNonWord   = regexp.MustCompile(`[^a-z0-9]`)
	reTVSanitize  = regexp.MustCompile(`[<>:"/\\|?*'"&]`)
	reTVSpaces    = regexp.MustCompile(`\s+`)
	reTVUnders    = regexp.MustCompile(`_+`)
	reTVYear      = regexp.MustCompile(`\(?(\d{4})\)?`)
	reTVQuality   = regexp.MustCompile(`\b(2160p|1080p|720p|480p|576p|4k|uhd|sd|hdr|dv|dovi|web|bluray|remux)\b.*`)
	reTVHashURL   = regexp.MustCompile(`link=([a-f0-9]{40})`)
)

var tvExcludedGenreIDs = map[int]bool{99: true, 10763: true, 10764: true, 10767: true, 16: true}

// NewTVGoEngine creates a new Go TV sync engine.
func NewTVGoEngine(cfg TVEngineConfig, db *metadb.DB) *TVGoEngine {
	var prowlarrClient *prowlarr.Client
	if cfg.ProwlarrCfg.Enabled {
		prowlarrClient = prowlarr.NewClient(cfg.ProwlarrCfg)
	}

	logPath := filepath.Join(cfg.LogsDir, "tv-sync.log")
	logFile, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	logger := log.New(io.MultiWriter(os.Stdout, logFile), "[TVSync] ", log.LstdFlags)

	regFile := filepath.Join(cfg.StateDir, "tv_episode_registry.json")
	blFile := filepath.Join(cfg.StateDir, "blacklist.json")

	e := &TVGoEngine{
		gostorm:          NewGoStormClient(cfg.GoStormURL),
		tmdb:             tmdb.NewClient(cfg.TMDBAPIKey),
		torrentio:        torrentio.NewClient(cfg.TorrentioURL, "sort=qualitysize|qualityfilter=scr,cam"),
		prowlarr:         prowlarrClient,
		plexURL:          cfg.PlexURL,
		plexToken:        cfg.PlexToken,
		plexTVLib:        cfg.PlexTVLib,
		tvDir:            cfg.TVDir,
		stateDir:         cfg.StateDir,
		limiter:          rate.NewLimiter(rate.Every(500*time.Millisecond), 1),
		logger:           logger,
		registryFile:     regFile,
		db:               db,
		processedThisRun: make(map[string]bool),
		blacklistFile:    blFile,
		qualityProfile:   cfg.QualityProfile,
		tmdbDiscovery:    cfg.TMDBDiscovery,
	}

	e.registry = e.loadRegistry()
	e.blacklist = e.loadBlacklist()

	return e
}

func (e *TVGoEngine) loadBlacklist() BlacklistData {
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

func (e *TVGoEngine) normalizeTitle(title string) string {
	t := strings.ToLower(title)
	t = reTVYear.ReplaceAllString(t, "")
	t = reTVQuality.ReplaceAllString(t, "")
	t = reTVNonWord.ReplaceAllString(t, "")
	return t
}

func (e *TVGoEngine) isBlacklisted(title string) bool {
	normalized := e.normalizeTitle(title)
	for _, bt := range e.blacklist.Titles {
		if bt == normalized {
			return true
		}
	}
	return false
}

func (e *TVGoEngine) isHashBlacklisted(hash string) bool {
	_, ok := e.blacklist.Hashes[strings.ToLower(hash)]
	return ok
}

func (e *TVGoEngine) Name() string { return "tv" }

func (e *TVGoEngine) Run(ctx context.Context) error {
	e.logger.Printf("Starting TV sync...")
	e.populateRegistryFromExisting()

	shows, err := e.discoverShows(ctx)
	if err != nil {
		e.logger.Printf("Discover error: %v", err)
		return fmt.Errorf("discover shows: %w", err)
	}
	if len(shows) == 0 {
		e.logger.Printf("No shows discovered")
		return nil
	}
	e.logger.Printf("Discovered %d shows", len(shows))

	for i, show := range shows {
		select {
		case <-ctx.Done():
			e.logger.Printf("Stopped after %d/%d shows (%d created)", i, len(shows), e.stats.EpisodesCreated)
			return ctx.Err()
		default:
		}

		e.logger.Printf("[%d/%d] %s", i+1, len(shows), show.Name)
		e.processShow(ctx, show)
	}

	e.saveRegistry()
	e.cleanupOrphanedFiles()
	e.cleanupOrphanedTorrents(ctx)
	e.rehydrateMissingTorrents(ctx)

	e.logger.Printf("TV sync complete: %d shows, %d episodes created, %d skipped, %d upgrades",
		e.stats.Shows, e.stats.EpisodesCreated, e.stats.EpisodesSkipped, e.stats.Upgrades)

	// Notify Plex
	if e.stats.EpisodesCreated > 0 && e.plexTVLib > 0 && e.plexURL != "" && e.plexToken != "" {
		url := fmt.Sprintf("%s/library/sections/%d/refresh?X-Plex-Token=%s", e.plexURL, e.plexTVLib, e.plexToken)
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		client := catalog.NewClient(10 * time.Second)
		resp, err := catalog.Do(context.Background(), client, req)
		if err == nil {
			resp.Body.Close()
		}
	}

	return nil
}

func (e *TVGoEngine) loadRegistry() map[string]TVEpisodeEntry {
	if e.db != nil {
		entries, err := e.db.AllEpisodes()
		if err != nil {
			e.logger.Printf("[TVSync] Warning: failed to load registry from DB: %v", err)
		} else {
			reg := make(map[string]TVEpisodeEntry)
			for _, entry := range entries {
				reg[entry.EpisodeKey] = TVEpisodeEntry{
					QualityScore: entry.QualityScore,
					Hash:         entry.Hash,
					FilePath:     entry.FilePath,
					Source:       entry.Source,
					Created:      entry.Created,
				}
			}
			e.logger.Printf("[TVSync] Loaded %d episodes from StateDB", len(reg))
			return reg
		}
	}
	data, err := os.ReadFile(e.registryFile)
	if err != nil {
		return make(map[string]TVEpisodeEntry)
	}
	var reg map[string]TVEpisodeEntry
	if err := json.Unmarshal(data, &reg); err != nil {
		return make(map[string]TVEpisodeEntry)
	}
	return reg
}

func (e *TVGoEngine) saveRegistry() {
	data, err := json.MarshalIndent(e.registry, "", "  ")
	if err != nil {
		return
	}
	tmp := e.registryFile + ".tmp"
	os.WriteFile(tmp, data, 0644)
	os.Rename(tmp, e.registryFile)
}

func (e *TVGoEngine) populateRegistryFromExisting() {
	if _, err := os.Stat(e.tvDir); err != nil {
		return
	}

	var torrents []TorrentStats
	var tsLoaded bool

	filepath.Walk(e.tvDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(strings.ToLower(path), ".mkv") {
			return nil
		}

		if _, ok := e.registryByPath(path); ok {
			return nil
		}

		filename := filepath.Base(path)
		m := reTVFileName.FindStringSubmatch(filename)
		if len(m) < 5 {
			return nil
		}

		showName := m[1]
		season, _ := strconv.Atoi(m[2])
		episode, _ := strconv.Atoi(m[3])
		hash8 := m[4]
		key := e.episodeKey(showName, "", season, episode)

		if _, exists := e.registry[key]; exists {
			return nil
		}

		// Try to read full hash from file content first
		fullHash := e.readHashFromMKV(path)
		if fullHash == "" {
			// Fallback: resolve via GoStorm lookup
			if !tsLoaded {
				torrents, _ = e.gostorm.ListTorrents(context.Background())
				tsLoaded = true
			}
			for _, t := range torrents {
				if strings.HasPrefix(t.Hash, hash8) {
					fullHash = t.Hash
					break
				}
			}
			if fullHash == "" {
				fullHash = hash8
			}
		}

		e.registry[key] = TVEpisodeEntry{
			QualityScore: 1,
			Hash:         fullHash,
			FilePath:     path,
			Source:       "existing",
			Created:      info.ModTime().Unix(),
		}

		return nil
	})
}

func (e *TVGoEngine) readHashFromMKV(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 10240 {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "{") {
		return ""
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(content), &obj); err != nil {
		return ""
	}
	url, _ := obj["url"].(string)
	m := reTVHashURL.FindStringSubmatch(url)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func (e *TVGoEngine) registryByPath(path string) (string, bool) {
	for key, entry := range e.registry {
		if entry.FilePath == path {
			return key, true
		}
	}
	return "", false
}

func (e *TVGoEngine) episodeKey(show string, firstAirDate string, season, episode int) string {
	normalized := reTVNonWord.ReplaceAllString(strings.ToLower(show), "")
	year := ""
	if len(firstAirDate) >= 4 {
		year = firstAirDate[:4]
	}
	if year != "" {
		return fmt.Sprintf("%s_%s_s%02de%02d", normalized, year, season, episode)
	}
	return fmt.Sprintf("%s_s%02de%02d", normalized, season, episode)
}

func (e *TVGoEngine) registerEpisode(key string, score int, hash, path, source string) {
	created := time.Now().Unix()
	e.registry[key] = TVEpisodeEntry{
		QualityScore: score,
		Hash:         hash,
		FilePath:     path,
		Source:       source,
		Created:      created,
	}
	if e.db != nil {
		if err := e.db.UpsertEpisode(key, metadb.EpisodeEntry{
			EpisodeKey:   key,
			QualityScore: score,
			Hash:         hash,
			FilePath:     path,
			Source:       source,
			Created:      created,
		}); err != nil {
			e.logger.Printf("[TVSync] Warning: failed to save episode to DB: %v", err)
		}
	} else {
		e.saveRegistry()
	}
}

func (e *TVGoEngine) discoverShows(ctx context.Context) ([]tmdb.TVShow, error) {
	if len(e.tmdbDiscovery.Endpoints) > 0 {
		return e.tmdb.DiscoverTVFromConfig(ctx, e.tmdbDiscovery)
	}
	return e.discoverShowsHardcoded(ctx)
}

// discoverShowsHardcoded is the original hardcoded TMDB discovery logic.
func (e *TVGoEngine) discoverShowsHardcoded(ctx context.Context) ([]tmdb.TVShow, error) {
	cutoff := time.Now().AddDate(0, 0, -tvMaxShowAgeDays).Format("2006-01-02")

	var all []tmdb.TVShow
	seen := make(map[int]bool)

	endpoints := []struct {
		fn    func(context.Context, int) ([]tmdb.TVShow, error)
		pages int
	}{
		{e.tmdb.TVOnTheAir, 3},
		{e.tmdb.TVAiringToday, 2},
		{e.tmdb.TVTrending, 3},
	}

	for _, ep := range endpoints {
		shows, err := ep.fn(ctx, ep.pages)
		if err != nil {
			continue
		}
		for _, s := range shows {
			if !seen[s.ID] && e.passesShowFilters(s) {
				seen[s.ID] = true
				all = append(all, s)
			}
		}
	}

	// Discover English recent shows
	discShows, err := e.tmdb.DiscoverTV(ctx, "en", cutoff, "", 5)
	if err == nil {
		for _, s := range discShows {
			if !seen[s.ID] && e.passesShowFilters(s) {
				seen[s.ID] = true
				all = append(all, s)
			}
		}
	}

	return all, nil
}

func (e *TVGoEngine) passesShowFilters(show tmdb.TVShow) bool {
	// Genre filter
	for _, gid := range show.GenreIDs {
		if tvExcludedGenreIDs[gid] {
			return false
		}
	}

	// Language: English always accepted, others need premium IT provider
	if show.Language == "en" {
		return true
	}

	details, err := e.tmdb.TVDetails(context.Background(), show.ID)
	if err != nil {
		return false
	}

	return tmdb.HasPremiumProvider(details.WatchProviders)
}

func (e *TVGoEngine) processShow(ctx context.Context, show tmdb.TVShow) {
	showName := show.Name
	if showName == "" {
		showName = show.OriginalName
	}
	if showName == "" {
		return
	}

	// Blacklist check at show level
	if e.isBlacklisted(showName) {
		e.logger.Printf("🚫 Blacklist: skipping show '%s'", showName)
		return
	}

	t0 := time.Now()
	imdbID, err := e.tmdb.TVExternalIDs(ctx, show.ID)
	if err != nil || imdbID == "" {
		return
	}

	details, err := e.tmdb.TVDetails(ctx, show.ID)
	if err != nil {
		return
	}
	e.logger.Printf("  TMDB lookups: %v", time.Since(t0).Round(time.Millisecond))

	// Check complete seasons
	completeSeasons := e.getCompleteSeasons(showName, details)
	skippedSeasons := make(map[int]bool)
	allComplete := true
	for sn, avgScore := range completeSeasons {
		if avgScore >= tvMinQualitySkip {
			skippedSeasons[sn] = true
		} else {
			allComplete = false
		}
	}

	// If ALL seasons are complete, skip entire show immediately
	if allComplete && len(completeSeasons) > 0 {
		e.logger.Printf("Skipping '%s' — all %d seasons complete", showName, len(completeSeasons))
		return
	}

	// Get streams
	t1 := time.Now()
	streams := e.getStreams(ctx, imdbID, show.ID, showName, details)
	e.logger.Printf("  getStreams: %v (%d streams)", time.Since(t1).Round(time.Millisecond), len(streams))
	if len(streams) == 0 {
		return
	}

	sort.Slice(streams, func(i, j int) bool {
		return streams[i].Rank.BetterThan(streams[j].Rank)
	})

	// V430: Save top 20 filtered streams as Torrent Alternatives Cache (TAC)
	e.saveTVAlternatives(imdbID, showName, details.FirstAirDate, streams)

	created := 0
	seasonsComplete := make(map[int]bool)
	seasonsEpCount := make(map[int]int)

	tmdbSeasonEps := make(map[int]int)
	for _, sd := range details.Seasons {
		if sd.SeasonNumber > 0 && sd.EpisodeCount > 0 {
			tmdbSeasonEps[sd.SeasonNumber] = sd.EpisodeCount
		}
	}

	// Process fullpacks first
	fpCount := 0
	for _, s := range streams {
		if s.IsFullpack {
			fpCount++
		}
	}
	e.logger.Printf("  %d fullpacks, %d singles, skippedSeasons=%v", fpCount, len(streams)-fpCount, skippedSeasons)

	for _, stream := range streams {
		if !stream.IsFullpack {
			continue
		}
		if skippedSeasons[stream.Season] {
			continue
		}
		if seasonsComplete[stream.Season] {
			continue
		}

		t2 := time.Now()
		count := e.processFullpack(ctx, showName, stream, show.FirstAirDate)
		e.logger.Printf("    fullpack S%02d: %d created in %v (%s)", stream.Season, count, time.Since(t2).Round(time.Millisecond), stream.Title[:min(60, len(stream.Title))])
		if count > 0 {
			created += count
			seasonsEpCount[stream.Season] += count
			expected := tmdbSeasonEps[stream.Season]
			total := seasonsEpCount[stream.Season]
			if expected > 0 && total >= expected {
				seasonsComplete[stream.Season] = true
			} else if !stream.IsPartialPack && expected == 0 && count >= 5 {
				seasonsComplete[stream.Season] = true
			}
		}
	}

	// Process singles
	singlesProcessed := 0
	for _, stream := range streams {
		if stream.IsFullpack {
			continue
		}
		if singlesProcessed >= tvSinglesLimit {
			break
		}
		if skippedSeasons[stream.Season] {
			continue
		}
		if seasonsComplete[stream.Season] {
			continue
		}

		count := e.processSingle(ctx, showName, stream, show.FirstAirDate)
		created += count
		singlesProcessed++
	}

	if created > 0 {
		e.stats.Shows++
		e.stats.EpisodesCreated += created
	}
}

type TVStream struct {
	Title         string
	Hash          string
	IsFullpack    bool
	IsPartialPack bool
	Is1080p       bool
	Is720p        bool
	Is480p        bool
	QualityScore  int
	Season        int
	Seeders       int
	SizeGB        float64
	Priority      int
	Rank          quality.StreamingRank
}

func (e *TVGoEngine) getStreams(ctx context.Context, imdbID string, tmdbID int, showName string, details *tmdb.TVDetail) []TVStream {
	numSeasons := details.NumberOfSeasons
	if numSeasons == 0 {
		numSeasons = 5
	}

	maxSeasons := 2
	startSeason := numSeasons - maxSeasons + 1
	if startSeason < 1 {
		startSeason = 1
	}
	endSeason := numSeasons

	var allStreams []prowlarr.Stream
	seenHashes := make(map[string]bool)

	// Prowlarr primary
	if e.prowlarr != nil {
		tp := time.Now()
		streams := e.prowlarr.FetchTorrents(imdbID, "series", showName, nil)
		for _, s := range streams {
			h := strings.ToLower(s.InfoHash)
			if h != "" && !seenHashes[h] {
				seenHashes[h] = true
				allStreams = append(allStreams, s)
			}
		}
		e.logger.Printf("    Prowlarr: %d streams in %v", len(allStreams), time.Since(tp).Round(time.Millisecond))
	}

	// Torrentio fallback
	if len(allStreams) == 0 {
		seasonEps := make(map[int]int)
		for _, sd := range details.Seasons {
			if sd.SeasonNumber > 0 {
				seasonEps[sd.SeasonNumber] = sd.EpisodeCount
			}
		}

		tt := time.Now()
		epsFetched := 0
		for season := startSeason; season <= endSeason; season++ {
			epCount := seasonEps[season]
			if epCount == 0 {
				epCount = 10
			}
			for ep := 1; ep <= epCount; ep++ {
				tioStreams, err := e.torrentio.FetchEpisodeStreams(ctx, imdbID, season, ep)
				if err != nil {
					continue
				}
				epsFetched++
				for _, s := range tioStreams {
					if s.InfoHash != "" && !seenHashes[s.InfoHash] {
						seenHashes[s.InfoHash] = true
						allStreams = append(allStreams, prowlarr.Stream{
							Name:     s.Name,
							Title:    s.Title,
							InfoHash: s.InfoHash,
						})
					}
				}
				e.limiter.Wait(ctx)
			}
		}
		e.logger.Printf("    Torrentio fallback: %d streams from %d eps in %v", len(allStreams), epsFetched, time.Since(tt).Round(time.Millisecond))
	}

	// Classify
	var classified []TVStream
	for _, s := range allStreams {
		c := e.classifyStream(s)
		if c == nil {
			continue
		}
		if c.Season < startSeason || c.Season > endSeason {
			continue
		}
		span := e.extractSeasonSpan(c.Title)
		if span != nil && span[0] < startSeason {
			continue
		}
		classified = append(classified, *c)
	}

	return classified
}

func (e *TVGoEngine) classifyStream(s prowlarr.Stream) *TVStream {
	title := s.Title
	name := s.Name
	fullText := title + " " + name

	// Hash blacklist check
	if e.isHashBlacklisted(s.InfoHash) {
		return nil
	}

	// Title blacklist check
	if e.isBlacklisted(title) {
		return nil
	}

	if reTVExclLang.MatchString(title) {
		return nil
	}

	seeders := e.extractSeeders(title)

	is4K := reTV4K.MatchString(fullText)
	is1080p := reTV1080p.MatchString(fullText) && !reTV720p.MatchString(fullText)
	is720p := reTV720p.MatchString(fullText) && !is1080p && !is4K
	is480p := reTV480p.MatchString(fullText) && !is720p && !is1080p && !is4K
	resolution := quality.DetectResolution(fullText)
	if resolution == quality.ResolutionUnknown {
		return nil
	}

	sizeGB := s.SizeGB
	if sizeGB == 0 {
		sizeGB = e.extractSizeGB(title)
	}

	// Check if this is a fullpack BEFORE size filtering
	isFullpack := e.isFullpack(title)
	estimatedEpisodes := 0
	if isFullpack {
		estimatedEpisodes = e.estimateEpisodeCount(title)
	}
	rank, ok := quality.RankStreamingCandidate(quality.StreamingCandidate{
		Hash:                  s.InfoHash,
		Title:                 fullText,
		MediaType:             quality.MediaTV,
		Resolution:            resolution,
		SizeGB:                sizeGB,
		Seeders:               seeders,
		IsPack:                isFullpack,
		EstimatedEpisodeCount: estimatedEpisodes,
	}, quality.TVStreamingPolicy())
	if !ok {
		return nil
	}

	season := e.extractSeason(title)
	isPartialPack := false
	if isFullpack {
		isPartialPack = reTVRange.MatchString(strings.Split(title, "\n")[0])
	}

	return &TVStream{
		Title:         title,
		Hash:          strings.ToLower(s.InfoHash),
		IsFullpack:    isFullpack,
		IsPartialPack: isPartialPack,
		Is1080p:       is1080p,
		Is720p:        is720p,
		Is480p:        is480p,
		QualityScore:  rank.Score,
		Season:        season,
		Seeders:       seeders,
		SizeGB:        sizeGB,
		Priority:      rank.Score,
		Rank:          rank,
	}
}

func (e *TVGoEngine) calculateQualityScore(text string, seeders int, is4K, is1080p, is720p, is480p bool, ceilingGB, sizeGB float64) int {
	w := e.qualityProfile.ScoreWeights
	t := strings.ToLower(text)
	score := 0

	// Resolution score
	if is4K && w.Resolution4K != nil {
		score += *w.Resolution4K
	} else if is1080p && w.Resolution1080p != nil {
		score += *w.Resolution1080p
	} else if is720p && w.Resolution720p != nil {
		score += *w.Resolution720p
	} else if is480p && w.Resolution480p != nil {
		score += *w.Resolution480p
	} else {
		return 0 // No recognized resolution
	}

	// HDR / Dolby Vision
	if reTVHDR.MatchString(t) && w.HDR != nil {
		score += *w.HDR
	}

	// Audio
	if reTVAtmos.MatchString(t) && w.Atmos != nil {
		score += *w.Atmos
	} else if reTV51.MatchString(t) && w.Audio51 != nil {
		score += *w.Audio51
	}

	// Italian language
	if reTVITA.MatchString(t) && w.ITA != nil {
		score += *w.ITA
	}

	// Seeder bonus tiers
	if seeders >= 100 && w.Seeder100Bonus != nil {
		score += *w.Seeder100Bonus
	} else if seeders >= 50 && w.Seeder50Bonus != nil {
		score += *w.Seeder50Bonus
	} else if seeders >= 20 && w.Seeder20Bonus != nil {
		score += *w.Seeder20Bonus
	}

	// Size bonus: +N points per GB under ceiling (uses actual size from Prowlarr)
	if sizeGB > 0 && ceilingGB > 0 && w.SizeBonusPerGBUnder != nil {
		underGB := ceilingGB - sizeGB
		if underGB > 0 {
			score += int(underGB) * (*w.SizeBonusPerGBUnder)
		}
	}

	return score
}

func (e *TVGoEngine) isFullpack(title string) bool {
	firstLine := strings.Split(title, "\n")[0]
	t := strings.ToLower(firstLine)

	if reTVFullpack.MatchString(t) {
		return true
	}
	if reTVRange.MatchString(t) {
		return true
	}
	if len(reTVMultiEp.FindAllString(t, -1)) >= 2 {
		return true
	}
	if reTVSeason.MatchString(t) && !reTVMultiEp.MatchString(t) {
		return true
	}
	if reTVSeasonP.MatchString(t) && !reTVMultiEp.MatchString(t) {
		return true
	}
	return false
}

func (e *TVGoEngine) estimateEpisodeCount(title string) int {
	firstLine := strings.Split(title, "\n")[0]
	if m := reTVEpRangeN.FindStringSubmatch(firstLine); len(m) >= 3 {
		first, _ := strconv.Atoi(m[1])
		last, _ := strconv.Atoi(m[2])
		if last >= first {
			return last - first + 1
		}
	}
	if matches := reTVMultiEp.FindAllString(firstLine, -1); len(matches) > 1 {
		return len(matches)
	}
	return 10
}

func (e *TVGoEngine) filterPackVideoFilesByStreamingPolicy(files []FileStat, stream TVStream) []FileStat {
	var valid []FileStat
	for _, f := range files {
		if !e.isVideoFile(f.Path) {
			continue
		}
		if f.Length < tvMinEpisodeSize {
			continue
		}
		resolution := quality.DetectResolution(stream.Title + " " + f.Path)
		if resolution == quality.ResolutionUnknown {
			if stream.Is480p {
				resolution = quality.Resolution480p
			} else if stream.Is720p {
				resolution = quality.Resolution720p
			} else if stream.Is1080p {
				resolution = quality.Resolution1080p
			} else {
				resolution = stream.Rank.Resolution
			}
		}
		sizeGB := float64(f.Length) / 1024 / 1024 / 1024
		_, ok := quality.RankExactStreamingFile(quality.StreamingCandidate{
			Hash:       stream.Hash,
			Title:      stream.Title + " " + f.Path,
			MediaType:  quality.MediaTV,
			Resolution: resolution,
			SizeGB:     sizeGB,
			Seeders:    stream.Seeders,
		}, quality.TVStreamingPolicy())
		if ok {
			valid = append(valid, f)
		}
	}
	return valid
}

func (e *TVGoEngine) extractSeason(title string) int {
	m := reTVSeasonN.FindStringSubmatch(title)
	if len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 1
}

func (e *TVGoEngine) extractSeasonSpan(title string) *[2]int {
	firstLine := strings.ToLower(strings.Split(title, "\n")[0])

	m := reTVSeasonR.FindStringSubmatch(firstLine)
	if len(m) > 2 {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a > b {
			a, b = b, a
		}
		return &[2]int{a, b}
	}

	m = reTVSeasonW.FindStringSubmatch(firstLine)
	if len(m) > 2 {
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a > b {
			a, b = b, a
		}
		return &[2]int{a, b}
	}

	if reTVCompleteS.MatchString(firstLine) {
		return &[2]int{1, 99}
	}

	return nil
}

func (e *TVGoEngine) extractSeeders(title string) int {
	m := reTVSeeders.FindStringSubmatch(title)
	if len(m) > 1 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func (e *TVGoEngine) extractSizeGB(title string) float64 {
	m := reTVSize.FindStringSubmatch(title)
	if len(m) >= 3 {
		v, _ := strconv.ParseFloat(m[1], 64)
		if strings.EqualFold(m[2], "TB") {
			return v * 1024
		}
		return v
	}
	return 0
}

func (e *TVGoEngine) processFullpack(ctx context.Context, showName string, stream TVStream, firstAirDate string) int {
	magnet := BuildMagnet(stream.Hash, stream.Title, DefaultTrackers())
	hash, err := e.gostorm.AddTorrent(ctx, magnet, stream.Title)
	if err != nil || hash == "" {
		return 0
	}

	info, err := e.gostorm.GetTorrentInfo(ctx, hash, 90)
	if err != nil {
		e.gostorm.RemoveTorrent(ctx, hash)
		return 0
	}

	videoFiles := e.filterPackVideoFilesByStreamingPolicy(info.FileStats, stream)
	if len(videoFiles) == 0 {
		e.gostorm.RemoveTorrent(ctx, hash)
		return 0
	}

	created := 0
	cleanShow := e.getShowFolderName(showName, firstAirDate)

	for _, vf := range videoFiles {
		filename := filepath.Base(vf.Path)
		epInfo := e.extractEpisodeFromFilename(filename)
		if epInfo[0] == 0 && epInfo[1] == 0 {
			continue
		}

		season, episode := epInfo[0], epInfo[1]
		key := e.episodeKey(showName, firstAirDate, season, episode)

		if e.processedThisRun[key] {
			continue
		}

		if existing, ok := e.registry[key]; ok {
			if float64(stream.QualityScore) <= float64(existing.QualityScore)*tvUpgradeThreshold {
				e.stats.EpisodesSkipped++
				e.processedThisRun[key] = true
				continue
			}
		}

		seasonDir := filepath.Join(e.tvDir, cleanShow, fmt.Sprintf("Season.%02d", season))
		epFilename := e.buildFilename(showName, season, episode, hash[:8])
		epPath := filepath.Join(seasonDir, epFilename)
		streamURL := fmt.Sprintf("%s/stream?link=%s&index=%d&play", e.gostorm.baseURL, hash, vf.ID)

		if e.createMKV(epPath, streamURL, vf.Length, magnet) {
			// Only remove old file AFTER confirming new one was created (prevents race condition)
			if existing, ok := e.registry[key]; ok && existing.FilePath != "" && existing.FilePath != epPath {
				os.Remove(existing.FilePath)
				e.stats.Upgrades++
			}
			e.registerEpisode(key, stream.QualityScore, hash, epPath, "fullpack")
			e.processedThisRun[key] = true
			created++
			e.logger.Printf("Created: %s (%.1fGB, score:%d)", epFilename, float64(vf.Length)/1024/1024/1024, stream.QualityScore)
		}
	}

	if created == 0 {
		e.gostorm.RemoveTorrent(ctx, hash)
	}

	return created
}

func (e *TVGoEngine) processSingle(ctx context.Context, showName string, stream TVStream, firstAirDate string) int {
	title := stream.Title
	m := reTVEpNum.FindStringSubmatch(title)
	if len(m) < 3 {
		return 0
	}
	episode, _ := strconv.Atoi(m[2])
	season := stream.Season

	key := e.episodeKey(showName, firstAirDate, season, episode)

	if e.processedThisRun[key] {
		return 0
	}

	if existing, ok := e.registry[key]; ok {
		if float64(stream.QualityScore) <= float64(existing.QualityScore)*tvUpgradeThreshold {
			e.stats.EpisodesSkipped++
			e.processedThisRun[key] = true
			return 0
		}
	}

	magnet := BuildMagnet(stream.Hash, title, DefaultTrackers())
	hash, err := e.gostorm.AddTorrent(ctx, magnet, title)
	if err != nil || hash == "" {
		return 0
	}

	info, err := e.gostorm.GetTorrentInfo(ctx, hash, 45)
	if err != nil {
		e.gostorm.RemoveTorrent(ctx, hash)
		return 0
	}

	var bestFile *FileStat
	for i := range info.FileStats {
		f := &info.FileStats[i]
		if e.isVideoFile(f.Path) && f.Length >= tvMinEpisodeSize {
			resolution := quality.DetectResolution(stream.Title + " " + f.Path)
			if resolution == quality.ResolutionUnknown {
				resolution = stream.Rank.Resolution
			}
			if _, ok := quality.RankExactStreamingFile(quality.StreamingCandidate{
				Hash:       stream.Hash,
				Title:      stream.Title + " " + f.Path,
				MediaType:  quality.MediaTV,
				Resolution: resolution,
				SizeGB:     float64(f.Length) / 1024 / 1024 / 1024,
				Seeders:    stream.Seeders,
			}, quality.TVStreamingPolicy()); !ok {
				continue
			}
			if bestFile == nil || f.Length > bestFile.Length {
				cp := *f
				bestFile = &cp
			}
		}
	}
	if bestFile == nil {
		e.gostorm.RemoveTorrent(ctx, hash)
		return 0
	}

	cleanShow := e.getShowFolderName(showName, firstAirDate)
	seasonDir := filepath.Join(e.tvDir, cleanShow, fmt.Sprintf("Season.%02d", season))
	epFilename := e.buildFilename(showName, season, episode, hash[:8])
	epPath := filepath.Join(seasonDir, epFilename)
	streamURL := fmt.Sprintf("%s/stream?link=%s&index=%d&play", e.gostorm.baseURL, hash, bestFile.ID)

	if e.createMKV(epPath, streamURL, bestFile.Length, magnet) {
		// Only remove old file AFTER confirming new one was created (prevents race condition)
		if existing, ok := e.registry[key]; ok && existing.FilePath != "" && existing.FilePath != epPath {
			os.Remove(existing.FilePath)
			e.stats.Upgrades++
		}
		e.registerEpisode(key, stream.QualityScore, hash, epPath, "single")
		e.processedThisRun[key] = true
		e.logger.Printf("Created: %s (%.1fGB, score:%d)", epFilename, float64(bestFile.Length)/1024/1024/1024, stream.QualityScore)
		return 1
	}

	return 0
}

func (e *TVGoEngine) getCompleteSeasons(showName string, details *tmdb.TVDetail) map[int]float64 {
	normalized := reTVNonWord.ReplaceAllString(strings.ToLower(showName), "")

	seasonEps := make(map[int]int)
	for _, sd := range details.Seasons {
		if sd.SeasonNumber > 0 && sd.EpisodeCount > 0 {
			seasonEps[sd.SeasonNumber] = sd.EpisodeCount
		}
	}

	complete := make(map[int]float64)
	for sn, expected := range seasonEps {
		var scores []int
		for key, entry := range e.registry {
			if strings.HasPrefix(key, normalized) && strings.Contains(key, fmt.Sprintf("_s%02de", sn)) {
				scores = append(scores, entry.QualityScore)
			}
		}
		if len(scores) >= expected {
			sum := 0
			for _, s := range scores {
				sum += s
			}
			complete[sn] = float64(sum) / float64(len(scores))
		}
	}

	return complete
}

func (e *TVGoEngine) cleanupOrphanedFiles() {
	if _, err := os.Stat(e.tvDir); err != nil {
		return
	}

	regPaths := make(map[string]bool)
	for _, entry := range e.registry {
		regPaths[entry.FilePath] = true
	}

	// First pass: remove orphaned MKV files
	filepath.Walk(e.tvDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(strings.ToLower(path), ".mkv") {
			return nil
		}
		if !regPaths[path] {
			os.Remove(path)
		}
		return nil
	})

	// Second pass: remove empty season and show directories
	// Walk in reverse order (deepest first) to clean up properly
	var dirs []string
	filepath.Walk(e.tvDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	// Reverse iterate (deepest first)
	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		// Skip the root tvDir
		if dir == e.tvDir {
			continue
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) == 0 {
			os.Remove(dir)
		}
	}
}

func (e *TVGoEngine) rehydrateMissingTorrents(ctx context.Context) {
	torrents, err := e.gostorm.ListTorrents(ctx)
	if err != nil {
		return
	}
	activeHashes := make(map[string]bool)
	for _, t := range torrents {
		activeHashes[t.Hash] = true
	}

	if _, err := os.Stat(e.tvDir); err != nil {
		return
	}

	rehydrated := 0
	e.logger.Printf("Scanning for missing torrents to rehydrate...")

	filepath.Walk(e.tvDir, func(path string, info os.FileInfo, err error) error {
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

		m := reTVHashURL.FindStringSubmatch(url)
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
			e.logger.Printf("Rehydrating #%d: %s...", rehydrated+1, info.Name())
			if _, err := e.gostorm.AddTorrent(ctx, freshMagnet, displayTitle); err == nil {
				e.createMKV(path, url, int64(size), freshMagnet)
				rehydrated++
				activeHashes[hash] = true
				time.Sleep(5 * time.Second)
			}
		}

		return nil
	})

	if rehydrated > 0 {
		e.logger.Printf("Rehydrated %d missing torrents", rehydrated)
	} else {
		e.logger.Printf("No torrents needed rehydration")
	}
}

func (e *TVGoEngine) cleanupOrphanedTorrents(ctx context.Context) {
	torrents, err := e.gostorm.ListTorrents(ctx)
	if err != nil {
		return
	}

	registryHashes := make(map[string]bool)
	for _, entry := range e.registry {
		registryHashes[strings.ToLower(entry.Hash)] = true
	}

	// Also collect hashes from disk files
	diskHashes := make(map[string]bool)
	filepath.Walk(e.tvDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(strings.ToLower(path), ".mkv") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := strings.TrimSpace(string(data))
		if strings.HasPrefix(content, "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal(data, &obj); err != nil {
				return nil
			}
			url, _ := obj["url"].(string)
			m := reTVHashURL.FindStringSubmatch(url)
			if len(m) >= 2 {
				diskHashes[strings.ToLower(m[1])] = true
			}
		} else {
			lines := strings.SplitN(content, "\n", 2)
			if len(lines) >= 1 {
				m := reTVHashURL.FindStringSubmatch(lines[0])
				if len(m) >= 2 {
					diskHashes[strings.ToLower(m[1])] = true
				}
			}
		}
		return nil
	})

	reTVSeries := regexp.MustCompile(`(?i)s\d+e\d+|season|episode`)
	removed := 0

	for _, t := range torrents {
		h := strings.ToLower(t.Hash)
		if h == "" {
			continue
		}
		if !reTVSeries.MatchString(t.Title) {
			continue
		}
		if registryHashes[h] || diskHashes[h] {
			continue
		}
		if e.gostorm.RemoveTorrent(ctx, h) == nil {
			removed++
			e.logger.Printf("Removed orphaned torrent: %s...", h[:8])
		}
	}

	if removed > 0 {
		e.logger.Printf("Removed %d orphaned torrents", removed)
	}
}

func (e *TVGoEngine) isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mkv" || ext == ".mp4" || ext == ".avi" || ext == ".mov" || ext == ".m4v"
}

func (e *TVGoEngine) extractEpisodeFromFilename(filename string) [2]int {
	m := reTVEpNum.FindStringSubmatch(filename)
	if len(m) >= 3 {
		s, _ := strconv.Atoi(m[1])
		ep, _ := strconv.Atoi(m[2])
		return [2]int{s, ep}
	}
	m = reTV1xEp.FindStringSubmatch(filename)
	if len(m) >= 3 {
		s, _ := strconv.Atoi(m[1])
		ep, _ := strconv.Atoi(m[2])
		return [2]int{s, ep}
	}
	return [2]int{0, 0}
}

func (e *TVGoEngine) sanitizeName(name string) string {
	clean := reTVSanitize.ReplaceAllString(name, "")
	clean = reTVSpaces.ReplaceAllString(clean, "_")
	clean = reTVUnders.ReplaceAllString(clean, "_")
	return strings.Trim(clean, "_")
}

func (e *TVGoEngine) getShowFolderName(showName, firstAirDate string) string {
	cleanName := e.sanitizeName(showName)
	year := ""
	if len(firstAirDate) >= 4 {
		year = firstAirDate[:4]
	}
	if year != "" {
		return fmt.Sprintf("%s (%s)", cleanName, year)
	}
	return cleanName
}

func (e *TVGoEngine) buildFilename(show string, season, episode int, hash8 string) string {
	cleanShow := e.sanitizeName(show)
	return fmt.Sprintf("%s_S%02dE%02d_%s.mkv", cleanShow, season, episode, hash8)
}

func (e *TVGoEngine) createMKV(path, streamURL string, fileSize int64, magnet string) bool {
	data := map[string]interface{}{
		"url":    streamURL,
		"size":   fileSize,
		"magnet": magnet,
		"imdb":   "",
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

// saveTVAlternatives saves the top 20 filtered streams as Torrent Alternatives Cache entries.
// Called after streams are classified and sorted in processShow.
func (e *TVGoEngine) saveTVAlternatives(imdbID, showName, firstAirDate string, streams []TVStream) {
	if e.db == nil {
		return // No SQLite backend
	}
	if len(streams) == 0 {
		return
	}

	// Cap at 20 alternatives
	max := 20
	if len(streams) < max {
		max = len(streams)
	}

	alts := make([]metadb.TorrentAlternative, 0, max)
	for i := 0; i < max; i++ {
		s := &streams[i]
		status := "active"
		if i == 0 {
			status = "active" // Rank 0 is the primary candidate
		}
		alts = append(alts, metadb.TorrentAlternative{
			ContentID:        imdbID,
			ContentType:      "tv",
			Rank:             i + 1,
			Hash:             s.Hash,
			Title:            s.Title,
			Size:             int64(s.SizeGB * 1024 * 1024 * 1024),
			Seeders:          s.Seeders,
			QualityScore:     s.QualityScore,
			Status:           status,
			LastHealthCheck:  time.Now().Unix(),
			AvgSpeedKBps:     0,
			ReplacementCount: 0,
		})
	}

	if err := e.db.SaveAlternativesForContent(imdbID, alts); err != nil {
		e.logger.Printf("[V430] Warning: failed to save TV alternatives for %s: %v", showName, err)
	} else {
		e.logger.Printf("[V430] Saved %d torrent alternatives for %s", len(alts), showName)
	}
}
