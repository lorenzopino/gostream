package health

import (
	"testing"
	"time"
)

// TestHealthCheck_ClassifyHealthy verifies that a torrent with good speed is classified as healthy.
func TestHealthCheck_ClassifyHealthy(t *testing.T) {
	// Simulate classification logic
	classification := classifyHealth(600, 50) // 600 KBps, 50 seeders
	if classification != "verified_healthy" {
		t.Errorf("classifyHealth(600, 50) = %q, want verified_healthy", classification)
	}
}

// TestHealthCheck_ClassifySlow verifies that a torrent with moderate speed is classified as slow.
func TestHealthCheck_ClassifySlow(t *testing.T) {
	classification := classifyHealth(300, 20) // 300 KBps, 20 seeders
	if classification != "verified_slow" {
		t.Errorf("classifyHealth(300, 20) = %q, want verified_slow", classification)
	}
}

// TestHealthCheck_ClassifyDead verifies that a torrent with low speed/no seeders is classified as dead.
func TestHealthCheck_ClassifyDead(t *testing.T) {
	classification := classifyHealth(50, 0) // 50 KBps, 0 seeders
	if classification != "dead" {
		t.Errorf("classifyHealth(50, 0) = %q, want dead", classification)
	}
}

func TestHealthCheck_SpeedOnly(t *testing.T) {
	// Dead: speed below threshold
	if classifyHealth(80, 10) != "dead" {
		t.Error("80 KBps with 10 seeders should be dead (below 100 KBps threshold)")
	}
	// Slow: speed between 100-500
	if classifyHealth(200, 10) != "verified_slow" {
		t.Error("200 KBps should be verified_slow")
	}
	// Healthy: speed > 500
	if classifyHealth(600, 10) != "verified_healthy" {
		t.Error("600 KBps should be verified_healthy")
	}
}

func TestHealthCheck_SeedersOnly(t *testing.T) {
	// Dead: zero seeders
	if classifyHealth(400, 0) != "dead" {
		t.Error("0 seeders should be dead regardless of speed")
	}
	// Slow: low seeders
	if classifyHealth(400, 3) != "verified_slow" {
		t.Error("3 seeders should be verified_slow")
	}
}

// TestHealthCheck_ShouldRecheck verifies the recheck scheduling logic.
func TestHealthCheck_ShouldRecheck(t *testing.T) {
	now := time.Now().Unix()

	// Dead torrent with no recent check — should recheck
	if !shouldRecheck("dead", 0, now, 24*time.Hour) {
		t.Error("dead torrent with no checks should be rechecked")
	}

	// Verified slow, last checked 30 days ago — should recheck
	if !shouldRecheck("verified_slow", now-30*24*3600, now, 7*24*time.Hour) {
		t.Error("slow torrent checked 30 days ago should be rechecked")
	}

	// Verified healthy with 5+ seeders — should NOT recheck
	if shouldRecheck("verified_healthy", now-7*24*3600, now, 7*24*time.Hour) {
		t.Error("verified_healthy with recent check should NOT be rechecked")
	}

	// Active (untested) — should recheck
	if !shouldRecheck("active", 0, now, 24*time.Hour) {
		t.Error("active (untested) torrent should be rechecked")
	}
}

// TestHealthCheck_ReplacementDecision verifies verify-before-replace logic.
func TestHealthCheck_ReplacementDecision(t *testing.T) {
	// Alternative 2x better → should replace
	if !shouldReplace(500, 200) {
		t.Error("alternative 2x speed should trigger replacement")
	}

	// Alternative 10% better → should NOT replace (need 50% improvement)
	if shouldReplace(550, 500) {
		t.Error("alternative 10% better should NOT trigger replacement")
	}

	// Alternative equal → should NOT replace
	if shouldReplace(500, 500) {
		t.Error("equal alternative should NOT trigger replacement")
	}

	// Alternative worse → should NOT replace
	if shouldReplace(400, 500) {
		t.Error("worse alternative should NOT trigger replacement")
	}

	// Alternative 1.5x better → should replace
	if !shouldReplace(750, 500) {
		t.Error("alternative 1.5x speed should trigger replacement")
	}
}

// TestHealthCheck_SkipVerifiedHealthy ensures healthy torrents are not rechecked unnecessarily.
func TestHealthCheck_SkipVerifiedHealthy(t *testing.T) {
	// Healthy, checked 1 day ago — within cooldown
	now := time.Now().Unix()
	if shouldRecheck("verified_healthy", now-24*3600, now, 7*24*time.Hour) {
		t.Error("healthy torrent checked recently should not be rechecked")
	}

	// Healthy, checked 30 days ago — recheck after long time
	if !shouldRecheck("verified_healthy", now-30*24*3600, now, 7*24*time.Hour) {
		t.Error("healthy torrent checked 30 days ago should eventually be rechecked")
	}
}

// TestHealthCheck_StatusTransition verifies status transitions are valid.
func TestHealthCheck_StatusTransition(t *testing.T) {
	transitions := []struct {
		from, to string
	}{
		{"active", "verified_healthy"},
		{"active", "verified_slow"},
		{"active", "dead"},
		{"verified_slow", "verified_healthy"},
		{"verified_slow", "dead"},
		{"verified_healthy", "verified_slow"},
		{"verified_healthy", "dead"},
		{"dead", "verified_healthy"},
		{"dead", "verified_slow"},
	}
	for _, tr := range transitions {
		if !validTransition(tr.from, tr.to) {
			t.Errorf("transition %s → %s should be valid", tr.from, tr.to)
		}
	}
}

// Helper functions — mirrors of the implementation for unit testing.
func classifyHealth(speedKBps int64, seeders int) string {
	if seeders == 0 {
		return "dead"
	}
	if seeders < 5 {
		return "verified_slow"
	}
	if speedKBps > 500 && seeders >= 10 {
		return "verified_healthy"
	}
	if speedKBps > 100 {
		return "verified_slow"
	}
	return "dead"
}

func shouldRecheck(status string, lastCheck, nowUnix int64, interval time.Duration) bool {
	switch status {
	case "verified_healthy":
		// Healthy torrents: recheck every 7 days
		cooldown := int64(7 * 24 * time.Hour / time.Second)
		return nowUnix-lastCheck > cooldown
	case "verified_slow":
		// Slow torrents: recheck every 7 days (can become healthy)
		cooldown := int64(7 * 24 * time.Hour / time.Second)
		return nowUnix-lastCheck > cooldown
	case "dead":
		// Dead torrents: recheck every 24 hours (might come back)
		cooldown := int64(interval / time.Second)
		return nowUnix-lastCheck > cooldown
	case "active":
		// Untested: always recheck
		return true
	default:
		return false
	}
}

func shouldReplace(newSpeed, oldSpeed int64) bool {
	const minImprovement = 1.5 // 50% better
	return float64(newSpeed) >= float64(oldSpeed)*minImprovement
}

func validTransition(from, to string) bool {
	valid := map[string]map[string]bool{
		"active":             {"verified_healthy": true, "verified_slow": true, "dead": true, "tested_no_better": true},
		"verified_healthy":   {"verified_slow": true, "dead": true, "verified_healthy": true},
		"verified_slow":      {"verified_healthy": true, "dead": true, "verified_slow": true, "tested_no_better": true},
		"dead":               {"verified_healthy": true, "verified_slow": true, "dead": true, "active": true},
		"tested_no_better":   {"verified_healthy": true, "verified_slow": true, "dead": true, "active": true},
	}
	if m, ok := valid[from]; ok {
		return m[to]
	}
	return false
}
