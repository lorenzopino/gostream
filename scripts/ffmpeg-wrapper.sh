#!/bin/bash
# ffmpeg-wrapper.sh — Intercepts Jellyfin FFmpeg calls and reduces probesize/analyzeduration
# to prevent massive reads from FUSE mount before playback starts.
#
# Original: -probesize 1G -analyzeduration 200M
# Reduced:  -probesize 100M -analyzeduration 30M
#
# Install: Copy to /Users/lorenzo/MediaCenter/jellyfin/ffmpeg-wrapper.sh
#          chmod +x it
#          Configure Jellyfin to use this as the FFmpeg path.

FFMPEG_REAL="/usr/lib/jellyfin-ffmpeg/ffmpeg"

# Override values
PROBESIZE_OVERRIDE="100M"
ANALYZEDURATION_OVERRIDE="30M"

# Build new args
NEW_ARGS=()
SKIP_NEXT=false

for arg in "$@"; do
    if $SKIP_NEXT; then
        SKIP_NEXT=false
        continue  # Skip the original value
    fi

    case "$arg" in
        -probesize)
            NEW_ARGS+=("-probesize" "$PROBESIZE_OVERRIDE")
            SKIP_NEXT=true
            ;;
        -analyzeduration)
            NEW_ARGS+=("-analyzeduration" "$ANALYZEDURATION_OVERRIDE")
            SKIP_NEXT=true
            ;;
        *)
            NEW_ARGS+=("$arg")
            ;;
    esac
done

exec "$FFMPEG_REAL" "${NEW_ARGS[@]}"
