# Jellyfin Health Skill

## Purpose
Check Jellyfin service health, playback sessions, subtitle availability, and library sync state. Used by the GoStream maintenance subagent during issue investigation.

## Configuration
Read from `~/.hermes/config/gostream.json`:
```json
{
  "jellyfin": {
    "url": "http://localhost:8096",
    "api_key": "<your-jellyfin-api-key>"
  }
}
```

Or from environment variables: `JELLYFIN_URL`, `JELLYFIN_API_KEY`

## Tools

### get_health
Check Jellyfin service status.
```bash
curl -s "<JELLYFIN_URL>/System/Info?api_key=<JELLYFIN_API_KEY>"
```
Returns: Server version, OS, runtime, WebSocket info. If fails → Jellyfin is down.

### get_active_sessions
Check current playback sessions.
```bash
curl -s "<JELLYFIN_URL>/Sessions?api_key=<JELLYFIN_API_KEY>"
```
Returns: Array of sessions with NowPlayingItem, Capabilities, PlayState.
Look for: Sessions with `NowPlayingItem` — these are actively playing.

### get_item_details(item_id)
Get full item details including subtitle tracks.
```bash
curl -s "<JELLYFIN_URL>/Items/<ITEM_ID>?api_key=<JELLYFIN_API_KEY>"
```
Returns: Item with `MediaStreams` array. Look for:
- `MediaStreams[].Type == "Subtitle"` and `MediaStreams[].Language == "eng"` → English subtitles available
- `MediaStreams[].Type == "Audio"` → Audio tracks
- `MediaStreams[].IsDefault` → Default track flags

### check_subtitles(item_id, language)
Check if subtitles exist for a specific language.
```bash
curl -s "<JELLYFIN_URL>/Items/<ITEM_ID>?api_key=<JELLYFIN_API_KEY>" | jq '.MediaStreams[] | select(.Type=="Subtitle")'
```
Returns: List of subtitle streams. Check `Language`, `IsExternal`, `DisplayTitle`.

### download_subtitle(item_id, language)
Trigger subtitle download via Jellyfin's OpenSubtitles plugin (if configured).
```bash
# Check if OpenSubtitles plugin is configured
curl -s "<JELLYFIN_URL>/Plugins?api_key=<JELLYFIN_API_KEY>" | jq '.[] | select(.Name | contains("OpenSubtitles"))'

# If configured, trigger library refresh which may pull subtitles
curl -s -X POST "<JELLYFIN_URL>/Items/<ITEM_ID>/Refresh?api_key=<JELLYFIN_API_KEY>&MetadataRefreshMode=FullRefresh&ImageRefreshMode=FullRefresh"
```
Note: Jellyfin's OpenSubtitles plugin auto-fetches subtitles during library scan or item refresh.

### get_library_status
Check library sync state.
```bash
curl -s "<JELLYFIN_URL>/Items?IncludeItemTypes=Movie,Series&Recursive=true&Fields=ProviderIds&api_key=<JELLYFIN_API_KEY>"
```
Returns: All items with provider IDs (TMDB, IMDB). Count items, check for missing metadata.

### trigger_library_scan
Start a full library refresh.
```bash
curl -s -X POST "<JELLYFIN_URL>/Library/Refresh?api_key=<JELLYFIN_API_KEY>"
```

## Health Check Protocol
1. Call `get_health` — if fails, Jellyfin is down
2. Call `get_active_sessions` — check for stuck playback
3. Call `get_library_status` — count items, check for recent additions
4. Return structured report:
```
Jellyfin Health:
  Status: UP/DOWN
  Version: <version>
  Active sessions: <N>
  Movies: <N>
  TV Series: <N>
  Issues: <list any problems found>
```

## Common Issues
- **503 Service Unavailable**: Jellyfin starting up or crashed
- **401 Unauthorized**: API key expired or invalid
- **No items in library**: Library paths misconfigured
- **Missing subtitles**: OpenSubtitles plugin not configured, or no matching subtitle found
