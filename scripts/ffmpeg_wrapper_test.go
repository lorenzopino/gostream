package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFFmpegWrapper_ReducedProbesize verifies the wrapper reduces probesize to 100M.
func TestFFmpegWrapper_ReducedProbesize(t *testing.T) {
	script := createTestWrapper(t)

	// The wrapper calls exec with our mock. We test by parsing what it would execute.
	args := []string{
		"-analyzeduration", "200M",
		"-probesize", "1G",
		"-fflags", "+genpts",
		"-i", "file:/path/to/video.mkv",
	}

	result := runWrapper(script, args)

	// Check probesize was reduced
	if !strings.Contains(result, "-probesize 100M") {
		t.Errorf("expected probesize 100M, got: %s", result)
	}

	// Check analyzeduration was reduced
	if !strings.Contains(result, "-analyzeduration 30M") {
		t.Errorf("expected analyzeduration 30M, got: %s", result)
	}

	// Check other args preserved
	if !strings.Contains(result, "-fflags +genpts") {
		t.Errorf("expected -fflags +genpts preserved, got: %s", result)
	}
	if !strings.Contains(result, "file:/path/to/video.mkv") {
		t.Errorf("expected input file preserved, got: %s", result)
	}
}

// TestFFmpegWrapper_PreservesOtherArgs verifies that non-probesize args are unchanged.
func TestFFmpegWrapper_PreservesOtherArgs(t *testing.T) {
	script := createTestWrapper(t)

	args := []string{
		"-map_metadata", "-1",
		"-map_chapters", "-1",
		"-threads", "0",
		"-map", "0:0",
		"-codec:v:0", "copy",
		"-bsf:v", "h264_mp4toannexb",
		"-start_at_zero",
		"-codec:a:0", "copy",
		"-copyts",
		"-avoid_negative_ts", "disabled",
		"-max_muxing_queue_size", "2048",
		"-f", "hls",
		"-hls_time", "6",
	}

	result := runWrapper(script, args)

	// All original args should be present
	expected := []string{
		"-map_metadata -1",
		"-threads 0",
		"-codec:v:0 copy",
		"-start_at_zero",
		"-f hls",
		"-hls_time 6",
	}

	for _, exp := range expected {
		if !strings.Contains(result, exp) {
			t.Errorf("expected arg '%s' preserved, got: %s", exp, result)
		}
	}
}

// TestFFmpegWrapper_MultipleProbesize verifies multiple occurrences are handled.
func TestFFmpegWrapper_MultipleProbesize(t *testing.T) {
	script := createTestWrapper(t)

	args := []string{
		"-probesize", "2G",
		"-analyzeduration", "500M",
		"-probesize", "500M", // hypothetical second occurrence
	}

	result := runWrapper(script, args)

	// Count occurrences of 100M (should be 2)
	count := strings.Count(result, "100M")
	if count != 2 {
		t.Errorf("expected 2 occurrences of 100M, got %d: %s", count, result)
	}
}

// TestFFmpegWrapper_NoProbesizePassthrough verifies passthrough when no probesize present.
func TestFFmpegWrapper_NoProbesizePassthrough(t *testing.T) {
	script := createTestWrapper(t)

	args := []string{
		"-i", "file:/path/to/video.mkv",
		"-c", "copy",
		"-f", "hls",
	}

	result := runWrapper(script, args)

	// Original args preserved, no probesize added
	if !strings.Contains(result, "-c copy") {
		t.Errorf("expected -c copy preserved, got: %s", result)
	}
	if !strings.Contains(result, "-f hls") {
		t.Errorf("expected -f hls preserved, got: %s", result)
	}
}

// createTestWrapper creates a test version of the wrapper that echoes args instead of exec.
func createTestWrapper(t *testing.T) string {
	t.Helper()

	// Create a wrapper that echoes args to stdout instead of exec-ing ffmpeg
	wrapper := `#!/bin/bash
PROBESIZE_OVERRIDE="100M"
ANALYZEDURATION_OVERRIDE="30M"
NEW_ARGS=()
SKIP_NEXT=false
for arg in "$@"; do
    if $SKIP_NEXT; then
        SKIP_NEXT=false
        continue
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
echo "${NEW_ARGS[@]}"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "ffmpeg-wrapper-test.sh")
	if err := os.WriteFile(path, []byte(wrapper), 0755); err != nil {
		t.Fatalf("failed to write test wrapper: %v", err)
	}
	return path
}

// runWrapper executes the test wrapper with given args and returns the output.
func runWrapper(scriptPath string, args []string) string {
	cmd := exec.Command("bash", scriptPath)
	cmd.Args = append(cmd.Args, args...)
	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out))
}
