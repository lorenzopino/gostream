# TMDB Lookup Skill

## Purpose
Query TheMovieDB API to verify TV series completeness, resolve IMDB↔TMDB IDs, and get episode metadata.

## Configuration
Read from `~/.hermes/config/gostream.json`:
```json
{
  "tmdb": {
    "api_key": "<your-tmdb-api-key>"
  }
}
```

Or from environment variable: `TMDB_API_KEY`

## Tools

### search_movie(query)
Search for a movie.
```bash
curl -s "https://api.themoviedb.org/3/search/movie?query=<QUERY>&api_key=<KEY>&language=en-US"
```
Returns: Array of movies with `id` (TMDB ID), `title`, `release_date`, `popularity`, `vote_average`.

### search_tv(query)
Search for a TV series.
```bash
curl -s "https://api.themoviedb.org/3/search/tv?query=<QUERY>&api_key=<KEY>&language=en-US"
```
Returns: Array of series with `id`, `name`, `first_air_date`, `number_of_seasons`, `number_of_episodes`.

### get_movie(tmdb_id)
Get movie details.
```bash
curl -s "https://api.themoviedb.org/3/movie/<TMDB_ID>?api_key=<KEY>&language=en-US"
```
Returns: Title, overview, runtime, budget, revenue, genres.

### get_tv(tmdb_id)
Get TV series details.
```bash
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>?api_key=<KEY>&language=en-US"
```
Returns: `name`, `number_of_seasons`, `number_of_episodes`, `seasons` array with `season_number` and `episode_count` per season.

### get_season_episodes(tmdb_id, season_number)
Get all episodes for a specific season.
```bash
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>/season/<SEASON_NUMBER>?api_key=<KEY>&language=en-US"
```
Returns: Array of episodes with `episode_number`, `name`, `air_date`, `runtime`, `still_path`.

### get_external_ids(tmdb_id, type)
Get external IDs (IMDB, TVDB).
```bash
# For movies
curl -s "https://api.themoviedb.org/3/movie/<TMDB_ID>/external_ids?api_key=<KEY>"
# For TV
curl -s "https://api.themoviedb.org/3/tv/<TMDB_ID>/external_ids?api_key=<KEY>"
```
Returns: `imdb_id`, `tvdb_id`, `wikidata_id`.

### resolve_imdb_to_tmdb(imdb_id, type)
Resolve IMDB ID to TMDB ID.
```bash
# Search by IMDB ID via find endpoint
curl -s "https://api.themoviedb.org/3/find/<IMDB_ID>?api_key=<KEY>&external_source=imdb_id"
```
Returns: Movie or TV results with TMDB ID.

## Common Use Cases

### Verify TV Series Completeness
1. `get_tv(tmdb_id)` → get `number_of_seasons`, `number_of_episodes`
2. For each season: `get_season_episodes(tmdb_id, season_number)`
3. Compare TMDB episode count vs available episodes in Jellyfin/GoStream
4. Report missing episodes

### Resolve IMDB → TMDB
1. `resolve_imdb_to_tmdb(imdb_id, type)`
2. Use TMDB ID for further lookups

### Get Episode Info for Missing Episode
1. `get_season_episodes(tmdb_id, season)` → find specific episode by number
2. Get `name`, `air_date` for identification
3. Report: "Missing S02E05: 'Episode Name' (aired 2024-03-15)"

## Error Handling
- **401 Unauthorized**: API key invalid — send resource request
- **404 Not Found**: TMDB ID doesn't exist — log warning
- **429 Rate Limited**: Wait 10s and retry once
- **Empty results**: Query may be wrong — try alternate search terms
