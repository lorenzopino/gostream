# AI Maintenance Agent — Design Spec

**Date:** 2026-04-24  
**Status:** Draft — awaiting review  
**Scope:** Embedded monitoring in GoStream + Hermes Agent skills + MCP tools for autonomous system maintenance

---

## 1. Problem Statement

GoStream currently has automated sync engines and a 24-hour health checker, but many issues require manual intervention:

- Movies that won't start (few seeders, slow startup, errors) — **highest priority**
- Wrong movies streaming (CAM, wrong language, wrong version)
- Incomplete TV series from favorites
- Missing subtitles (especially English)
- Dead torrents that the existing health checker fails to replace
- General system maintenance and performance tuning

The user wants an LLM-powered agent that can autonomously detect, diagnose, and fix these issues, with Telegram communication for high-risk decisions.

---

## 2. Architecture Overview

```
┌──────────────────────────────────────────────────────────┐
│                    GoStream (Go)                          │
│                                                          │
│  ┌──────────────┐  ┌──────────────────┐  ┌────────────┐  │
│  │ Issue        │  │ LogMonitor       │  │ Deep Scan  │  │
│  │ Detectors    │  │ (heuristic)      │  │ Response   │  │
│  └──────┬───────┘  └────────┬─────────┘  └─────┬──────┘  │
│         │                   │                   │         │
│         ▼                   ▼                   ▼         │
│  ┌─────────────────────────────────────────────────────┐  │
│  │           IssueBuffer (debounce 5 min)              │  │
│  │           dedup + priority + merge                  │  │
│  └──────────────────────┬──────────────────────────────┘  │
│                         │                                  │
│              (flush: timeout or size)                      │
│                         │                                  │
│              ┌──────────▼──────────┐                       │
│              │ Queue (disk JSON)   │                       │
│              │ issues-batch.json   │                       │
│              └──────────┬──────────┘                       │
│                         │                                  │
│              ┌──────────▼──────────┐                       │
│              │ Webhook Pusher      │                       │
│              │ POST → Hermes       │                       │
│              └─────────────────────┘                       │
│                         │                                  │
│                    MCP Server (future)                     │
└─────────────────────────┼──────────────────────────────────┘
                          │ HTTP webhook
                          ▼
┌──────────────────────────────────────────────────────────┐
│                    Hermes Agent                           │
│                                                          │
│  ┌───────────────┐                                       │
│  │ Webhook       │                                       │
│  │ Receiver      │                                       │
│  └───────┬───────┘                                       │
│          │                                                │
│          ▼                                                │
│  ┌───────────────┐        ┌─────────────────────┐        │
│  │ Dispatcher    │───────►│ Queue (Hermes-side) │        │
│  │ Skill         │        │ dedup + priority    │        │
│  │ (busy check)  │        └──────────┬──────────┘        │
│  └───────────────┘                   │                    │
│          │                           ▼                    │
│          │              ┌─────────────────────┐           │
│          ├─────────────►│ Subagent (isolated) │           │
│          │              │                     │           │
│          │              │ 1. Read batch       │           │
│          │              │ 2. Check history    │           │
│          │              │ 3. Activate skill   │           │
│          │              │ 4. Investigate (MCP)│           │
│          │              │ 5. Decide + act     │           │
│          │              │ 6. Save solution    │           │
│          │              │ 7. Notify Telegram  │           │
│          │              └─────────────────────┘           │
│          │                                                │
│  ┌───────▼───────┐                                        │
│  │ Periodic      │                                        │
│  │ Deep Scan     │                                        │
│  │ (cron ~30min) │──► same subagent flow                  │
│  └───────────────┘                                        │
│                                                          │
│  ┌───────────────────┐                                    │
│  │ Telegram Gateway  │◄─── Bidirectional (approve/ask)   │
│  └───────────────────┘                                    │
└──────────────────────────────────────────────────────────┘
```

### Three Entry Points

| Entry Point | Mechanism | When |
|---|---|---|
| **Real-time** | GoStream IssueBuffer → webhook → Hermes | Live events (playback failure, torrent death, FUSE error) |
| **Log Monitor** | GoStream log tail → heuristic → webhook → Hermes | Error spikes, unknown patterns in logs |
| **Deep Scan** | Hermes cron (~30 min) → full audit via MCP tools | Incomplete series, missing subtitles, quality check |

---

## 3. GoStream Components (Embedded in Go)

### 3.1 Issue Detectors

Each detector is a lightweight goroutine observing an existing signal source:

| Detector | Source | Issue Types | Priority |
|---|---|---|---|
| **TorrentHealth** | Health checker + GoStorm API (`:8090/torrents`) | `dead_torrent` (0 seeders), `low_seeders` (<3), `no_download` (0 KBps after 60s) | B |
| **StartupLatency** | Playback registry + webhook timing | `slow_startup` (>15s first byte), `timeout_startup` (>30s) | B |
| **WebhookMatcher** | Plex/Jellyfin webhook handler | `wrong_match` (title mismatch), `unconfirmed_play` | A |
| **FuseAccess** | FUSE Read/Lookup/Getattr | `fuse_error` (mount inaccessible), `read_stall` (>5s/block) | B |
| **LogMonitor** | Tail `gostream.log` with heuristic | `error_spike` (>5 errors in 2 min), `pattern_anomaly` (unknown pattern) | B/A |
| **SubtitleChecker** | Jellyfin API (post-play) | `missing_subtitles` (no ENG subtitle available) | D |
| **SeriesCompleteness** | Episode registry + TMDB (periodic) | `incomplete_series` (missing seasons/episodes in favorites) | C |
| **FavoritesCheck** | GoStorm + Jellyfin + TMDB (periodic) | `incomplete_download` (movie not 100% pre-downloaded), `incomplete_season_pack` | C |

### 3.2 Issue Buffer (Debounce + Dedup + Priority)

**Location:** `internal/ai-agent/buffer.go`

**Behavior:**
- Accumulates issues in a sliding window (default: 5 minutes)
- **Flush triggers:**
  - Timeout: 5 minutes without new issues
  - Size: max 20 issues per batch
- **Deduplication:** Same torrent/file/issue_type → merge into single entry with counter
- **Priority ordering:** B > A > C > D (by annoyance ranking)
- On flush → writes batch to disk queue + sends webhook to Hermes

**Issue struct:**
```go
type Issue struct {
    Type       string            `json:"type"`        // e.g., "dead_torrent"
    Priority   string            `json:"priority"`    // "B", "A", "C", "D"
    TorrentID  string            `json:"torrent_id,omitempty"`
    File       string            `json:"file,omitempty"`
    IMDBID     string            `json:"imdb_id,omitempty"`
    Details    map[string]any    `json:"details"`     // detector-specific context
    FirstSeen  time.Time         `json:"first_seen"`
    Occurrences int              `json:"occurrences"`
    LogSnippet string            `json:"log_snippet,omitempty"` // for pattern_anomaly
}

type IssueBatch struct {
    ID       string   `json:"id"`
    Issues   []Issue  `json:"issues"`
    Created  time.Time `json:"created"`
    Source   string   `json:"source"` // "realtime", "log_monitor", "deep_scan"
}
```

### 3.3 Queue (Disk-based)

**Location:** `state/ai-agent-queue.json`

Simple JSON file on disk — survives restarts:
```json
{
  "batches": [
    {
      "id": "batch-20260424-103000",
      "issues": [...],
      "created": "2026-04-24T10:30:00Z",
      "source": "realtime",
      "status": "pending"
    }
  ]
}
```

### 3.4 Webhook Pusher

**Location:** `internal/ai-agent/webhook.go`

- POSTs batch to Hermes webhook endpoint (configurable URL in `config.json`)
- Retries on failure (3 attempts, exponential backoff)
- If Hermes unreachable → batch stays in queue for next retry cycle

### 3.5 Enhanced Structured Logging

**File:** `logs/gostream-ai.log` (separate from main log)

All detectors emit structured JSON log entries:
```json
{"ts":"2026-04-24T10:30:00Z","level":"warn","detector":"torrent_health",
 "issue":"dead_torrent","torrent_id":"abc123","file":"Movie.mkv",
 "seeders":0,"peers":0,"age_seconds":3600,"action_needed":"replace"}
```

**Coverage additions** (areas currently under-instrumented):
- Webhook matching: log match attempts, failures, confidence scores
- Torrent health: log seeders change over time, not just point-in-time
- FUSE access: log read latency per file, stall detection
- Subtitle checks: log subtitle query results from Jellyfin/OpenSubtitles

### 3.6 MCP Server (GoStream side)

**Location:** `internal/ai-agent/mcp_server.go`

Lightweight HTTP endpoint exposing GoStream state to Hermes MCP tools:

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/ai/torrent-state` | GET | `get_torrent_state` tool |
| `/api/ai/active-playback` | GET | `list_active_playback` tool |
| `/api/ai/fuse-health` | GET | `get_fuse_health` tool |
| `/api/ai/replace-torrent` | POST | `replace_torrent` tool |
| `/api/ai/remove-torrent` | POST | `remove_torrent` tool |
| `/api/ai/add-torrent` | POST | `add_torrent` tool |
| `/api/ai/config` | GET/PUT | `get_config` / `set_config` tools |
| `/api/ai/recent-logs` | GET | `get_recent_logs` tool |
| `/api/ai/queue-status` | GET | `get_queue_status` tool |
| `/api/ai/favorites-check` | GET | Check favorites completeness |

---

## 4. Hermes Components

### 4.1 Dispatcher Skill

**File:** `~/.hermes/skills/gostream-dispatcher.md`

**Behavior:**
1. Receives webhook payload (IssueBatch)
2. Checks: "Is a subagent already active for this batch?"
3. If no → spawn isolated subagent with the batch
4. If yes → enqueue batch (append to `~/.hermes/gostream-queue.json`)
5. Queue is ordered by priority (B > A > C > D)
6. Dispatcher is also triggered by cron (~30 min) for deep scan

### 4.2 Maintenance Skill (Main)

**File:** `~/.hermes/skills/gostream-maintenance.md`

**Activation:** By dispatcher when subagent starts

**Workflow:**
1. Read the IssueBatch
2. **Check action history** (`~/.hermes/gostream-action-history.json`) — skip if same issue was addressed < 30 min ago
3. For each issue, investigate using MCP tools:
   - Query GoStream MCP for torrent state
   - Query Jellyfin MCP for playback/subtitle info
   - Query Prowlarr MCP for alternatives
   - Query TMDB MCP for series completeness data
4. Categorize actions:
   - **LOW-RISK** (auto-execute): download subtitles, retry dead torrents, re-queue failed downloads
   - **HIGH-RISK** (Telegram approval): remove/replace movie, change quality selection, delete files, code changes
   - **STUCK** (ask user): unclear situation, needs human judgment
5. Execute approved actions
6. Save solution to action history + update skill if new pattern discovered
7. Notify user via Telegram (result summary)

### 4.3 Code Fix Skill

**File:** `~/.hermes/skills/gostream-code-fix.md`

**Trigger:** When maintenance skill determines a code/config change is needed

**Workflow:**
1. Check git status, recent commits
2. Create git worktree: `git worktree add ../gostream-fix-<issue-id> -b fix/<issue-id>`
3. Make focused, atomic change
4. Test (build + relevant test)
5. Commit: `fix: <what> (<issue_id>)`
6. Report branch and commit to user
7. Clean up worktree after merge

### 4.4 Jellyfin Health Skill

**File:** `~/.hermes/skills/jellyfin-health.md`

**Tools:** Jellyfin REST API calls via shell (`curl`) or Hermes MCP tools

**Capabilities:**
- Check service health
- Check active playback sessions
- Check subtitle availability (including OpenSubtitles plugin — already installed in Jellyfin)
- Download subtitles via OpenSubtitles plugin (may require API key — request via Resource Request if not configured)
- Check library sync state
- Trigger library refresh

### 4.5 Prowlarr Search Skill

**File:** `~/.hermes/skills/prowlarr-search.md`

**Capabilities:**
- Search for torrent alternatives
- Filter by quality, size, seeders
- Compare results (user preference: smallest file + most seeders)
- Extract magnet links

### 4.6 TMDB Lookup Skill

**File:** `~/.hermes/skills/tmdb-lookup.md`

**Capabilities:**
- Search TMDB by title/IMDB ID
- Get season/episode counts
- Compare available episodes vs. TMDB data
- Identify missing content

### 4.7 Deep Scan Skill (Cron)

**File:** `~/.hermes/skills/gostream-deep-scan.md`

**Trigger:** Hermes cron job every ~30 minutes

**Checks:**
1. All torrents: seeders, download speed, health
2. Favorites: movie pre-download completion %, TV series completeness
3. Subtitles: missing English subtitles on recently-played content
4. System health: GoStream, Jellyfin, Prowlarr connectivity
5. Continuous improvement: performance metrics, config tuning opportunities

---

## 5. MCP Tools (Hermes-side)

### GoStream MCP Tools (Phase 1: REST calls via shell/curl)

**Note:** In Phase 1, Hermes calls GoStream's `/api/ai/*` endpoints directly via shell commands (`curl`). A proper MCP server adapter (Phase 2) provides tool-native integration. Each tool maps to a GoStream REST endpoint:

| Tool | GoStream Endpoint | Action |
|---|---|---|
| `get_torrent_state` | `GET /api/ai/torrent-state?id=X` | Get detailed torrent health |
| `list_active_playback` | `GET /api/ai/active-playback` | Get current playback sessions |
| `get_fuse_health` | `GET /api/ai/fuse-health` | FUSE mount health |
| `replace_torrent` | `POST /api/ai/replace-torrent` | Replace a torrent with a new magnet |
| `remove_torrent` | `POST /api/ai/remove-torrent` | Remove a torrent |
| `add_torrent` | `POST /api/ai/add-torrent` | Add a torrent |
| `get_config` | `GET /api/ai/config` | Get current config |
| `set_config` | `PUT /api/ai/config` | Apply config change (high-risk) |
| `get_recent_logs` | `GET /api/ai/recent-logs` | Get recent log entries |
| `get_queue_status` | `GET /api/ai/queue-status` | Check pending issue batches |
| `favorites_check` | `GET /api/ai/favorites-check` | Verify favorites completeness |

### Jellyfin MCP Tools (call Jellyfin REST API)

| Tool | Action |
|---|---|
| `get_health` | Jellyfin service status |
| `get_playback_info` | Active session details, subtitle/audio tracks |
| `check_subtitles` | Subtitle availability (incl. OpenSubtitles) |
| `download_subtitle` | Download subtitle from OpenSubtitles |
| `get_library_status` | Library scan state |
| `trigger_library_scan` | Start library refresh |

### Prowlarr MCP Tools (call Prowlarr REST API)

| Tool | Action |
|---|---|
| `search` | Search for torrents |
| `get_indexer_health` | Indexer connectivity |
| `get_result_details` | Torrent details (magnet, tracker) |

### TMDB MCP Tools (call TMDB REST API)

| Tool | Action |
|---|---|
| `search` | Search movies/shows |
| `get_details` | Movie/show details by TMDB ID |
| `get_seasons` | Season list with episode counts |
| `get_episodes` | Episode list for a season |

---

## 6. Communication Pattern (Telegram via Hermes Gateway)

### Message Types

| Type | Format | Example |
|---|---|---|
| **Notification** | "✅ X fixed: details" | "✅ Dead torrent fixed: The Matrix → replaced with 150-seeder alternative (4K REMUX, 12GB)" |
| **Approval** | "⚠️ HIGH-RISK: problem → proposed fix → reply yes/no" | "⚠️ Replace 'Movie X'. New: 4K, Italian, 12GB (was 28GB). OK?" |
| **Question** | "❓ STUCK: context → question → free reply" | "❓ 3 alternatives for Movie X but none have Italian audio. Proceed with best available or wait?" |
| **Resource Request** | "🔧 I need: [what] to [why]" | "🔧 I need: OpenSubtitles API key to auto-download English subtitles for Movie Y. Can you provide it?" |

### Resource Request Format

When the agent identifies a gap in its capabilities (missing API key, unconfigured tool, inaccessible system, missing documentation), it sends:

```
🔧 RESOURCE REQUEST

I need: [specific thing — API key, tool, config, access]
To: [what this would enable the agent to do]
Currently: [what happens without it — failing, skipping, etc.]

Reply: provide it / skip / any question
```

Examples of resource requests:
- "🔧 I need: OpenSubtitles API credentials to download English subtitles automatically. Currently skipping subtitle fixes for 3 movies."
- "🔧 I need: Jellyfin API key with subtitle management permissions. Currently can check but not download subtitles."
- "🔧 I need: Access to the Prowlarr admin URL (currently getting 403). Cannot verify indexer health without it."

### Approval Message Format

For high-risk actions:
```
⚠️ HIGH-RISK ACTION REQUIRED

Problem: [what's broken]
Diagnosis: [GoStream/Jellyfin/Prowlarr root cause]
Proposed fix: [what the agent wants to do]
Alternative if nothing: [what happens if we don't act]

Reply: yes / no / any question
```

### Bidirectional Communication

The agent can:
- Notify about completed actions
- Request approval for high-risk actions
- Ask questions when stuck (requires human judgment)
- Report periodic deep scan results

User can reply:
- "yes" / "no" for approvals
- Free text for questions
- "/status" for current agent state

---

## 7. Action History & Self-Learning

### Action History Store

**Location:** `~/.hermes/gostream-action-history.json`

```json
{
  "actions": [
    {
      "ts": "2026-04-24T10:30:00Z",
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

### Pre-Action Check

Before any action, the subagent MUST:
1. Read action history — skip if same issue addressed < 30 min ago
2. Check `git log` for recent code changes related to the issue
3. Only proceed if the action is new or the previous attempt failed

### Self-Learning Loop

After each successful action:
1. Append to action history
2. If a new pattern was discovered → update the relevant skill file
3. Store lesson in Hermes `MEMORY.md` for cross-session recall
4. Track metrics (success rate, average resolution time, recurring issues)

---

## 8. User Preferences (Hard-coded in Skills)

### Torrent Selection Preferences
- **Smallest file size** within the same quality tier
- **Maximum seeders** — higher is always better
- Trade-off: small file + high seeders > large file + low seeders

### Issue Priority (by annoyance)
1. **B** — Films that don't start (few seeds, slow startup, errors)
2. **A** — Wrong films (CAM, wrong language, wrong version)
3. **C** — Incomplete TV series (missing seasons/episodes in favorites)
4. **D** — Missing subtitles (especially English)

### Favorites Requirements
- **Movies**: Must be 100% pre-downloaded (checked via GoStorm API download % — GoStream knows completion state; Jellyfin knows if the file is streamable)
- **TV Series**: Must be complete — all seasons/episodes available and accessible (verified via TMDB episode count vs. GoStream episode registry)

---

## 9. Error Handling & Resilience

### Mac Restart Resilience (Global Requirement)

**ALL components** (GoStream AI agent, Hermes skills, action history, queue) MUST survive and recover from a macOS restart:

- **GoStream AI Agent**: Disk queue persists on disk, reloads on startup. Detectors restart with GoStream. Buffered issues in memory are lost (acceptable — they'll be re-detected on next cycle).
- **Hermes Queue**: File-based queue on disk survives restart. Hermes cron resumes polling on next scheduled run.
- **Action History**: JSON file persists — no in-memory state that isn't reloaded.
- **Hermes Skills**: Markdown files on disk — inherently persistent. No runtime state to recover.
- **launchd Configuration**: Both GoStream and Hermes must have `RunAtLoad: true` and `KeepAlive` configured so they auto-start after reboot.
- **GoStream Startup**: AI agent initializes after GoStorm engine is up — no race condition on torrent API availability.
- **Graceful Degradation**: If Hermes is not yet running when GoStream starts, webhook pushes fail gracefully (batch stays in queue). GoStream never crashes because Hermes is unavailable.

### API Error Handling Protocol

All `/api/ai/*` endpoints return structured errors, never bare HTTP status codes:

```json
{
  "error": "validation_failed",
  "details": "Field 'torrent_id' is required but was null",
  "schema_hint": "replace_torrent requires: {\"torrent_id\": \"string\", \"new_magnet\": \"magnet:?xt=...\"}",
  "retry_allowed": true
}
```

### Retry Strategy (Two Categories)

| Error Type | Examples | Retry Behavior |
|---|---|---|
| **Validation** (bad payload) | Missing field, wrong type, invalid format | Max 2 attempts: original + auto-correction from `schema_hint`. If fails → STOP, escalation |
| **Transient** (infrastructure) | Timeout, connection refused, 503, rate limit | 3-5 attempts with exponential backoff (1s, 2s, 4s). If fails → STOP, escalation |
| **Auth** (401/403) | Invalid/missing API key | 0 retries → STOP immediately |

### Self-Correction Flow

When Hermes makes a tool call that returns a 400 validation error:
1. Parse `schema_hint` and `details` from the response
2. Fix the payload
3. Retry ONCE
4. If it fails again → STOP and report: `"❌ Cannot call GoStream API after self-correction: [error]"`

### Additional Scenarios

| Scenario | Behavior |
|---|---|
| Hermes unreachable | Queue persists on disk, retry on next flush cycle |
| Subagent crashes | Dispatcher detects dead subagent, logs error, retries from queue |
| MCP tool fails after retries | Subagent logs failure, reports to user, continues with other issues |
| Action history full | Prune entries older than 7 days |
| Queue full | Drop lowest-priority batch (shouldn't happen with normal loads) |
| API rate limit (TMDB/Prowlarr) | Back off and retry after cooldown |

---

## 10. Implementation Phases

### Phase 1: GoStream Issue Infrastructure
- `internal/ai-agent/` package (buffer, queue, webhook, detectors)
- Enhanced structured logging
- `/api/ai/*` endpoints
- Config additions (`ai_agent.enabled`, `ai_agent.webhook_url`)

### Phase 2: Hermes Skills + MCP Tools
- Deploy all skills to Hermes
- Configure MCP tools (GoStream endpoints, Jellyfin API, Prowlarr API, TMDB API)
- Set up Hermes gateway with Telegram
- Configure cron job for deep scan

### Phase 3: Integration + Testing
- Connect GoStream webhook → Hermes webhook receiver
- Test real-time issue detection flow
- Test deep scan flow
- Test bidirectional Telegram communication
- Test action history and deduplication

### Phase 4: Continuous Improvement
- Refine heuristics based on real usage
- Add more detectors as needed
- Optimize token usage in subagent prompts
- Tune debounce window and queue behavior

---

## 11. Out of Scope

- Replacing the existing health checker (we augment it, not replace it)
- Building a web UI for the AI agent (Telegram is the interface)
- Multi-user support (single-user personal system)
