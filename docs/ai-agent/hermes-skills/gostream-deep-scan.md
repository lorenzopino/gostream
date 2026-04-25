# GoStream Deep Scan Skill

**Trigger:** Cron job every ~30 minutes

## Workflow

Execute checks sequentially. If any issue found, dispatch to maintenance skill.

### Step 1: Torrent Health Check
```bash
curl -s "http://localhost:9080/api/ai/torrent-state" | python3 -c "
import sys,json
data = json.load(sys.stdin)
# Check for torrents with 0 seeders or no download
for t in data.get('result',[]):
    if t.get('stats',{}).get('seeders',0) == 0:
        print(f'DEAD: {t.get(\"title\",\"?\")} (ID: {t.get(\"id\",\"?\")})')
    elif t.get('stats',{}).get('seeders',0) < 3:
        print(f'LOW: {t.get(\"title\",\"?\")} ({t[\"stats\"][\"seeders\"]} seeders)')
"
```

### Step 2: Favorites Completeness Check
```bash
curl -s "http://localhost:9080/api/ai/favorites-check" | python3 -c "
import sys,json
report = json.load(sys.stdin)
for m in report.get('movies',[]):
    if m.get('status') not in ('ok','ready'):
        print(f'MOVIE ISSUE: {m.get(\"title\",\"?\")} — {m.get(\"status\",\"?\")}')
for s in report.get('tv_series',[]):
    if s.get('status') not in ('complete','ready'):
        print(f'TV ISSUE: {s.get(\"title\",\"?\")} — {s.get(\"status\",\"?\")}')
"
```

### Step 3: System Health
```bash
# GoStorm API
curl -sf "http://localhost:8090/torrents" > /dev/null && echo "GoStorm: OK" || echo "GoStorm: DOWN"

# FUSE mount
stat /Users/lorenzo/MediaCenter/gostream-fuse > /dev/null 2>&1 && echo "FUSE: OK" || echo "FUSE: DOWN"

# Jellyfin
curl -sf "$JELLYFIN_URL/System/Info?api_key=$JELLYFIN_API_KEY" > /dev/null 2>&1 && echo "Jellyfin: OK" || echo "Jellyfin: DOWN"

# Prowlarr
curl -sf "$PROWLARR_URL/api/v1/indexer?api_key=$PROWLARR_API_KEY" > /dev/null 2>&1 && echo "Prowlarr: OK" || echo "Prowlarr: DOWN"
```

### Step 4: Subtitle Check
Check recently-played items for missing English subtitles (via Jellyfin health skill).

### Step 5: Continuous Improvement
Check GoStream metrics for optimization opportunities:
```bash
curl -s "http://localhost:9080/metrics" | head -50
```

## Output

If issues found → create IssueBatch JSON and dispatch to maintenance skill:
```json
{
  "id": "batch-<timestamp>",
  "issues": [...],
  "created": "<ISO8601>",
  "source": "deep_scan"
}
```

If all clear → append to `~/.hermes/gostream-scan.log`:
```
[<timestamp>] deep scan: all healthy
```
