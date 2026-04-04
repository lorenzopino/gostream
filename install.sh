#!/usr/bin/env bash
# ==============================================================================
# GoStream Installer
# GoStream + GoStorm (Unified Engine)
# Target: auto-detected at install time
# ==============================================================================
set -e

# ------------------------------------------------------------------------------
# Color output (graceful fallback when not running in a terminal)
# ------------------------------------------------------------------------------
if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
    RED=$(tput setaf 1)
    GREEN=$(tput setaf 2)
    YELLOW=$(tput setaf 3)
    BLUE=$(tput setaf 4)
    BOLD=$(tput bold)
    NC=$(tput sgr0)
else
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
fi

# ------------------------------------------------------------------------------
# Helper: print a colored section header
# ------------------------------------------------------------------------------
print_header() {
    echo ""
    echo "${BOLD}${BLUE}=== $1 ===${NC}"
    echo ""
}

print_ok()   { echo "  ${GREEN}✓${NC} $1"; }
print_warn() { echo "  ${YELLOW}⚠${NC}  $1"; }
print_err()  { echo "  ${RED}✗${NC} $1"; }
print_info() { echo "  ${BLUE}→${NC} $1"; }

# ------------------------------------------------------------------------------
# Helper: ask "Prompt" "default" VAR_NAME
#   Displays [default] hint; if user presses Enter, uses default.
# ------------------------------------------------------------------------------
ask() {
    local prompt="$1"
    local default="$2"
    local var_name="$3"
    local user_input

    if [ -n "$default" ]; then
        printf "  %s [%s]: " "$prompt" "$default"
    else
        printf "  %s: " "$prompt"
    fi

    read -r user_input
    if [ -z "$user_input" ]; then
        user_input="$default"
    fi
    # Assign to the caller's variable by name
    printf -v "$var_name" '%s' "$user_input"
}

# ------------------------------------------------------------------------------
# Helper: ask_secret "Prompt" VAR_NAME
#   Hidden input (no echo); no default shown for security.
# ------------------------------------------------------------------------------
ask_secret() {
    local prompt="$1"
    local var_name="$2"
    local user_input

    printf "  %s: " "$prompt"
    read -rs user_input
    echo ""  # newline after hidden input
    printf -v "$var_name" '%s' "$user_input"
}

# ------------------------------------------------------------------------------
# Helper: ask_yn "Question" [default_yn]
#   Returns 0 for yes, 1 for no.
#   default_yn should be "y" or "n" (case-insensitive).
# ------------------------------------------------------------------------------
ask_yn() {
    local question="$1"
    local default="${2:-n}"
    local hint user_input

    if [ "${default,,}" = "y" ]; then
        hint="Y/n"
    else
        hint="y/N"
    fi

    printf "  %s [%s]: " "$question" "$hint"
    read -r user_input

    if [ -z "$user_input" ]; then
        user_input="$default"
    fi

    case "${user_input,,}" in
        y|yes) return 0 ;;
        *)     return 1 ;;
    esac
}

# ------------------------------------------------------------------------------
# ASCII Banner
# ------------------------------------------------------------------------------
show_banner() {
    echo ""
    echo "${BOLD}${BLUE}╔══════════════════════════════════════╗${NC}"
    echo "${BOLD}${BLUE}║          GoStream Installer          ║${NC}"
    echo "${BOLD}${BLUE}╚══════════════════════════════════════╝${NC}"
    echo ""
    echo "  GoStream + GoStorm — Unified BitTorrent + FUSE Streaming Engine"
    echo "  Target:         $(uname -m) / $(uname -s), 24/7 production"
    echo "  Includes:       cron setup, sudoers, Plex webhook guide"
    echo ""
}

# ==============================================================================
# [0a] Auto-install system dependencies via apt
# ==============================================================================
install_system_deps() {
    # Only run on Debian/Ubuntu-based systems
    if ! command -v apt-get >/dev/null 2>&1; then
        print_warn "apt-get not found — skipping automatic dependency installation."
        print_warn "Please install manually: git python3-pip fuse3 curl samba"
        return 0
    fi

    # Map: package name → apt package to install
    local -A needed=()

    command -v git         >/dev/null 2>&1 || needed["git"]="git"
    command -v fusermount3 >/dev/null 2>&1 || needed["fusermount3"]="fuse3"
    command -v curl        >/dev/null 2>&1 || needed["curl"]="curl"
    command -v samba       >/dev/null 2>&1 || needed["samba"]="samba"
    # libfuse3-dev is required for CGO_ENABLED=1 compilation (provides fuse.h)
    dpkg -s libfuse3-dev   >/dev/null 2>&1 || needed["libfuse3-dev"]="libfuse3-dev"
    # gcc is required for CGO
    command -v gcc         >/dev/null 2>&1 || needed["gcc"]="gcc"

    if [ "${#needed[@]}" -eq 0 ]; then
        print_ok "All system dependencies already installed."
        return 0
    fi

    print_header "Installing System Dependencies"
    print_info "Missing packages: ${needed[*]}"
    print_info "Running: sudo apt-get update && sudo apt-get install -y ${needed[*]}"
    echo ""

    sudo apt-get update -qq
    sudo apt-get install -y "${needed[@]}"

    echo ""
    print_ok "System dependencies installed."

    # Enable user_allow_other in /etc/fuse.conf (required for FUSE allow_other mount option)
    if [ -f /etc/fuse.conf ]; then
        if ! grep -q "^user_allow_other" /etc/fuse.conf; then
            sudo sed -i 's/^#\s*user_allow_other/user_allow_other/' /etc/fuse.conf
            grep -q "^user_allow_other" /etc/fuse.conf || echo "user_allow_other" | sudo tee -a /etc/fuse.conf >/dev/null
            print_ok "FUSE: user_allow_other enabled in /etc/fuse.conf"
        fi
    fi
}

# ==============================================================================
# [0] Prerequisite checks
# ==============================================================================
check_prerequisites() {
    print_header "Checking Prerequisites"

    local all_ok=true

    # fusermount3 or fusermount (FUSE userspace tool)
    if command -v fusermount3 >/dev/null 2>&1; then
        print_ok "fusermount3"
        FUSERMOUNT_CMD="fusermount3"
    elif command -v fusermount >/dev/null 2>&1; then
        print_ok "fusermount (fusermount3 preferred but fusermount found)"
        FUSERMOUNT_CMD="fusermount"
    else
        print_err "fusermount3/fusermount not found (install: sudo apt install fuse3)"
        all_ok=false
    fi

    # systemctl
    if command -v systemctl >/dev/null 2>&1; then
        print_ok "systemctl"
    else
        print_err "systemctl not found (systemd required for service management)"
        all_ok=false
    fi

    # curl (used for Plex auto-discovery and health-check — not strictly fatal)
    if command -v curl >/dev/null 2>&1; then
        print_ok "curl"
    else
        print_warn "curl not found — Plex library auto-discovery will be skipped"
    fi

    echo ""
    if [ "$all_ok" = false ]; then
        print_err "One or more required prerequisites are missing. Please install them and re-run."
        exit 1
    fi
}

# ==============================================================================
# [1/5] System Paths
# ==============================================================================
collect_paths() {
    print_header "[1/5] System Paths"

    # V1.4.6: Auto-detect current directory, user and group for a seamless 'press-enter' experience
    local default_install_dir="${SCRIPT_DIR}"
    local default_user
    default_user=$(whoami)
    local default_group
    default_group=$(id -gn "$default_user" 2>/dev/null || echo "$default_user")

    ask "GoStream install directory" "$default_install_dir" INSTALL_DIR
    ask "Physical MKV source path   (physical_source_path)" "/mnt/gostream-mkv-real" STORAGE_PATH
    ask "FUSE virtual mount path     (fuse_mount_path)"     "/mnt/gostream-mkv-virtual" FUSE_MOUNT
    ask "System user that owns GoStream" "$default_user" SYSTEM_USER
    ask "System group" "$default_group" SYSTEM_GROUP

    # Derive BASE_DIR as INSTALL_DIR (V1.4.6: logs/ and STATE/ are now inside)
    BASE_DIR="${INSTALL_DIR}"

    echo ""
    print_info "Derived base directory : ${BASE_DIR}"
    print_info "Logs directory         : ${BASE_DIR}/logs/"
    print_info "State directory        : ${BASE_DIR}/STATE/"
}

# ==============================================================================
# [2/5] Plex Configuration
# ==============================================================================
collect_plex() {
    print_header "[2/5] Plex Configuration"

    ask       "Plex server URL"  "http://127.0.0.1:32400" PLEX_URL
    ask_secret "Plex token (hidden)" PLEX_TOKEN

    PLEX_LIBRARY_ID=0
    PLEX_TV_LIBRARY_ID=0

    # Auto-discover library sections if we have curl and a non-empty token
    if [ -n "$PLEX_TOKEN" ] && command -v curl >/dev/null 2>&1; then
        echo ""
        print_info "Querying Plex for library sections..."
        local sections_xml
        if sections_xml=$(curl -sf --max-time 8 \
                "${PLEX_URL}/library/sections?X-Plex-Token=${PLEX_TOKEN}" 2>/dev/null); then
            # Parse XML with grep/sed: extract (key, title, type)
            local parsed
            parsed=$(echo "$sections_xml" | grep -oP '<Directory[^>]+key="([^"]+)"[^>]+type="([^"]+)"[^>]+title="([^"]+)"' | while IFS= read -r line; do
                local key lib_type title
                key=$(echo "$line" | grep -oP 'key="\K[^"]+')
                lib_type=$(echo "$line" | grep -oP 'type="\K[^"]+')
                title=$(echo "$line" | grep -oP 'title="\K[^"]+')
                if [ "$lib_type" = "movie" ] || [ "$lib_type" = "show" ]; then
                    echo "${key}:${title}:${lib_type}"
                fi
            done) || true

            if [ -n "$parsed" ]; then
                echo ""
                echo "  Available Plex libraries:"
                local i=1
                local -a lib_keys lib_titles lib_types
                while IFS= read -r line; do
                    local key title lib_type
                    key="${line%%:*}"
                    rest="${line#*:}"
                    lib_type="${rest##*:}"
                    title="${rest%:*}"
                    lib_keys+=("$key")
                    lib_titles+=("$title")
                    lib_types+=("$lib_type")
                    printf "    %d) [%s] %s (%s)\n" "$i" "$key" "$title" "$lib_type"
                    (( i++ )) || true
                done <<< "$parsed"

                echo ""
                ask "Movies library number (0 to enter manually)" "0" _movies_choice
                if [ "$_movies_choice" -gt 0 ] 2>/dev/null && \
                   [ "$_movies_choice" -le "${#lib_keys[@]}" ] 2>/dev/null; then
                    local idx=$(( _movies_choice - 1 ))
                    PLEX_LIBRARY_ID="${lib_keys[$idx]}"
                    print_ok "Movies library: ${lib_titles[$idx]} (ID: ${PLEX_LIBRARY_ID})"
                else
                    ask "Movies library ID (numeric)" "1" PLEX_LIBRARY_ID
                fi

                ask "TV library number (0 to enter manually, 0 if none)" "0" _tv_choice
                if [ "$_tv_choice" -gt 0 ] 2>/dev/null && \
                   [ "$_tv_choice" -le "${#lib_keys[@]}" ] 2>/dev/null; then
                    local idx=$(( _tv_choice - 1 ))
                    PLEX_TV_LIBRARY_ID="${lib_keys[$idx]}"
                    print_ok "TV library: ${lib_titles[$idx]} (ID: ${PLEX_TV_LIBRARY_ID})"
                else
                    ask "TV library ID (numeric, 0 to skip)" "0" PLEX_TV_LIBRARY_ID
                fi
            else
                print_warn "No movie/TV libraries found — entering manually."
                ask "Movies library ID (numeric)" "1" PLEX_LIBRARY_ID
                ask "TV library ID (numeric, 0 to skip)" "0" PLEX_TV_LIBRARY_ID
            fi
        else
            print_warn "Could not reach Plex at ${PLEX_URL} — entering library IDs manually."
            ask "Movies library ID (numeric)" "1" PLEX_LIBRARY_ID
            ask "TV library ID (numeric, 0 to skip)" "0" PLEX_TV_LIBRARY_ID
        fi
    else
        if [ -z "$PLEX_TOKEN" ]; then
            print_warn "No Plex token provided — skipping auto-discovery."
        fi
        ask "Movies library ID (numeric)" "1" PLEX_LIBRARY_ID
        ask "TV library ID (numeric, 0 to skip)" "0" PLEX_TV_LIBRARY_ID
    fi
}

# ==============================================================================
# [3/5] External APIs
# ==============================================================================
collect_apis() {
    print_header "[3/5] External APIs"

    ask "TMDB API key (optional — leave empty to skip movie sync)" "" TMDB_API_KEY

    if [ -z "$TMDB_API_KEY" ]; then
        print_warn "No TMDB key entered. Movie sync (gostorm-sync-complete.py) will not function."
    else
        print_ok "TMDB API key set."
    fi
}

# ==============================================================================
# [4/5] Hardware & Network
# ==============================================================================
collect_hardware() {
    print_header "[4/5] Hardware & Network"

    ask "GOMEMLIMIT (MiB)  — 2200 is optimal for Pi 4 / 4GB" "2200" GOMEMLIMIT_MB

    ask "Proxy listen port        (proxy_listen_port)" "8080" PROXY_PORT
    ask "Metrics/dashboard port   (metrics_port)"      "9080" METRICS_PORT

    # NAT-PMP
    echo ""
    NATPMP_ENABLED=false
    NATPMP_GATEWAY=""
    VPN_INTERFACE="wg0"

    if ask_yn "Enable NAT-PMP (WireGuard port forwarding)?" "n"; then
        NATPMP_ENABLED=true
        ask "NAT-PMP gateway IP" "" NATPMP_GATEWAY
        ask "VPN interface" "wg0" VPN_INTERFACE
        print_ok "NAT-PMP enabled (gateway: ${NATPMP_GATEWAY}, interface: ${VPN_INTERFACE})"
    else
        print_info "NAT-PMP disabled."
        echo ""
        print_warn "Without NAT-PMP/WireGuard, all BitTorrent traffic (DHT, peer"
        print_warn "connections, tracker queries) flows directly through your home"
        print_warn "router. This can saturate the router's NAT connection-tracking"
        print_warn "table, causing instability or reboots on some devices."
        print_warn "Recommended: keep ConnectionsLimit <= 25 and enable WireGuard"
        print_warn "on the Pi before running GoStream in production."
        echo ""
    fi
}

# ==============================================================================
# [5/5] Installing — step implementations
# ==============================================================================

# ------------------------------------------------------------------------------
# 5a. Generate config.json from config.json.example
# ------------------------------------------------------------------------------
generate_config_json() {
    local output_path="${INSTALL_DIR}/config.json"

    # Ensure INSTALL_DIR exists before writing anything
    mkdir -p "${INSTALL_DIR}"

    # Look for config.json.example: first in the repo (SCRIPT_DIR), then in INSTALL_DIR
    local example_path
    if [ -f "${SCRIPT_DIR}/config.json.example" ]; then
        example_path="${SCRIPT_DIR}/config.json.example"
    else
        example_path="${INSTALL_DIR}/config.json.example"
    fi

    print_info "Generating config.json..."

    if [ ! -f "$example_path" ]; then
        print_warn "config.json.example not found — using built-in template."
        example_path="${INSTALL_DIR}/config.json.example"
        mkdir -p "${INSTALL_DIR}"
        # Write the embedded template so the python script below can update it
        cat > "$example_path" <<'TEMPLATE_EOF'
{
  "master_concurrency_limit": 25,
  "read_ahead_budget_mb": 256,
  "metadata_cache_size_mb": 50,
  "write_buffer_size_kb": 64,
  "read_buffer_size_kb": 1024,
  "fuse_block_size_bytes": 1048576,
  "streaming_threshold_kb": 128,
  "log_level": "INFO",
  "attr_timeout_seconds": 1,
  "entry_timeout_seconds": 1,
  "negative_timeout_seconds": 0,
  "http_client_timeout_seconds": 30,
  "max_retry_attempts": 6,
  "retry_delay_ms": 500,
  "rescue_grace_period_seconds": 240,
  "rescue_cooldown_hours": 24,
  "preload_workers_count": 4,
  "preload_initial_delay_ms": 1000,
  "warm_start_idle_seconds": 6,
  "max_concurrent_prefetch": 3,
  "cache_cleanup_interval_min": 5,
  "max_cache_entries": 10000,
  "gostorm_url": "http://127.0.0.1:8090",
  "proxy_listen_port": 8080,
  "metrics_port": 9080,
  "blocklist_url": "https://github.com/Naunter/BT_BlockLists/raw/master/bt_blocklists.gz",
  "physical_source_path": "/mnt/gostream-mkv-real",
  "fuse_mount_path": "/mnt/gostream-mkv-virtual",
  "disk_warmup_quota_gb": 12,
  "warmup_head_size_mb": 64,
  "natpmp": {
    "enabled": false,
    "gateway": "",
    "local_port": 8091,
    "vpn_interface": "wg0",
    "lifetime": 60,
    "refresh": 45
  },
  "plex": {
    "url": "http://127.0.0.1:32400",
    "token": "",
    "library_id": 0,
    "tv_library_id": 0
  },
  "tmdb_api_key": "",
  "media_server_type": "plex",
  "prowlarr": {
    "enabled": false,
    "api_key": "",
    "url": ""
  }
}
TEMPLATE_EOF
    fi

    # Use python3 to safely read JSON, update fields, and write output
    python3 - <<PYEOF
import json, sys

example_path = "${example_path}"
output_path  = "${output_path}"

with open(example_path, 'r') as fh:
    cfg = json.load(fh)

# --- Paths ---
cfg['physical_source_path'] = "${STORAGE_PATH}"
cfg['fuse_mount_path']       = "${FUSE_MOUNT}"

# --- Network ---
cfg['proxy_listen_port'] = int("${PROXY_PORT}")
cfg['metrics_port']      = int("${METRICS_PORT}")

# --- Media Server ---
cfg['media_server_type'] = "plex"

# --- External APIs ---
if "${TMDB_API_KEY}":
    cfg['tmdb_api_key'] = "${TMDB_API_KEY}"

# --- Plex block ---
if 'plex' not in cfg or not isinstance(cfg.get('plex'), dict):
    cfg['plex'] = {}
cfg['plex']['url']        = "${PLEX_URL}"
cfg['plex']['token']      = "${PLEX_TOKEN}"
try:
    cfg['plex']['library_id'] = int("${PLEX_LIBRARY_ID}")
except ValueError:
    cfg['plex']['library_id'] = 0
try:
    cfg['plex']['tv_library_id'] = int("${PLEX_TV_LIBRARY_ID}")
except ValueError:
    cfg['plex']['tv_library_id'] = 0

# --- NAT-PMP block ---
if 'natpmp' not in cfg or not isinstance(cfg.get('natpmp'), dict):
    cfg['natpmp'] = {
        "local_port": 8091,
        "lifetime": 60,
        "refresh": 45
    }
natpmp_enabled_str = "${NATPMP_ENABLED}"
cfg['natpmp']['enabled']       = natpmp_enabled_str.lower() == 'true'
cfg['natpmp']['gateway']       = "${NATPMP_GATEWAY}"
cfg['natpmp']['vpn_interface'] = "${VPN_INTERFACE}"

# --- Sync Scheduler ---
if 'scheduler' not in cfg or not isinstance(cfg.get('scheduler'), dict):
    cfg['scheduler'] = {
        "enabled": True,
        "movies_sync": {"enabled": True, "days_of_week": [1, 4], "hour": 3, "minute": 0},
        "tv_sync": {"enabled": True, "days_of_week": [3, 5], "hour": 4, "minute": 0},
        "watchlist_sync": {"enabled": True, "interval_hours": 1}
    }

# --- Quality Scoring ---
if 'quality_scoring' not in cfg or not isinstance(cfg.get('quality_scoring'), dict):
    cfg['quality_scoring'] = {
        "movies": {
            "res_4k": 200, "res_1080p": 50, "hdr": 60, "dolby_vision": 150,
            "hdr10_plus": 100, "atmos": 50, "audio_5_1": 25, "stereo": -50,
            "bluray": 10, "seeder_bonus": 5, "seeder_threshold": 50
        },
        "tv": {
            "res_4k": 200, "res_1080p": 50, "hdr": 60, "dolby_vision": 150,
            "hdr10_plus": 100, "atmos": 50, "audio_5_1": 25, "stereo": -50,
            "bluray": 10, "seeder_bonus": 5, "seeder_threshold": 50,
            "fullpack_bonus": 300
        }
    }

with open(output_path, 'w') as fh:
    json.dump(cfg, fh, indent=2)
    fh.write('\n')

print(f"  Written: {output_path}")
PYEOF

    print_ok "config.json written to ${output_path}"
}

# ------------------------------------------------------------------------------
# 5a2. Deploy files from repo to INSTALL_DIR
# ------------------------------------------------------------------------------
deploy_files() {
    print_info "Deploying files to ${INSTALL_DIR}..."

    # Copy config.json.example (useful reference for the user)
    if [ -f "${SCRIPT_DIR}/config.json.example" ]; then
        cp "${SCRIPT_DIR}/config.json.example" "${INSTALL_DIR}/config.json.example"
        print_ok "config.json.example deployed to ${INSTALL_DIR}/"
    fi
}

# ------------------------------------------------------------------------------
# 5b. Create directories
# ------------------------------------------------------------------------------
create_directories() {
    print_info "Creating required directories..."

    # Directories that belong to the system user (no sudo needed if running as that user)
    local user_dirs=(
        "${BASE_DIR}/STATE"
        "${BASE_DIR}/logs"
    )

    for d in "${user_dirs[@]}"; do
        if mkdir -p "$d" 2>/dev/null; then
            print_ok "Created: $d"
        elif sudo mkdir -p "$d"; then
            sudo chown "${SYSTEM_USER}:${SYSTEM_USER}" "$d"
            print_ok "Created (sudo): $d"
        else
            print_err "Failed to create: $d"
            exit 1
        fi
    done

    # Directories under /mnt may require sudo
    local root_dirs=(
        "${STORAGE_PATH}/movies"
        "${STORAGE_PATH}/tv"
        "${FUSE_MOUNT}"
    )

    for d in "${root_dirs[@]}"; do
        if mkdir -p "$d" 2>/dev/null; then
            chown "${SYSTEM_USER}:${SYSTEM_USER}" "$d" 2>/dev/null || true
            print_ok "Created: $d"
        else
            print_info "Creating ${d} requires sudo..."
            sudo mkdir -p "$d"
            sudo chown "${SYSTEM_USER}:${SYSTEM_USER}" "$d"
            print_ok "Created (sudo): $d"
        fi
    done
}

# ------------------------------------------------------------------------------
# 5d. Install systemd service files
# ------------------------------------------------------------------------------
install_services() {
    print_info "Installing systemd service files (requires sudo)..."

    # -- gostream.service --
    local wg_after=""
    if [ "$NATPMP_ENABLED" = "true" ] && [ -n "$VPN_INTERFACE" ]; then
        wg_after=" wg-quick@${VPN_INTERFACE}.service"
    fi

    sudo tee /etc/systemd/system/gostream.service > /dev/null <<SERVICE_EOF
[Unit]
Description=GoStream + GoStorm (Unified Streaming Engine)
After=network-online.target systemd-resolved.service nss-lookup.target local-fs.target remote-fs.target${wg_after}
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
# Memory tuning — GOMEMLIMIT=${GOMEMLIMIT_MB}MiB is optimal for Pi 4 / 4GB
Environment=GOMEMLIMIT=${GOMEMLIMIT_MB}MiB
Environment=GOGC=100

Type=simple
User=${SYSTEM_USER}
Group=${SYSTEM_GROUP}

WorkingDirectory=${INSTALL_DIR}

# Wait for DNS before starting (required for tracker + blocklist resolution)
ExecStartPre=/bin/sh -c 'for i in 1 2 3 4 5; do getent hosts google.com >/dev/null 2>&1 && exit 0 || sleep 2; done; exit 1'

# FUSE mount cleanup and creation
ExecStartPre=-/usr/bin/${FUSERMOUNT_CMD} -uz ${FUSE_MOUNT}
ExecStartPre=/bin/mkdir -p ${FUSE_MOUNT}

# V1.4.6: Main binary — using --path . for true portability (STATE stays in WorkingDirectory)
ExecStart=./gostream --path .

# Allow gostream to stabilize, then restart Samba so it sees the FUSE mount
ExecStartPost=/bin/sleep 2
ExecStartPost=/usr/bin/sudo /bin/systemctl restart smbd

Restart=always
RestartSec=10
LimitNOFILE=65536
LimitNPROC=4096

# Centralized logging inside the GoStream directory (relative to WorkingDirectory)
StandardOutput=append:logs/gostream.log
StandardError=append:logs/gostream.log

# Cleanly unmount FUSE on stop
ExecStop=/usr/bin/${FUSERMOUNT_CMD} -uz ${FUSE_MOUNT}

[Install]
WantedBy=multi-user.target
SERVICE_EOF

    print_ok "Wrote /etc/systemd/system/gostream.service"
}

# ------------------------------------------------------------------------------
# 5e. Enable services via systemd
# ------------------------------------------------------------------------------
enable_services() {
    print_info "Reloading systemd and enabling services..."

    sudo systemctl daemon-reload
    sudo systemctl enable gostream

    print_ok "Services enabled: gostream"
}

# ------------------------------------------------------------------------------
# 5g. Sudoers entry so gostream.service can restart smbd without a password
# ------------------------------------------------------------------------------
GO_BIN=""
GO_ARCH=""
GO_OS=""

detect_go_arch() {
    local machine
    machine="$(uname -m)"
    GO_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"

    case "$machine" in
        aarch64|arm64)    GO_ARCH="arm64"   ;;
        x86_64|amd64)     GO_ARCH="amd64"   ;;
        armv7l|armv7)     GO_ARCH="arm"     ;;
        armv6l|armv6)     GO_ARCH="armv6l"  ;;
        i686|i386)        GO_ARCH="386"     ;;
        *)
            print_err "Unsupported architecture: ${machine}"
            exit 1
            ;;
    esac

    print_info "Detected platform: ${GO_OS}/${GO_ARCH}"
}

ensure_go() {
    local go_install_dir="/usr/local/go"

    detect_go_arch

    # Find an existing Go binary that matches the detected OS/arch
    local candidates=("${go_install_dir}/bin/go" "$(command -v go 2>/dev/null)")
    for candidate in "${candidates[@]}"; do
        if [ -x "$candidate" ]; then
            local info
            info=$("$candidate" version 2>/dev/null)
            if echo "$info" | grep -q "${GO_OS}/${GO_ARCH}"; then
                GO_BIN="$candidate"
                print_ok "Go found: $info"
                return 0
            fi
        fi
    done

    # Fetch the latest stable Go version number from go.dev
    local go_version
    go_version=$(curl -fsSL "https://go.dev/VERSION?m=text" | head -1)
    if [ -z "$go_version" ]; then
        go_version="go1.24.0"   # fallback if network unavailable
    fi

    print_info "${go_version} (${GO_OS}/${GO_ARCH}) not found — downloading..."

    local tarball="${go_version}.${GO_OS}-${GO_ARCH}.tar.gz"
    local url="https://go.dev/dl/${tarball}"
    local tmp="/tmp/${tarball}"

    curl -fL --progress-bar -o "$tmp" "$url"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "$tmp"
    rm -f "$tmp"

    GO_BIN="${go_install_dir}/bin/go"
    print_ok "Go installed: $($GO_BIN version)"
}

# ------------------------------------------------------------------------------
# 5f3. Compile the GoStream binary from source
# ------------------------------------------------------------------------------
ensure_swap() {
    # Go compilation can OOM on Pi with little/no swap — ensure at least 1GB
    local swap_total
    swap_total=$(free -m | awk '/^Swap:/ {print $2}')
    if [ "${swap_total:-0}" -lt 1024 ]; then
        print_info "Swap < 1 GB detected (${swap_total} MB) — creating 1 GB swapfile for compilation..."
        if [ ! -f /swapfile ]; then
            sudo fallocate -l 1G /swapfile 2>/dev/null || sudo dd if=/dev/zero of=/swapfile bs=1M count=1024 status=none
            sudo chmod 600 /swapfile
            sudo mkswap /swapfile >/dev/null
        fi
        sudo swapon /swapfile 2>/dev/null || true
        print_ok "Swapfile active ($(free -m | awk '/^Swap:/ {print $2}') MB total swap)"
    fi
}

compile_binary() {
    print_info "Compiling GoStream binary (this takes a few minutes on Pi 4)..."

    ensure_go
    ensure_swap

    local src_dir="${SCRIPT_DIR}"
    local out_bin="${INSTALL_DIR}/gostream"

    # Verify we have Go source files in the expected location
    if [ ! -f "${src_dir}/main.go" ]; then
        print_err "main.go not found in ${src_dir} — cannot compile."
        exit 1
    fi

    cd "${src_dir}"

    print_info "Running go mod tidy..."
    GOTOOLCHAIN=local "$GO_BIN" mod tidy

    # Use -pgo=off if no default.pgo present (fresh install)
    local pgo_flag="-pgo=off"
    if [ -f "${src_dir}/default.pgo" ]; then
        pgo_flag="-pgo=auto"
        print_info "PGO profile found — building with -pgo=auto"
    else
        print_info "No PGO profile — building with -pgo=off (regenerate later for 5-7% CPU gain)"
    fi

    # -p 2 limits parallel jobs to keep peak RAM under control on Pi 4
    # GOTMPDIR on the main FS avoids OOM on small /tmp tmpfs during linking
    local go_tmp="${HOME}/go-tmp"
    mkdir -p "${go_tmp}"

    # Embed version from git tag into the binary
    # If no tag found, skip -ldflags so the hardcoded version.go value takes effect
    local app_version
    app_version=$(git describe --tags --abbrev=0 2>/dev/null || true)

    local ldflags=""
    if [ -n "$app_version" ]; then
        ldflags="-X main.AppVersion=${app_version}"
        print_info "Embedding version: ${app_version} (from git tag)"
    else
        print_info "No git tag found — using hardcoded version from version.go"
    fi

    print_info "Building binary (GOARCH=${GO_ARCH}, -p 2)..."
    GOTOOLCHAIN=local GOARCH="${GO_ARCH}" CGO_ENABLED=1 GOTMPDIR="${go_tmp}" \
        "$GO_BIN" build ${pgo_flag} -p 2 ${ldflags:+-ldflags "${ldflags}"} -o "${out_bin}" .
    rm -rf "${go_tmp}"

    chmod +x "${out_bin}"
    print_ok "Binary compiled and deployed: ${out_bin}"

    cd - >/dev/null
}

# ------------------------------------------------------------------------------
# 5h. Verify installation (non-fatal — binary may not be deployed yet)
# ------------------------------------------------------------------------------
verify_install() {
    print_info "Verifying installation (checking metrics endpoint)..."

    local url="http://127.0.0.1:${METRICS_PORT}/metrics"
    if command -v curl >/dev/null 2>&1; then
        if curl -sf --max-time 5 "$url" >/dev/null 2>&1; then
            print_ok "GoStream metrics endpoint is reachable at ${url}"
        else
            print_warn "GoStream is not running yet (metrics endpoint not reachable)."
            print_warn "This is expected if the binary has not been copied and started."
        fi
    else
        print_warn "curl not available — skipping endpoint verification."
    fi
}

# ------------------------------------------------------------------------------
# 5i. Cron jobs for sync scripts (optional — built-in scheduler is preferred)
# ------------------------------------------------------------------------------
setup_cron_jobs() {
    print_info "Configuring sudoers for smbd restart..."

    local sudoers_file="/etc/sudoers.d/gostream-smbd"
    local sudoers_line="${SYSTEM_USER} ALL=(ALL) NOPASSWD: /bin/systemctl restart smbd"

    # Check whether the entry already exists anywhere in sudoers
    if sudo grep -qF "$sudoers_line" /etc/sudoers /etc/sudoers.d/* 2>/dev/null; then
        print_ok "Sudoers entry already present — no change needed."
        return 0
    fi

    if sudo sh -c "echo '${sudoers_line}' | tee ${sudoers_file} > /dev/null"; then
        sudo chmod 440 "${sudoers_file}"
        print_ok "Sudoers entry written: ${sudoers_file}"
    else
        print_warn "Could not write sudoers entry (sudo unavailable or permission denied)."
        print_warn "To add manually, run:"
        print_warn "  echo '${sudoers_line}' | sudo tee ${sudoers_file} && sudo chmod 440 ${sudoers_file}"
    fi
}

# ------------------------------------------------------------------------------
# 5i. Cron jobs removed — use built-in scheduler from Control Panel
# ------------------------------------------------------------------------------
setup_cron_jobs() {
    print_info "Built-in scheduler is enabled by default. Configure it from the Control Panel at :9080/control."
}

# ==============================================================================
# Final summary
# ==============================================================================
show_summary() {
    echo ""
    echo "${BOLD}${GREEN}╔══════════════════════════════════════╗${NC}"
    echo "${BOLD}${GREEN}║  Installation Complete!              ║${NC}"
    echo "${BOLD}${GREEN}╚══════════════════════════════════════╝${NC}"
    echo ""
    echo "  Configuration written to:"
    echo "    ${BOLD}${INSTALL_DIR}/config.json${NC}"
    echo ""
    echo "  Service files installed:"
    echo "    /etc/systemd/system/gostream.service"
    echo ""
    echo "${BOLD}Next steps:${NC}"
    echo ""
    echo "  1. Start the service:"
    echo "     ${YELLOW}sudo systemctl start gostream${NC}"
    echo ""
    echo "  2. Configure Plex Webhook:"
    echo "     Open Plex → Settings → Webhooks → Add:"
    echo "     ${BOLD}http://<your-pi-ip>:${METRICS_PORT}/plex/webhook${NC}"
    echo ""
    echo "  3. Configure Samba (critical for stability):"
    echo "     Edit /etc/samba/smb.conf and ensure your share has:"
    echo "       oplocks = no"
    echo "       aio read size = 1"
    echo "       deadtime = 15"
    echo "       vfs objects = fileid"
    echo "     Then: ${YELLOW}sudo systemctl restart smbd${NC}"
    echo ""
    echo "  4. Check status:"
    echo "     ${YELLOW}sudo systemctl status gostream${NC}"
    echo "     ${YELLOW}curl http://127.0.0.1:${METRICS_PORT}/metrics${NC}"
    echo ""
    echo "  5. Dashboards:"
    echo "     ${BOLD}http://<your-ip>:${METRICS_PORT}/control${NC}  (Control Panel + Scheduler)"
    echo "     ${BOLD}http://<your-ip>:${METRICS_PORT}/dashboard${NC}  (Health Monitor)"
    echo ""
    echo "  6. Trigger sync manually:"
    echo "     ${YELLOW}curl -X POST http://127.0.0.1:${METRICS_PORT}/api/scheduler/movies/run${NC}  # Movies"
    echo "     ${YELLOW}curl -X POST http://127.0.0.1:${METRICS_PORT}/api/scheduler/tv/run${NC}         # TV"
    echo "     ${YELLOW}curl -X POST http://127.0.0.1:${METRICS_PORT}/api/scheduler/watchlist/run${NC}  # Watchlist"
    echo ""
    echo "  7. Logs:"
    echo "     ${YELLOW}tail -f ${BASE_DIR}/logs/gostream.log${NC}"
    echo "     ${YELLOW}tail -f ${BASE_DIR}/logs/movies-sync.log${NC}"
    echo "     ${YELLOW}tail -f ${BASE_DIR}/logs/tv-sync.log${NC}"
    echo ""
}

# ==============================================================================
# Main
# ==============================================================================
main() {
    show_banner

    # Directory containing this script (= cloned repo root)
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    # Note: FUSERMOUNT_CMD is set inside check_prerequisites
    FUSERMOUNT_CMD="fusermount3"

    install_system_deps
    check_prerequisites

    collect_paths
    collect_plex
    collect_apis
    collect_hardware

    print_header "[5/5] Installing"

    deploy_files
    echo ""
    generate_config_json
    echo ""
    create_directories
    echo ""
    install_services
    echo ""
    enable_services
    echo ""
    setup_sudoers
    echo ""
    compile_binary
    echo ""
    verify_install
    echo ""
    setup_cron_jobs

    show_summary
}

main "$@"