# TMDB Lookup Skill

## Purpose
Query TMDB API for movie/TV series metadata, episode counts, and external IDs. Used by GoStream maintenance subagent to verify content completeness.

## Configuration
Read from `~/.hermes/config/gostream.json`:
```json
{
  "tmdb": {
    "api_key": "<your-tmdb-api-key>"
  }
}
```
Or environment variable: `TMDB_API_KEY`

Base URL: `https://api.themoviedb.org/3`

## Tools

### search_movie(query)
```bash
curl -s "https://api.themoviedb.org/3/search/movie?query=<QUERY>&api_key=<KEY>"
```
Returns: `results[]` with `id`, `title`, `release_date`, `popularity`, `vote_average`.

### search_tv(query)
```bash
curl -s "https://api.themoviedb.org/3/search/tv?query=<QUERY>&api_key=<KEY>"
```
Returns: `results[]` with `id`, `name`, `first_air_date`, `popularity`.

### get_movie_details(tmdb_id)
```bash
curl -s "https://api.themoviedb.org/3/movie/<ID>?api_key=<KEY>&append_to_response=external_ids"
```
Returns: `title`, `runtime`, `budget`, `revenue`, `genres`, `external_ids.imdb_id`.

### get_tv_details(tmdb_id)
```bash
curl -s "https://api.themoviedb.org/3/tv/<ID>?api_key=<KEY>&append_to_response=external_ids"
```
Returns: `name`, `number_of_seasons`, `number_of_episodes`, `status`, `external_ids.imdb_id`.

### get_tv_season(tmdb_id, season_number)
```bash
curl -s "https://api.themoviedb.org/3/tv/<ID>/season/<N>?api_key=<KEY>"
```
Returns: `episodes[]` with `episode_number`, `name`, `air_date`, `still_path`.

### get_external_ids(tmdb_id, type)
type = "movie" or "tv"
```bash
curl -s "https://api.themoviedb.org/3/<type>/<ID>/external_ids?api_key=<KEY>"
```
Returns: `imdb_id`, `facebook_id`, `instagram_id`, `twitter_id`.

### resolve_imdb_to_tmdb(imdb_id)
Given an IMDB ID (e.g., "tt0133093"), find the TMDB ID:
```bash
# Try movie first
curl -s "https://api.themoviedb.org/3/find/<IMDB_ID>?api_key=<KEY>&external_source=imdb_id"
# If no movie results, try TV
curl -s "https://api.themoviedb.org/3/find/<IMDB_ID>?api_key=<KEY>&external_source=imdb_id"
```
Returns: `movie_results[]` or `tv_results[]` with `id`.

## Common Use Cases

### Check if TV series is complete
1. `get_tv_details(tmdb_id)` → `number_of_episodes`
2. Compare with Jellyfin available episode count
3. If Jellyfin count < TMDB count → series is incomplete

### Find missing episodes
1. `get_tv_details(tmdb_id)` → list of seasons
2. For each season: `get_tv_season(tmdb_id, season)` → episode list
3. Compare with Jellyfin episode list by season/episode number

### Resolve IMDB to TMDB for cross-referencing
1. `resolve_imdb_to_tmdb(imdb_id)` → TMDB ID
2. Use TMDB ID for detailed queries
