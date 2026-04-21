# Design: DiskPiece Persistent Cache + Favorite Pre-Download

**Date:** 2026-04-21
**Status:** Draft — pending review
**Branch:** `feature/disk-piece-cache`

## Problem Statement

1. **No persistent download cache**: GoStorm stores torrent pieces 100% in RAM. When a torrent expires (30s after playback stops), all downloaded data is lost. Re-playing the same content the next day requires re-downloading from scratch.

2. **No pre-download for favorites**: When users mark a movie as favorite in Jellyfin, no preemptive download occurs. Movies with few seeders can take minutes to start.

3. **TV series partial downloads**: The TV sync engine creates MKV stubs but the torrent is removed from RAM after 30s of inactivity. Rehydration re-adds the magnet but must wait for metadata again.

## Architecture Overview

Three interconnected features:

1. **DiskPiece persistent storage**: Replace `MemPiece` (RAM-only) with `DiskPiece` (mmap-backed files on disk) as the default piece storage. All downloaded torrent data persists on disk. LRU eviction when quota exceeded.

2. **Favorite pre-download**: When a movie is favorited in Jellyfin, ws-bridge triggers a full background download via GoStorm. Upload/seeding is disabled for these torrents.

3. **TV series favorites**: Unchanged behavior — creates MKV stubs + Jellyfin refresh, no background download.

## Component 1: DiskPiece Persistent Storage

### Current Architecture

```
MemPiece (current default):
  Piece struct → mPiece (*MemPiece) → []byte buffer in RAM
  Cache.Close() → pieces nil'd → Go GC reclaims all memory
  Result: All data lost when torrent is removed
```

### New Architecture

```
DiskPiece (new default):
  Piece struct → dPiece (*DiskPiece) → mmap-backed file on disk
  Cache.Close() → pieces nil'd but files persist on disk
  Cache.Init() → checks disk for existing pieces, restores them
  Result: All data persists until LRU eviction
```

### File: `internal/gostorm/torr/storage/torrstor/diskpiece.go` (NEW)

```go
type DiskPiece struct {
    piece *Piece
    file  *os.File
    data  []byte  // mmap view of the file
    path  string  // TorrentsSavePath/<hash>/pieces/<id>.dat
    size  int64
}

func NewDiskPiece(p *Piece) *DiskPiece
func (dp *DiskPiece) WriteAt(b []byte, off int64) (int, error)
func (dp *DiskPiece) ReadAt(b []byte, off int64) (int, error)
func (dp *DiskPiece) Release()  // syncs to disk, keeps file
func (dp *DiskPiece) HasData() bool
func (dp *DiskPiece) Size() int64
```

### File: `internal/gostorm/torr/storage/torrstor/piece.go` (MODIFY)

```go
func NewPiece(id int, cache *Cache) *Piece {
    p := &Piece{Id: id, cache: cache}
    p.dPiece = NewDiskPiece(p)  // DiskPiece is now the default
    return p
}

func (p *Piece) WriteAt(b []byte, off int64) (int, error) {
    return p.dPiece.WriteAt(b, off)  // delegates to DiskPiece
}

func (p *Piece) ReadAt(b []byte, off int64) (int, error) {
    return p.dPiece.ReadAt(b, off)
}

func (p *Piece) MarkNotComplete() {
    // DiskPiece: delete the file to mark as corrupted
    if p.dPiece != nil {
        p.dPiece.Release()  // closes file handle
        os.Remove(p.dPiece.path)
        p.dPiece = NewDiskPiece(p)  // fresh empty piece
    }
}
```

### File: `internal/gostorm/torr/storage/torrstor/cache.go` (MODIFY)

`Cache.Init()` — add disk restore step after creating empty pieces:
```go
func (c *Cache) Init() {
    // ... existing piece creation ...
    
    // Check disk for existing pieces from previous session
    piecesDir := filepath.Join(settings.BTsets.TorrentsSavePath, c.hash.HexString(), "pieces")
    if entries, _ := os.ReadDir(piecesDir); entries != nil {
        for _, entry := range entries {
            // Parse piece ID from filename
            // Read file into mmap view
            // Restore piece data
        }
    }
}
```

### Directory Structure

```
TorrentsSavePath/
├── <hash1>/
│   ├── pieces/
│   │   ├── 0.dat
│   │   ├── 1.dat
│   │   └── ...
│   └── meta.json  ← metadata for LRU tracking
├── <hash2>/
│   ├── pieces/
│   │   └── ...
│   └── meta.json
└── .warmup files (existing, count toward quota)
```

### meta.json format

```json
{
  "hash": "abc123...",
  "title": "Free Guy (2021)",
  "size_bytes": 2147483648,
  "piece_count": 8192,
  "created_at": "2026-04-21T10:00:00Z",
  "last_access": "2026-04-21T15:30:00Z"
}
```

## Component 2: LRU Cache Eviction

### File: `cleanup.go` (MODIFY)

Add `enforceDiskCacheQuota()` to the existing `CleanupManager.runCleanup()` loop (runs every 5 minutes):

```go
func (m *CleanupManager) enforceDiskCacheQuota() {
    quota := int64(settings.BTsets.DiskCacheQuotaGB) * 1024 * 1024 * 1024
    totalSize := calculateTotalDiskCacheSize()
    
    for totalSize > quota {
        oldest := findOldestTorrentByAccess()
        if oldest == nil { break }
        
        removedSize := removeTorrentFromDisk(oldest.Hash)
        totalSize -= removedSize
    }
}
```

### Config

```json
{
  "disk_cache_quota_gb": 50
}
```

Default: 50 GB. Configurable in `config.json` and via `GOSTREAM_DISK_CACHE_QUOTA_GB` env var.

## Component 3: Favorite Pre-Download for Movies

### ws-bridge.py (MODIFY)

Update the polling logic to differentiate between movies and series:

```python
def poll_favorites(cfg, synced_ids):
    # ... get favorited items ...
    for item in items:
        item_type = item.get("Type", "")
        tmdb_id = int(item.get("ProviderIds", {}).get("Tmdb", 0))
        
        if item_type == "Series":
            # Existing behavior: trigger demand sync (MKV creation only)
            trigger_demand(tmdb_id, item_id, cfg)
        elif item_type == "Movie":
            # NEW: trigger pre-download
            trigger_movie_download(tmdb_id, item_id, cfg)
```

### New ws-bridge function

```python
def trigger_movie_download(tmdb_id, jellyfin_item_id, cfg):
    """Trigger full movie download in background."""
    url = f"{cfg['gostream_url']}/api/movie-cache/download"
    data = json.dumps({
        "tmdb_id": tmdb_id,
        "jellyfin_item_id": jellyfin_item_id
    }).encode()
    # POST request...
```

### New GoStream endpoint (main.go)

```
POST /api/movie-cache/download
Body: {"tmdb_id": 550, "jellyfin_item_id": "abc123"}
Response: {"job_id": "movie-550", "status": "started"}

GET /api/movie-cache/status/{job_id}
Response: {
    "job_id": "movie-550",
    "status": "downloading",
    "progress": 0.75,
    "downloaded_bytes": 1610612736,
    "total_bytes": 2147483648
}
```

### Background download handler

```go
func handleMovieDownload(job *MovieDownloadJob) {
    // 1. Find best torrent via Prowlarr (same size-first rules)
    // 2. Add torrent to GoStorm with IsPriority=false
    // 3. Set upload limit to 0 (no seeding)
    // 4. Poll download progress
    // 5. When complete: trigger Jellyfin refresh
}
```

## Component 4: No Upload/Seeding for Pre-Downloads

### Torrent seeding disabled

When a torrent is added via the pre-download endpoint:

```go
func addTorrentForPreDownload(magnet, title string) error {
    hash, err := gostorm.AddTorrent(ctx, magnet, title)
    if err != nil { return err }
    
    t := gostorm.GetTorrent(hash)
    t.IsPriority = false       // background priority
    t.SetUploadLimit(0)        // zero upload bandwidth
    t.SetSeedMode(false)       // don't announce as seed
    
    return nil
}
```

### Config

```json
{
  "disable_preload_seeding": true  // default: true
}
```

When `true`, any torrent added via `/api/movie-cache/download` has seeding disabled. Torrents added via normal playback flow are unaffected.

## Component 5: TV Series Favorites — Unchanged

When a TV series is favorited:
1. ws-bridge triggers `/api/tv-sync/demand` (existing endpoint)
2. GoStream creates MKV stubs for all episodes
3. Jellyfin refresh is triggered
4. **No background download** — actual torrent download happens only when user presses Play

## Execution Flows

### Flow 1: Movie Favorite Pre-Download

```
User on Jellyfin TV → ❤️ "Free Guy"
    ↓
Jellyfin fires: UserDataChanged (IsFavorite=true, Type=Movie)
    ↓
ws-bridge polls favorites → finds new movie
    ↓
POST /api/movie-cache/download {"tmdb_id": 550, "jellyfin_item_id": "xyz"}
    ↓
GoStream searches Prowlarr with "Free Guy (2021)" → finds best torrent
    ↓
AddTorrent(magnet) + SetUploadLimit(0) + SetSeedMode(false)
    ↓
Background download in progress (low bandwidth priority)
    ↓
Download complete → POST /Items/xyz/Refresh → Jellyfin updates library
    ↓
User presses Play → Reads from disk (DiskPiece) → Instant playback
```

### Flow 2: TV Series Favorite (unchanged)

```
User on Jellyfin TV → ❤️ "Lost"
    ↓
ws-bridge polls favorites → finds new series
    ↓
POST /api/tv-sync/demand {"tmdb_id": 4607, "jellyfin_item_id": "abc"}
    ↓
GoStream creates MKV stubs for all episodes
    ↓
POST /Items/abc/Refresh → Jellyfin updates library
    ↓
User presses Play on S01E01 → Torrent activated → DiskPiece caches to disk
```

### Flow 3: Playback with existing disk cache

```
User presses Play on "Free Guy" (already pre-downloaded)
    ↓
FUSE → Open() → Wake(magnet) → GotInfo()
    ↓
ReadAt() → DiskPiece.ReadAt() → reads from mmap file on disk
    ↓
Instant playback — zero download needed
```

### Flow 4: LRU eviction

```
CleanupManager runs every 5 minutes
    ↓
Calculates total disk usage: 52 GB (quota: 50 GB)
    ↓
Finds oldest torrent by last_access: "OldMovie (2019)"
    ↓
Removes /TorrentsSavePath/<hash>/pieces/ directory
    ↓
Updates meta.json for remaining torrents
    ↓
Total usage: 48 GB → OK
```

## Config Summary

```json
{
  "disk_cache_quota_gb": 50,
  "disable_preload_seeding": true,
  "jellyfin": {
    "url": "http://localhost:8096",
    "api_key": "xxx"
  }
}
```

## File Changes Summary

| File | Change |
|------|--------|
| `internal/gostorm/torr/storage/torrstor/diskpiece.go` | NEW: DiskPiece struct with mmap-backed piece storage |
| `internal/gostorm/torr/storage/torrstor/piece.go` | MODIFY: Use DiskPiece as default instead of MemPiece |
| `internal/gostorm/torr/storage/torrstor/cache.go` | MODIFY: Add disk restore in Init(), update Close() |
| `internal/gostorm/torr/storage/torrstor/cache_deadlock_test.go` | MODIFY: Update tests for DiskPiece |
| `internal/gostorm/torr/torrent.go` | MODIFY: Add SetUploadLimit(), SetSeedMode() methods |
| `internal/gostorm/settings/btsets.go` | MODIFY: Add DiskCacheQuotaGB, DisablePreloadSeeding |
| `cleanup.go` | MODIFY: Add enforceDiskCacheQuota() to cleanup loop |
| `config.go` | MODIFY: Add DiskCacheQuotaGB, DisablePreloadSeeding fields |
| `main.go` | MODIFY: Register new /api/movie-cache/* endpoints |
| `demand_handler.go` | MODIFY: Add movie download handler |
| `ws-bridge.py` | MODIFY: Differentiate movie vs series favorites |

## Error Handling

| Error | Handling |
|-------|----------|
| Disk full during piece write | Log error, mark piece as incomplete, retry on next read |
| mmap file corruption | Delete corrupted file, re-download piece from swarm |
| Pre-download torrent has 0 seeders | Job fails after timeout, log warning, user can retry |
| Quota exceeded but no evictable torrents | Log warning, stop accepting new pre-downloads until space freed |
| Jellyfin refresh fails | Log warning, retry once after 30s |

## Testing Strategy

1. **Unit tests**: DiskPiece WriteAt/ReadAt/Release — verify data persistence
2. **Unit tests**: Cache.Init() restores pieces from disk
3. **Unit tests**: LRU eviction — verify oldest-by-access is removed first
4. **Integration test**: Full movie pre-download flow (ws-bridge → GoStream → disk)
5. **Integration test**: Playback from cached disk pieces
6. **Integration test**: TV series favorite → MKV creation only, no download
7. **Load test**: 50 GB cache with multiple movies — verify eviction works correctly
8. **Performance test**: Playback latency from DiskPiece vs MemPiece (should be comparable due to mmap)

## Migration

No migration needed. Existing torrents in RAM will be lost on restart (as before). New torrents will use DiskPiece and persist to disk. The existing `.warmup` and `.warmup-tail` files continue to work alongside the new disk cache.

## Rollback

To revert: set `disk_cache_quota_gb: 0` in config.json. This disables disk caching — pieces will still be written to disk but the cleanup worker will immediately evict them. Effectively restores RAM-only behavior.
