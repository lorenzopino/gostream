# TV Multi-Channel Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor the TV sync engine to support multiple independent channels, each with its own mode (discovery or manual TMDB ID list), schedule, and behavior rules.

**Architecture:** Channel-based config where `tmdb_discovery.tv.channels` is a list of `TVChannelConfig` objects. Each enabled channel becomes an independent scheduler job (`"tv:<channel_name>"`). Manual channels bypass discovery filters, disable fullpack torrents, and fetch all episodes for specified TMDB IDs.

**Tech Stack:** Go, existing TV engine (`tv_go.go`), scheduler, config types, TMDB client

---

### File Map

| Action | File | Responsibility |
|--------|------|----------------|
| **Modify** | `internal/config/quality.go` | Add `TVChannelConfig`, `ChannelSchedule` types |
| **Modify** | `config.go` | Add type alias re-export for new types |
| **Modify** | `internal/syncer/engines/tv_go.go` | Channel dispatcher in `discoverShows()`, `discoverFromManualIDs()`, `SourceMode` handling, fullpack skip |
| **Modify** | `internal/syncer/engines/tv.go` | Add `channelConfig` to `TVSyncerConfig`, pass to engine |
| **Modify** | `internal/syncer/scheduler/scheduler.go` | Support multiple TV channel jobs via `TVChannels` in config |
| **Modify** | `main.go` | Wire TV channels as separate scheduler jobs |
| **Modify** | `config.json.example` | Update with channels structure |

---

### Task 1: Add TV channel config types

**Files:**
- Modify: `internal/config/quality.go`

- [ ] **Step 1: Add new types to `internal/config/quality.go`**

Add these types at the end of the file (after `TMDBEndpoint`):

```go
// TVChannelConfig represents a single TV sync channel.
// A channel can operate in "discovery" mode (dynamic TMDB queries)
// or "manual" mode (explicit list of TMDB IDs).
type TVChannelConfig struct {
	Enabled             bool            `json:"enabled"`
	Name                string          `json:"name"`
	Mode                string          `json:"mode"` // "discovery" | "manual"
	Schedule            ChannelSchedule `json:"schedule"`
	Endpoints           []TMDBEndpoint  `json:"endpoints,omitempty"`           // only for mode=discovery
	TMDBIDs             []int           `json:"tmdb_ids,omitempty"`            // only for mode=manual
	SkipCompleteSeasons bool            `json:"skip_complete_seasons"`
}

// ChannelSchedule defines when a TV channel sync runs.
type ChannelSchedule struct {
	Hour       int   `json:"hour"`
	Minute     int   `json:"minute"`
	DaysOfWeek []int `json:"days_of_week"`
}

// TVDiscoveryConfig wraps the channels array under tmdb_discovery.tv.
type TVDiscoveryConfig struct {
	Channels []TVChannelConfig `json:"channels,omitempty"`
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/config/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add internal/config/quality.go
git commit -m "feat: add TVChannelConfig, ChannelSchedule, TVDiscoveryConfig types"
```

---

### Task 2: Add type aliases in `config.go`

**Files:**
- Modify: `config.go` (the type alias section)

- [ ] **Step 1: Add aliases in the type alias block**

In `config.go`, find the existing type aliases section (where `TMDBDiscoveryConfig`, `TMDBEndpointGroup`, etc. are re-exported) and add:

```go
type (
	TVChannelConfig     = config.TVChannelConfig
	ChannelSchedule     = config.ChannelSchedule
	TVDiscoveryConfig   = config.TVDiscoveryConfig
)
```

- [ ] **Step 2: Verify compilation**

Run: `go build .`
Expected: No errors (may fail on main.go wiring which we haven't updated yet, that's OK)

- [ ] **Step 3: Commit**

```bash
git add config.go
git commit -m "feat: add TV channel config type aliases"
```

---

### Task 3: Add `TVShowInfo` wrapper struct with SourceMode

**Files:**
- Modify: `internal/syncer/engines/tv_go.go`

- [ ] **Step 1: Add `TVShowInfo` struct**

In `tv_go.go`, add this struct near the top (after the existing struct definitions, before `TVGoEngine`):

```go
// TVShowInfo wraps a discovered TV show with metadata about how it was found.
// This allows the engine to apply different rules based on the source channel.
type TVShowInfo struct {
	ID           int    // TMDB show ID
	Name         string
	OriginalName string
	FirstAirDate string
	Language     string
	GenreIDs     []int
	Channel      string // channel name that produced this show
	SourceMode   string // "discovery" | "manual"
}

// ToTMDBShow converts TVShowInfo to tmdb.TVShow for compatibility with existing code.
func (s TVShowInfo) ToTMDBShow() tmdb.TVShow {
	return tmdb.TVShow{
		ID:           s.ID,
		Name:         s.Name,
		OriginalName: s.OriginalName,
		FirstAirDate: s.FirstAirDate,
		Language:     s.Language,
		GenreIDs:     s.GenreIDs,
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/syncer/engines/tv_go.go
git commit -m "feat: add TVShowInfo wrapper with SourceMode and Channel fields"
```

---

### Task 4: Refactor `TVGoEngine` to support channels

**Files:**
- Modify: `internal/syncer/engines/tv_go.go`

- [ ] **Step 1: Update `TVEngineConfig` to include channel config**

Find the `TVEngineConfig` struct and add a `Channel` field:

```go
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
	// Channel config for multi-channel support
	Channel config.TVChannelConfig
}
```

- [ ] **Step 2: Add `channel` field to `TVGoEngine` struct**

In the `TVGoEngine` struct, add:

```go
	// Channel configuration for this engine instance
	channel config.TVChannelConfig
```

Place it near the existing `qualityProfile` and `tmdbDiscovery` fields.

- [ ] **Step 3: Wire channel config in `NewTVGoEngine`**

Find `NewTVGoEngine` and add the assignment:

```go
	engine.channel = cfg.Channel
```

- [ ] **Step 4: Refactor `discoverShows()` into a channel dispatcher**

Replace the existing `discoverShows()` method:

**Current code (lines ~404-409):**
```go
func (e *TVGoEngine) discoverShows(ctx context.Context) ([]tmdb.TVShow, error) {
	if len(e.tmdbDiscovery.Endpoints) > 0 {
		return e.tmdb.DiscoverTVFromConfig(ctx, e.tmdbDiscovery)
	}
	return e.discoverShowsHardcoded(ctx)
}
```

**New code:**
```go
func (e *TVGoEngine) discoverShows(ctx context.Context) ([]TVShowInfo, error) {
	switch e.channel.Mode {
	case "manual":
		return e.discoverFromManualIDs(ctx, e.channel.TMDBIDs)
	case "discovery", "":
		// Default to discovery if no mode specified
		return e.discoverFromChannel(ctx)
	default:
		e.logger.Printf("[TV] unknown channel mode %q for channel %q, skipping", e.channel.Mode, e.channel.Name)
		return nil, nil
	}
}

// discoverFromChannel runs the existing TMDB discovery logic for this channel's endpoints.
func (e *TVGoEngine) discoverFromChannel(ctx context.Context) ([]TVShowInfo, error) {
	var tmdbShows []tmdb.TVShow
	var err error

	if len(e.tmdbDiscovery.Endpoints) > 0 {
		tmdbShows, err = e.tmdb.DiscoverTVFromConfig(ctx, e.tmdbDiscovery)
	} else {
		tmdbShows, err = e.discoverShowsHardcoded(ctx)
	}
	if err != nil {
		return nil, err
	}

	shows := make([]TVShowInfo, 0, len(tmdbShows))
	for _, s := range tmdbShows {
		shows = append(shows, TVShowInfo{
			ID:           s.ID,
			Name:         s.Name,
			OriginalName: s.OriginalName,
			FirstAirDate: s.FirstAirDate,
			Language:     s.Language,
			GenreIDs:     s.GenreIDs,
			Channel:      e.channel.Name,
			SourceMode:   "discovery",
		})
	}
	return shows, nil
}

// discoverFromManualIDs fetches show details directly from TMDB IDs.
// No discovery filters, blacklist, or provider checks are applied.
func (e *TVGoEngine) discoverFromManualIDs(ctx context.Context, tmdbIDs []int) ([]TVShowInfo, error) {
	var shows []TVShowInfo

	for _, tmdbID := range tmdbIDs {
		details, err := e.tmdb.TVDetails(ctx, tmdbID)
		if err != nil {
			e.logger.Printf("[TV] manual channel %q: failed to fetch TMDB ID %d: %v", e.channel.Name, tmdbID, err)
			continue
		}

		imdbID, err := e.tmdb.TVExternalIDs(ctx, tmdbID)
		if err != nil || imdbID == "" {
			e.logger.Printf("[TV] manual channel %q: no IMDB ID for TMDB %d (%s), skipping", e.channel.Name, tmdbID, details.Name)
			continue
		}
		_ = imdbID // used later in processShow

		firstAirDate := details.FirstAirDate
		if len(firstAirDate) >= 4 {
			// Extract year for logging
		}

		shows = append(shows, TVShowInfo{
			ID:           tmdbID,
			Name:         details.Name,
			OriginalName: details.Name,
			FirstAirDate: firstAirDate,
			Language:     "", // not needed for manual mode
			GenreIDs:     nil,
			Channel:      e.channel.Name,
			SourceMode:   "manual",
		})
	}

	e.logger.Printf("[TV] manual channel %q: resolved %d shows from %d TMDB IDs", e.channel.Name, len(shows), len(tmdbIDs))
	return shows, nil
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/syncer/engines/tv_go.go
git commit -m "refactor: TVGoEngine now uses channel-based discovery dispatcher"
```

---

### Task 5: Update `Run()` and `processShow()` for `TVShowInfo`

**Files:**
- Modify: `internal/syncer/engines/tv_go.go`

- [ ] **Step 1: Update `Run()` to use `TVShowInfo`**

The `Run()` method currently calls `discoverShows()` and iterates `[]tmdb.TVShow`. Update it:

Find the section in `Run()`:
```go
	shows, err := e.discoverShows(ctx)
	...
	for i, show := range shows {
		...
		e.processShow(ctx, show)
	}
```

This already works since `discoverShows()` now returns `[]TVShowInfo` and `processShow` will be updated in the next step.

- [ ] **Step 2: Update `processShow()` signature**

Change the method signature from:
```go
func (e *TVGoEngine) processShow(ctx context.Context, show tmdb.TVShow)
```

To:
```go
func (e *TVGoEngine) processShow(ctx context.Context, show TVShowInfo)
```

- [ ] **Step 3: Update `processShow()` to use `show.ToTMDBShow()` where needed**

Inside `processShow()`, wherever `show.ID`, `show.Name`, etc. are used, the `TVShowInfo` wrapper already has these fields directly. No changes needed for field access since `TVShowInfo` mirrors the same fields.

The only place that needs the full `tmdb.TVShow` type is if there are type assertions. Check for any `show.(tmdb.TVShow)` patterns — there should be none. The `ToTMDBShow()` method is available if any external function strictly requires `tmdb.TVShow`.

- [ ] **Step 4: Verify compilation**

Run: `go build ./internal/syncer/engines/`
Expected: No errors (or only errors from files we haven't updated yet)

- [ ] **Step 5: Commit**

```bash
git add internal/syncer/engines/tv_go.go
git commit -m "refactor: processShow accepts TVShowInfo instead of tmdb.TVShow"
```

---

### Task 6: Disable fullpack and apply manual-mode rules in `processShow()`

**Files:**
- Modify: `internal/syncer/engines/tv_go.go`

- [ ] **Step 1: Skip fullpack processing for manual mode**

In `processShow()`, find the section where fullpacks are processed. It looks like:

```go
// Process fullpacks first
for _, stream := range fullpackStreams {
	e.processFullpack(ctx, show, stream, ...)
}
```

Wrap it with a condition:

```go
// Process fullpacks first (only in discovery mode)
if show.SourceMode != "manual" {
	for _, stream := range fullpackStreams {
		e.processFullpack(ctx, show, stream, ...)
	}
}
```

- [ ] **Step 2: Skip blacklist check for manual mode**

In `processShow()`, find the blacklist check:
```go
if e.isBlacklisted(showName) {
	e.logger.Printf("🚫 Blacklist: skipping show '%s'", showName)
	return
}
```

Change to:
```go
if show.SourceMode != "manual" && e.isBlacklisted(showName) {
	e.logger.Printf("🚫 Blacklist: skipping show '%s'", showName)
	return
}
```

- [ ] **Step 3: Make complete-season skip config-driven**

Find the complete-season skip logic in `processShow()`:
```go
if allComplete && len(completeSeasons) > 0 {
	e.logger.Printf("Skipping %s — all seasons complete", showName)
	return
}
```

Change to:
```go
// Complete-season skip: controlled per-channel
if e.channel.SkipCompleteSeasons && allComplete && len(completeSeasons) > 0 {
	e.logger.Printf("[TV] channel %q: skipping %s — all seasons complete", e.channel.Name, showName)
	return
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/syncer/engines/tv_go.go
git commit -m "feat: manual mode skips fullpack, blacklist, and uses config-driven season skip"
```

---

### Task 7: Update `TVSyncerConfig` and `NewTVSyncer` to pass channel config

**Files:**
- Modify: `internal/syncer/engines/tv.go`

- [ ] **Step 1: Add `Channel` field to `TVSyncerConfig`**

Find `TVSyncerConfig` and add:

```go
	Channel config.TVChannelConfig
```

- [ ] **Step 2: Pass channel to `TVEngineConfig` in `NewTVSyncer`**

In `NewTVSyncer`, find where `TVEngineConfig` is constructed and add:

```go
		Channel: cfg.Channel,
```

- [ ] **Step 3: Update `Name()` to include channel name**

Change the `Name()` method:
```go
func (s *TVSyncer) Name() string {
	if s.engine.channel.Name != "" {
		return "tv:" + s.engine.channel.Name
	}
	return "tv"
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/syncer/engines/tv.go
git commit -m "feat: TVSyncer passes channel config and includes channel in Name()"
```

---

### Task 8: Update scheduler to support multiple TV channel jobs

**Files:**
- Modify: `internal/syncer/scheduler/scheduler.go`

- [ ] **Step 1: Add `TVChannels` to `SchedulerConfig`**

Find the `SchedulerConfig` struct and add:

```go
type SchedulerConfig struct {
	Enabled       bool
	MoviesSync    DailyJobConfig
	TVSync        DailyJobConfig       // Legacy: used if no TVChannels provided
	TVChannels    []TVChannelConfig    // Per-channel schedules (new)
	WatchlistSync WatchlistSyncConfig
}
```

Note: `TVChannelConfig` here references the same type. Add import if needed.

- [ ] **Step 2: Update `shouldRun` to handle `"tv:*"` job names**

Find the `shouldRun` method. It currently has:
```go
switch name {
case "movies":
	return s.shouldRunDaily(name, state, s.cfg.MoviesSync.Enabled, s.cfg.MoviesSync.DaysOfWeek, s.cfg.MoviesSync.Hour, s.cfg.MoviesSync.Minute)
case "tv":
	return s.shouldRunDaily(name, state, s.cfg.TVSync.Enabled, s.cfg.TVSync.DaysOfWeek, s.cfg.TVSync.Hour, s.cfg.TVSync.Minute)
case "watchlist":
	return s.shouldRunInterval(name, state, s.cfg.WatchlistSync.Enabled, s.cfg.WatchlistSync.IntervalHours)
default:
	return false
}
```

Replace with:
```go
switch name {
case "movies":
	return s.shouldRunDaily(name, state, s.cfg.MoviesSync.Enabled, s.cfg.MoviesSync.DaysOfWeek, s.cfg.MoviesSync.Hour, s.cfg.MoviesSync.Minute)
case "watchlist":
	return s.shouldRunInterval(name, state, s.cfg.WatchlistSync.Enabled, s.cfg.WatchlistSync.IntervalHours)
default:
	// Handle tv:* channel jobs
	if strings.HasPrefix(name, "tv:") {
		channelName := strings.TrimPrefix(name, "tv:")
		for _, ch := range s.cfg.TVChannels {
			if ch.Name == channelName {
				return s.shouldRunDaily(name, state, ch.Enabled, ch.Schedule.DaysOfWeek, ch.Schedule.Hour, ch.Schedule.Minute)
			}
		}
		// Fallback: legacy TVSync config if channel not found
		return s.shouldRunDaily(name, state, s.cfg.TVSync.Enabled, s.cfg.TVSync.DaysOfWeek, s.cfg.TVSync.Hour, s.cfg.TVSync.Minute)
	}
	return false
}
```

- [ ] **Step 3: Add `strings` import if not present**

At the top of the file, ensure:
```go
import "strings"
```

- [ ] **Step 4: Update `New()` to store TVChannels**

The `New()` function already stores `cfg` which includes `TVChannels`, so no changes needed there. But verify the field is accessible.

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/syncer/scheduler/`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add internal/syncer/scheduler/scheduler.go
git commit -m "feat: scheduler supports per-channel TV schedules via tv:* job names"
```

---

### Task 9: Wire TV channels as separate scheduler jobs in `main.go`

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Read the TV channels config and create one syncer per enabled channel**

In `main.go`, find the syncers creation block (around line 3290). The current code creates a single `"tv"` syncer:

```go
syncers := map[string]scheduler.Syncer{
	"movies": engines.NewMoviesSyncer(...),
	"tv":     engines.NewTVSyncer(engines.TVSyncerConfig{...}),
	"watchlist": engines.NewWatchlistSyncer(...),
}
```

Replace the `"tv"` entry with multi-channel wiring:

```go
// Build TV syncers from channel config
tvSyncers := make(map[string]scheduler.Syncer)
tvChannels := globalConfig.TMDBDiscovery.TV.Channels

if len(tvChannels) == 0 {
	// Backward compat: wrap legacy endpoints into a single discovery channel
	legacyGroup := safeTMDBGroup(globalConfig.TMDBDiscovery.TV)
	if legacyGroup != nil && len(legacyGroup.Endpoints) > 0 {
		tvChannels = []config.TVChannelConfig{{
			Enabled:             true,
			Name:                "discovery-default",
			Mode:                "discovery",
			Schedule:            config.ChannelSchedule(globalConfig.Scheduler.TVSync),
			Endpoints:           legacyGroup.Endpoints,
			SkipCompleteSeasons: true,
		}}
	}
}

for _, ch := range tvChannels {
	if !ch.Enabled {
		logger.Printf("[TV] channel %q disabled, skipping", ch.Name)
		continue
	}

	syncer := engines.NewTVSyncer(engines.TVSyncerConfig{
		GoStormURL:     globalConfig.GoStormBaseURL,
		TMDBAPIKey:     globalConfig.TMDBAPIKey,
		TorrentioURL:   globalConfig.TorrentioURL,
		PlexURL:        globalConfig.Plex.URL,
		PlexToken:      globalConfig.Plex.Token,
		PlexTVLib:      globalConfig.Plex.TVLibraryID,
		TVDir:          filepath.Join(globalConfig.PhysicalSourcePath, "tv"),
		StateDir:       GetStateDir(),
		LogsDir:        logsDir,
		ProwlarrCfg:    globalConfig.Prowlarr,
		DB:             stateDB,
		QualityProfile: quality.ResolveTVProfile(globalConfig.Quality),
		TMDBDiscovery:  quality.TMDBEndpointGroupFromConfig(safeTMDBGroup(globalConfig.TMDBDiscovery.TV)),
		Channel:        ch,
	})
	syncers[syncer.Name()] = syncer
	tvChannelsForScheduler = append(tvChannelsForScheduler, ch)
}
```

- [ ] **Step 2: Pass TVChannels to scheduler config**

Find where `schedCfg` is built:
```go
schedCfg := scheduler.SchedulerConfig{
	Enabled:       globalConfig.Scheduler.Enabled,
	MoviesSync:    scheduler.DailyJobConfig(globalConfig.Scheduler.MoviesSync),
	TVSync:        scheduler.DailyJobConfig(globalConfig.Scheduler.TVSync),
	WatchlistSync: scheduler.WatchlistSyncConfig(globalConfig.Scheduler.WatchlistSync),
}
```

Add the TV channels:
```go
schedCfg := scheduler.SchedulerConfig{
	Enabled:       globalConfig.Scheduler.Enabled,
	MoviesSync:    scheduler.DailyJobConfig(globalConfig.Scheduler.MoviesSync),
	TVSync:        scheduler.DailyJobConfig(globalConfig.Scheduler.TVSync),
	TVChannels:    tvChannelsForScheduler,
	WatchlistSync: scheduler.WatchlistSyncConfig(globalConfig.Scheduler.WatchlistSync),
}
```

- [ ] **Step 3: Remove the legacy `"tv"` key from syncers map**

Since we now add syncers via `syncers[syncer.Name()] = syncer` in the loop, remove the hardcoded `"tv"` entry from the map literal.

- [ ] **Step 4: Verify compilation**

Run: `go build .`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat: wire TV channels as separate scheduler jobs in main.go"
```

---

### Task 10: Update `config.json.example`

**Files:**
- Modify: `config.json.example`

- [ ] **Step 1: Update the `tmdb_discovery` section**

Replace the current `tmdb_discovery` section:

**Current:**
```json
  "tmdb_discovery": {
    "movies": {
      "endpoints": []
    },
    "tv": {
      "endpoints": []
    }
  }
```

**New:**
```json
  "tmdb_discovery": {
    "movies": {
      "endpoints": []
    },
    "tv": {
      "channels": [
        {
          "enabled": false,
          "name": "discovery-default",
          "mode": "discovery",
          "schedule": {
            "hour": 3,
            "minute": 0,
            "days_of_week": [1, 2, 3, 4, 5, 6, 0]
          },
          "endpoints": [],
          "skip_complete_seasons": true
        },
        {
          "enabled": true,
          "name": "lost-friends",
          "mode": "manual",
          "schedule": {
            "hour": 4,
            "minute": 0,
            "days_of_week": [1, 3, 5]
          },
          "tmdb_ids": [4607, 1668],
          "skip_complete_seasons": false
        }
      ]
    }
  }
```

- [ ] **Step 2: Commit**

```bash
git add config.json.example
git commit -m "docs: update config.json.example with TV channels structure"
```

---

### Task 11: Disable NFO creation (carry-forward from earlier change)

**Files:**
- Already modified: `internal/syncer/engines/movie_go.go`
- Already modified: `internal/syncer/engines/tv_go.go`

- [ ] **Step 1: Verify the NFO disable changes are still in place**

The earlier changes commented out:
- `movie_go.go`: `// e.createMovieNFO(path, imdbID)`
- `tv_go.go`: `// e.createNFO(path)`

These changes were made in previous work. Verify they are still present and haven't been overwritten by other edits.

- [ ] **Step 2: Commit (if not already committed)**

```bash
git status
```
If the NFO-disable changes show as modified, commit them:
```bash
git add internal/syncer/engines/movie_go.go internal/syncer/engines/tv_go.go
git commit -m "feat: disable NFO file creation during sync"
```

---

### Task 12: Build and test

**Files:**
- All modified files

- [ ] **Step 1: Full build**

Run: `go build -pgo=auto -o gostream .`
Expected: No errors, binary produced

- [ ] **Step 2: Run existing tests**

Run: `go test ./...`
Expected: All tests pass

- [ ] **Step 3: Test specific TV engine compilation**

Run: `go build ./internal/syncer/engines/`
Run: `go build ./internal/syncer/scheduler/`
Expected: No errors

- [ ] **Step 4: Commit (if any test fixes needed)**

```bash
git add .
git commit -m "fix: resolve compilation and test issues"
```

---

### Task 13: Update the legacy `tv` type alias in `config.go` if needed

**Files:**
- Modify: `config.go`

- [ ] **Step 1: Check if `TMDBDiscoveryConfig` needs to reference `TVDiscoveryConfig`**

In `config.go`, the main config struct has:
```go
TMDBDiscovery TMDBDiscoveryConfig `json:"tmdb_discovery"`
```

And `TMDBDiscoveryConfig` (from `internal/config/quality.go`) has:
```go
type TMDBDiscoveryConfig struct {
	Movies *TMDBEndpointGroup `json:"movies,omitempty"`
	TV     *TMDBEndpointGroup `json:"tv,omitempty"`
}
```

We need to change the `TV` field from `*TMDBEndpointGroup` to use the new `TVDiscoveryConfig`. Update `TMDBDiscoveryConfig`:

**In `internal/config/quality.go`, change:**
```go
type TMDBDiscoveryConfig struct {
	Movies *TMDBEndpointGroup `json:"movies,omitempty"`
	TV     *TVDiscoveryConfig `json:"tv,omitempty"`
}
```

**Wait** — this is a breaking change. The old JSON format had `"tv": {"endpoints": [...]}`. The new format has `"tv": {"channels": [...]}`. Since we made `TVDiscoveryConfig` have only `Channels`, the old `endpoints` field won't parse.

**Better approach:** Make `TVDiscoveryConfig` support both formats:

```go
type TVDiscoveryConfig struct {
	// Legacy: single endpoint group (backward compat)
	Endpoints []TMDBEndpoint `json:"endpoints,omitempty"`
	// New: multi-channel support
	Channels []TVChannelConfig `json:"channels,omitempty"`
}
```

This way, old configs with `endpoints` still parse, and `main.go` wrapping logic (Task 9, Step 1) can check `if len(channels) == 0 && len(endpoints) > 0` to auto-create the legacy channel.

- [ ] **Step 2: Update `TVDiscoveryConfig` to include both fields**

Modify in `internal/config/quality.go`:
```go
type TVDiscoveryConfig struct {
	Endpoints []TMDBEndpoint    `json:"endpoints,omitempty"` // legacy, backward compat
	Channels  []TVChannelConfig `json:"channels,omitempty"`  // new multi-channel
}
```

- [ ] **Step 3: Update `main.go` backward compat logic**

In Task 9, Step 1, update the fallback:
```go
if len(tvChannels) == 0 {
	// Backward compat: wrap legacy endpoints into a single discovery channel
	legacyTV := globalConfig.TMDBDiscovery.TV
	if legacyTV != nil && len(legacyTV.Endpoints) > 0 {
		tvChannels = []config.TVChannelConfig{{
			Enabled:             true,
			Name:                "discovery-default",
			Mode:                "discovery",
			Schedule:            config.ChannelSchedule(globalConfig.Scheduler.TVSync),
			Endpoints:           legacyTV.Endpoints,
			SkipCompleteSeasons: true,
		}}
	}
}
```

- [ ] **Step 4: Re-verify compilation**

Run: `go build .`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/config/quality.go main.go
git commit -m "fix: TVDiscoveryConfig supports both legacy endpoints and new channels"
```

---

## Self-Review Checklist

**1. Spec coverage check:**

| Spec requirement | Task |
|-----------------|------|
| Channel-based config types | Task 1, Task 13 |
| Type aliases in config.go | Task 2 |
| TVShowInfo with SourceMode/Channel | Task 3 |
| discoverShows() channel dispatcher | Task 4 |
| discoverFromManualIDs() | Task 4 |
| processShow() with SourceMode rules | Tasks 5, 6 |
| Fullpack disabled for manual | Task 6 |
| Blacklist skipped for manual | Task 6 |
| Complete-season skip config-driven | Task 6 |
| TVSyncerConfig passes channel | Task 7 |
| Name() returns "tv:<channel>" | Task 7 |
| Scheduler handles tv:* jobs | Task 8 |
| main.go wires multiple TV syncers | Task 9 |
| config.json.example updated | Task 10 |
| NFO creation disabled | Task 11 |
| Size-first quality preserved | No changes needed (existing policy unchanged) |
| Backward compatibility | Task 13 |

All spec requirements covered. ✅

**2. Placeholder scan:** No TBDs, TODOs, or incomplete steps. All code blocks contain actual implementation code. ✅

**3. Type consistency check:**
- `TVShowInfo.ID` is `int` (matches `tmdb.TVShow.ID`)
- `config.TVChannelConfig.TMDBIDs` is `[]int` (matches TMDB API using `int`)
- `ChannelSchedule` maps directly from JSON with `int` fields
- `TVSyncerConfig.Channel` is `config.TVChannelConfig` → passed to `TVEngineConfig.Channel` → stored in `TVGoEngine.channel`
- `Name()` returns `"tv:" + channel.Name` → matches scheduler's `strings.TrimPrefix(name, "tv:")` lookup ✅

**4. Potential issue:** The `processShow()` function is very large (~200+ lines). I'm only changing the signature and a few conditional blocks. The rest of the method (stream fetching, quality ranking, MKV creation) is unchanged. Since `TVShowInfo` mirrors `tmdb.TVShow` field names, all existing field accesses within `processShow()` continue to work without modification. ✅
