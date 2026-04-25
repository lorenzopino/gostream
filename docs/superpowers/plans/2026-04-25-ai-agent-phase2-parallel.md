# Phase 2: Hermes Skills + GoStream Extensions — Parallel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan. Tasks within the same Wave can be dispatched to parallel subagents. Waves must complete in order.

**Goal:** Complete the AI maintenance system — deploy Hermes skills, extend GoStream detectors, add favorites check with TMDB/Jellyfin integration, and ensure Mac restart resilience.

**Architecture:** Two parallel work streams — Hermes skills (Markdown/Python files) and GoStream extensions (Go code) — converge in Wave 2, then integration-test in Wave 3.

**Tech Stack:** Go 1.24+ (GoStream extensions), Markdown (Hermes skills), Python (Hermes tool wrappers), curl (API calls).

---

## Parallelization Strategy

### Wave 1: 8 Fully Parallel Subagents (no file conflicts)

Each subagent creates **new files only** — zero risk of merge conflicts:

| ID | Subagent | Creates | Time |
|---|---|---|---|
| **W1-A** | Extended Detectors | `detectors_extended.go` | ~15 min |
| **W1-B** | Favorites Check Endpoint | `favorites_check.go` | ~15 min |
| **W1-C** | Hermes Dispatcher Skill | `gostream-dispatcher.md` | ~10 min |
| **W1-D** | Hermes Maintenance Skill | `gostream-maintenance.md` | ~20 min |
| **W1-E** | Hermes Jellyfin + Prowlarr Skills | `jellyfin-health.md`, `prowlarr-search.md` | ~15 min |
| **W1-F** | Hermes TMDB + Deep Scan Skills | `tmdb-lookup.md`, `gostream-deep-scan.md` | ~15 min |
| **W1-G** | Hermes Code Fix Skill | `gostream-code-fix.md` | ~10 min |
| **W1-H** | Action History Store | `action-history-schema.json` + skill file | ~10 min |

**These 8 run in parallel** — no shared files, no dependencies between them.

### Wave 2: 2 Sequential Subagents (modify existing files)

Must run after Wave 1 completes, one at a time to avoid conflicts:

| ID | Subagent | Modifies | Depends On |
|---|---|---|---|
| **W2-A** | Enhanced Logging | `webhook.go`, `main.go` (webhook handler), FUSE files | Phase 1 code |
| **W2-B** | Mac Resilience | `agent.go`, `main.go` (startup), `config.go`, config.json.example | Phase 1 + W1-A |

### Wave 3: 1 Integration Subagent

| ID | Subagent | Scope | Depends On |
|---|---|---|---|
| **W3-A** | Integration + Resilience Test | End-to-end verification, restart recovery test | Wave 2 complete |

---

## Wave 1: Task Specifications

### W1-A: Extended Detectors

**File:** Create `internal/ai-agent/detectors_extended.go`

Add 5 new detector goroutines to complement the existing TorrentHealth and LogMonitor (Phase 1):

1. **StartupLatency Detector** — Observes playback registry for slow startups:
   - Monitors `logs/gostream-ai.log` and main log for "startup" timing entries
   - Detects `slow_startup` (>15s to first byte) and `timeout_startup` (>30s)
   - Uses playback registry to correlate torrent ID with startup time
   - Priority: B

2. **WebhookMatcher Detector** — Detects unconfirmed plays:
   - Scans main log for "webhook" entries (the existing Plex/Jellyfin webhook handler logs match attempts)
   - Detects `unconfirmed_play` (play started without webhook match) and `wrong_match` (webhook matched different content)
   - Priority: A

3. **FuseAccess Detector** — Detects FUSE mount issues:
   - Polls FUSE mount accessibility (stat the mount point)
   - Detects `fuse_error` (mount inaccessible) and `read_stall` (>5s per block read)
   - Uses the existing FUSE layer metrics if available
   - Priority: B

4. **SubtitleChecker Detector** — Checks for missing English subtitles:
   - After playback completes (detected via webhook `media.scrobble` event), queries Jellyfin API to check if English subtitles are available for the item
   - Detects `missing_subtitles` (no ENG subtitle)
   - Priority: D

5. **SeriesCompleteness Detector** — Checks favorited TV series:
   - Periodically (every 30 min) compares favorited series in Jellyfin against TMDB episode counts
   - Detects `incomplete_series` (missing seasons/episodes)
   - Priority: C

**Dependencies:** Uses types from Phase 1 (`Issue`, `PriorityB`, `TypeDeadTorrent`, etc.). Calls Jellyfin API (needs `globalConfig.Jellyfin.URL` and `globalConfig.Jellyfin.APIKey`) and TMDB API (needs `globalConfig.TMDBAPIKey`).

**Implementation note:** Each detector is a separate goroutine with its own ticker + stopCh pattern, following the existing detectors.go convention. They share the same `Buffer` and `AILogger` injected via constructor.

---

### W1-B: Favorites Check Endpoint

**File:** Create `internal/ai-agent/favorites_check.go`

Implement the full `/api/ai/favorites-check` endpoint (Phase 1 has a stub that just lists torrents):

**Endpoints:**

| Endpoint | Method | Purpose |
|---|---|---|
| `GET /api/ai/favorites-check` | GET | Returns completeness report for all favorited content |
| `GET /api/ai/favorites-check?type=movie&tmdb_id=X` | GET | Check single movie pre-download status |
| `GET /api/ai/favorites-check?type=tv&tmdb_id=X` | GET | Check TV series completeness |

**Movie Check Logic:**
1. Query GoStorm API for the torrent associated with the movie (via `.mkv` stub metadata)
2. Check download completion % (GoStorm reports bytes downloaded vs total)
3. If < 100% → issue `incomplete_download`
4. If 100% but torrent has 0 seeders → issue `dead_torrent` (can't stream)

**TV Series Check Logic:**
1. Query TMDB API for series details (season count, episode count per season)
2. Query Jellyfin API for available episodes of the series
3. Query GoStream episode registry for downloaded episodes
4. Compare TMDB vs Jellyfin vs GoStream → identify missing episodes
5. If missing → issue `incomplete_series` with details

**Response Format:**
```json
{
  "movies": [
    {
      "tmdb_id": "123",
      "title": "The Matrix",
      "download_pct": 100,
      "seeders": 150,
      "status": "ready"
    }
  ],
  "tv_series": [
    {
      "tmdb_id": "456",
      "title": "Breaking Bad",
      "tmdb_seasons": 5,
      "tmdb_episodes": 62,
      "available_episodes": 58,
      "missing": ["S05E14", "S05E15", "S05E16"],
      "status": "incomplete"
    }
  ]
}
```

**Dependencies:** Needs `http.Client` for TMDB (`api.themoviedb.org`) and Jellyfin APIs. Uses `globalConfig.Jellyfin` and `globalConfig.TMDBAPIKey`.

---

### W1-C: Hermes Dispatcher Skill

**File:** `~/.hermes/skills/gostream-dispatcher.md` (create file in plan, user deploys to Hermes)

**Trigger:** Webhook from GoStream (IssueBatch POST) OR cron job trigger

**Behavior:**
1. Parse incoming webhook payload (IssueBatch JSON)
2. Check: "Is a subagent already processing GoStream issues?"
3. If busy → enqueue batch to `~/.hermes/gostream-queue.json` (append with priority)
4. If free → spawn isolated subagent with the batch
5. On cron (~30 min): trigger deep scan by calling the deep scan skill

**Queue Behavior:**
- File: `~/.hermes/gostream-queue.json`
- Format: Array of `{batch, status: "pending|processing|failed", priority}`
- On dispatch: dequeue highest-priority pending batch, mark as "processing"
- On completion: mark as "complete" and remove after 24h
- On failure: mark as "failed", retry up to 3 times

**Priority ordering:** B (1) > A (2) > C (3) > D (4) — same as GoStream

---

### W1-D: Hermes Maintenance Skill (Main)

**File:** `~/.hermes/skills/gostream-maintenance.md`

**Activation:** By dispatcher when subagent starts with an IssueBatch

**Workflow:**
1. Read the IssueBatch
2. **Check action history** (`~/.hermes/gostream-action-history.json`) — skip if same issue addressed < 30 min ago
3. For each issue, investigate using tool calls:
   - `curl` to GoStream `/api/ai/*` endpoints for torrent state
   - `curl` to Jellyfin API for playback/subtitle info
   - `curl` to Prowlarr API for alternatives
   - `curl` to TMDB API for series completeness
4. Categorize actions:
   - **LOW-RISK** (auto-execute): download subtitles, retry dead torrents, re-queue failed downloads
   - **HIGH-RISK** (Telegram approval): remove/replace movie, change quality selection, delete files, code changes
   - **STUCK** (ask user): unclear situation, needs human judgment
5. Execute approved actions
6. Save solution to action history + update skill if new pattern discovered
7. Notify user via Telegram (result summary)

**User Preferences (hardcoded in skill):**
- Torrent selection: smallest file + most seeders
- Issue priority: B > A > C > D
- Favorites: movies must be 100% pre-downloaded, TV series must be complete
- Communication: Telegram via Hermes gateway (already configured)

**Resource Request Pattern:**
If the agent identifies a missing resource (API key, tool, config, access):
```
🔧 I need: [specific thing]
To: [what this would enable]
Currently: [what happens without it]
Reply: provide it / skip / any question
```

---

### W1-E: Hermes Jellyfin + Prowlarr Skills

**File 1:** `~/.hermes/skills/jellyfin-health.md`

**Capabilities:**
- `GET /System/Info` → Service health
- `GET /Sessions` → Active playback sessions
- `GET /Items/{id}` → Item details (subtitle tracks, audio tracks)
- `GET /Items/{id}/PlaybackInfo` → Stream availability
- `POST /Items/{id}/Refresh` → Trigger library refresh
- **Subtitle check**: Parse item's `MediaStreams` for `Type: "Subtitle"` and `Language: "eng"`
- **Subtitle download**: Use Jellyfin's OpenSubtitles plugin endpoint (if configured) or suggest manual download
- **Health reporting**: Return structured health status for the maintenance skill

**File 2:** `~/.hermes/skills/prowlarr-search.md`

**Capabilities:**
- `GET /api/v1/search?query=X&categories=Y` → Search torrents
- Filter results by: resolution, seeders, size, quality
- Score results per user preferences (small file + most seeders)
- Extract magnet link and title from result
- Return top 3 alternatives with comparison table

**Configuration:** Both skills expect `JELLYFIN_URL`, `JELLYFIN_API_KEY`, `PROWLARR_URL`, `PROWLARR_API_KEY` as environment variables or in a `~/.hermes/config/gostream.json` config file.

---

### W1-F: Hermes TMDB + Deep Scan Skills

**File 1:** `~/.hermes/skills/tmdb-lookup.md`

**Capabilities:**
- `GET /3/search/movie?query=X&api_key=Y` → Search movies
- `GET /3/search/tv?query=X&api_key=Y` → Search TV series
- `GET /3/tv/{id}?api_key=Y` → Series details (season count)
- `GET /3/tv/{id}/season/{n}?api_key=Y` → Season episode count
- `GET /3/movie/{id}/external_ids?api_key=Y` → Get IMDB ID
- `GET /3/tv/{id}/external_ids?api_key=Y` → Get IMDB ID

**Use cases:**
- Verify if a TV series is complete (compare TMDB episode count vs available)
- Resolve TMDB ID from IMDB ID (and vice versa)
- Get episode titles/numbers for missing episode identification

**File 2:** `~/.hermes/skills/gostream-deep-scan.md`

**Trigger:** Cron job every ~30 minutes

**Checks (executed sequentially):**
1. All torrents: seeders, download speed, health (via `/api/ai/torrent-state`)
2. Favorites: movie pre-download completion %, TV series completeness (via `/api/ai/favorites-check`)
3. Subtitles: missing English subtitles on recently-played content (via Jellyfin health skill)
4. System health: GoStream, Jellyfin, Prowlarr connectivity
5. Continuous improvement: performance metrics, config tuning opportunities

**Output:** If issues found → creates IssueBatch and dispatches to maintenance skill. If all clear → logs "deep scan: all healthy" to `~/.hermes/gostream-scan.log`.

---

### W1-G: Hermes Code Fix Skill

**File:** `~/.hermes/skills/gostream-code-fix.md`

**Trigger:** When maintenance skill determines a code/config change is needed

**Workflow:**
1. Check git status, recent commits (`git log -10 --oneline`, `git status`)
2. Create git worktree: `git worktree add ../gostream-fix-{issue_type}-{timestamp} -b fix/{issue_type}`
3. Make focused, atomic change in the worktree
4. Build: `go build -o /dev/null .`
5. Test: `go test ./... -count=1`
6. Commit: `git add ... && git commit -m "fix: {description} ({issue_type})"`
7. Report branch path and commit to user via Telegram
8. Wait for user merge decision

**Rules:**
- Never commit to the main worktree
- Always create a new branch per fix
- Never force-push or amend commits
- If build or test fails → report failure to user, don't commit

---

### W1-H: Action History Store

**File 1:** `~/.hermes/gostream-action-history-schema.json` — JSON schema for action history entries

**File 2:** `~/.hermes/skills/gostream-action-history.md` — Skill for reading/writing action history

**Schema:**
```json
{
  "actions": [
    {
      "ts": "2026-04-25T09:00:00Z",
      "issue_type": "dead_torrent",
      "file": "The.Matrix.1999.REMUX.4K.mkv",
      "torrent_id": "abc123",
      "root_cause": "Original torrent had 0 seeders for 48h",
      "action": "replaced",
      "details": "Found alternative with 150 seeders, same quality, 12GB (was 28GB)",
      "result": "success",
      "confidence": 0.95,
      "user_approved": true,
      "lessons": "For REMUX 4K, prefer torrents with >50 seeders and size within 5% of original"
    }
  ]
}
```

**Skill capabilities:**
- Read last N actions (for pre-action check)
- Append new action
- Search by issue type or file name
- Prune entries older than 7 days

---

## Wave 2: Task Specifications

### W2-A: Enhanced Logging

**Modifies:** Existing GoStream files (not ai-agent package)

**Goal:** Add structured logging to under-instrumented areas so the LogMonitor detector has richer data.

**Changes:**

1. **Webhook matching** — In `main.go` `handlePlexWebhook`:
   - Log each match attempt with confidence score
   - Log IMDB ID, hash8, title matching results
   - Log "no match" and "unconfirmed" events

2. **FUSE access** — In FUSE layer files:
   - Log read latency per file (sampled, not every read)
   - Log stall detection (>5s without data)
   - Log mount health check results

3. **Subtitle checks** — In any subtitle-related code:
   - Log subtitle query results from Jellyfin/OpenSubtitles
   - Log when subtitles are found vs not found

**Pattern:** Use the same structured JSON format as `AILogger` — each entry writes to `logs/gostream-ai.log` with `ts`, `level`, `detector`, `issue`, etc.

---

### W2-B: Mac Resilience

**Modifies:** `agent.go`, `main.go`, `config.go`, `config.json.example`

**Goal:** Ensure the entire AI agent subsystem survives and recovers from a macOS restart.

**Changes:**

1. **Startup ordering in `main.go`**:
   - Verify GoStorm API is responding before starting AI agent detectors
   - Add retry logic: if `/torrents` fails, wait 5s and retry (up to 3 times)
   - If all retries fail → log warning, skip detector startup (queue will catch up later)

2. **Queue recovery in `agent.go`**:
   - On startup, reload pending batches from disk queue
   - If queue file is corrupted → log warning, start fresh (batches will be re-detected)
   - If webhook URL is not yet configured → queue persists, no crash

3. **Config defaults in `config.go`**:
   - All `AIAgentConfig` fields have safe defaults (disabled, empty URL, reasonable timeouts)
   - If config.json is missing the `ai_agent` section → uses defaults, no crash

4. **launchd verification**:
   - Document the required launchd settings: `RunAtLoad: true`, `KeepAlive`, `ThrottleInterval: 10`
   - These are already configured for GoStream — just document for Hermes

5. **Graceful degradation**:
   - If Hermes is unreachable → webhook fails gracefully, batch stays in queue
   - If Jellyfin is down → detectors skip subtitle checks, log warning
   - If TMDB API is rate-limited → back off and retry on next cycle

---

## Wave 3: Task Specifications

### W3-A: Integration + Resilience Test

**Scope:** End-to-end verification

**Checklist:**

1. **GoStream → Hermes flow:**
   - Enable AI agent in config.json
   - Point webhook_url to a test endpoint (e.g., `http://localhost:9999/test`)
   - Start GoStream, verify detectors emit entries to `logs/gostream-ai.log`
   - Verify buffer flushes after timeout/size trigger
   - Verify queue persists to disk
   - Verify webhook POSTs to configured URL

2. **API endpoints:**
   - `curl localhost:9080/api/ai/torrent-state?id=test` → 400 with schema_hint
   - `curl localhost:9080/api/ai/queue-status` → JSON with counts
   - `curl localhost:9080/api/ai/favorites-check` → JSON with torrent list

3. **Hermes skills:**
   - Verify all 7 skill files exist in `~/.hermes/skills/`
   - Verify action history file exists at `~/.hermes/gostream-action-history.json`
   - Verify queue file exists at `~/.hermes/gostream-queue.json`

4. **Mac restart resilience:**
   - Restart GoStream (or simulate with stop/start)
   - Verify queue reloads from disk
   - Verify detectors restart without crash
   - Verify no data loss beyond in-flight buffer entries

5. **Build verification:**
   - `go build ./...` → clean
   - `go test ./internal/ai-agent/... -v` → all pass

---

## Dependency Graph

```
Wave 1 (8 parallel):
  W1-A ───────┐
  W1-B ───────┤
  W1-C ───────┤
  W1-D ───────┤
  W1-E ───────┤  ← All independent, no cross-file dependencies
  W1-F ───────┤
  W1-G ───────┤
  W1-H ───────┘

Wave 2 (2 sequential):
  W2-A (Enhanced Logging)    ← Depends on Phase 1 code existing
  W2-B (Mac Resilience)      ← Depends on Phase 1 + W1-A (detector types known)

Wave 3 (1):
  W3-A (Integration Test)    ← Depends on Wave 2 complete
```

## Execution Order Summary

| Wave | Subagents | Parallel? | Duration |
|------|-----------|-----------|----------|
| 1 | 8 | ✅ All 8 in parallel | ~20 min |
| 2 | 2 | ❌ Sequential (W2-A then W2-B) | ~20 min |
| 3 | 1 | ✅ | ~10 min |
| **Total** | | | **~50 min** |
