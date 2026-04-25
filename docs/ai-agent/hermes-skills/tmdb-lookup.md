# TMDB Lookup Skill

**Purpose:** Verify TV series completeness and resolve metadata IDs.

## Configuration

Expected environment variable: `TMDB_API_KEY`

## Tools

### Search Movie
```bash
curl -s "https://api.themoviedb.org/3/search/movie?query=<query>&api_key=$TMDB_API_KEY" | python3 -c "
import sys,json
for r in json.load(sys.stdin).get('results',[])[:5]:
    print(f\"{r['id']} | {r['title']} ({r.get('release_date','?')[:4]}) | Vote:{r.get('vote_average','?')}\")
"
```

### Search TV Series
```bash
curl -s "https://api.themoviedb.org/3/search/tv?query=<query>&api_key=$TMDB_API_KEY" | python3 -c "
import sys,json
for r in json.load(sys.stdin).get('results',[])[:5]:
    print(f\"{r['id']} | {r['name']} ({r.get('first_air_date','?')[:4]}) | Vote:{r.get('vote_average','?')}\")
"
```

### Get TV Series Details (Season Count)
```bash
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>?api_key=$TMDB_API_KEY" | python3 -c "
import sys,json
d=json.load(sys.stdin)
print(f\"Name: {d['name']}\")
print(f\"Seasons: {d['number_of_seasons']}\")
print(f\"Episodes: {d['number_of_episodes']}\")
for s in d.get('seasons',[]):
    print(f\"  S{s['season_number']:02d}: {s['episode_count']} episodes\")
"
```

### Get Season Episodes
```bash
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>/season/<SEASON_NUM>?api_key=$TMDB_API_KEY" | python3 -c "
import sys,json
d=json.load(sys.stdin)
print(f\"Season {d['season_number']}: {len(d.get('episodes',[]))} episodes\")
for e in d.get('episodes',[]):
    print(f\"  E{e['episode_number']:02d}: {e['name']}\")
"
```

### Get External IDs (IMDB)
```bash
# Movie
curl -s "https://api.themoviedb.org/3/movie/<TMDB_ID>/external_ids?api_key=$TMDB_API_KEY"
# TV
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>/external_ids?api_key=$TMDB_API_KEY"
```

### Resolve IMDB ID → TMDB ID
```bash
# Search by IMDB ID
curl -s "https://api.themoviedb.org/3/find/<IMDB_ID>?api_key=$TMDB_API_KEY&external_source=imdb_id" | python3 -c "
import sys,json
d=json.load(sys.stdin)
if d.get('movie_results'):
    print(f\"Movie: {d['movie_results'][0]['id']}\")
elif d.get('tv_results'):
    print(f\"TV: {d['tv_results'][0]['id']}\")
else:
    print('Not found')
"
```

## Use Cases

### Verify TV Series Completeness
1. Get TMDB ID from Jellyfin provider IDs
2. Get TMDB season/episode counts
3. Get Jellyfin episode counts via API
4. Compare → identify missing episodes

### Find Movie Metadata
1. Search TMDB by title
2. Get IMDB ID for cross-referencing
3. Match against GoStream torrent metadata
