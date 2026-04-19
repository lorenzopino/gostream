# GoStream — Context for AI Assistance

## Project Overview

**GoStream** is a high-performance BitTorrent streaming engine combined with a custom FUSE virtual filesystem, designed to stream 4K HDR Dolby Vision content to media servers (Plex/Jellyfin) without downloading files to disk. Every `.mkv` file appears as a real file on the filesystem, but bytes are served live from BitTorrent swarms on demand.

### Key Architecture Components

- **FUSE Virtual Filesystem**: Custom implementation using `hanwen/go-fuse/v2` that presents torrent-backed `.mkv` files as real files
- **GoStorm Engine**: Forked and patched torrent engine (`internal/anacrolix-torrent-v1.55`) running in-process with the FUSE layer
- **Native Bridge**: Zero-network, zero-copy communication between FUSE layer and torrent engine via `io.Pipe()`
- **Three-Layer Cache System**:
  - L1: 256 MB in-memory read-ahead cache (32-shard LRU)
  - L2: Optional SSD warmup head cache (64 MB/file)
  - L3: Optional SSD warmup tail cache (16 MB/file for MKV Cues)
- **Built-in Sync Engine**: Native Go implementation for TMDB discovery, Prowlarr/Torrentio integration, and automated library management
- **Webhook Integration**: Plex/Jellyfin webhook receiver for priority mode activation
- **Control Panel & Dashboard**: Embedded web UI at `:9080/control` and `:9080/dashboard`

### Port Map

| Port | Purpose |
|------|---------|
| `:8090` | GoStorm API (JSON torrent management) |
| `:9080` | Control Panel, Metrics, Dashboard, Webhook, Scheduler |

## Technology Stack

- **Language**: Go 1.24+
- **Targets**: `linux/amd64`, `linux/arm64` (minimum: Raspberry Pi 4 with 4 GB RAM)
- **FUSE Library**: `github.com/hanwen/go-fuse/v2`
- **Torrent Engine**: Forked `github.com/anacrolix/torrent v1.55`
- **Web Framework**: `github.com/gin-gonic/gin`
- **Database**: SQLite (`modernc.org/sqlite`) and BoltDB (`go.etcd.io/bbolt`)
- **Build**: Standard Go toolchain with PGO (`-pgo=auto`)

## Project Structure

```
gostream/
├── main.go                 # Entry point: FUSE mount, HTTP servers, initialization
├── config.go               # Configuration management (JSON + env overrides)
├── settings.html           # Embedded control panel UI
├── gostream.service        # systemd service file
├── install.sh              # Interactive installer
├── *.go                    # Core FUSE proxy modules (cache, locks, rate limiting, etc.)
├── internal/
│   ├── gostorm/           # Torrent engine (forked TorrServer + anacrolix)
│   ├── anacrolix-torrent-v1.55/  # Patched torrent library
│   ├── config/            # Configuration types and quality profiles
│   ├── syncer/            # Sync engines (movies, TV, watchlist)
│   │   └── engines/       # Native Go sync implementations
│   ├── prowlarr/          # Prowlarr API client
│   ├── monitor/           # Health dashboard and metrics
│   │   └── dashboard/     # Dashboard HTML and real-time stats
│   ├── metadb/            # SQLite metadata layer
│   ├── opentracker/       # O(1) open handle tracking
│   ├── catalog/           # Content catalog management
│   └── updater/           # Self-update mechanism
├── scripts/               # Legacy Python sync scripts (fallback)
├── docs/                  # Documentation and screenshots
└── ai/                    # AI Pilot experimental features
```

## Building and Running

### Build from Source

```bash
# Ensure Go 1.24+ is installed
go version

# Build with PGO (profile-guided optimization)
go build -pgo=auto -o gostream .
```

### Quick Install (Production)

```bash
curl -fsSL https://raw.githubusercontent.com/MrRobotoGit/gostream/main/install.sh -o install.sh
chmod +x install.sh
./install.sh
```

### Running the Service

```bash
# Start via systemd
sudo systemctl start gostream

# View logs
journalctl -u gostream -f
# or
tail -f logs/gostream.log
```

### Development Run

```bash
# Set memory limits for Pi 4
export GOMEMLIMIT=2200MiB
export GOGC=100

# Run (requires FUSE setup and config.json)
sudo ./gostream --path .
```

### Configuration

Configuration is stored in `config.json` (co-located with the binary). See `config.json.example` for a template.

Key configuration sections:
- `master_concurrency_limit`: Global concurrency (default: 25)
- `read_ahead_budget_mb`: Read-ahead cache size (default: 512)
- `scheduler`: Built-in sync scheduler
- `prowlarr`: Prowlarr indexer integration
- `quality`: Quality profiles for movies/TV
- `tmdb_discovery`: TMDB API settings

### Testing

```bash
# Run all tests
go test ./...

# Run specific test
go test -run TestFetchBlock -v .
```

## Key Design Patterns

### 1. Native Bridge (Zero-Network Communication)
FUSE layer calls directly into GoStorm via `io.Pipe()` — no HTTP round-trips, no serialization overhead.

### 2. Seek-Master Architecture
Five coordinated fixes enable low-latency seeking in multi-gigabyte files:
- Eager offset updates
- Atomic pipe interruption
- Reactive jump detection
- Pump survival on errors
- Tail probe detection for MKV Cues

### 3. Adaptive Responsive Shield
Two read modes managed automatically:
- **Responsive**: Serve data before SHA1 verification (default)
- **Strict**: Only serve SHA1-verified pieces (activated on corruption detection for 60s)

### 4. Inode Persistence
SQLite-backed inode map ensures files retain stable IDs across restarts, preventing Plex/Jellyfin metadata rescans.

### 5. Priority Mode
Webhook-triggered aggressive piece prioritization for currently-playing content, with automatic deprioritization of background activity.

## Development Conventions

### Code Style
- Standard Go formatting (use `gofmt`)
- Comments in English (codebase has mixed Italian/English comments from original author)
- Error handling with explicit checks and logging
- Atomic operations for hot-path concurrency (avoid mutex contention)

### Configuration Changes
- All config changes should be made in `config.go` or `internal/config/`
- New fields need JSON tags and env override support in `applyEnvOverrides()`
- Use `finalize()` to map JSON fields to internal logic fields

### Adding Features
- Prefer native Go implementations over external scripts
- Embed HTML assets using `//go:embed` directives
- Use `sync.Map` for concurrent maps with frequent reads
- Track open handles with O(1) structures (see `opentracker/`)

### Important Constraints
- **Never block the FUSE hot path** — use async operations where possible
- **Defensive copies** for cache entries returned via channels
- **Atomic operations** for frequently updated shared state
- **Profile-guided optimization** enabled — hot paths matter on Pi 4

## Troubleshooting Common Issues

### FUSE Mount Problems
```bash
# Force unmount
sudo fusermount -uz /mnt/torrserver-go

# Recreate mount point
sudo mkdir -p /mnt/torrserver-go
```

### Memory Issues
- Adjust `GOMEMLIMIT` in `gostream.service` (2200MiB for Pi 4)
- Reduce `read_ahead_budget_mb` in config.json
- Check for goroutine leaks with `pprof`

### Torrent Resolution
- Service waits for DNS before starting (`nslookup www.google.com`)
- Check tracker connectivity in logs
- Verify blocklist download succeeded

## Key Files to Understand

| File | Purpose |
|------|---------|
| `main.go` | Entry point, FUSE setup, HTTP servers |
| `config.go` | All configuration management |
| `lrucache.go` | 32-shard read-ahead cache |
| `inodemap.go` | Persistent inode tracking |
| `disk_warmup.go` | SSD warmup cache management |
| `startup.go` | Initialization sequence |
| `locks.go` | File-level locking for concurrent reads |
| `synccache.go` | Sync cache for torrent metadata |
| `internal/gostorm/` | Torrent engine implementation |
| `internal/syncer/engines/` | Native sync implementations |

## Comprehensive Wiki

A complete, auto-generated wiki with detailed code analysis and documentation is available at:

- **Location**: `/Users/lorenzo/VSCodeWorkspace/gostream/.gitnexus/wiki`
- **Entry Point**: Open `/Users/lorenzo/VSCodeWorkspace/gostream/.gitnexus/wiki/index.html` in a browser
- **Content**: Module-by-module breakdown, architecture docs, implementation details for all subsystems (torrent core, FUSE layer, sync engine, NAT-PMP, health monitoring, AI integration, etc.)

The wiki is generated by GitNexus (`npx gitnexus wiki`) and provides deep references for understanding any part of the codebase. Always check the wiki first when exploring unfamiliar subsystems.

## macOS Deployment (This System)

GoStream runs **natively on macOS** — not inside any container (no Docker, no VM). It is deployed as a system-level service via `launchd`.

### LaunchAgent Configuration

- **Plist File**: `~/Library/LaunchAgents/com.gostream.daemon.plist`
- **Label**: `com.gostream.daemon`
- **Binary Location**: `/Users/lorenzo/VSCodeWorkspace/gostream/gostream`
- **Working Directory**: `/Users/lorenzo/MediaCenter/gostream/state`

### Runtime Arguments

```
/Users/lorenzo/VSCodeWorkspace/gostream/gostream
  -path /Users/lorenzo/MediaCenter/gostream/state
  /Users/lorenzo/MediaCenter/gostream-real
  /Users/lorenzo/MediaCenter/gostream-fuse
```

| Argument | Purpose |
|----------|---------|
| `-path <dir>` | State directory (config, DB, persistent data) |
| `<physical_source_path>` | `/Users/lorenzo/MediaCenter/gostream-real` — real backing directory |
| `<fuse_mount_path>` | `/Users/lorenzo/MediaCenter/gostream-fuse` — FUSE mount point |

### Log Files

| Log | Path |
|-----|------|
| stdout | `/Users/lorenzo/MediaCenter/gostream/logs/gostream-launchd.stdout.log` |
| stderr | `/Users/lorenzo/MediaCenter/gostream/logs/gostream-launchd.stderr.log` |

### Deployment Workflow (After Building a New Version)

After making code changes and building a new binary:

```bash
# 1. Build the new binary in the project directory
cd /Users/lorenzo/VSCodeWorkspace/gostream
go build -pgo=auto -o gostream .

# 2. Stop the running service
launchctl stop com.gostream.daemon

# 3. Start the service (launchd will pick up the new binary)
launchctl start com.gostream.daemon

# 4. Verify it's running
ps aux | grep gostream | grep -v grep
launchctl list | grep com.gostream.daemon
```

### Useful launchctl Commands

```bash
# Check service status
launchctl list | grep com.gostream.daemon

# Stop the service
launchctl stop com.gostream.daemon

# Start the service
launchctl start com.gostream.daemon

# Unload (remove from launchd) — use if you need to edit the plist
launchctl unload ~/Library/LaunchAgents/com.gostream.daemon.plist

# Reload after editing plist
launchctl load ~/Library/LaunchAgents/com.gostream.daemon.plist
```

### Key Characteristics

- **RunAtLoad**: true — starts automatically on login/boot
- **KeepAlive**: with `SuccessfulExit=false` — restarts on crash
- **ThrottleInterval**: 10 seconds — minimum restart delay
- **No container isolation**: runs as user `lorenzo` with full system access to the specified paths

## GitNexus Integration

This project is indexed by GitNexus. Before making code changes:

1. Run impact analysis: `gitnexus_impact({target: "symbolName", direction: "upstream"})`
2. Check blast radius before editing any function/class
3. Use `gitnexus_detect_changes()` before committing
4. Reindex after significant changes: `npx gitnexus analyze`

## Hippo Memory — Persistent Session Memory

This project uses [Hippo Memory](https://github.com/kitfunso/hippo-memory) for biologically-inspired persistent memory across sessions.

### Automatic Hooks (handled by `.qwen/settings.json`)
- **SessionStart**: `hippo last-sleep` — loads consolidated memories from previous session
- **SessionEnd**: `hippo session-end` — consolidates memories, sleeps, and learns from transcript

### Manual Commands (use during session)
| Command | When to use |
|---------|-------------|
| `hippo context --auto` | Load relevant memories before starting a task |
| `hippo recall "query" --budget 2000` | Search for specific lessons |
| `hippo remember "lesson" --tag error` | Store a learned lesson (use `--tag error` for errors) |
| `hippo outcome --good` | Previous recall was helpful |
| `hippo outcome --bad` | Previous recall was irrelevant |
| `hippo status` | Check memory health |
| `hippo conflicts` | Check for contradictory memories |
| `hippo decide "decision" --context "reason"` | Record architectural decisions (90-day half-life) |

### Rules
- When you learn something important during a session → `hippo remember "..."`
- When an error occurs → `hippo remember "error details" --error --tag error`
- Before starting a non-trivial task → `hippo context --auto --budget 1500`
- When resolving a conflict → `hippo resolve <conflict_id> --keep <memory_id>`

<!-- hippo:start -->
<!-- hippo:end -->

## External Dependencies

- **TMDB API**: Movie/TV metadata (requires API key)
- **Prowlarr**: Torrent indexer (optional, with Torrentio fallback)
- **Plex/Jellyfin**: Media server integration via webhooks
- **Samba**: Network file sharing with FUSE-backed mounts

## Performance Targets (Raspberry Pi 4 Baseline)

| Metric | Target |
|--------|--------|
| Cold start | 2–4 s |
| Warm start (SSD hit) | 0.1–0.5 s |
| Seek latency (cached) | < 0.01 s |
| CPU at 4K streaming | 20–23% of one core |
| Binary size | ~33 MB |
| Memory footprint | 2200 MiB (GOMEMLIMIT) |
| Peak throughput | 400+ Mbps |
