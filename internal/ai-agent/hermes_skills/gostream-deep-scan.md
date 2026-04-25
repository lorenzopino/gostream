# GoStream Deep Scan Skill

## Purpose
Periodic comprehensive audit of the entire GoStream + Jellyfin + Prowlarr ecosystem. Triggered by cron every 30 minutes when no urgent issues are queued.

## Trigger
Cron job or dispatcher manual trigger.

## Configuration
Read from `~/.hermes/config/gostream.json`:
```json
{
  "gostream": {
    "api_url": "http://localhost:9080"
  },
  "jellyfin": {
    "url": "http://localhost:8096",
    "api_key": "<key>"
  },
  "prowlarr": {
    "url": "http://localhost:9696",
    "api_key": "<key>"
  },
  "tmdb": {
    "api_key": "<key>"
  }
}
```

## Scan Procedure (executed sequentially)

### Phase 1: GoStream Health
```bash
# 1. Check all torrents
curl -s "http://localhost:9080/api/ai/active-playback"
# For each torrent: check seeders, download speed
curl -s "http://localhost:9080/api/ai/torrent-state?id=<ID>"

# 2. Check FUSE health
curl -s "http://localhost:9080/api/ai/fuse-health"

# 3. Check queue
curl -s "http://localhost:9080/api/ai/queue-status"

# 4. Get recent logs
curl -s "http://localhost:9080/api/ai/recent-logs?lines=50"
```

### Phase 2: Favorites Completeness
```bash
# Full favorites report
curl -s "http://localhost:9080/api/ai/favorites-check"
```
For each item in report:
- Movies: verify download % is 100
- TV: verify all seasons/episodes present via TMDB cross-check

### Phase 3: Jellyfin Health
```bash
# Service health
curl -s "<JELLYFIN_URL>/System/Info?api_key=<KEY>"

# Active sessions
curl -s "<JELLYFIN_URL>/Sessions?api_key=<KEY>"

# Library count
curl -s "<JELLYFIN_URL>/Items?IncludeItemTypes=Movie,Series&Recursive=true&api_key=<KEY>" | jq '.TotalRecordCount'
```

### Phase 4: Prowlarr Health
```bash
# Indexer status
curl -s "<PROWLARR_URL>/api/v1/indexer?apikey=<KEY>"
```
Check: all indexers enabled and responding.

### Phase 5: Subtitle Audit
For each recently played movie (from Jellyfin sessions):
1. Get item details → check for English subtitles
2. If missing → log as issue

### Phase 6: Performance Review
Check GoStream config for optimization opportunities:
- Read cache sizes vs actual usage
- Concurrency limits vs actual load
- Timeout settings vs observed latencies

## Output Format
```
Deep Scan Report - {timestamp}

GoStream:
  Active torrents: {N}
  Dead torrents: {N} (list)
  Low seeder torrents: {N} (list)
  FUSE health: {OK/ERROR}

Jellyfin:
  Status: {UP/DOWN}
  Active sessions: {N}
  Movies in library: {N}
  TV series in library: {N}

Prowlarr:
  Indexers up: {N}/{total}
  Indexers down: {N} (list)

Favorites:
  Movies complete: {N}/{total}
  TV series complete: {N}/{total}

Subtitles:
  Missing ENG subtitles: {N} (list)

Issues found: {total count}
Actions needed: {list}
```

## Issue Dispatch
If issues are found:
1. Create IssueBatch JSON with all findings
2. Pass to dispatcher for queuing
3. If urgent (priority B or A): attempt immediate dispatch to maintenance subagent
4. If lower priority (C or D): queue for next available slot

If all clear:
Log: "Deep scan {timestamp}: all healthy" to `~/.hermes/gostream-scan.log`
