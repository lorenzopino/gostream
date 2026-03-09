#!/bin/sh
set -eu

CONFIG_PATH="${MKV_PROXY_CONFIG_PATH:-/config.json}"
SOURCE_PATH="${GOSTREAM_SOURCE_PATH:-/mnt/gostream-mkv-real}"
MOUNT_PATH="${GOSTREAM_MOUNT_PATH:-/mnt/gostream-mkv-virtual}"

mkdir -p "$SOURCE_PATH" "$MOUNT_PATH"

# Clean up any stale FUSE mount left by a previous crash. Without this,
# fs.Mount() will fail with "transport endpoint is not connected".
# -u unmounts, -z defers cleanup until the mountpoint is no longer busy.
#
# IMPORTANT: only unmount if the existing mount is actually FUSE. Docker's
# rshared bind mount for this path may already be present when the container
# starts. Unmounting it would destroy the peer group link needed for
# rshared propagation back to the host, causing an empty virtual directory.
if mountpoint -q "$MOUNT_PATH" 2>/dev/null; then
  if grep -q " $MOUNT_PATH fuse" /proc/mounts 2>/dev/null; then
    echo "Stale FUSE mount detected at $MOUNT_PATH, cleaning up..." >&2
    fusermount3 -uz "$MOUNT_PATH" || true
  else
    echo "Non-FUSE mountpoint at $MOUNT_PATH (Docker bind), leaving intact." >&2
  fi
fi

if [ ! -f "$CONFIG_PATH" ]; then
  echo "Missing required config file at $CONFIG_PATH" >&2
  exit 1
fi

# Run gostream in the background rather than exec'ing it, so we can trap
# SIGTERM (forwarded by tini) and explicitly unmount FUSE before exit.
# Without this, the kernel leaves the mount in a dead-but-present state and
# the host-side rshared bind mount becomes "transport endpoint not connected"
# on every container restart.
/usr/local/bin/gostream "$SOURCE_PATH" "$MOUNT_PATH" &
GOSTREAM_PID=$!

cleanup() {
  echo "[entrypoint] Shutdown signal received, unmounting FUSE at $MOUNT_PATH..." >&2
  kill -TERM "$GOSTREAM_PID" 2>/dev/null || true
  wait "$GOSTREAM_PID" 2>/dev/null || true
  fusermount3 -uz "$MOUNT_PATH" 2>/dev/null || true
}
trap cleanup TERM INT

# Wait for gostream; also unmount if it exits on its own (crash, restart, etc.)
wait "$GOSTREAM_PID"
EXIT_CODE=$?
fusermount3 -uz "$MOUNT_PATH" 2>/dev/null || true
exit "$EXIT_CODE"
