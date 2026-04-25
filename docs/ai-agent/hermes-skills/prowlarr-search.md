# Prowlarr Search Skill

**Purpose:** Search for torrent alternatives and filter by quality/seeders/size.

## Configuration

Expected environment variables or `~/.hermes/config/gostream.json`:
```json
{
  "prowlarr_url": "http://localhost:9696",
  "prowlarr_api_key": "your-api-key"
}
```

## Tools

### Search Torrents
```bash
curl -s "$PROWLARR_URL/api/v1/search?query=<query>&type=movie&api_key=$PROWLARR_API_KEY" | python3 -c "
import sys,json
results = json.load(sys.stdin)
for r in results[:20]:  # Top 20
    print(f\"Title: {r.get('title','?')} | Size: {r.get('size',0)//(1024**3)}GB | Seeders: {r.get('seeders','?')} | Indexer: {r.get('indexer','?')}\")
"
```

### Filter by Quality
Parse results to identify quality:
```python
import re

def detect_quality(title):
    title_upper = title.upper()
    if any(x in title_upper for x in ['REMUX', 'BLURAY REMUX']):
        return 'REMUX'
    if '4K' in title_upper or '2160P' in title_upper:
        return '4K'
    if '1080P' in title_upper:
        return '1080p'
    if '720P' in title_upper:
        return '720p'
    return 'unknown'
```

### Score Results (User Preferences)
Score = favor smaller size + more seeders:
```python
def score_result(result):
    """User preference: smallest file + most seeders."""
    size_gb = result.get('size', 0) / (1024**3)
    seeders = result.get('seeders', 0)
    # Lower size = better (normalized), more seeders = better
    size_score = max(0, 100 - size_gb * 5)  # Penalize large files
    seeder_score = min(100, seeders * 2)     # Reward many seeders
    return size_score * 0.6 + seeder_score * 0.4  # 60% size, 40% seeders
```

### Get Result Details (Magnet Link)
```bash
curl -s "$PROWLARR_URL/api/v1/search/<GUID>?api_key=$PROWLARR_API_KEY"
# Or use the infoUrl/magnetUrl from search results
```

### Search for TV Episodes
```bash
# By IMDB ID (more accurate for TV)
curl -s "$PROWLARR_URL/api/v1/search?query=<IMDB_ID>&type=tvsearch&api_key=$PROWLARR_API_KEY"

# By title + season
curl -s "$PROWLARR_URL/api/v1/search?query=<ShowName>+S01&type=tvsearch&api_key=$PROWLARR_API_KEY"
```

## Workflow for Finding Alternatives

1. Search Prowlarr with movie/show title
2. Filter results by quality tier (must match or exceed original)
3. Score results using user preference (smallest + most seeders)
4. Return top 3 alternatives with:
   - Title, size, seeders, indexer
   - Magnet link or download URL
   - Score

## Comparison Table Format

When presenting alternatives to user:
```
| # | Title | Size | Seeders | Quality | Score |
|---|-------|------|---------|---------|-------|
| 1 | Movie.2024.1080p.BluRay.x264 | 8.2GB | 150 | 1080p | 87 |
| 2 | Movie.2024.1080p.WEB-DL | 4.1GB | 320 | 1080p | 92 |
| 3 | Movie.2024.720p.BluRay | 3.5GB | 85 | 720p | 78 |
```
