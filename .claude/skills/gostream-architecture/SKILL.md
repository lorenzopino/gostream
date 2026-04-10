---
name: gostream-architecture
description: Current GoStream + Jellyfin deployment architecture. BOTH run NATIVELY on macOS host. ONLY Prowlarr and FlareSolverr run in Docker containers. NEVER put GoStream or Jellyfin in containers.
type: reference
---

# GoStream Architecture — Current Setup (April 2026)

## CRITICAL RULES

**GoStream and Jellyfin run NATIVELY on macOS. They MUST NOT be placed in Docker containers.**

- GoStream = native binary, launched manually or via launchd
- Jellyfin = native app (`/Applications/Jellyfin.app`)
- ONLY Prowlarr and FlareSolverr run in Docker containers

## Topology

```
┌─ macOS Host ───────────────────────────────────────────────┐
│                                                             │
│  GoStream (native binary)                                   │
│    ├── Binary: ~/VSCodeWorkspace/gostream/gostream          │
│    ├── FUSE mount: ~/MediaCenter/gostream-fuse              │
│    ├── Source data: ~/MediaCenter/gostream-real             │
│    ├── State: ~/MediaCenter/gostream/state                  │
│    ├── GoStorm API: :8090 (internal torrent engine)         │
│    └── Control/Dashboard: :9080 (HTTP)                      │
│                                                             │
│  Jellyfin (native app)                                      │
│    ├── App: /Applications/Jellyfin.app                       │
│    ├── Data: ~/Library/Application Support/jellyfin         │
│    ├── Config: ~/MediaCenter/jellyfin/config/               │
│    ├── FFmpeg wrapper: ~/MediaCenter/jellyfin/ffmpeg-wrapper│
│    └── Port: :8096 (HTTP)                                   │
│                                                             │
│  The FUSE mount bridges GoStream to Jellyfin:               │
│    Jellyfin reads ~/MediaCenter/gostream-fuse/*.mkv         │
│    → GoStream intercepts reads                              │
│    → Fetches bytes from BitTorrent peers in real-time       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
         │
         │ (Docker — helper services only)
         ▼
┌─ Docker Compose (~/Webapps/media-center) ──────────────────┐
│                                                             │
│  prowlarr (:9696) — torrent indexer aggregation            │
│  flaresolverr (:8191) — Cloudflare bypass                   │
│                                                             │
│  NO Jellyfin. NO GoStream.                                   │
└─────────────────────────────────────────────────────────────┘
```

## Service Details

### GoStream (Native macOS)

- **Binary**: `~/VSCodeWorkspace/gostream/gostream`
- **Command**: `gostream -path ~/MediaCenter/gostream/state ~/MediaCenter/gostream-real ~/MediaCenter/gostream-fuse`
- **Ports**:
  - `:8090` — GoStorm API (torrent management, internal)
  - `:9080` — Control Panel, Metrics, Dashboard, Webhook, Scheduler
- **Config**: `~/VSCodeWorkspace/gostream/config.json`
- **Settings**: `~/VSCodeWorkspace/gostream/settings.json`

### Jellyfin (Native macOS)

- **App**: `/Applications/Jellyfin.app`
- **Data dir**: `~/Library/Application Support/jellyfin`
- **FFmpeg**: `/Applications/Jellyfin.app/Contents/MacOS/ffmpeg` (bundled)
- **Port**: `:8096` (HTTP)
- **FFmpeg wrapper**: `~/MediaCenter/jellyfin/ffmpeg-wrapper.sh` — reduces probesize to 100M, analyzeduration to 30M
- **Encoding config**: `~/MediaCenter/jellyfin/config/encoding.xml`
  - VideoToolbox HW acceleration
  - HEVC encoding enabled
  - Throttling enabled (120s delay)

### Prowlarr (Docker)

- **Image**: `linuxserver/prowlarr:latest`
- **Port**: `9696`
- **Compose**: `~/Webapps/media-center/docker-compose.yml`
- **DNS**: Uses Google DNS (8.8.8.8) to bypass ISP blocks

### FlareSolverr (Docker)

- **Image**: `ghcr.io/flaresolverr/flaresolverr:latest`
- **Port**: `8191`
- **Role**: Cloudflare bypass for Prowlarr indexers

## Key Paths on macOS Host

| Path | Purpose |
|------|---------|
| `~/MediaCenter/gostream-fuse` | FUSE mount (virtual files for Jellyfin to read) |
| `~/MediaCenter/gostream-real` | Source data (movies/, tv/ directories) |
| `~/MediaCenter/gostream/state` | GoStream state (SQLite DB, inode map) |
| `~/MediaCenter/jellyfin/` | Jellyfin config + FFmpeg wrapper |
| `~/MediaCenter/jellyfin/config/encoding.xml` | HW acceleration, throttling settings |
| `~/MediaCenter/prowlarr/` | Prowlarr config + indexer definitions |
| `~/MediaCenter/strm-library/` | .strm files pointing to GoStorm HTTP streams |
| `~/MediaCenter/media/` | Traditional media directory |

## Restart Procedures

### Restart GoStream
```bash
pkill -9 gostream
sleep 2
nohup ~/VSCodeWorkspace/gostream/gostream \
  -path ~/MediaCenter/gostream/state \
  ~/MediaCenter/gostream-real \
  ~/MediaCenter/gostream-fuse \
  > ~/MediaCenter/gostream/gostream.log 2>&1 &
mount | grep gostream
ls ~/MediaCenter/gostream-fuse/
```

### Restart Jellyfin
```bash
pkill -f Jellyfin
open /Applications/Jellyfin.app
# Wait ~30s, then:
curl http://localhost:8096/  # should return 302 redirect
```

### Helper services (Docker only)
```bash
cd ~/Webapps/media-center
docker compose up -d  # only Prowlarr + FlareSolverr
```

## What Was Removed (Legacy)

- **MediaCenterPlugin** — Jellyfin plugin using `http://localhost:3001/api/v1/stream/movie/{id}`. Removed.
- **Backend Python API** (`:3001`) — Old mediacenter backend in `infrastructure/docker-compose.yml`. Removed.
- **Docker deployment of GoStream** — `docker/` and `docker-windows/` directories removed from repo.
- **Docker Hub publishing** — `.github/workflows/docker-publish.yml` removed.
- **Jellyfin in Docker** — Jellyfin runs as native macOS app. Container removed from compose.
- **Redis, qBittorrent, PostgreSQL** — Orphan containers from old infrastructure stack. Not used.
