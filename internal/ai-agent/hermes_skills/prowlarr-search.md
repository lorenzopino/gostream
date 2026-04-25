# Prowlarr Search Skill

## Purpose
Search for torrent alternatives via Prowlarr API. Used when replacing dead torrents or finding better alternatives.

## Configuration
Read from `~/.hermes/config/gostream.json`:
```json
{
  "prowlarr": {
    "url": "http://localhost:9696",
    "api_key": "<your-prowlarr-api-key>"
  }
}
```

Or from environment variables: `PROWLARR_URL`, `PROWLARR_API_KEY`

## Tools

### search(query, category)
Search for torrents.
```bash
curl -s "<PROWLARR_URL>/api/v1/search?query=<QUERY>&categories=<CAT>&apikey=<PROWLARR_API_KEY>"
```
Categories: `2000` (Movies), `5000` (TV). Returns array of results with:
- `title`, `size`, `indexer`, `seeders` (if available), `guid`, `infoUrl`

### get_result_details(guid)
Get details for a specific result.
```bash
curl -s -L -I "<PROWLARR_URL>/api/v1/download?guid=<GUID>&apikey=<PROWLARR_API_KEY>"
```
Follows redirect to actual torrent file/magnet.

### get_indexer_health
Check which indexers are alive.
```bash
curl -s "<PROWLARR_URL>/api/v1/indexer?apikey=<PROWLARR_API_KEY>" | jq '.[] | {name: .name, enabled: .enabled, status: .status}'
```

## Search Strategy (User Preferences)

When searching for alternatives:
1. **Search broad**: Use movie/show title + year
2. **Filter by quality**: Look for resolution tag (2160p, 1080p, 720p)
3. **Score results**:
   - Prefer SMALLER file size within same quality tier
   - Prefer HIGHER seeder count
   - Score = (seeders * 2) - (size_in_gb / 10)
4. **Return top 3** with comparison:

```
Alternatives for "Movie Title":
| # | Title | Size | Seeders | Score |
|---|-------|------|---------|-------|
| 1 | Movie.2024.1080p.BluRay | 12GB | 150 | 298.8 |
| 2 | Movie.2024.1080p.WEB | 8GB | 80 | 159.2 |
| 3 | Movie.2024.2160p.REMUX | 45GB | 200 | 395.5 |

Recommendation: #1 (best balance of size + seeders)
```

## Fallback: Torrentio
If Prowlarr returns no results, try Torrentio:
```bash
curl -s "https://torrentio.strem.fun/search?query=<QUERY>"
```

## Error Handling
- **401 Unauthorized**: API key invalid — send resource request
- **Empty results**: Try broader search terms (remove year, quality tags)
- **All indexers down**: Report to user — Prowlarr needs indexer maintenance
