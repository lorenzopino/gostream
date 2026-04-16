# Design: TV Multi-Channel Sync

**Date:** 2026-04-16
**Author:** AI-assisted
**Status:** Draft — pending review

## Problem Statement

The current TV sync engine uses dynamic discovery (trending, on-air, etc.) to find TV shows. The user needs the ability to:

1. **Disable all dynamic TV discovery** (trending, on-air, etc.) for now
2. **Configure specific TV series by TMDB ID** (e.g., Lost `4607`, Friends `1668`)
3. **Download ALL episodes** of those specific series
4. **Support multiple sync channels** with different modes:
   - `discovery` mode: existing dynamic discovery behavior (can be re-enabled later)
   - `manual` mode: direct TMDB ID list, fetch all episodes
5. Each channel must have its **own schedule** (different run times/days)

## Configuration

### New JSON Structure

```jsonc
{
  "tmdb_discovery": {
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
          "endpoints": [
            { "type": "discover", "language": "en", "pages": 3 },
            { "type": "trending", "pages": 3 }
          ],
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
}
```

### Go Types (in `internal/config/quality.go`)

```go
// TVChannelConfig represents a single TV sync channel
type TVChannelConfig struct {
    Enabled             bool            `json:"enabled"`
    Name                string          `json:"name"`
    Mode                string          `json:"mode"` // "discovery" | "manual"
    Schedule            ChannelSchedule `json:"schedule"`
    Endpoints           []TVEndpoint    `json:"endpoints,omitempty"`
    TMDBIDs             []int64         `json:"tmdb_ids,omitempty"`
    SkipCompleteSeasons bool            `json:"skip_complete_seasons"`
}

// ChannelSchedule replaces the top-level schedule for TV channels
type ChannelSchedule struct {
    Hour       int   `json:"hour"`
    Minute     int   `json:"minute"`
    DaysOfWeek []int `json:"days_of_week"`
}

// TVDiscoveryConfig wraps the channels array
type TVDiscoveryConfig struct {
    Channels []TVChannelConfig `json:"channels"`
}
```

### Backward Compatibility

If `tv.channels` is empty but `tv.endpoints` exists (old format), automatically create a single `discovery-default` channel with those endpoints.

### Finalization

`finalize()` must validate:
- `mode == "manual"` → `tmdb_ids` must not be empty
- `mode == "discovery"` → `endpoints` must not be empty (if no endpoints, skip the channel)
- `mode` must be one of `"discovery"` or `"manual"`
- `name` must be unique across all channels

## Architecture

### TVGoEngine Changes

**File:** `internal/syncer/engines/tv_go.go`

#### `discoverShows()` becomes a dispatcher

```
discoverShows(ctx)
├── iterate config.Channels
│   ├── if !Enabled → skip
│   ├── if mode == "discovery" → discoverFromChannel(ctx, ch)
│   │   └── existing discovery logic (on-air, trending, discover endpoints)
│   └── if mode == "manual" → discoverFromManualIDs(ctx, ch.TMDBIDs)
│       └── for each tmdbID:
│           ├── TVDetails() → get show name, season count
│           ├── TVExternalIDs() → resolve IMDB ID
│           └── append to shows list (no filters, no blacklist)
└── return combined shows from all channels
```

#### New `TVShowInfo.SourceMode` flag

```go
type TVShowInfo struct {
    TMDBID    int64
    IMDBID    string
    Name      string
    Year      int
    Channel   string    // channel name that produced this show
    SourceMode string   // "discovery" | "manual"
}
```

#### `processShow()` behavior changes based on SourceMode

| Check | discovery mode | manual mode |
|-------|---------------|-------------|
| Blacklist check | ✅ Apply | ❌ Skip |
| Premium provider filter | ✅ Apply | ❌ Skip |
| Language filter | ✅ Apply | ❌ Skip |
| Complete-season skip | ✅ Based on config | ✅ Based on `skip_complete_seasons` in channel config |
| Season range | New/incomplete only | All seasons (1 to last) |
| Fullpack torrents | ✅ Process first | ❌ Disabled — only single episodes |

#### New method: `discoverFromManualIDs()`

```
discoverFromManualIDs(ctx, tmdbIDs []int64) ([]TVShowInfo, error)
├── for each tmdbID:
│   ├── details, err := tmdb.TVDetails(ctx, tmdbID)
│   │   └── if err → log and continue
│   ├── imdbID, err := tmdb.TVExternalIDs(ctx, tmdbID)
│   │   └── if err or empty → log and continue
│   └── append TVShowInfo{SourceMode: "manual", Channel: ch.Name}
└── return shows
```

#### Fullpack disable for manual mode

In `processShow()`, the existing code calls `processFullpack()` for fullpack torrents before processing singles:

```go
// Existing code in processShow():
for _, stream := range fullpacks {
    e.processFullpack(stream, show, ...)
}
for _, stream := range singles {
    e.processSingle(stream, show, ...)
}
```

Change to:
```go
// Only process fullpacks in discovery mode
if show.SourceMode != "manual" {
    for _, stream := range fullpacks {
        e.processFullpack(stream, show, ...)
    }
}
// Always process singles
for _, stream := range singles {
    e.processSingle(stream, show, ...)
}
```

Additionally, in `classifyStream()`, when `SourceMode == "manual"`:
- Force `isFullpack = false` so that fullpack torrents are classified as singles (they will fail S##E## matching and be rejected naturally)
- OR: keep fullpack detection but skip them in `processShow()` as shown above (recommended — cleaner)

### Scheduler Changes

**File:** `internal/syncer/scheduler/scheduler.go`

Instead of a single `"tv"` job, register one job per enabled channel:

```
Job naming: "tv:<channel_name>"
Example: "tv:lost-friends", "tv:discovery-default"
```

Each job:
- Has its own schedule (from `Channel.Schedule`)
- Runs the same `TVGoEngine.Run()` but filtered to only process shows from that channel
- Independent failure/success tracking in the scheduler state

#### Implementation approach

Add a `channelFilter string` field to the TV syncer config. When the scheduler runs job `"tv:lost-friends"`:
- It calls `TVGoEngine.Run(ctx)` with `channelFilter = "lost-friends"`
- `Run()` passes the filter to `discoverShows()`
- `discoverShows()` only processes the matching channel

### Quality & Size Filtering — No Changes

The existing size-first policy remains **unchanged and active** for all modes:

| Policy | TV Value | Effect |
|--------|----------|--------|
| `IdealGB` | 0.5 | Episodes ≤ 500MB get max score (+300,000) |
| `CompactGB` | 1.0 | Episodes ≤ 1GB get good score (+200,000) |
| `FallbackGB` | 1.5 | **HARD REJECT** above 1.5GB per episode |
| `StrictFileMaxGB` | 1.0 | **HARD REJECT** individual files > 1GB |
| `MinSeeders` | 5 | **MINIMUM** 5 seeders (1-4 = reject) |

Scoring is dominated by size tier (300k/200k/100k), all other factors combined < 30k. Size-first behavior is preserved.

### File Changes Summary

| File | Change |
|------|--------|
| `internal/config/quality.go` | Add `TVChannelConfig`, `ChannelSchedule`, `TVDiscoveryConfig` types |
| `internal/syncer/engines/tv_go.go` | Refactor `discoverShows()` into channel dispatcher; add `discoverFromManualIDs()`; add `SourceMode` field; add fullpack skip for manual mode |
| `internal/syncer/engines/tv.go` | Add `channelFilter` to syncer config; pass to engine |
| `internal/syncer/scheduler/scheduler.go` | Register one job per enabled TV channel |
| `main.go` | Wire TV channels as separate scheduler jobs |
| `config.json.example` | Update with new channels structure |

## Execution Flow

### Manual Channel (e.g., Lost)

```
Scheduler tick → "tv:lost-friends"
└── TVGoEngine.Run(channelFilter="lost-friends")
    ├── populateRegistryFromExisting()
    ├── discoverShows(channelFilter="lost-friends")
    │   └── channel "lost-friends" (mode=manual):
    │       ├── TMDB ID 4607 → Lost (2004), IMDB tt0436992
    │       └── returns [TVShowInfo{Name: "Lost", SourceMode: "manual"}]
    ├── processShow(Lost, SourceMode="manual")
    │   ├── Skip blacklist ✅
    │   ├── Skip premium provider check ✅
    │   ├── Skip language filter ✅
    │   ├── Check skip_complete_seasons (false → process anyway)
    │   ├── getStreams(imdbID)
    │   │   ├── Prowlarr query → classifyStream (no fullpack)
    │   │   └── Torrentio fallback per episode if needed
    │   ├── Sort by quality score (size-first)
    │   ├── processSingle() for each episode (S01E01, S01E02, ...)
    │   │   ├── Add magnet to GoStorm
    │   │   ├── Wait 45s for metadata
    │   │   ├── Select largest file passing RankExactStreamingFile (≤1GB)
    │   │   └── createMKV() → JSON stub on filesystem
    │   └── (no processFullpack call)
    ├── saveRegistry()
    ├── cleanupOrphanedFiles()
    ├── cleanupOrphanedTorrents()
    └── rehydrateMissingTorrents()
```

### Discovery Channel (when re-enabled)

```
Scheduler tick → "tv:discovery-default"
└── TVGoEngine.Run(channelFilter="discovery-default")
    └── Same as current behavior (unchanged)
```

## Error Handling

- **TMDB API failure**: Log and skip the specific show, continue with others
- **IMDB ID resolution failure**: Log warning, skip show (need IMDB ID for torrent search)
- **Prowlarr/Torrentio failure**: Log and continue; torrent fallback handles individual failures
- **Invalid config**: Fail at startup with clear error message during `finalize()`
- **Channel schedule overlap**: Allowed — channels run independently

## Testing Strategy

1. **Unit tests**: `discoverFromManualIDs()` with mock TMDB client
2. **Unit tests**: `processShow()` with `SourceMode="manual"` verifies no blacklist/provider/language filters applied
3. **Unit tests**: Fullpack rejection in manual mode
4. **Integration test**: Full sync run with known TMDB IDs against test TMDB API
5. **Config validation tests**: Invalid modes, empty IDs, duplicate names

## Migration

No migration needed. The old `tv.endpoints` format is automatically wrapped into a `discovery-default` channel. Existing MKV files and registry entries are unaffected.

## Rollback

To revert: restore old `config.json` format with `tv.endpoints` directly under `tv`. Remove `channels` array. Code auto-detects old format.
