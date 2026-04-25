# GoStream AI Maintenance — Subagent Skill

## Purpose
Investigate and resolve GoStream maintenance issues received as an IssueBatch. This is the core decision-making skill.

## Activation
Activated by the dispatcher when a batch is dequeued. Receives the full IssueBatch JSON.

## User Preferences (HARDCODED — do not deviate)

### Torrent Selection Preferences
- **Smallest file size** within the same quality tier (prefer smaller files with same quality)
- **Maximum seeders** — higher is always better
- Trade-off: small file + high seeders > large file + low seeders

### Issue Priority (by annoyance)
1. **B** — Films that won't start (dead torrents, slow startup, errors)
2. **A** — Wrong films (CAM, wrong language, wrong version)
3. **C** — Incomplete TV series (missing seasons/episodes in favorites)
4. **D** — Missing subtitles (especially English)

### Favorites Requirements
- **Movies**: Must be 100% pre-downloaded for instant Jellyfin playback
- **TV Series**: Must be COMPLETE — all seasons and episodes available

## Workflow

### Step 1: Read and validate batch
1. Parse the IssueBatch JSON
2. **Check action history** at `~/.hermes/gostream-action-history.json`:
   - If the same issue_type + same file/torrent_id was addressed < 30 min ago → SKIP it
   - Log: "Skipping duplicate issue {type} for {file} (addressed {N} min ago)"
3. For each remaining issue, proceed to investigation

### Step 2: Investigate each issue

Use these tools to gather information:

#### GoStream API (curl to localhost:9080)
```bash
# Check torrent state
curl -s "http://localhost:9080/api/ai/torrent-state?id=<torrent_id>"

# Check active playback
curl -s "http://localhost:9080/api/ai/active-playback"

# Check FUSE health
curl -s "http://localhost:9080/api/ai/fuse-health"

# Check queue status
curl -s "http://localhost:9080/api/ai/queue-status"

# Get recent logs
curl -s "http://localhost:9080/api/ai/recent-logs?lines=100"

# Check favorites
curl -s "http://localhost:9080/api/ai/favorites-check"
```

#### Jellyfin API (curl)
```bash
# Health
curl -s "<JELLYFIN_URL>/System/Info?api_key=<KEY>"

# Active sessions
curl -s "<JELLYFIN_URL>/Sessions?api_key=<KEY>"

# Item details with subtitle info
curl -s "<JELLYFIN_URL>/Items/<ID>?api_key=<KEY>"

# Library status
curl -s "<JELLYFIN_URL>/Items?IncludeItemTypes=Movie,Series&Recursive=true&api_key=<KEY>"
```

#### Prowlarr API (curl)
```bash
# Search
curl -s "<PROWLARR_URL>/api/v1/search?query=<QUERY>&categories=<CAT>&api_key=<KEY>"

# Indexer health
curl -s "<PROWLARR_URL>/api/v1/indexer?api_key=<KEY>"
```

#### TMDB API (curl)
```bash
# Search movie
curl -s "https://api.themoviedb.org/3/search/movie?query=<QUERY>&api_key=<KEY>"

# TV series details
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>?api_key=<KEY>"

# Season episodes
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>/season/<N>?api_key=<KEY>"
```

### Step 3: Categorize actions

After investigation, categorize each issue:

**LOW-RISK (auto-execute):**
- Download subtitles via Jellyfin/OpenSubtitles
- Retry a failed download
- Re-queue a stuck torrent (remove + re-add same magnet)
- Clear stale entries from GoStorm

**HIGH-RISK (requires Telegram approval):**
- Remove a torrent and replace with a different one
- Delete files from disk
- Change GoStream quality/config settings
- Modify GoStream code

**STUCK (ask user):**
- Unclear root cause after investigation
- Multiple conflicting issues found
- No alternatives available (e.g., no other torrent with acceptable quality)

### Step 4: Execute actions

**For LOW-RISK:**
Execute directly. Log each action.

**For HIGH-RISK:**
Send Telegram message via gateway:
```
⚠️ HIGH-RISK ACTION REQUIRED

Problem: {what's broken}
Diagnosis: {root cause — GoStream/Jellyfin/Prowlarr}
Proposed fix: {exactly what you want to do}
Alternative if nothing: {what happens if we don't act}

Reply: yes / no / any question
```

Wait for user reply before executing.

**For STUCK:**
Send Telegram message:
```
❓ STUCK — Need your judgment

Context: {what you found}
Issue: {why you can't resolve it}
Options: {what could be tried if user provides direction}

What should I do?
```

### Step 5: Record results

After all actions, append to `~/.hermes/gostream-action-history.json`:
```json
{
  "ts": "<ISO timestamp>",
  "issue_type": "<type from batch>",
  "file": "<filename>",
  "torrent_id": "<id>",
  "root_cause": "<what you found>",
  "action": "<what you did>",
  "result": "success|partial|failed",
  "confidence": 0.95,
  "user_approved": true,
  "lessons": "<what you learned for next time>"
}
```

### Step 6: Notify user

Send Telegram summary:
```
✅ Maintenance batch {batch_id} processed

Fixed: {N} issues
Skipped: {M} duplicates
Pending approval: {K}
Actions taken:
  1. {action 1 summary}
  2. {action 2 summary}
```

## Self-Learning
If you discover a new pattern or a more efficient way to solve a problem:
1. Add a note to this skill file under a "## Lessons Learned" section
2. The note should be specific: "For 4K REMUX files, search with 'REMUX' + '4K' in Prowlarr, filter seeders > 50, prefer size < 50GB"

## Error Handling
- If GoStorm API is unreachable: log, retry once after 10s, report failure
- If Jellyfin API returns 401/403: send resource request for API key
- If TMDB API returns 429 (rate limit): wait 60s and retry once
- If Prowlarr returns no results: try Torrentio fallback URL
- If action fails: mark as "failed" in history, report to user
