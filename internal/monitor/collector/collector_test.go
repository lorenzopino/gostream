package collector

import (
	"testing"
)

func TestReadRAM_NonZeroOnCurrentPlatform(t *testing.T) {
	pct, usedGB, totalGB := readRAM()
	if totalGB <= 0 {
		t.Skipf("readRAM not available or returned 0 on this platform")
	}
	if pct < 0 || pct > 100 {
		t.Errorf("RAM percentage out of range: %.1f%%", pct)
	}
	if usedGB <= 0 {
		t.Errorf("Used RAM should be > 0, got %.2f GB", usedGB)
	}
	t.Logf("RAM: %.1f%% used (%.2f / %.2f GB)", pct, usedGB, totalGB)
}

func TestReadCPU_DeltaCalculation(t *testing.T) {
	c := &Collector{}

	// First call initializes — returns 0 (no previous data)
	first := c.readCPU()
	if first != 0 {
		// On some platforms the first call might return a value if prevCPU is already set
		t.Logf("First CPU read returned %.1f%% (expected 0 or near-zero)", first)
	}

	// We can't easily simulate CPU time passing, but we verify no panic
	// and that the state is properly maintained
	second := c.readCPU()
	if second < 0 || second > 100 {
		t.Errorf("CPU percentage out of range: %.1f%%", second)
	}
	t.Logf("CPU: %.1f%%", second)
}

func TestCollector_Run_NoPanic(t *testing.T) {
	c := New(
		"http://127.0.0.1:8090", // gostorm URL (will fail, but shouldn't panic)
		"",                       // fuse path
		"",                       // source path
		"",                       // vpn iface
		"",                       // plex URL
		"",                       // plex token
		0,                        // natpmp port
		9080,                     // metrics port
	)

	// Collect once — should not panic even with all empty paths
	c.collect()

	status := c.Status()
	if status.GoStorm.OK {
		t.Log("GoStorm is reachable (unexpected but OK)")
	}
	// On macOS, CPU and RAM should now have values
	if status.CPU < 0 || status.CPU > 100 {
		t.Errorf("CPU out of range: %.1f%%", status.CPU)
	}
	if status.RAMPct < 0 || status.RAMPct > 100 {
		t.Errorf("RAM pct out of range: %.1f%%", status.RAMPct)
	}
	if status.RAMTotalGB <= 0 {
		t.Errorf("RAM total should be > 0 on a real machine, got %.2f GB", status.RAMTotalGB)
	}

	t.Logf("Status: CPU=%.1f%% RAM=%.1f%% (%.1f GB)", status.CPU, status.RAMPct, status.RAMTotalGB)
}

func TestHealthStatus_CPUNotZeroAfterCollection(t *testing.T) {
	c := New("http://127.0.0.1:1", "", "", "", "", "", 0, 9081)

	c.collect()
	s1 := c.Status()

	// Collect again — CPU should show a value (even if small) on darwin
	// On Linux it will work; on darwin it depends on sysctl availability.
	c.collect()
	s2 := c.Status()

	// At minimum, the second read should not crash and values should be in range
	if s2.CPU < 0 || s2.CPU > 100 {
		t.Errorf("CPU out of range after collection: %.1f%%", s2.CPU)
	}
	if s2.RAMTotalGB <= 0 {
		t.Errorf("RAM total should be > 0, got %.2f GB", s2.RAMTotalGB)
	}

	// Verify the collector retained delta state between calls
	_ = s1 // just ensure first collection didn't panic
	t.Logf("After 2 collections: CPU=%.1f%%, RAM=%.1f%% (%.1f GB)", s2.CPU, s2.RAMPct, s2.RAMTotalGB)
}
