# GoStream Deep Scan Skill

## Purpose
Periodic comprehensive audit of the entire GoStream/Jellyfin/Prowlarr system. Runs every 30 minutes via cron.

## Trigger
Cron job in Hermes: `hermes cron add "deep-scan" "*/30 * * * *" "activate gostream-deep-scan skill"`

## Scan Sequence

### 1. GoStream Health
```bash
# Check all torrents
curl -s "http://localhost:9080/api/ai/torrent-state"

# Check queue
curl -s "http://localhost:9080/api/ai/queue-status"

# Check FUSE
curl -s "http://localhost:9080/api/ai/fuse-health"

# Check recent logs for errors
curl -s "http://localhost:9080/api/ai/recent-logs?lines=50"
```
Look for: Dead torrents (0 seeders), low seeders (<3), FUSE errors, error spikes in logs.

### 2. Favorites Completeness
```bash
curl -s "http://localhost:9080/api/ai/favorites-check"
```
Parse response:
- **Movies**: Check `download_pct` — if < 100% → issue `incomplete_download`
- **TV Series**: Check `available_episodes` vs TMDB total — if missing → issue `incomplete_series`

### 3. Subtitle Audit
Activate `jellyfin-health` skill:
1. Get active/recently played items from Jellyfin sessions
2. For each movie played in the last 24h, check for English subtitles
3. If missing → issue `missing_subtitles`

### 4. External Service Health
```bash
# Jellyfin
curl -s "<JELLYFIN_URL>/System/Info?api_key=<KEY>"

# Prowlarr
curl -s "<PROWLARR_URL>/api/v1/indexer?apikey=<KEY>"

# GoStorm
curl -s "http://localhost:8090/torrents"
```
Check: All services responding. Any 5xx errors or timeouts.

### 5. Continuous Improvement
Review recent action history:
```bash
cat ~/.hermes/gostream-action-history.json | tail -20
```
Look for:
- Recurring issues (same issue_type > 3 times in 24h) → suggest systemic fix
- Failed actions → retry with different approach
- Successful patterns → add to skill's "Lessons Learned" section

### 6. Config Review
Check GoStream config for optimization opportunities:
```bash
curl -s "http://localhost:9080/api/ai/config"
```
Look for:
- `master_concurrency_limit` — if too low for workload, suggest increase
- `read_ahead_budget_mb` — if too high for available RAM, suggest decrease
- `ai_agent.debounce_seconds` — if too short (excessive batching), suggest increase

## Output
If issues found:
1. Create an IssueBatch with all findings
2. Dispatch to gostream-maintenance skill for resolution
3. Log to `~/.hermes/gostream-scan.log`: "Deep scan at {time}: found {N} issues"

If all clear:
1. Log: "Deep scan at {time}: all healthy"
2. No action needed
