# GoStream Configuration Reference

> **Version:** 1.5.x+ (with quality profiles & TMDB discovery)
> **Last updated:** 2026-04-08

---

## Table of Contents

1. [File Locations & Loading Order](#1-file-locations--loading-order)
2. [Environment Variable Overrides](#2-environment-variable-overrides)
3. [Core Tuning](#3-core-tuning)
4. [FUSE Timeouts](#4-fuse-timeouts)
5. [HTTP Resilience](#5-http-resilience)
6. [Preload Engine](#6-preload-engine)
7. [Cache Management](#7-cache-management)
8. [Connectivity](#8-connectivity)
9. [AI Optimizer](#9-ai-optimizer)
10. [FUSE Paths](#10-fuse-paths)
11. [Disk Warmup](#11-disk-warmup)
12. [NAT-PMP](#12-nat-pmp)
13. [External Services (Plex / Jellyfin)](#13-external-services--plex--jellyfin)
14. [External Services (Prowlarr / TMDB / Torrentio)](#14-external-services--prowlarr--tmdb--torrentio)
15. [Scheduler](#15-scheduler)
16. [Quality Profiles](#16-quality-profiles)
17. [TMDB Discovery](#17-tmdb-discovery)
18. [Telemetry](#18-telemetry)
19. [State DB](#19-state-db)
20. [Directory Structure](#20-directory-structure)

---

## 1. File Locations & Loading Order

### Config File Resolution (priority order)

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | `MKV_PROXY_CONFIG_PATH` env var | Explicit override — absolute path to `config.json` |
| 2 | `<binary_directory>/config.json` | Co-located with the compiled binary |
| 3 | `config.json` (CWD) | Fallback to current working directory |

**Example:**
```bash
# Binary at /usr/local/bin/gostream → reads /usr/local/bin/config.json
# Override:
MKV_PROXY_CONFIG_PATH=/etc/gostream/config.json gostream ...
```

### Root Path (state directory)

| Source | Default |
|--------|---------|
| `--path <dir>` CLI flag | User-specified directory |
| Auto-detected | `/home/pi` (Linux) or directory of config.json |

The **STATE** directory is always `<RootPath>/STATE`.

### JSON Format

The config file is standard JSON with **comment support**:
- `// single-line comments` are stripped before parsing
- `# hash comments` are NOT supported
- Trailing commas are NOT supported

---

## 2. Environment Variable Overrides

Environment variables **always win** over `config.json` values.

| Environment Variable | Config Field | Type | Notes |
|---------------------|--------------|------|-------|
| `MKV_PROXY_CONFIG_PATH` | — | path | Overrides config.json location |
| `MKV_PROXY_CONCURRENCY` | `master_concurrency_limit` | int | Must be > 0 |
| `MKV_PROXY_READ_AHEAD_BUDGET` | `read_ahead_budget_mb` | int/string | Accepts `"256"`, `"256MB"`, `"128KB"` |
| `MKV_PROXY_GOSTORM_URL` | `gostorm_url` | URL | GoStorm HTTP endpoint |
| `MKV_PROXY_AI_URL` | `ai_url` | URL | AI sidecar endpoint |
| `AI_PROVIDER` | `ai_provider` | string | `"local"`, `"openrouter"`, `"openai"` |
| `AI_MODEL` | `ai_model` | string | Model ID for cloud providers |
| `AI_API_KEY` | `ai_api_key` | string | API key for cloud providers |
| `GOSTREAM_PLEX_URL` | `plex.url` | URL | GOSTREAM_ prefix takes priority |
| `PLEX_URL` | `plex.url` | URL | Fallback to PLEX_ prefix |
| `GOSTREAM_PLEX_TOKEN` | `plex.token` | string | GOSTREAM_ prefix takes priority |
| `PLEX_TOKEN` | `plex.token` | string | Fallback to PLEX_ prefix |
| `MKV_PROXY_LOG_LEVEL` | `log_level` | string | `"DEBUG"`, `"INFO"`, `"WARN"`, `"ERROR"` |
| `MKV_PROXY_UID` | (internal UID) | uint32 | FUSE file owner |
| `MKV_PROXY_GID` | (internal GID) | uint32 | FUSE file group |

**Legacy compatibility:** The JSON key `torrserver_url` is still read as a fallback for `gostorm_url`.

---

## 3. Core Tuning

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `master_concurrency_limit` | `int` | `25` | > 0 | ❌ | Global limit for concurrent HTTP requests to GoStorm. Also derives `MaxIdleConns`, `MaxConnsPerHost`, `MaxIdleConnsPerHost`. |
| `read_ahead_budget_mb` | `int64` | `512` | Min 10 (enforced) | ❌ | Budget for read-ahead buffering in MB. Internally: `MB × 1024 × 1024` bytes. |
| `metadata_cache_size_mb` | `int64` | `50` | Min 1 (enforced) | ❌ | Size of metadata LRU cache in MB. |
| `write_buffer_size_kb` | `int` | `64` | > 0 | ❌ | Write buffer size in KB. Internally: `KB × 1024` bytes. |
| `read_buffer_size_kb` | `int` | `64` | > 0 | ❌ | Read buffer size in KB. |
| `fuse_block_size_bytes` | `int` | `1048576` (1 MB) | > 0 | ❌ | FUSE block size for virtual filesystem. |
| `streaming_threshold_kb` | `int64` | `128` | > 0 | ❌ | Threshold in KB before streaming mode activates. |
| `log_level` | `string` | `"INFO"` | `"DEBUG"`, `"INFO"`, `"WARN"`, `"ERROR"`, `"FATAL"` | ✅ | Log verbosity level. |

---

## 4. FUSE Timeouts

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `attr_timeout_seconds` | `float64` | `1.0` | ≥ 0 | ✅ | FUSE attribute cache timeout. How long the kernel caches file attributes. |
| `entry_timeout_seconds` | `float64` | `1.0` | ≥ 0 | ✅ | FUSE directory entry cache timeout. |
| `negative_timeout_seconds` | `float64` | `0.0` | ≥ 0 | ✅ | FUSE negative entry cache (for non-existent files). Set to 0 to disable. |

---

## 5. HTTP Resilience

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `http_client_timeout_seconds` | `int` | `30` | > 0 | ✅ | HTTP client connect and read timeout in seconds. |
| `max_retry_attempts` | `int` | `6` | > 0 | ✅ | Max retry attempts for failed HTTP requests. Uses exponential backoff. |
| `retry_delay_ms` | `int` | `500` | > 0 | ✅ | Initial delay between retries in milliseconds. |
| `rescue_grace_period_seconds` | `int` | `0` (off) | > 0 | ✅ | Grace period before rescue mechanism activates for stalled torrents. |
| `rescue_cooldown_hours` | `int` | `0` (off) | > 0 | ✅ | Cooldown between rescue attempts. |

---

## 6. Preload Engine

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `preload_workers_count` | `int` | `4` | > 0 | ❌ | Number of preload worker goroutines for background prefetching. |
| `preload_initial_delay_ms` | `int` | `1000` | > 0 | ✅ | Initial delay before preload starts (milliseconds). |
| `warm_start_idle_seconds` | `int` | `6` | > 0 | ✅ | Idle time before warm start triggers prefetching of head/tail regions. |
| `max_concurrent_prefetch` | `int` | `3` | > 0 | ✅ | Max concurrent prefetch operations. Safety: resets to 3 if ≤ 0. |

---

## 7. Cache Management

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `cache_cleanup_interval_min` | `int` | `5` | > 0 | ✅ | Interval between cache cleanup runs (minutes). |
| `max_cache_entries` | `int` | `10000` | > 0 | ✅ | Maximum number of entries in the FUSE metadata LRU cache. |

---

## 8. Connectivity

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `gostorm_url` | `string` | `"http://127.0.0.1:8090"` | HTTP URL | ✅ | GoStorm BitTorrent engine HTTP endpoint. |
| `proxy_listen_port` | `int` | `8080` | > 0 | ❌ | FUSE proxy HTTP listen port. |
| `metrics_port` | `int` | `9080` | > 0 | ✅ | Prometheus metrics and dashboard HTTP port. |
| `blocklist_url` | `string` | `"https://github.com/Naunter/BT_BlockLists/raw/master/bt_blocklists.gz"` | GZIP URL | ✅ | BitTorrent IP blocklist URL. Downloaded and refreshed every 24 hours (~700k ranges). |

---

## 9. AI Optimizer

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `ai_url` | `string` | `""` (disabled) | HTTP URL | ✅ | AI Optimizer sidecar URL. Controls adaptive BitTorrent tuning. |
| `ai_provider` | `string` | `""` (local) | `"local"`, `"openrouter"`, `"openai"` | ✅ | Provider type. `"local"` = on-device, others = cloud API. |
| `ai_model` | `string` | `""` | Model ID string | ✅ | Model ID for cloud AI providers. |
| `ai_api_key` | `string` | `""` | Secret string | ✅ | API key for cloud AI providers. |

---

## 10. FUSE Paths

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `physical_source_path` | `string` | **Required** (no default) | Absolute filesystem path | ❌ | Real MKV storage directory. This is where torrent data is actually stored. |
| `fuse_mount_path` | `string` | **Required** (no default) | Absolute filesystem path | ❌ | FUSE virtual mount point. Where Plex/Jellyfin reads the virtual `.mkv` stub files. |

---

## 11. Disk Warmup

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `disk_warmup_quota_gb` | `int64` | `32` | > 0 | ✅ | Total SSD quota for warmup cache (head/tail regions of files). |
| `warmup_head_size_mb` | `int64` | `64` | > 0 | ✅ | **Deprecated** — now hardcoded at 64 MB. Field retained for backward-compatible JSON unmarshaling. |

---

## 12. NAT-PMP

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `natpmp.enabled` | `bool` | `false` | `true` / `false` | ❌ | Enable NAT-PMP port forwarding (for VPN tunneling). |
| `natpmp.gateway` | `string` | `""` (required when enabled) | IPv4 address | ❌ | VPN gateway IP address. Must be set when enabled. |
| `natpmp.local_port` | `int` | `8091` | > 0 | ❌ | Internal TCP port for port mapping. |
| `natpmp.vpn_interface` | `string` | `"wg0"` | Interface name | ❌ | VPN interface name for `iptables` PREROUTING rules. |
| `natpmp.lifetime` | `int` | `60` | > 0 | ✅ | NAT-PMP mapping lifetime in seconds. |
| `natpmp.refresh` | `int` | `45` | > 0, < lifetime | ✅ | Refresh interval in seconds (must be less than lifetime). |

---

## 13. External Services — Plex / Jellyfin

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `plex.url` | `string` | `""` | HTTP URL | ✅ | Plex or Jellyfin server URL. |
| `plex.token` | `string` | `""` | Auth token | ✅ | Plex authentication token. Empty = no auth. |
| `plex.library_id` | `int` | `0` | ≥ 0 | ✅ | Plex movie library section ID. Used for library refresh triggers after sync. |
| `plex.tv_library_id` | `int` | `0` | ≥ 0 | ✅ | Plex TV library section ID. |
| `media_server_type` | `string` | `""` | `"plex"`, `"jellyfin"`, `""` (auto-detect) | ✅ | Media server type for integration. Auto-detect if empty. |

---

## 14. External Services — Prowlarr / TMDB / Torrentio

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `tmdb_api_key` | `string` | `""` (required for sync) | TMDB API key | ✅ | TMDB API key for movie/TV series discovery. Get one at https://www.themoviedb.org/settings/api |
| `torrentio_url` | `string` | `"https://torrentio.strem.fun"` | HTTP URL | ✅ | Torrentio base URL. Used as fallback when Prowlarr is disabled. |
| `prowlarr.enabled` | `bool` | `false` | `true` / `false` | ✅ | Enable Prowlarr indexer integration. Primary source for torrent/magnet links. |
| `prowlarr.api_key` | `string` | `""` | API key | ✅ | Prowlarr API key. |
| `prowlarr.url` | `string` | `""` | HTTP URL | ✅ | Prowlarr server URL (e.g., `http://localhost:9696`). |

**Fallback behavior:** When `prowlarr.enabled` is `false`, the engine uses Torrentio as the torrent source.

---

## 15. Scheduler

The built-in scheduler replaces system cron for running sync jobs.

### Scheduler Top-Level

| JSON Key | Go Type | Default | Hot Reload | Description |
|----------|---------|---------|:----------:|-------------|
| `scheduler.enabled` | `bool` | `false` | ✅ | Enable the built-in sync scheduler. |

### Movies Sync Job

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `scheduler.movies_sync.enabled` | `bool` | `true` | `true` / `false` | ✅ | Enable movie discovery sync job. |
| `scheduler.movies_sync.days_of_week` | `[]int` | `[1, 4]` (Mon, Thu) | 0–6 (0=Sun, 6=Sat) | ✅ | Days of the week to run the job. |
| `scheduler.movies_sync.hour` | `int` | `3` | 0–23 | ✅ | Hour of the day to run. |
| `scheduler.movies_sync.minute` | `int` | `0` | 0–59 | ✅ | Minute of the hour to run. |

### TV Sync Job

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `scheduler.tv_sync.enabled` | `bool` | `true` | `true` / `false` | ✅ | Enable TV series discovery sync job. |
| `scheduler.tv_sync.days_of_week` | `[]int` | `[3, 5]` (Wed, Fri) | 0–6 | ✅ | Days of the week. |
| `scheduler.tv_sync.hour` | `int` | `4` | 0–23 | ✅ | Hour of the day. |
| `scheduler.tv_sync.minute` | `int` | `0` | 0–59 | ✅ | Minute of the hour. |

### Watchlist Sync Job

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `scheduler.watchlist_sync.enabled` | `bool` | `true` | `true` / `false` | ✅ | Enable Plex cloud watchlist sync job. |
| `scheduler.watchlist_sync.interval_hours` | `int` | `1` | `1`, `2`, `3`, `4`, `6`, `8`, `12`, `24` | ✅ | Interval between watchlist sync runs. Must evenly divide 24. |

**Day of week reference:**

| Value | Day |
|-------|-----|
| `0` | Sunday |
| `1` | Monday |
| `2` | Tuesday |
| `3` | Wednesday |
| `4` | Thursday |
| `5` | Friday |
| `6` | Saturday |

---

## 16. Quality Profiles

Quality profiles control **how torrents are selected and scored** during sync. Two built-in presets are available, and custom overrides can be layered on top.

### Top-Level

| JSON Key | Go Type | Default | Range | Hot Reload | Description |
|----------|---------|---------|-------|:----------:|-------------|
| `quality.profile` | `string` | `"quality-first"` | `"quality-first"` or `"size-first"` | ✅ | Active quality preset. Controls resolution priority order, size ceilings/floors, and scoring weights. |
| `quality.profiles` | `map[string]QualityProfileSet` | `null` (use built-in defaults) | Object with named presets | ✅ | Custom profile overrides. Any field omitted falls back to the built-in default for that preset. |

### Built-in Presets Comparison

#### Movies — `quality-first` (default)

| Parameter | Value |
|-----------|-------|
| **Priority order** | 4K → 1080p → 720p |
| **Include 4K** | ✅ |
| **Include 1080p** | ✅ |
| **Include 720p** | ❌ |
| **Size floor (GB)** | 4K: 10, 1080p: 4 |
| **Size ceiling (GB)** | 4K: 60, 1080p: 20 |
| **Min seeders** | 15 |
| **4K fallback** | Disabled (4K is primary, not fallback) |

#### Movies — `size-first`

| Parameter | Value |
|-----------|-------|
| **Priority order** | 720p → 1080p → 4K |
| **Include 4K** | ✅ (fallback only, requires ≥50 seeders) |
| **Include 1080p** | ✅ |
| **Include 720p** | ✅ |
| **Size floor (GB)** | 720p: 0.5, 1080p: 0.8, 4K: 1 |
| **Size ceiling (GB)** | 720p: 3, 1080p: 5, 4K: 8 |
| **Min seeders** | 15 |
| **4K fallback seeders** | 50 |

#### TV — `quality-first`

| Parameter | Value |
|-----------|-------|
| **Priority order** | 4K → 1080p → 720p |
| **Size ceiling (GB)** | 4K: 30, 1080p: 30 |
| **Min seeders (4K)** | 10 |
| **Min seeders (other)** | 5 |
| **Fullpack bonus** | 500 |

#### TV — `size-first`

| Parameter | Value |
|-----------|-------|
| **Priority order** | 720p → 1080p → 4K |
| **Size ceiling (GB)** | 720p: 1, 1080p: 2, 4K: 3 |
| **Min seeders (4K)** | 50 |
| **Min seeders (other)** | 10 |
| **Fullpack bonus** | 300 |

---

### MovieScoreWeights

All weight values are **pointers** — omit a field to use the preset default.

| JSON Key | quality-first | size-first | Description |
|----------|:-------------:|:----------:|-------------|
| `resolution_4k` | `200` | `100` | Base score for 4K/2160p/UHD resolution. |
| `resolution_1080p` | `50` | `300` | Base score for 1080p resolution. |
| `resolution_720p` | `0` (nil) | `500` | Base score for 720p resolution. |
| `dolby_vision` | `150` | `50` | Bonus for Dolby Vision (`DV`, `DoVi`). |
| `hdr` | `100` | `40` | Bonus for HDR (`HDR`, `HDR10`, `HDR10+`). |
| `hdr10_plus` | `100` | `40` | Bonus for HDR10+ specifically. |
| `atmos` | `50` | `25` | Bonus for Dolby Atmos audio. |
| `audio_5_1` | `25` | `15` | Bonus for 5.1 surround (`5.1`, `DTS`, `DDP5`, `EAC3`, `AC3`). |
| `audio_stereo` | `-50` | `10` | Stereo bonus/penalty. Negative in quality-first, positive in size-first. |
| `bluray` | `10` | `5` | Bonus for BluRay source (`BluRay`, `BDRip`, `BDRemux`). |
| `remux` | **`-500`** | **`-500`** | **Hard veto** — intentionally excludes REMUX releases (very large files). |
| `ita` | `60` | `60` | Bonus for Italian language (`ITA`, `🇮🇹`). |
| `seeder_bonus` | `5` | `5` | Multiplier per seeder (capped at `seeder_threshold`). |
| `seeder_threshold` | `50` | `30` | Max seeder count for bonus calculation. |
| `unknown_size_penalty` | `-5` | `-5` | Penalty when torrent size cannot be determined. |

**How scoring works:**

```
Total Score = Σ(weight for each detected attribute in torrent title)

Example: "Movie.2025.720p.WEB-DL.DDP5.1.ITA.x264"
→ resolution_720p:  500
→ audio_5_1:         15  (DDP5.1 matches)
→ ita:               60
→ Total:            575
```

Negative values are **penalties**. A score of `-500` for `remux` effectively vetoes any title containing "REMUX".

---

### TVScoreWeights

| JSON Key | quality-first | size-first | Description |
|----------|:-------------:|:----------:|-------------|
| `resolution_4k` | `200` | `100` | Base score for 4K. |
| `resolution_1080p` | `50` | `300` | Base score for 1080p. |
| `resolution_720p` | `0` (nil) | `500` | Base score for 720p. |
| `hdr` | `100` | `40` | Bonus for HDR/Dolby Vision. |
| `atmos` | `50` | `25` | Bonus for Dolby Atmos. |
| `audio_5_1` | `25` | `15` | Bonus for 5.1 surround. |
| `ita` | `40` | `40` | Bonus for Italian language. |
| `seeder_100_bonus` | `100` | `100` | Flat bonus for torrents with ≥100 seeders. |
| `seeder_50_bonus` | `50` | `50` | Flat bonus for torrents with ≥50 seeders. |
| `seeder_20_bonus` | `10` | `10` | Flat bonus for torrents with ≥20 seeders. |

---

### Custom Profile Example

```json
{
  "quality": {
    "profile": "size-first",
    "profiles": {
      "size-first": {
        "movies": {
          "include_720p": true,
          "include_1080p": true,
          "include_4k": false,
          "min_seeders": 20,
          "size_ceiling_gb": { "720p": 4, "1080p": 6 },
          "score_weights": {
            "resolution_720p": 600,
            "remux": -500
          }
        },
        "tv": {
          "size_ceiling_gb": { "720p": 1.5, "1080p": 2.5 },
          "fullpack_bonus": 400
        }
      }
    }
  }
}
```

In this example:
- `include_4k` is explicitly `false` (overrides default `true`)
- `min_seeders` is `20` (overrides default `15`)
- Only `resolution_720p` and `remux` weights are customized; all other weights come from the `size-first` default

---

### Merge Strategy

Custom profiles use a **"fill nil"** merge:

1. Start with the built-in default for the active preset (`quality-first` or `size-first`)
2. For each field in the custom profile:
   - If the field is a **pointer** and is `null`/omitted → keep the default
   - If the field is a **pointer** and is set → override with the custom value
   - If the field is a **map** (`size_floor_gb`, `size_ceiling_gb`) and non-empty → replace entirely
   - If the field is a **slice** (`priority_order`) and non-empty → replace entirely
3. Weight sub-objects (`score_weights`): each `*int` field is individually evaluated

**This means you only need to specify the fields you want to change.**

---

## 17. TMDB Discovery

TMDB Discovery controls **which content is found** from The Movie Database. When `tmdb_discovery` is absent or has empty `endpoints`, the engine falls back to hardcoded defaults (backward compatible).

### Top-Level

| JSON Key | Go Type | Default | Hot Reload | Description |
|----------|---------|---------|:----------:|-------------|
| `tmdb_discovery.movies` | `*TMDBEndpointGroup` | `null` (hardcoded defaults) | ✅ | Custom TMDB movie discovery endpoints. |
| `tmdb_discovery.tv` | `*TMDBEndpointGroup` | `null` (hardcoded defaults) | ✅ | Custom TMDB TV discovery endpoints. |

### TMDBEndpointGroup

| JSON Key | Go Type | Description |
|----------|---------|-------------|
| `endpoints` | `[]TMDBEndpoint` | Array of endpoint configurations. Processed in order, deduplicated by TMDB ID. |

### TMDBEndpoint — Required Fields

Only three fields are **required**: `name`, `enabled`, `type`. All other fields are optional — when omitted, the parameter is **not added** to the TMDB API query, and TMDB uses its server-side defaults.

| JSON Key | Go Type | Description |
|----------|---------|-------------|
| `name` | `string` | Unique identifier for this endpoint (used in logs). |
| `enabled` | `bool` | Whether this endpoint is active. Set `false` to disable without removing. |
| `type` | `string` | One of: `"discover"`, `"trending"`, `"list"` |

### TMDBEndpoint — Common Fields (all `discover` endpoints)

| JSON Key | Go Type | Default | Description |
|----------|---------|---------|-------------|
| `language` | `*string` | `"en-US"` | ISO 639-1 language code (e.g., `"en"`, `"it"`, `"fr"`). |
| `sort_by` | `*string` | `"popularity.desc"` | See Sort By Enumerations below. |
| `pages` | `*int` | `1` | Number of pages to fetch (TMDB returns 20 results per page). |
| `vote_average_gte` | `*float64` | — | Minimum vote average. Range: `0.0` – `10.0`. |
| `vote_count_gte` | `*int` | — | Minimum number of votes. Range: `0` – ∞. |
| `with_genres` | `*string` | — | Comma-separated genre IDs (AND) or pipe-separated (OR). See Genre IDs below. |
| `without_genres` | `*string` | — | Comma-separated genre IDs to exclude. |
| `with_keywords` | `*string` | — | Comma-separated keyword IDs (AND) or pipe-separated (OR). |
| `without_keywords` | `*string` | — | Comma-separated keyword IDs to exclude. |
| `with_original_language` | `*string` | — | ISO 639-1 language code for original language. |
| `with_origin_country` | `*string` | — | ISO 3166-1 alpha-2 country code for production country. |
| `with_runtime_gte` | `*int` | — | Minimum runtime in minutes. |
| `with_runtime_lte` | `*int` | — | Maximum runtime in minutes. |
| `watch_region` | `*string` | — | ISO 3166-1 alpha-2 region code. Required for watch provider filtering. |
| `include_adult` | `*bool` | `false` | Include adult-rated content. |

### TMDBEndpoint — Movie-Specific Fields

| JSON Key | Go Type | Default | Description |
|----------|---------|---------|-------------|
| `primary_release_date_gte` | `*string` | — | Minimum release date. Supports `YYYY-MM-DD` or relative: `"-6months"`, `"+1y"`, `"-30d"`. |
| `primary_release_date_lte` | `*string` | — | Maximum release date. Same format. |
| `primary_release_year` | `*int` | — | Exact release year (4-digit). |
| `with_release_type` | `*string` | — | Release type filter. See Release Types below. Pipe or comma separated. |
| `region` | `*string` | — | ISO 3166-1 alpha-2 region code (for certification filtering). |
| `include_video` | `*bool` | `false` | Include video results. |

### TMDBEndpoint — TV-Specific Fields

| JSON Key | Go Type | Default | Description |
|----------|---------|---------|-------------|
| `first_air_date_gte` | `*string` | — | Minimum first air date. Supports `YYYY-MM-DD` or relative. |
| `first_air_date_lte` | `*string` | — | Maximum first air date. Same format. |
| `first_air_date_year` | `*int` | — | Exact first air year (4-digit). |
| `with_status` | `*string` | — | Show status filter. See TV Status below. Pipe or comma separated. |
| `with_type` | `*string` | — | Show type filter. See TV Type below. Pipe or comma separated. |
| `with_networks` | `*string` | — | Comma-separated network IDs (e.g., `213` for Netflix). |
| `include_null_first_air_dates` | `*bool` | `false` | Include shows with missing first air dates. |

### TMDBEndpoint — Type: `list`

| JSON Key | Go Type | Required | Description |
|----------|---------|:--------:|-------------|
| `endpoint` | `*string` | ✅ | TMDB list endpoint path (e.g., `"/movie/now_playing"`, `"/tv/on_the_air"`). |
| `region` | `*string` | — | ISO 3166-1 alpha-2 region code. |
| `pages` | `*int` | — | Number of pages. |

### TMDBEndpoint — Type: `trending`

| JSON Key | Go Type | Required | Description |
|----------|---------|:--------:|-------------|
| `time_window` | `*string` | ✅ | `"day"` or `"week"`. |
| `pages` | `*int` | — | Number of pages. |

---

### Relative Date Syntax

| Format | Example | Meaning |
|--------|---------|---------|
| Absolute | `"2024-01-15"` | Exact date |
| Negative months | `"-6months"` | 6 months ago from today |
| Positive months | `"+12months"` | 12 months from today |
| Negative years | `"-1y"` | 1 year ago |
| Positive years | `"+2y"` | 2 years from now |
| Negative days | `"-30d"` | 30 days ago |
| Positive days | `"+7d"` | 7 days from now |

---

### Enumerations

#### Genre IDs — Movies

| ID | Name (EN) | Name (IT) |
|----|-----------|-----------|
| 28 | Action | Azione |
| 12 | Adventure | Avventura |
| 16 | Animation | Animazione |
| 35 | Comedy | Commedia |
| 80 | Crime | Crime |
| 99 | Documentary | Documentario |
| 18 | Drama | Drammatico |
| 10751 | Family | Famiglia |
| 14 | Fantasy | Fantasy |
| 36 | History | Storia |
| 27 | Horror | Horror |
| 10402 | Music | Musica |
| 9648 | Mystery | Mistero |
| 10749 | Romance | Romance |
| 878 | Science Fiction | Fantascienza |
| 10770 | TV Movie | TV Movie |
| 53 | Thriller | Thriller |
| 10752 | War | Guerra |
| 37 | Western | Western |

#### Genre IDs — TV Series

| ID | Name (EN) | Name (IT) |
|----|-----------|-----------|
| 10759 | Action & Adventure | Azione e Avventura |
| 16 | Animation | Animazione |
| 35 | Comedy | Commedia |
| 80 | Crime | Crime |
| 99 | Documentary | Documentario |
| 18 | Drama | Drammatico |
| 10762 | Kids | Kids |
| 9648 | Mystery | Mistero |
| 10763 | News | Notizie |
| 10764 | Reality | Reality |
| 10765 | Sci-Fi & Fantasy | Sci-Fi & Fantasy |
| 10766 | Soap | Soap |
| 10767 | Talk | Talk |
| 10768 | War & Politics | Guerra e Politica |
| 37 | Western | Western |

**Usage:**
```json
// Include ONLY Action OR Adventure (OR logic with pipe)
"with_genres": "28|12"

// Include Action AND Comedy (AND logic with comma)
"with_genres": "28,35"

// Exclude Documentary AND News (TV)
"without_genres": "99,10763"
```

#### Release Types (Movies)

| ID | Type |
|----|------|
| 1 | Premiere |
| 2 | Theatrical (limited) |
| 3 | Theatrical |
| 4 | Digital |
| 5 | Physical |
| 6 | TV |

#### TV Status

| ID | Status |
|----|--------|
| 0 | Returning Series |
| 1 | Planned |
| 2 | In Production |
| 3 | Torn Off |
| 4 | Ended |
| 5 | Canceled |
| 6 | Pilot |

#### TV Type

| ID | Type |
|----|------|
| 0 | Documentary |
| 1 | News |
| 2 | Miniseries |
| 3 | Reality |
| 4 | Scripted |
| 5 | Talk Show |
| 6 | Video |

#### Sort By — Movies

| Value | Description |
|-------|-------------|
| `popularity.desc` | Most popular |
| `vote_average.desc` | Highest rated |
| `vote_count.desc` | Most votes |
| `primary_release_date.desc` | Newest releases |
| `revenue.desc` | Highest grossing |
| `title.desc` | Title Z→A |
| `original_title.desc` | Original title Z→A |

(Each also has `.asc` for ascending order.)

#### Sort By — TV

| Value | Description |
|-------|-------------|
| `popularity.desc/asc` | Popularity |
| `vote_average.desc/asc` | Rating |
| `vote_count.desc/asc` | Vote count |
| `first_air_date.desc/asc` | Air date |
| `name.desc/asc` | Name |
| `original_name.desc/asc` | Original name |

#### Watch Monetization Types

| Value | Meaning |
|-------|---------|
| `flatrate` | Subscription (Netflix, Prime Video, etc.) |
| `free` | Free |
| `ads` | Ad-supported |
| `rent` | Rental |
| `buy` | Purchase |

#### Language Codes (common)

| Code | Language |
|------|----------|
| `en` | English |
| `it` | Italian |
| `fr` | French |
| `de` | German |
| `es` | Spanish |
| `pt` | Portuguese |
| `ja` | Japanese |
| `ko` | Korean |
| `zh` | Chinese |

#### Region Codes (common)

| Code | Country |
|------|---------|
| `IT` | Italy |
| `US` | United States |
| `GB` | United Kingdom |
| `FR` | France |
| `DE` | Germany |
| `ES` | Spain |

---

### Complete TMDB Discovery Examples

#### Example 1: Recent popular English movies only

```json
{
  "tmdb_discovery": {
    "movies": {
      "endpoints": [
        {
          "name": "recent_popular_en",
          "enabled": true,
          "type": "discover",
          "language": "en",
          "sort_by": "popularity.desc",
          "primary_release_date_gte": "-6months",
          "primary_release_date_lte": "+12months",
          "pages": 12,
          "vote_count_gte": 50,
          "without_genres": "99",
          "include_adult": false
        }
      ]
    },
    "tv": { "endpoints": [] }
  }
}
```

#### Example 2: Highly-rated Italian movies with genre filter

```json
{
  "tmdb_discovery": {
    "movies": {
      "endpoints": [
        {
          "name": "top_rated_it",
          "enabled": true,
          "type": "discover",
          "language": "it",
          "sort_by": "vote_average.desc",
          "primary_release_date_gte": "-1y",
          "pages": 5,
          "vote_average_gte": 7.0,
          "vote_count_gte": 100,
          "with_genres": "18|53|878",
          "without_genres": "99",
          "with_original_language": "it"
        }
      ]
    },
    "tv": { "endpoints": [] }
  }
}
```

#### Example 3: Returning scripted TV only

```json
{
  "tmdb_discovery": {
    "movies": { "endpoints": [] },
    "tv": {
      "endpoints": [
        {
          "name": "returning_scripted",
          "enabled": true,
          "type": "discover",
          "language": "en",
          "sort_by": "popularity.desc",
          "first_air_date_gte": "-6months",
          "pages": 5,
          "vote_average_gte": 7.0,
          "vote_count_gte": 50,
          "with_status": "0",
          "with_type": "4",
          "without_genres": "99,10763,10764,10767"
        },
        {
          "name": "trending_tv",
          "enabled": true,
          "type": "trending",
          "time_window": "week",
          "pages": 3
        }
      ]
    }
  }
}
```

#### Example 4: Netflix originals (network ID 213)

```json
{
  "tmdb_discovery": {
    "movies": { "endpoints": [] },
    "tv": {
      "endpoints": [
        {
          "name": "netflix_originals",
          "enabled": true,
          "type": "discover",
          "language": "en",
          "sort_by": "popularity.desc",
          "with_networks": "213",
          "pages": 5,
          "vote_average_gte": 7.0
        }
      ]
    }
  }
}
```

---

### Hardcoded Default Endpoints (when `tmdb_discovery` is empty)

**Movies (6 endpoints):**
1. `DiscoverMovies` — English, last 6 months → next year, 12 pages
2. `DiscoverMovies` — Italian, last 6 months → next year, 3 pages
3. `DiscoverMoviesByRegion` — `/movie/now_playing`, region US, 1 page
4. `DiscoverMoviesByRegion` — `/movie/now_playing`, region GB, 1 page
5. `DiscoverMoviesByRegion` — `/movie/popular`, region US, 3 pages
6. `TrendingMovies` — weekly, 1 page

**TV (4 endpoints):**
1. `TVOnTheAir` — 3 pages
2. `TVAiringToday` — 2 pages
3. `TVTrending` — weekly, 3 pages
4. `DiscoverTV` — English, last 6 months, 5 pages

---

## 18. Telemetry

| JSON Key | Go Type | Default | Hot Reload | Description |
|----------|---------|---------|:----------:|-------------|
| `telemetry` | `bool` | `true` | ✅ | Enable anonymous telemetry reporting. |
| `telemetry_id` | `string` | Auto-generated UUID | ❌ | Unique installation identifier. Auto-generated on first run. |
| `telemetry_url` | `string` | `"https://telemetry.gostream.workers.dev"` | ✅ | Telemetry endpoint URL. |

---

## 19. State DB

| JSON Key | Go Type | Default | Hot Reload | Description |
|----------|---------|---------|:----------:|-------------|
| `enable_state_db` | `bool` | `true` | ❌ | Enable SQLite state backend. When `false`, falls back to JSON files in STATE directory. |
| `state_db_path` | `string` | `<STATE>/gostream.db` | ❌ | Path to the SQLite state database file. Auto-set to STATE dir if empty. |

The state DB stores: episode registry (TV), scheduler state, negative caches, blacklist.

---

## 20. Directory Structure

```
<RootPath>/
├── STATE/                          # State directory
│   ├── gostream.db                 # SQLite state backend
│   ├── scheduler_state.json        # Scheduler job tracking
│   ├── no_mkv_hashes.json          # Negative cache: torrents with no MKV
│   ├── movie_no_streams_cache.json # Negative cache: no valid streams
│   ├── movie_recheck_cache.json    # Negative cache: recheck queue
│   ├── movie_add_fail_cache.json   # Negative cache: add failures
│   ├── movie_imdb_cache.json       # TMDB → IMDB mapping cache
│   ├── tv_episode_registry.json    # TV episode tracking (JSON fallback)
│   └── blacklist.json              # Blocked hashes and titles
│
<binary_directory>/
├── config.json                     # Main configuration file
├── gostream                        # Compiled binary
├── scripts/                        # Engine scripts
│   ├── gostorm-sync-complete.py
│   ├── gostorm-tv-sync.py
│   └── plex-watchlist-sync.py
└── logs/
    ├── gostream.log                # Main log
    ├── movies-sync.log             # Movie sync engine log
    └── tv-sync.log                 # TV sync engine log
```

**Physical vs Virtual paths:**

| Path | Purpose | Content |
|------|---------|---------|
| `physical_source_path` | Real storage | Torrent data cache, downloaded pieces |
| `fuse_mount_path` | Virtual mount | `.mkv` stub files visible to Plex/Jellyfin |

---

## Quick Reference: Minimal Config

```json
{
  "physical_source_path": "/path/to/gostream-real",
  "fuse_mount_path": "/path/to/gostream-fuse",
  "tmdb_api_key": "YOUR_TMDB_KEY",
  "plex": {
    "url": "http://localhost:32400",
    "token": "YOUR_PLEX_TOKEN",
    "library_id": 1
  },
  "quality": {
    "profile": "size-first"
  },
  "scheduler": {
    "enabled": true,
    "movies_sync": {
      "enabled": true,
      "days_of_week": [1, 4],
      "hour": 3,
      "minute": 0
    },
    "tv_sync": {
      "enabled": true,
      "days_of_week": [3, 5],
      "hour": 4,
      "minute": 0
    },
    "watchlist_sync": {
      "enabled": true,
      "interval_hours": 1
    }
  }
}
```

All other fields use sensible defaults.
