//go:build darwin

package collector

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDarwinCPU_TopParsing(t *testing.T) {
	idle, total, ok := readPlatformCPUImpl()
	if !ok {
		t.Fatal("readPlatformCPUImpl failed on darwin — top -l 1 not available?")
	}
	if total == 0 {
		t.Error("total CPU ticks should be > 0")
	}
	if idle > total {
		t.Errorf("idle (%d) should not exceed total (%d)", idle, total)
	}
	idlePct := float64(idle) / float64(total) * 100
	t.Logf("CPU: %.1f%% idle (from top snapshot)", idlePct)
}

func TestDarwinCPU_FloatParseFromLine(t *testing.T) {
	tests := []struct {
		line string
		key  string
		want float64
	}{
		{"CPU usage: 9.11% user, 13.67% sys, 77.20% idle", "idle", 77.20},
		{"CPU usage: 9.11% user, 13.67% sys, 77.20% idle", "user", 9.11},
		{"CPU usage: 9.11% user, 13.67% sys, 77.20% idle", "sys", 13.67},
		{"CPU usage: 50.00% user, 50.00% sys, 0.00% idle", "idle", 0.00},
		{"no cpu data here", "idle", -1},
	}
	for _, tt := range tests {
		got := parseFloatFromLine(tt.line, tt.key)
		if got < 0 && tt.want >= 0 {
			t.Errorf("parseFloatFromLine(%q, %q) = %.2f, want %.2f", tt.line, tt.key, got, tt.want)
		}
		if got >= 0 && tt.want >= 0 && (got < tt.want-0.01 || got > tt.want+0.01) {
			t.Errorf("parseFloatFromLine(%q, %q) = %.2f, want %.2f", tt.line, tt.key, got, tt.want)
		}
	}
}

func TestDarwinRAM_SysctlParsing(t *testing.T) {
	totalBytes, availBytes, ok := readPlatformRAMImpl()
	if !ok {
		t.Fatal("readPlatformRAMImpl failed on darwin — sysctl hw.memsize not available?")
	}
	if totalBytes == 0 {
		t.Error("total RAM should be > 0")
	}
	// Sanity check: RAM between 1GB and 512GB
	if totalBytes < 1<<30 || totalBytes > 512<<30 {
		t.Errorf("RAM total out of reasonable range: %d bytes (%.1f GB)", totalBytes, float64(totalBytes)/(1<<30))
	}
	if availBytes > totalBytes {
		t.Errorf("available RAM (%.1f GB) exceeds total (%.1f GB)", float64(availBytes)/(1<<30), float64(totalBytes)/(1<<30))
	}
	totalGB := float64(totalBytes) / (1 << 30)
	availGB := float64(availBytes) / (1 << 30)
	t.Logf("RAM: %.1f GB total, %.1f GB available", totalGB, availGB)
}

func TestDarwinVMStat_OutputFormat(t *testing.T) {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		t.Skipf("vm_stat not available: %v", err)
	}

	output := string(out)
	required := []string{"Pages free:", "Pages inactive:"}
	for _, field := range required {
		if !strings.Contains(output, field) {
			t.Errorf("vm_stat output missing expected field: %s", field)
		}
	}

	if !strings.Contains(output, "page size of") {
		t.Log("vm_stat output doesn't show page size line — using default 4096")
	}
}
