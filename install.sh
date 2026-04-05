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
    echo "  Includes:       Samba (optional), built-in scheduler"
    echo ""
}

# ==============================================================================
# [0a] Auto-install system dependencies via apt
# ==============================================================================
install_system_deps() {
    # Only run on Debian/Ubuntu-based systems
    if ! command -v apt-get >/dev/null 2>&1; then
        print_warn "apt-get not found — skipping automatic dependency installation."
        print_warn "Please install manually: git fuse3 curl"
        if [ "${INSTALL_SAMBA:-true}" = "true" ]; then
            print_warn "Samba: samba"
        fi
        return 0
    fi

    # Map: package name → apt package to install
    local -A needed=()

    command -v git         >/dev/null 2>&1 || needed["git"]="git"
    command -v fusermount3 >/dev/null 2>&1 || needed["fusermount3"]="fuse3"
    command -v curl        >/dev/null 2>&1 || needed["curl"]="curl"
    # Samba is optional — only check if user wants it
    if [ "${INSTALL_SAMBA:-true}" = "true" ]; then
        dpkg -s samba        >/dev/null 2>&1 || needed["samba"]="samba"
    fi
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
# [1/3] System Paths + User/Group
# ==============================================================================
collect_paths() {
    print_header "[1/3] System Paths"

    # Default: GoStream subdirectory next to the installer
    local default_install_dir="${SCRIPT_DIR}/GoStream"
    local default_user
    default_user=$(whoami)
    local default_group
    default_group=$(id -gn "$default_user" 2>/dev/null || echo "$default_user")

    ask "GoStream install directory" "$default_install_dir" INSTALL_DIR
    ask "Physical MKV source path   (physical_source_path)" "/mnt/gostream-mkv-real" STORAGE_PATH
    ask "FUSE virtual mount path     (fuse_mount_path)"     "/mnt/gostream-mkv-virtual" FUSE_MOUNT
    ask "System user that owns GoStream" "$default_user" SYSTEM_USER
    ask "System group" "$default_group" SYSTEM_GROUP

    # Resolve to absolute path
    INSTALL_DIR="$(cd "$(dirname "${INSTALL_DIR}")" 2>/dev/null && pwd)/$(basename "${INSTALL_DIR}")" || INSTALL_DIR="$(pwd)/${INSTALL_DIR}"
    mkdir -p "${INSTALL_DIR}"

    # Derive BASE_DIR as INSTALL_DIR
    BASE_DIR="${INSTALL_DIR}"

    echo ""
    print_info "Install directory    : ${INSTALL_DIR}"
    print_info "Physical source path : ${STORAGE_PATH}"
    print_info "FUSE mount path      : ${FUSE_MOUNT}"
}

# ==============================================================================
# [2/3] Samba (optional)
# ==============================================================================
collect_options() {
    print_header "[2/3] Options"

    INSTALL_SAMBA=true
    if ask_yn "Install and configure Samba?" "y"; then
        INSTALL_SAMBA=true
        print_ok "Samba will be installed and configured."
    else
        INSTALL_SAMBA=false
        print_info "Samba skipped. You will need to configure an alternative access method for the FUSE mount."
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
    local example_path="${INSTALL_DIR}/config.json.example"

    print_info "Generating config.json..."

    if [ ! -f "$example_path" ]; then
        print_err "config.json.example not found — cannot generate config."
        exit 1
    fi

    # Copy the example and patch only the path fields with sed
    cp "$example_path" "$output_path"
    sed -i "s|\"physical_source_path\": \".*\"|\"physical_source_path\": \"${STORAGE_PATH}\"|" "$output_path"
    sed -i "s|\"fuse_mount_path\": \".*\"|\"fuse_mount_path\": \"${FUSE_MOUNT}\"|" "$output_path"

    print_ok "config.json written to ${output_path}"
    print_info "Configure Plex, TMDB, NAT-PMP, ports, and scheduler from the Control Panel at :9080/control"
}

# ------------------------------------------------------------------------------
# 5a. Clone or use existing repo source
# ------------------------------------------------------------------------------
clone_repo() {
    print_info "Preparing source in ${INSTALL_DIR}..."

    # Check if this is already a cloned repo (main.go exists in SCRIPT_DIR)
    if [ -f "${SCRIPT_DIR}/main.go" ]; then
        # If INSTALL_DIR is inside SCRIPT_DIR (e.g. SCRIPT_DIR=/home/pi, INSTALL_DIR=/home/pi/GoStream),
        # don't rsync the entire parent directory — clone fresh instead
        case "${INSTALL_DIR}" in
            "${SCRIPT_DIR}"/*)
                print_info "Install directory is inside source directory — cloning fresh from GitHub..."
                ;;
            *)
                if [ "$(realpath "${SCRIPT_DIR}")" != "$(realpath "${INSTALL_DIR}")" ]; then
                    print_info "Copying source to ${INSTALL_DIR}..."
                    rsync -a "${SCRIPT_DIR}/" "${INSTALL_DIR}/" --exclude='.git'
                    print_ok "Source copied to ${INSTALL_DIR}"
                    return 0
                fi
                print_ok "Source found in ${SCRIPT_DIR} — using existing clone."
                return 0
                ;;
        esac
    fi

    # Clone from GitHub
    local repo_url="https://github.com/MrRobotoGit/gostream.git"
    local tmp_clone="/tmp/gostream-clone-$$"

    print_info "Cloning source from GitHub..."
    if command -v git >/dev/null 2>&1; then
        git clone --depth 1 "$repo_url" "$tmp_clone"
        mkdir -p "${INSTALL_DIR}"
        rsync -a "${tmp_clone}/" "${INSTALL_DIR}/"
        rm -rf "$tmp_clone"

        # Remove committed Go module cache (causes go mod tidy errors)
        if [ -d "${INSTALL_DIR}/go/pkg/mod" ]; then
            rm -rf "${INSTALL_DIR}/go/pkg/mod"
        fi

        print_ok "Source cloned to ${INSTALL_DIR}"
    else
        print_err "git not found — cannot clone source."
        exit 1
    fi
}

# ------------------------------------------------------------------------------
# 5a2. Deploy config.json.example
# ------------------------------------------------------------------------------
deploy_files() {
    print_info "Deploying files to ${INSTALL_DIR}..."

    # Ensure INSTALL_DIR exists
    mkdir -p "${INSTALL_DIR}"

    # config.json.example should already be in INSTALL_DIR from clone_repo
    if [ -f "${INSTALL_DIR}/config.json.example" ]; then
        print_ok "config.json.example present in ${INSTALL_DIR}/"
    else
        print_info "config.json.example not found — downloading from GitHub..."
        local raw_url="https://raw.githubusercontent.com/MrRobotoGit/gostream/refs/heads/main/config.json.example"
        if curl -sfL -o "${INSTALL_DIR}/config.json.example" "$raw_url"; then
            print_ok "config.json.example downloaded from GitHub"
        else
            print_err "Failed to download config.json.example from GitHub."
            exit 1
        fi
    fi
}

# ------------------------------------------------------------------------------
# 5b. Create directories
# ------------------------------------------------------------------------------
create_directories() {
    print_info "Creating required directories..."

    # Directories inside INSTALL_DIR (user-writable)
    local local_dirs=(
        "${INSTALL_DIR}/STATE"
        "${INSTALL_DIR}/logs"
    )

    for d in "${local_dirs[@]}"; do
        mkdir -p "$d"
        print_ok "Created: $d"
    done

    # Data directories (MKV source + FUSE mount point — may need sudo)
    local data_dirs=(
        "${STORAGE_PATH}/movies"
        "${STORAGE_PATH}/tv"
        "${FUSE_MOUNT}"
    )

    for d in "${data_dirs[@]}"; do
        if mkdir -p "$d" 2>/dev/null; then
            chown "${SYSTEM_USER}:${SYSTEM_GROUP}" "$d" 2>/dev/null || true
            print_ok "Created: $d"
        else
            print_info "Creating ${d} requires sudo..."
            sudo mkdir -p "$d"
            sudo chown "${SYSTEM_USER}:${SYSTEM_GROUP}" "$d"
            print_ok "Created (sudo): $d"
        fi
    done
}

# ------------------------------------------------------------------------------
# 5d. Install systemd service files
# ------------------------------------------------------------------------------
install_services() {
    print_info "Installing systemd service files (requires sudo)..."

    local samba_restart=""
    if [ "$INSTALL_SAMBA" = "true" ]; then
        samba_restart='
# Allow gostream to stabilize, then restart Samba so it sees the FUSE mount
ExecStartPost=/bin/sleep 2
ExecStartPost=/usr/bin/sudo /bin/systemctl restart smbd'
    else
        samba_restart='
# Allow gostream to stabilize
ExecStartPost=/bin/sleep 2'
    fi

    sudo tee /etc/systemd/system/gostream.service > /dev/null <<SERVICE_EOF
[Unit]
Description=GoStream + GoStorm (Unified Streaming Engine)
After=network-online.target systemd-resolved.service nss-lookup.target local-fs.target remote-fs.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
# Memory tuning — GOMEMLIMIT=2200MiB is optimal for Pi 4 / 4GB
Environment=GOMEMLIMIT=2200MiB
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
ExecStart=${INSTALL_DIR}/gostream --path .${samba_restart}

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

    local src_dir="${INSTALL_DIR}"
    local out_bin="${INSTALL_DIR}/gostream"

    # Verify we have Go source files in the expected location
    if [ ! -f "${src_dir}/main.go" ]; then
        print_err "main.go not found in ${src_dir} — cannot compile."
        exit 1
    fi

    cd "${src_dir}"

    # Clean Go module cache if committed (causes go mod tidy errors with @version paths)
    if [ -d "${src_dir}/go/pkg/mod" ]; then
        rm -rf "${src_dir}/go/pkg/mod"
    fi

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

    local url="http://127.0.0.1:9080/metrics"
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
# Sudoers entry so gostream.service can restart smbd without a password
# ------------------------------------------------------------------------------
setup_sudoers() {
    if [ "$INSTALL_SAMBA" != "true" ]; then
        print_info "Samba not installed — skipping sudoers entry."
        return 0
    fi

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

    if [ "$INSTALL_SAMBA" = "true" ]; then
        echo "${BOLD}Samba configuration:${NC}"
        echo "  Edit /etc/samba/smb.conf and ensure your share has:"
        echo "    oplocks = no"
        echo "    aio read size = 1"
        echo "    deadtime = 15"
        echo "    vfs objects = fileid"
        echo "  Then: ${YELLOW}sudo systemctl restart smbd${NC}"
        echo ""
    fi

    echo "${BOLD}Next steps:${NC}"
    echo ""
    echo "  1. Start the service:"
    echo "     ${YELLOW}sudo systemctl start gostream${NC}"
    echo ""
    echo "  2. Check status:"
    echo "     ${YELLOW}sudo systemctl status gostream${NC}"
    echo "     ${YELLOW}curl http://127.0.0.1:9080/metrics${NC}"
    echo ""
    echo "  3. Configure everything from the Control Panel:"
    echo "     ┌─────────────────────────────────────────────────────────┐"
    echo "     │  Plex, TMDB, NAT-PMP, ports, scheduler, and more       │"
    echo "     │  http://<your-ip>:9080/control                          │"
    echo "     └─────────────────────────────────────────────────────────┘"
    echo ""
    echo "  4. Dashboards:"
    echo "     ${BOLD}http://<your-ip>:9080/control${NC}    (Control Panel + Scheduler)"
    echo "     ${BOLD}http://<your-ip>:9080/dashboard${NC}  (Health Monitor)"
    echo ""
    echo "  5. Logs:"
    echo "     ${YELLOW}tail -f ${INSTALL_DIR}/logs/gostream.log${NC}"
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

    # Default: Samba enabled (can be overridden by collect_options)
    INSTALL_SAMBA=true

    install_system_deps
    check_prerequisites

    collect_paths
    collect_options

    print_header "[3/3] Installing"

    clone_repo
    echo ""
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

    show_summary
}

main "$@"