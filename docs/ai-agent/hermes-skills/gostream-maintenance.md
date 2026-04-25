# GoStream Maintenance Skill

**Activation:** By dispatcher when subagent starts with an IssueBatch

## Workflow

1. Read the IssueBatch from context
2. **Check action history** (`~/.hermes/gostream-action-history.json`) — skip if same issue addressed < 30 min ago
3. For each issue, investigate using tool calls (see Tools section below)
4. Categorize actions by risk level
5. Execute or request approval
6. Save solution to action history
7. Notify user via Telegram

## Issue Investigation Tools

### GoStream API (curl)
```bash
# Torrent state
curl -s "http://localhost:9080/api/ai/torrent-state?id=<torrent_id>"

# Active playback
curl -s "http://localhost:9080/api/ai/active-playback"

# FUSE health
curl -s "http://localhost:9080/api/ai/fuse-health"

# Recent logs
curl -s "http://localhost:9080/api/ai/recent-logs?lines=100"

# Queue status
curl -s "http://localhost:9080/api/ai/queue-status"

# Favorites check
curl -s "http://localhost:9080/api/ai/favorites-check"

# Replace torrent
curl -s -X POST "http://localhost:9080/api/ai/replace-torrent" \
  -H "Content-Type: application/json" \
  -d '{"torrent_id": "abc123", "new_magnet": "magnet:?xt=..."}'

# Remove torrent
curl -s -X POST "http://localhost:9080/api/ai/remove-torrent" \
  -H "Content-Type: application/json" \
  -d '{"torrent_id": "abc123"}'

# Add torrent
curl -s -X POST "http://localhost:9080/api/ai/add-torrent" \
  -H "Content-Type: application/json" \
  -d '{"magnet": "magnet:?xt=...", "title": "Movie Name"}'
```

### Jellyfin API (curl)
```bash
# Service health
curl -s "$JELLYFIN_URL/System/Info?api_key=$JELLYFIN_API_KEY"

# Active sessions
curl -s "$JELLYFIN_URL/Sessions?api_key=$JELLYFIN_API_KEY"

# Item details (subtitle tracks)
curl -s "$JELLYFIN_URL/Items/<id>?api_key=$JELLYFIN_API_KEY"

# Library scan state
curl -s "$JELLYFIN_URL/Library/VirtualFolders?api_key=$JELLYFIN_API_KEY"

# Trigger library refresh
curl -s -X POST "$JELLYFIN_URL/Library/Refresh?api_key=$JELLYFIN_API_KEY"
```

### Prowlarr API (curl)
```bash
# Search torrents
curl -s "$PROWLARR_URL/api/v1/search?query=<query>&categories=<cat>&api_key=$PROWLARR_API_KEY"

# Indexer health
curl -s "$PROWLARR_URL/api/v1/indexer?api_key=$PROWLARR_API_KEY"
```

### TMDB API (curl)
```bash
# Search movie
curl -s "https://api.themoviedb.org/3/search/movie?query=<query>&api_key=$TMDB_API_KEY"

# Search TV series
curl -s "https://api.themoviedb.org/3/search/tv?query=<query>&api_key=$TMDB_API_KEY"

# Series details (season count)
curl -s "https://api.themoviedb.org/3/tv/<id>?api_key=$TMDB_API_KEY"

# Season episodes
curl -s "https://api.themoviedb.org/3/tv/<id>/season/<n>?api_key=$TMDB_API_KEY"
```

## Action Risk Categories

### LOW-RISK (auto-execute, no approval needed)
- Download subtitles for a movie/episode
- Retry a dead torrent (same magnet)
- Re-queue a failed download
- Trigger library refresh

### HIGH-RISK (requires Telegram approval)
- Replace a torrent with a different release
- Change quality settings
- Delete files
- Modify GoStream config
- Code changes to GoStream

### STUCK (ask user for guidance)
- No suitable torrent alternatives found
- Unclear root cause after investigation
- Conflicting information from different sources

## Action History Protocol

Before ANY action, check `~/.hermes/gostream-action-history.json`:
```bash
cat ~/.hermes/gostream-action-history.json | python3 -c "import sys,json; data=json.load(sys.stdin); [print(a['ts'],a['issue_type'],a['action'],a['result']) for a in data.get('actions',[])]"
```
If the same issue_type + file was addressed < 30 min ago → skip and log "recently addressed".

## After Action Protocol

1. Append to action history:
```json
{
  "ts": "<ISO8601>",
  "issue_type": "<type>",
  "file": "<filename>",
  "root_cause": "<what caused it>",
  "action": "<what was done>",
  "details": "<specifics>",
  "result": "success|partial|failed",
  "confidence": 0.0-1.0,
  "user_approved": true/false,
  "lessons": "<what to remember for next time>"
}
```

2. If a new pattern was discovered → update this skill's investigation section
3. Notify user via Telegram

## Telegram Message Formats

**Notification (completed action):**
```
✅ <issue_type> fixed: <brief details>
File: <filename>
Action: <what was done>
Result: <outcome>
```

**Approval request (high-risk action):**
```
⚠️ HIGH-RISK ACTION REQUIRED

Problem: <what's broken>
Diagnosis: <root cause found>
Proposed fix: <what I want to do>
Alternative if nothing: <what happens if we don't act>

Reply: yes / no / any question
```

**Question (stuck):**
```
❓ STUCK: <issue_type> on <filename>

Context: <what I've tried>
Problem: <why I can't proceed>

Can you help?
```

**Resource request:**
```
🔧 RESOURCE REQUEST

I need: <specific thing>
To: <what this would enable>
Currently: <what happens without it>

Reply: provide it / skip / any question
```

## User Preferences

- **Torrent selection:** Prefer smallest file with most seeders within the same quality tier
- **Issue priority:** B > A > C > D
- **Movies:** Must be 100% pre-downloaded for instant playback
- **TV Series:** Must have ALL seasons/episodes
- **Subtitles:** English subtitles are required — auto-download when missing
