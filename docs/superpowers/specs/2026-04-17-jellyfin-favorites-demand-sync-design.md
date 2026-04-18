# Design: Jellyfin Favorites → GoStream On-Demand Sync + Fullpack Re-enable

**Date:** 2026-04-17
**Status:** Draft — pending review

## Problem Statement

1. **On-Demand Series Sync**: Users discover series on Jellyfin TV/web clients and want GoStream to download ALL episodes of a specific series on demand. No way to trigger this from the Jellyfin UI.

2. **Partial Series Completion**: Series that were partially synced (some seasons, some episodes) should be completed with missing episodes on demand.

3. **Fullpack Disabled**: Fullpack (season/series complete packs) are currently disabled for manual mode sync, but they are the most efficient way to get complete series with many episodes.

## Architecture

### System Overview

```
Jellyfin TV/Web App → ❤️ "Add to Favorites"
         ↓
   Jellyfin Server → WebSocket broadcast (built-in, no plugin needed)
   Event: { MessageType: "UserDataChanged", Data: { ItemId, UserId, UserData: { IsFavorite: true } } }
         ↓
   [ws-bridge.py] — lightweight WebSocket listener (~100 lines Python)
   1. Connected to Jellyfin WebSocket
   2. Filters: UserDataChanged + IsFavorite=true + Type=Series
   3. GET /Items/{ItemId}?Fields=ProviderIds → extract TMDB ID
   4. POST http://gostream:9080/api/tv-sync/demand {"tmdb_id": N}
         ↓
   [GoStream] — new endpoint + sync engine changes
   1. POST /api/tv-sync/demand → validates TMDB ID, starts background sync job
   2. Re-enabled fullpack processing for manual/on-demand modes
   3. Sync fills missing episodes (existing ones skipped unless upgrade)
   4. GET /api/tv-sync/demand/{job_id} → check job status
```

### Component 1: WebSocket Bridge (`ws-bridge.py`)

**Location:** `/Users/lorenzo/MediaCenter/gostream/bin/ws-bridge.py`

A lightweight Python script (~100 lines) that:
1. Connects to Jellyfin WebSocket: `ws://{jellyfin_url}/socket?api_key={key}`
2. Listens for `UserDataChanged` messages
3. When `IsFavorite` transitions to `true` AND the item is a Series:
   - Calls `GET /Items/{ItemId}?Fields=ProviderIds` to get TMDB ID
   - Sends `POST /api/tv-sync/demand` to GoStream with `{"tmdb_id": N}`
   - Logs the action
4. Handles reconnects on disconnect

**Config file:** `/Users/lorenzo/MediaCenter/gostream/state/ws-bridge.json`
```json
{
  "jellyfin_url": "http://192.168.x.x:8096",
  "jellyfin_api_key": "xxx",
  "gostream_url": "http://localhost:9080",
  "log_file": "/Users/lorenzo/MediaCenter/gostream/logs/ws-bridge.log"
}
```

**Launchd service:** `~/Library/LaunchAgents/com.gostream.ws-bridge.plist`

### Component 2: GoStream On-Demand Endpoint

**New endpoints:**

```
POST /api/tv-sync/demand
Body: {"tmdb_id": 4607}
Response: {"job_id": "uuid", "status": "started", "show_name": "Lost"}

GET /api/tv-sync/demand/{job_id}
Response: {"job_id": "uuid", "status": "running|completed|failed", 
           "episodes_created": 12, "episodes_skipped": 3, 
           "error": null}
```

**How it works:**
1. Validates TMDB ID is a TV series
2. Creates a temporary `TVSyncerConfig` with:
   - `Channel.Mode = "demand"` (new mode)
   - `Channel.TMDBIDs = [requested_tmdb_id]`
   - Same quality profile as manual mode
3. Runs `TVGoEngine.Run()` for that single series
4. Results tracked in a concurrent map by job_id
5. Accessible via HTTP for status polling

### Component 3: Re-enable Fullpack Processing

**Current behavior** (`tv_go.go` line ~770):
```go
if show.SourceMode != "manual" {
    // fullpack processing block — currently SKIPPED for manual mode
}
```

**New behavior:** Remove the guard. Fullpack processing runs for ALL modes:
- `discovery` — unchanged
- `manual` — re-enabled (was disabled due to unfounded streaming concerns — the FUSE layer correctly streams individual files from multi-file torrents)
- `demand` — re-enabled (on-demand sync benefits most from fullpacks)

### Component 4: Partial Series Completion

The existing sync engine already handles this correctly:
- `processShow()` iterates all available streams
- `processFullpack()` creates MKV stubs for EACH video file in the pack
- `processSingle()` creates MKV for individual episodes
- Episodes already in the registry are **skipped** (upgrade check: new score > existing * 1.2)
- For `demand` mode: `skip_complete_seasons` should be `true` — if a season is complete, skip it. But individual missing episodes are still created.

**No changes needed** to the episode creation logic. The existing `processShow()` → `processFullpack()`/`processSingle()` pipeline already:
- Creates MKV for episodes not in registry
- Upgrades episodes with better quality (20% improvement threshold)
- Skips episodes with same or worse quality

### New `demand` Mode Behavior

| Rule | demand mode | manual mode | discovery mode |
|------|-------------|-------------|----------------|
| Show source | Single TMDB ID (API request) | Config TMDB IDs list | TMDB discovery endpoints |
| Fullpack | ✅ Enabled | ✅ Enabled | ✅ Enabled |
| Blacklist | ❌ Skipped | ❌ Skipped | ✅ Applied |
| Language filter | ❌ Skipped | ❌ Skipped | ✅ Applied |
| Premium provider filter | ❌ Skipped | ❌ Skipped | ✅ Applied |
| Complete-season skip | ✅ Config-driven (default: false) | ✅ Config-driven | ✅ Config-driven |
| Season range | ALL seasons | ALL seasons | Last 2 seasons |
| Quality profile | Same as manual | Same as manual | Same as manual |

### Component 5: Jellyfin Library Refresh (Series-Only)

After an on-demand sync completes, GoStream triggers a **targeted library refresh** for ONLY the synced series:

**Method:** `POST /Items/{ItemId}/Refresh?Recursive=true&ImageRefreshMode=Default&MetadataRefreshMode=Default&ReplaceAllMetadata=false`

**How the ItemId is resolved:**
1. When the ws-bridge sends the demand request, it includes the Jellyfin `ItemId` alongside the TMDB ID:
   ```json
   {"tmdb_id": 4607, "jellyfin_item_id": "abc123xyz"}
   ```
2. GoStream stores the `jellyfin_item_id` in the job context
3. When the sync completes, GoStream calls Jellyfin API to refresh ONLY that item
4. This avoids a full library scan — Jellyfin only re-scans the specific series directory

**Why targeted refresh matters:**
- Full library scan takes minutes for large libraries
- Targeted refresh takes seconds (one directory)
- No impact on other library items

### Execution Flow: Favorites → Sync → Refresh

```
1. User on Jellyfin TV app → navigates to "Lost" → presses ❤️
2. Jellyfin server fires: { "Type": "UserDataChanged", 
     "Data": { "UserId": "abc", "ItemId": "xyz", 
               "UserData": { "IsFavorite": true, "PlayedPercentage": 0 } } }
3. ws-bridge.py receives via WebSocket
4. ws-bridge.py calls: GET /Items/xyz?Fields=ProviderIds
   → { "ProviderIds": { "Tmdb": "4607" }, "Type": "Series" }
5. ws-bridge.py calls: POST http://localhost:9080/api/tv-sync/demand
   → {"tmdb_id": 4607, "jellyfin_item_id": "xyz"}
6. GoStream validates TMDB ID, creates demand syncer, starts background job
7. Demand sync fetches show details, gets all seasons, finds fullpacks + singles
8. Creates MKV stubs for missing episodes
9. User can poll: GET /api/tv-sync/demand/{job_id}
10. **On completion:** GoStream calls POST /Items/xyz/Refresh?Recursive=true
11. Jellyfin re-scans ONLY the Lost directory → new episodes appear
12. Total refresh time: ~2-5 seconds (one series, not full library)
```

### File Changes Summary

| File | Change |
|------|--------|
| `internal/syncer/engines/tv_go.go` | Remove fullpack guard for manual mode; add "demand" mode handling; add Jellyfin refresh callback after sync |
| `main.go` | Register new `/api/tv-sync/demand` HTTP endpoints; add jellyfin_url config to demand endpoint |
| `internal/syncer/engines/tv.go` | Add support for `demand` mode in `Name()`; add JellyfinItemID to TVEngineConfig |
| `ws-bridge.py` | New file: WebSocket bridge script (passes jellyfin_item_id in demand request) |
| `ws-bridge.json` | New file: Bridge config template |

### Execution Flow: Favorites → Sync

```
1. User on Jellyfin TV app → navigates to "Lost" → presses ❤️
2. Jellyfin server fires: { "Type": "UserDataChanged", 
     "Data": { "UserId": "abc", "ItemId": "xyz", 
               "UserData": { "IsFavorite": true, "PlayedPercentage": 0 } } }
3. ws-bridge.py receives via WebSocket
4. ws-bridge.py calls: GET /Items/xyz?Fields=ProviderIds
   → { "ProviderIds": { "Tmdb": "4607" }, "Type": "Series" }
5. ws-bridge.py calls: POST http://localhost:9080/api/tv-sync/demand
   → {"tmdb_id": 4607}
6. GoStream validates TMDB ID, creates demand syncer, starts background job
7. Demand sync fetches show details, gets all seasons, finds fullpacks + singles
8. Creates MKV stubs for missing episodes
9. User can poll: GET /api/tv-sync/demand/{job_id}
10. Jellyfin library scan picks up new MKV files → series appears complete
```

### Error Handling

| Error | Handling |
|-------|----------|
| TMDB ID not found | Return 400, log error |
| TMDB ID is a movie (not series) | Return 400, log "item is not a series" |
| GoStream already syncing this series | Return existing job_id, don't duplicate |
| No streams found for series | Job completes with 0 created, log warning |
| Jellyfin WebSocket disconnect | ws-bridge reconnects with exponential backoff |
| GoStream unreachable from bridge | Retry 3x with 5s backoff, then log error |

### Security

- ws-bridge uses Jellyfin API key (read-only access to library metadata)
- GoStream demand endpoint has no auth (same as existing scheduler endpoints)
- Both run on localhost/internal network

### Migration

No migration needed. Existing manual mode channels continue to work. Fullpacks are automatically re-enabled for all modes.
