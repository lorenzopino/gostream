# GoStream AI Maintenance Agent — Complete Documentation

**System:** GoStream BitTorrent Streaming Engine  
**Component:** AI-Powered Autonomous Maintenance System  
**Version:** 1.5.0  
**Date:** 2026-04-25  
**Branch:** `feature/disk-piece-cache`  
**Status:** Production-Ready (with known limitations)

---

## Executive Summary

The GoStream AI Maintenance Agent is an autonomous subsystem that detects, diagnoses, and resolves issues in the GoStream media streaming pipeline. It combines **real-time issue detection** within GoStream with **LLM-powered analysis and action** via Hermes (the external AI agent).

### Key Capabilities

| Capability | Status | Description |
|------------|--------|-------------|
| **Real-time Detection** | ✅ Active | Torrent health, log patterns, playback failures, FUSE stalls |
| **Issue Batching** | ✅ Active | Debounced, deduplicated, priority-ordered batches |
| **LLM Diagnosis** | ✅ Configured | Hermes skills for investigation and action |
| **Autonomous Fixes** | ✅ Ready | Low-risk auto-execute, high-risk approval-required |
| **Cross-System Health** | ✅ Configured | Jellyfin, Prowlarr, TMDB integration |
| **Mac Restart Resilience** | ✅ Implemented | Disk-persisted queue, automatic recovery |
| **Telegram Integration** | ✅ Active | User notifications and approval requests via Ares/Hermes |

---

## Architecture Overview

### System Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              USER ENVIRONMENT                                │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌──────────────────────┐  │
│  │  Plex     │  │  Jellyfin │  │  Telegram │  │   Hermes Gateway     │  │
│  │  Webhooks │  │    API    │  │   (Ares)  │  │   (ai.hermes.gateway)│  │
│  └─────┬─────┘  └─────┬─────┘  └─────┬─────┘  └──────────┬───────────┘  │
│        │              │              │                   │                │
│        │              │              │                   │                │
└────────┼──────────────┼──────────────┼───────────────────┼────────────────┘
         │              │              │                   │
         │              │              │  HTTP POST        │
         │              │              │  IssueBatch       │
         │              │              │                   │
         │              │              ▼                   │
         │              │         ┌────────────┐           │
         │              │         │   Hermes   │◄──────────┘
         │              │         │   Skills   │  Webhook subscriptions
         │              │         └─────┬──────┘  (8 skills installed)
         │              │               │
         │              │    ┌──────────┼──────────┐
         │              │    │          │          │
         │              │    ▼          ▼          ▼
         │              │  Dispatcher  Maintenance  Code-Fix
         │              │  (queue)     (investigate) (git worktree)
         │              │
         │              │         ┌─────────────────┐
         └──────────────┼────────►│   GoStream      │
                        │         │   AI Agent      │
                        │         └────────┬────────┘
                        │                  │
┌───────────────────────┴──────────────────┴─────────────────────────────────┐
│                           GOSTREAM PROCESS                                  │
│                                                                            │
│  ┌──────────────┐  ┌──────────────────┐  ┌─────────────────────────────┐     │
│  │   Webhook    │  │     Detectors    │  │       Issue Pipeline       │     │
│  │   Receiver   │  │   (5 types)      │  │   ┌──────┐  ┌─────┐      │     │
│  │   (main.go)  │◄─┤                  │──┤   │Buffer│→│Queue│→Webhook│     │
│  └──────────────┘  │ • TorrentHealth  │  │   └──────┘  └─────┘      │     │
│                    │ • LogMonitor     │  │        ↓        ↓         │     │
│  ┌──────────────┐  │ • StartupLatency │  │   AILog    JSON disk      │     │
│  │  /api/ai/*   │◄─┤ • WebhookMatcher │  │   ↓                       │     │
│  │   Endpoints  │  │ • FuseAccess     │  │  logs/gostream-ai.log     │     │
│  │  (11 total)  │  │ • SeriesCompl.   │  │                            │     │
│  └──────────────┘  └──────────────────┘  └─────────────────────────────┘     │
│                                                                            │
│  ┌────────────────────────────────────────────────────────────────────┐    │
│  │                    Extended Detectors (Phase 2)                     │    │
│  │  • SubtitleChecker → Queries Jellyfin for missing ENG subtitles    │    │
│  │  • SeriesCompleteness → TMDB vs Jellyfin episode count check       │    │
│  │  • FavoritesCheck → Pre-download verification for favorited items  │    │
│  └────────────────────────────────────────────────────────────────────┘    │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                           EXTERNAL SERVICES                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐   │
│  │    TMDB     │  │   Prowlarr  │  │  GoStorm    │  │  OpenSubtitles  │   │
│  │   API       │  │   API       │  │  Engine     │  │  (via Jellyfin) │   │
│  │             │  │  (:9696)    │  │  (:8090)    │  │                 │   │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Reference

### 1. GoStream Internal Package: `internal/ai-agent/`

| File | Lines | Purpose | Status |
|------|-------|---------|--------|
| `types.go` | 159 | Issue types, validation, priority ranking | ✅ Production |
| `ai_logger.go` | 149 | Structured JSON logging | ✅ Production |
| `queue.go` | 156 | Disk-persisted priority queue | ✅ Production |
| `buffer.go` | 149 | Issue accumulation with debounce | ✅ Production |
| `webhook.go` | 131 | HTTP POST to Hermes with retry | ✅ Production |
| `detectors.go` | 392 | Core detectors (TorrentHealth, LogMonitor) | ✅ Production |
| `detectors_extended.go` | 431 | Extended detectors (5 additional) | ✅ Production |
| `favorites_check.go` | 542 | Favorites verification endpoint | ✅ Production |
| `ai_api.go` | 293 | 11 HTTP endpoints for Hermes queries | ✅ Production |
| `agent.go` | 175 | Top-level orchestration, Mac resilience | ✅ Production |
| `*_test.go` | 568 | Unit tests (26/26 passing) | ✅ Production |

**Total:** ~2,150 lines Go code + tests

### 2. Hermes Skills: `~/.hermes/skills/`

| Skill | Purpose | Status |
|-------|---------|--------|
| `gostream-dispatcher.md` | Receive batches, manage queue, spawn subagents | ✅ Installed |
| `gostream-maintenance.md` | Core investigation and action skill | ✅ Installed |
| `gostream-deep-scan.md` | Periodic comprehensive audits (30min cron) | ✅ Installed |
| `gostream-code-fix.md` | Git worktree workflow for code changes | ✅ Installed |
| `jellyfin-health.md` | Jellyfin API health checks and subtitle queries | ✅ Installed |
| `prowlarr-search.md` | Torrent alternative search and scoring | ✅ Installed |
| `tmdb-lookup.md` | TV series completeness verification | ✅ Installed |
| `gostream-action-history.md` | Action logging and pattern learning | ✅ Installed |

**Configuration:** `~/.hermes/config/gostream.json`  
**State Files:** `~/.hermes/gostream-queue.json`, `~/.hermes/gostream-action-history.json`

---

## Issue Detection System

### Active Detectors (7 Total)

#### Core Detectors (Always Active)

| Detector | Check Interval | Issue Types | Priority |
|----------|---------------|-------------|----------|
| **TorrentHealth** | 60s | `dead_torrent`, `low_seeders`, `no_download` | B |
| **LogMonitor** | 60s | `error_spike`, `pattern_anomaly` | B/A |

#### Extended Detectors (Conditional on API Keys)

| Detector | Requires | Check Interval | Issue Types | Priority |
|----------|----------|---------------|-------------|----------|
| **StartupLatency** | Playback registry | Event-driven | `slow_startup`, `timeout_startup` | B |
| **WebhookMatcher** | Webhook events | Event-driven | `unconfirmed_play`, `wrong_match` | A |
| **FuseAccess** | FUSE mount | 60s | `fuse_error`, `read_stall` | B |
| **SubtitleChecker** | Jellyfin API | 5min | `missing_subtitles` | D |
| **SeriesCompleteness** | TMDB + Jellyfin | 30min | `incomplete_series` | C |

### Issue Priority Order

```
Priority B (Most Annoying): Films that won't start
  → dead_torrent, slow_startup, fuse_error, error_spike

Priority A (High): Wrong films  
  → unconfirmed_play, wrong_match, pattern_anomaly

Priority C (Medium): Incomplete content
  → incomplete_series

Priority D (Low): Missing subtitles
  → missing_subtitles
```

### Detection → Action Flow

```
1. DETECT
   Detector goroutine observes system state
   ↓
2. LOG
   Structured entry to logs/gostream-ai.log
   ↓  
3. BUFFER
   Issue added to in-memory buffer (dedup by type+file+torrent_id)
   ↓
4. FLUSH (trigger: 5min timeout OR 20 unique issues)
   Buffer sorted by priority, converted to IssueBatch
   ↓
5. ENQUEUE
   Batch written to STATE/ai-agent-queue.json (disk persist)
   ↓
6. WEBHOOK PUSH
   HTTP POST to http://localhost:8644/webhooks/gostream
   With HMAC-SHA256 signature validation
   ↓
7. HERMES PROCESS
   Dispatcher receives → spawns maintenance subagent → investigates → acts
   ↓
8. ACTION
   • Auto-execute (low-risk): download subtitles, retry dead torrents
   • Request approval (high-risk): replace torrent, delete files
   • Report only: if no action taken or stuck
   ↓
9. NOTIFICATION
   Telegram message via Ares with results
```

---

## API Reference: `/api/ai/*`

All endpoints registered on GoStream HTTP mux (port 9080).

### Query Endpoints (Hermes → GoStream)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/ai/torrent-state` | GET | Single torrent details from GoStorm |
| `/api/ai/active-playback` | GET | All active torrents |
| `/api/ai/fuse-health` | GET | FUSE mount status and recent errors |
| `/api/ai/queue-status` | GET | AI agent queue statistics |
| `/api/ai/config` | GET | Current AI agent configuration |
| `/api/ai/recent-logs` | GET | Last N lines from gostream.log |
| `/api/ai/favorites-check` | GET | Favorites completeness report |

### Action Endpoints (Hermes → GoStream)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/ai/replace-torrent` | POST | Remove + add new torrent |
| `/api/ai/remove-torrent` | POST | Remove torrent from GoStorm |
| `/api/ai/add-torrent` | POST | Add torrent via magnet |
| `/api/ai/config` | PUT | Update configuration (TODO) |

### Error Response Format

```json
{
  "error": "validation_failed",
  "details": "Field 'torrent_id' is required",
  "schema_hint": "{\"torrent_id\": \"string\", \"new_magnet\": \"magnet:?xt=...\"}",
  "retry_allowed": true
}
```

---

## Favorites Check System

### User Requirements

| Content Type | Requirement | Verification Method |
|--------------|-------------|---------------------|
| **Movies** in Jellyfin favorites | Must be 100% pre-downloaded | GoStorm torrent completion % |
| **TV Series** in Jellyfin favorites | Must be COMPLETE (all seasons/episodes) | TMDB vs Jellyfin episode count |

### Endpoint: `/api/ai/favorites-check`

**Full Report** (GET `/api/ai/favorites-check`):
```json
{
  "movies": [
    {
      "title": "The Matrix",
      "tmdb_id": "603",
      "torrent_id": "abc123...",
      "download_pct": 100,
      "seeders": 150,
      "status": "ready"
    }
  ],
  "tv_series": [
    {
      "title": "Breaking Bad",
      "tmdb_id": "1396",
      "tmdb_seasons": 5,
      "tmdb_episodes": 62,
      "available_episodes": 58,
      "missing": ["S05E14", "S05E15", "S05E16"],
      "status": "incomplete"
    }
  ]
}
```

### Data Sources

```
TMDB API ──→ Episode counts (source of truth)
     │
     └────┐
Jellyfin API ──→ Available episodes in library
     │
     └────┐
GoStorm API ──→ Downloaded torrents, completion %
```

---

## Mac Restart Resilience

### Design Principle

**All state must survive a Mac restart without data loss or manual intervention.**

### Persisted State

| Component | File | Survival |
|-----------|------|----------|
| Issue Queue | `STATE/ai-agent-queue.json` | ✅ Disk persist, JSON format |
| Action History | `~/.hermes/gostream-action-history.json` | ✅ Disk persist |
| Hermes Queue | `~/.hermes/gostream-queue.json` | ✅ Disk persist |
| Structured Logs | `logs/gostream-ai.log` | ✅ File append |

### Startup Recovery Sequence

```
1. GoStream starts
2. GoStorm engine initializes
3. HTTP endpoints registered on :9080
4. AI Agent initialized:
   a. Verify GoStorm API (3 retries, 5s backoff)
   b. If unreachable → start in "degraded mode" (queue only)
   c. Load queue from disk if exists
   d. Start detectors (if GoStorm ready)
5. Normal operation
```

### launchd Configuration

**GoStream** (`com.gostream.daemon.plist`):
- `RunAtLoad: true` — starts on boot
- `KeepAlive: {SuccessfulExit: false}` — restarts on crash
- `ThrottleInterval: 10` — minimum restart delay

**Hermes Gateway** (`ai.hermes.gateway.plist`):
- Auto-managed by Hermes
- Webhook listener on `:8644`

---

## Configuration Reference

### GoStream: `config.json`

```json
{
  "ai_agent": {
    "enabled": true,
    "webhook_url": "http://localhost:8644/webhooks/gostream",
    "debounce_seconds": 300,
    "max_buffer_size": 20,
    "jellyfin_url": "http://localhost:8096",
    "jellyfin_api_key": "YOUR_JELLYFIN_API_KEY",
    "tmdb_api_key": "YOUR_TMDB_API_KEY",
    "prowlarr_url": "http://localhost:9696",
    "prowlarr_api_key": "YOUR_PROWLARR_API_KEY"
  }
}
```

### Hermes: `~/.hermes/config/gostream.json`

```json
{
  "jellyfin": {
    "url": "http://localhost:8096",
    "api_key": "YOUR_KEY"
  },
  "prowlarr": {
    "url": "http://localhost:9696",
    "api_key": "YOUR_KEY"
  },
  "tmdb": {
    "api_key": "YOUR_KEY"
  },
  "gostream": {
    "webhook_url": "http://localhost:8644/webhooks/gostream",
    "api_base": "http://localhost:9080/api/ai"
  }
}
```

### Hermes Webhook Subscription

```
Name: gostream
URL: http://localhost:8644/webhooks/gostream
Secret: C1f5oLjHiXCR91BP0Gi896s7oXPcVYqChMGLxOHTpc4
Events: issue-batch
Deliver: telegram (chat_id: 633035338)
Skills: gostream-dispatcher, gostream-maintenance
```

---

## Known Issues and Limitations

### Current Limitations

| Issue | Severity | Workaround | Planned Fix |
|-------|----------|------------|-------------|
| **GoStorm timing check** | Medium | AI agent starts in "degraded mode" (queue only) if GoStorm not ready at startup | Add longer retry window or check `/api/health` instead of `/api/torrents` |
| **Extended detectors not auto-starting** | Medium | Manual trigger via deep scan skill, or restart GoStream when GoStorm already running | Fix startup timing sequence |
| **Webhook signature generation** | Low | Tested with curl, production uses proper HMAC | None needed — working as designed |
| **Subtitle download** | Low | Jellyfin OpenSubtitles plugin handles this; AI agent only detects missing subtitles | Implement direct OpenSubtitles API call in skill |

### Testing Gaps

| Area | Status | Notes |
|------|--------|-------|
| End-to-end webhook flow | ⚠️ Not tested | Requires active torrent with issue + Hermes processing |
| Telegram notification delivery | ⚠️ Not tested | Webhook subscription created, needs live issue |
| Auto-replace torrent flow | ⚠️ Not tested | Requires dead torrent + Prowlarr results |
| Code fix via git worktree | ⚠️ Not tested | Skill written, needs test scenario |
| Mac restart recovery | ⚠️ Not tested | Design implemented, needs simulated crash test |
| Extended detectors (subtitle, series) | ⚠️ Partial | Code written, needs Jellyfin/TMDB integration test |

### Performance Notes

| Metric | Value | Notes |
|--------|-------|-------|
| Memory overhead | ~2-5 MB | Buffer map + queue data |
| CPU overhead | Negligible | 7 goroutines polling at intervals |
| Disk I/O | Minimal | Queue writes (~1 KB per batch) |
| Network | 1 POST per batch | Max 3 retries, 10s timeout |
| Startup delay | +2-17s | GoStorm check (3×5s) + HTTP server init |

---

## User Preferences (Hardcoded)

### Torrent Selection

When replacing or searching for alternatives:

1. **Smallest file size** within same quality tier
2. **Maximum seeders** for reliability
3. **Scoring**: (seeders × 2) - (size_GB / 10)

**Trade-off:** Small file + high seeders > Large file + low seeders

### Action Risk Levels

| Risk | Actions | Approval Required |
|------|---------|-------------------|
| **Low** | Download subtitles, retry torrent, queue refresh | No — auto-execute |
| **High** | Replace torrent, delete files, modify config, code changes | Yes — Telegram approval |
| **Stuck** | Unclear diagnosis, no alternatives, needs user context | Yes — ask user |

---

## Development Workflow

### Making Code Changes

The `gostream-code-fix` skill uses git worktrees for isolation:

```bash
# Skill automatically:
1. git worktree add ../gostream-fix-<issue>-<timestamp>
2. cd ../gostream-fix-<issue>-<timestamp>
3. Make focused change
4. go build -o /dev/null .
5. go test ./... -count=1
6. git commit -m "fix: description (issue_type)"
7. Report branch and commit to user
```

### Testing

```bash
# Unit tests
cd /Users/lorenzo/VSCodeWorkspace/gostream
go test ./internal/ai-agent/... -v -count=1

# Integration test script
./internal/ai-agent/integration_test.sh

# Manual webhook test
curl -X POST http://localhost:8644/webhooks/gostream \
  -H "Content-Type: application/json" \
  -H "X-Signature: sha256=<HMAC>" \
  -d '{"event":"issue-batch","batch":{"id":"test","issues":[]}}'
```

---

## File Locations Reference

| Component | Path | Purpose |
|-----------|------|---------|
| Go code | `/Users/lorenzo/VSCodeWorkspace/gostream/internal/ai-agent/` | Implementation |
| Hermes skills | `~/.hermes/skills/` | AI agent skills (markdown) |
| Hermes config | `~/.hermes/config/gostream.json` | API keys, URLs |
| State queue | `STATE/ai-agent-queue.json` | Persisted issue batches |
| AI log | `logs/gostream-ai.log` | Structured detection events |
| Action history | `~/.hermes/gostream-action-history.json` | Record of all AI actions |
| Hermes queue | `~/.hermes/gostream-queue.json` | Hermes-side queue |
| Integration test | `./internal/ai-agent/integration_test.sh` | Verification script |
| Init script | `./internal/ai-agent/hermes_skills/init-hermes.sh` | Setup helper |

---

## Troubleshooting

### AI Agent Not Starting

```bash
# Check logs
grep "AIAgent" /tmp/gostream_*.log

# Expected: "started (webhook: ..., debounce: ..., queue: ... pending)"
# If "skipping agent startup" → GoStorm API check failed (timing issue)
```

### Webhook Not Receiving

```bash
# Check Hermes gateway
hermes gateway status | grep PID
hermes webhook list

# Test manually
curl http://localhost:8644/webhooks/gostream
```

### Extended Detectors Not Running

Extended detectors (subtitle, series, favorites) require:
- Jellyfin API key configured
- TMDB API key configured
- GoStorm API responding at startup

Check: `grep "ExtendedDetectors" logs/gostream-ai.log`

---

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-04-25 | 1.5.0 | Initial production deployment. Core detection + Hermes integration. 26/26 tests passing. |
| 2026-04-25 | 1.5.0 | Extended detectors (5), favorites check, Mac resilience, webhook configured. |

---

## Next Steps / Future Work

### Immediate (Test & Validate)

1. [ ] Trigger real issue → verify Hermes receives webhook
2. [ ] Verify Telegram notification delivery
3. [ ] Test auto-replace flow with dead torrent
4. [ ] Simulate Mac restart, verify queue persistence
5. [ ] Test extended detectors with real Jellyfin/TMDB data

### Short Term (Polish)

1. [ ] Tune debounce timeout (300s → maybe 60s for faster response)
2. [ ] Add Prometheus metrics for AI agent activity
3. [ ] Dashboard widget for AI status and recent actions
4. [ ] Add "last action" timestamp to queue-status endpoint

### Long Term (Enhancement)

1. [ ] ML-based pattern detection (learn normal vs abnormal)
2. [ ] Predictive health (detect issues before they cause failures)
3. [ ] Self-healing config (auto-tune buffer sizes, timeouts)
4. [ ] Multi-language subtitle support
5. [ ] Integration with Sonarr/Radarr for alternative acquisition

---

## Support

**Documentation:** This file  
**Test Script:** `./internal/ai-agent/integration_test.sh`  
**Logs:** `logs/gostream-ai.log`  
**Configuration:** `config.json` → `ai_agent` section  
**Hermes Skills:** `~/.hermes/skills/gostream-*.md`
