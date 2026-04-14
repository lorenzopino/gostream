package health

import (
	"context"
	"fmt"
	"log"
	"time"

	"gostream/internal/metadb"
)

// TorrentTester defines the interface for testing a torrent's health.
type TorrentTester interface {
	// TestTorrent adds the torrent (if not already present), waits for metadata/info,
	// and returns download speed in KBps and seeder count.
	TestTorrent(ctx context.Context, hash, magnet, title string) (speedKBps int64, seeders int, err error)

	// CurrentTorrentStatus returns the current speed and seeders for an active torrent,
	// or returns 0,0,false if the torrent is not active.
	CurrentTorrentStatus(ctx context.Context, hash string) (speedKBps int64, seeders int, active bool)
}

// TorrentReplacer defines the interface for replacing a torrent with an alternative.
type TorrentReplacer interface {
	// ReplaceTorrent replaces the current torrent for a content_id with a new one.
	// Returns true if replacement succeeded, false if it was rejected or failed.
	ReplaceTorrent(ctx context.Context, contentID, oldHash, newHash, newMagnet, newTitle string) (bool, error)
}

// Config holds configuration for the offline health checker.
type Config struct {
	CheckInterval   time.Duration // How often to run the full health check cycle (default 24h)
	MinSpeedSlow    int64         // KBps threshold between slow and dead (default 100)
	MinSpeedFast    int64         // KBps threshold between healthy and slow (default 500)
	MinSeeders      int           // Minimum seeders for healthy classification (default 10)
	SlowSeeders     int           // Minimum seeders for slow classification (default 5)
	MaxReplacements int           // Max replacements per cycle to prevent thrashing (default 5)
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		CheckInterval:   24 * time.Hour,
		MinSpeedSlow:    100,
		MinSpeedFast:    500,
		MinSeeders:      10,
		SlowSeeders:     5,
		MaxReplacements: 5,
	}
}

// OfflineHealthChecker periodically checks torrent health and replaces dead/slow ones.
type OfflineHealthChecker struct {
	db       *metadb.DB
	tester   TorrentTester
	replacer TorrentReplacer
	cfg      Config
	logger   *log.Logger
}

// New creates a new OfflineHealthChecker.
func New(db *metadb.DB, tester TorrentTester, replacer TorrentReplacer, cfg Config, logger *log.Logger) *OfflineHealthChecker {
	return &OfflineHealthChecker{
		db:       db,
		tester:   tester,
		replacer: replacer,
		cfg:      cfg,
		logger:   logger,
	}
}

// Run starts the periodic health check loop. Blocks until ctx is cancelled.
func (h *OfflineHealthChecker) Run(ctx context.Context) {
	if h.logger != nil {
		h.logger.Printf("[HealthChecker] Starting offline health checker (interval: %v)", h.cfg.CheckInterval)
	}

	// Run initial check immediately
	h.runCheck(ctx)

	ticker := time.NewTicker(h.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if h.logger != nil {
				h.logger.Printf("[HealthChecker] Stopping offline health checker")
			}
			return
		case <-ticker.C:
			h.runCheck(ctx)
		}
	}
}

// runCheck performs a single health check cycle.
func (h *OfflineHealthChecker) runCheck(ctx context.Context) {
	if h.logger != nil {
		h.logger.Printf("[HealthChecker] Starting health check cycle")
	}

	unhealthy, err := h.db.GetUnhealthyAlternatives()
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] Failed to get unhealthy alternatives: %v", err)
		}
		return
	}

	if len(unhealthy) == 0 {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] No unhealthy torrents to check — all torrents are healthy")
		}
		return
	}

	if h.logger != nil {
		h.logger.Printf("[HealthChecker] Found %d unhealthy torrents to check", len(unhealthy))
	}

	replacements := 0
	now := time.Now().Unix()

	for _, alt := range unhealthy {
		if ctx.Err() != nil {
			break
		}
		if replacements >= h.cfg.MaxReplacements {
			if h.logger != nil {
				h.logger.Printf("[HealthChecker] Reached max replacements (%d) for this cycle", replacements)
			}
			break
		}

		h.checkAndUpdate(ctx, &alt, now)

		// If this was the primary torrent (rank 1) and it's dead or slow, try to replace.
		if alt.Rank == 1 && (alt.Status == "dead" || alt.Status == "verified_slow") {
			if h.tryReplace(ctx, &alt) {
				replacements++
			}
		}
	}

	// Print summary
	if h.logger != nil {
		summary, _ := h.db.CountAlternativesByStatus()
		h.logger.Printf("[HealthChecker] Check cycle complete. Replacements: %d. Status summary: %v", replacements, summary)
	}
}

// checkAndUpdate tests a single torrent and updates its health status.
func (h *OfflineHealthChecker) checkAndUpdate(ctx context.Context, alt *metadb.TorrentAlternative, nowUnix int64) {
	// First check if torrent is currently active in GoStorm — if so, get live stats
	if speed, seeders, active := h.tester.CurrentTorrentStatus(ctx, alt.Hash); active {
		newStatus := classifyHealthWithCfg(h.cfg, speed, seeders)
		h.db.UpdateAlternativeHealth(alt.ContentID, alt.Hash, newStatus, speed, seeders, nowUnix)
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] %s: active — speed=%d KBps, seeders=%d → %s",
				alt.Title[:min(40, len(alt.Title))], speed, seeders, newStatus)
		}
		return
	}

	// Not active — test it
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	speed, seeders, err := h.tester.TestTorrent(ctx, alt.Hash, "", alt.Title)
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] %s: test failed — %v → dead",
				alt.Title[:min(40, len(alt.Title))], err)
		}
		h.db.UpdateAlternativeHealth(alt.ContentID, alt.Hash, "dead", 0, 0, nowUnix)
		return
	}

	newStatus := classifyHealthWithCfg(h.cfg, speed, seeders)
	h.db.UpdateAlternativeHealth(alt.ContentID, alt.Hash, newStatus, speed, seeders, nowUnix)
	if h.logger != nil {
		h.logger.Printf("[HealthChecker] %s: speed=%d KBps, seeders=%d → %s",
			alt.Title[:min(40, len(alt.Title))], speed, seeders, newStatus)
	}
}

// tryReplace attempts to replace a dead rank-1 torrent with the best available alternative.
// Implements verify-before-replace: tests the alternative first, only replaces if significantly better.
func (h *OfflineHealthChecker) tryReplace(ctx context.Context, dead *metadb.TorrentAlternative) bool {
	next, found, err := h.db.GetNextBestAlternative(dead.ContentID, dead.Hash)
	if err != nil || !found {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] No alternatives available for %s", dead.ContentID)
		}
		return false
	}

	// Test the alternative
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	speed, seeders, err := h.tester.TestTorrent(ctx, next.Hash, "", next.Title)
	if err != nil {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] Alternative %s: test failed — %v → marked tested_no_better",
				next.Title[:min(40, len(next.Title))], err)
		}
		h.db.UpdateAlternativeStatus(next.ContentID, next.Hash, "tested_no_better")
		return false
	}

	// Verify-before-replace: only replace if alternative is at least 1.5x better
	oldSpeed := dead.AvgSpeedKBps
	if oldSpeed <= 0 {
		oldSpeed = 1 // Dead torrent, treat as near-zero
	}

	if !shouldReplaceWithCfg(h.cfg, speed, oldSpeed) && !shouldReplaceStreamableWithCfg(h.cfg, dead, next, speed, seeders) {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] Alternative %s: speed=%d KBps (current=%d KBps) — not significant improvement, skipping",
				next.Title[:min(40, len(next.Title))], speed, oldSpeed)
		}
		h.db.UpdateAlternativeStatus(next.ContentID, next.Hash, "tested_no_better")
		h.db.IncrementReplacementCount(next.ContentID, next.Hash)
		return false
	}

	// Update the alternative's status with test results
	newStatus := classifyHealthWithCfg(h.cfg, speed, seeders)
	h.db.UpdateAlternativeHealth(next.ContentID, next.Hash, newStatus, speed, seeders, time.Now().Unix())

	// Replace
	magnet := BuildMagnet(next.Hash, next.Title)
	replaced, err := h.replacer.ReplaceTorrent(ctx, dead.ContentID, dead.Hash, next.Hash, magnet, next.Title)
	if err != nil || !replaced {
		if h.logger != nil {
			h.logger.Printf("[HealthChecker] Replacement failed for %s: %v", dead.ContentID, err)
		}
		h.db.UpdateAlternativeStatus(next.ContentID, next.Hash, "tested_no_better")
		return false
	}

	// Update statuses
	h.db.UpdateAlternativeStatus(next.ContentID, next.Hash, "active")
	h.db.UpdateAlternativeStatus(dead.ContentID, dead.Hash, "dead")
	h.db.IncrementReplacementCount(next.ContentID, next.Hash)

	if h.logger != nil {
		h.logger.Printf("[HealthChecker] ✓ Replaced dead torrent for %s: %s → %s (speed: %d KBps vs %d KBps)",
			dead.ContentID,
			dead.Title[:min(30, len(dead.Title))],
			next.Title[:min(30, len(next.Title))],
			speed, oldSpeed)
	}
	return true
}

// classifyHealthWithCfg classifies torrent health based on config thresholds.
func classifyHealthWithCfg(cfg Config, speedKBps int64, seeders int) string {
	if seeders == 0 {
		return "dead"
	}
	if seeders < cfg.SlowSeeders {
		return "verified_slow"
	}
	if speedKBps > cfg.MinSpeedFast && seeders >= cfg.MinSeeders {
		return "verified_healthy"
	}
	if speedKBps > cfg.MinSpeedSlow {
		return "verified_slow"
	}
	return "dead"
}

// shouldReplaceWithCfg checks if the new torrent is significantly better than the old one.
func shouldReplaceWithCfg(cfg Config, newSpeed, oldSpeed int64) bool {
	if oldSpeed <= 0 {
		return true // Old is dead, any working alternative is better
	}
	// Require at least 50% improvement (1.5x)
	const minImprovement = 1.5
	return float64(newSpeed) >= float64(oldSpeed)*minImprovement
}

func shouldReplaceStreamableWithCfg(cfg Config, current, next *metadb.TorrentAlternative, newSpeed int64, newSeeders int) bool {
	if current == nil || next == nil {
		return false
	}
	if current.Status != "verified_slow" {
		return false
	}
	if current.Size <= 0 || next.Size <= 0 {
		return false
	}
	if newSeeders < cfg.MinSeeders || newSpeed <= cfg.MinSpeedSlow {
		return false
	}
	const meaningfulSizeReduction = 0.80
	return float64(next.Size) <= float64(current.Size)*meaningfulSizeReduction
}

// BuildMagnet creates a magnet URI from a hash and title.
func BuildMagnet(hash, title string) string {
	return fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=%s", hash, title)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
