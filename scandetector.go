package main

import (
	"sync"
	"sync/atomic"
	"time"
)

// ScanDetector detects Jellyfin/Jellyfin-style scanning vs real playback.
//
// Scanning pattern: many files opened rapidly with tiny reads (headers, probes).
// Playback pattern: one file opened with large sequential reads.
//
// When scan mode is active, the read-ahead pump is suppressed for unconfirmed handles.
type ScanDetector struct {
	mu sync.Mutex

	openCount     uint64 // atomic: number of opens in current window
	totalReadBytes uint64 // atomic: total bytes read across all handles
	windowStart   time.Time
	windowSize    time.Duration
	openThreshold int // number of opens within window to trigger scan mode

	scanMode atomic.Bool
}

// newScanDetector creates a detector that triggers scan mode when
// openThreshold files are opened within windowSize without sufficient read volume.
func newScanDetector(openThreshold int, windowSize time.Duration) *ScanDetector {
	return &ScanDetector{
		windowStart:   time.Now(),
		windowSize:    windowSize,
		openThreshold: openThreshold,
	}
}

// RecordOpen tracks a file open event.
func (sd *ScanDetector) RecordOpen(path string) {
	atomic.AddUint64(&sd.openCount, 1)
	sd.checkWindow()
}

// RecordRead tracks a read operation with its byte count.
func (sd *ScanDetector) RecordRead(path string, bytes int64) {
	atomic.AddUint64(&sd.totalReadBytes, uint64(bytes))
}

// IsScanMode returns true if the detector believes we're in scan mode.
func (sd *ScanDetector) IsScanMode() bool {
	sd.checkWindow()
	return sd.scanMode.Load()
}

// checkWindow evaluates whether we're in scan mode based on current metrics.
// It resets the window if it has expired.
func (sd *ScanDetector) checkWindow() {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	now := time.Now()
	if now.Sub(sd.windowStart) > sd.windowSize {
		// Window expired — reset
		sd.openCount = 0
		sd.totalReadBytes = 0
		sd.windowStart = now
		sd.scanMode.Store(false)
		return
	}

	opens := atomic.LoadUint64(&sd.openCount)
	readBytes := atomic.LoadUint64(&sd.totalReadBytes)

	// Scan mode: many opens with little data read
	// Heuristic: if opens >= threshold AND avg read per open < 1MB
	if int(opens) >= sd.openThreshold {
		avgRead := float64(readBytes) / float64(opens)
		if avgRead < 1<<20 { // < 1MB average per file
			sd.scanMode.Store(true)
		}
	}
}

// PreBufferGate blocks the first read until enough data is pre-buffered.
// Prevents the user from seeing a stall when starting playback of a file
// that has no cached data yet.
type PreBufferGate struct {
	cache      *ReadAheadCache
	minBytes   int64 // minimum bytes to buffer before unblocking (default 8MB)

	mu         sync.Mutex
	readCounts map[string]int64 // path → total bytes read so far
}

func newPreBufferGate(cache *ReadAheadCache, minBytes int64) *PreBufferGate {
	return &PreBufferGate{
		cache:      cache,
		minBytes:   minBytes,
		readCounts: make(map[string]int64),
	}
}

// RecordRead tracks bytes read for a path. Called from the FUSE Read handler.
func (pg *PreBufferGate) RecordRead(path string, offset, length int64) {
	pg.mu.Lock()
	defer pg.mu.Unlock()
	pg.readCounts[path] += length
}

// WaitUntilReady blocks until minBytes are cached for the path+offset,
// or the timeout expires. Returns true if ready, false on timeout.
// For small files (< minBytes), returns immediately.
func (pg *PreBufferGate) WaitUntilReady(path string, offset int64, timeout time.Duration) bool {
	// Check if any reads have been recorded for this path
	pg.mu.Lock()
	readBytes := pg.readCounts[path]
	pg.mu.Unlock()

	// No reads yet — this is the very first read, let it through
	if readBytes == 0 {
		return true
	}

	// Nil cache — can't check buffering, let it through
	if pg.cache == nil {
		return true
	}

	// Check file size from the cache's max offset
	// If file is smaller than minBytes, skip gating
	maxCached := pg.cache.MaxCachedOffset(path)
	if maxCached > 0 && maxCached < pg.minBytes {
		return true // small file, skip gate
	}

	// Check if enough data is buffered at or after the requested offset
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if we have a chunk covering the requested offset
		if pg.cache.Exists(path, offset) {
			return true
		}
		// Also check if enough total data is buffered from offset 0
		if pg.cache.MaxCachedOffset(path) >= pg.minBytes {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
