//go:build darwin

package collector

import (
	"os/exec"
	"strconv"
	"strings"
)

// readPlatformCPUImpl reads CPU usage via "top -l 1" snapshot on macOS.
// kern.cp_time is not available on Apple Silicon macOS.
// top -l 1 provides a one-shot CPU usage snapshot: "CPU usage: X% user, Y% sys, Z% idle"
// Returns (idle_ticks, total_ticks, ok) — we convert percentage to pseudo-ticks
// for compatibility with the delta-based readCPU() in collector.go.
func readPlatformCPUImpl() (idle, total uint64, ok bool) {
	// top -l 1 -n 0 avoids per-process detail (faster)
	out, err := exec.Command("top", "-l", "1", "-n", "0").Output()
	if err != nil {
		return 0, 0, false
	}

	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "CPU usage") {
			continue
		}
		// Parse: "CPU usage: 9.11% user, 13.67% sys, 77.20% idle"
		idlePct := parseFloatFromLine(line, "idle")
		if idlePct < 0 {
			return 0, 0, false
		}
		// Convert to pseudo-ticks: use percentage * 1000 as tick base
		// This gives enough resolution for delta calculations
		total = 100000
		idle = uint64(idlePct * 1000)
		return idle, total, true
	}

	return 0, 0, false
}

// parseFloatFromLine extracts a percentage value from a line like
// "CPU usage: 9.11% user, 13.67% sys, 77.20% idle"
func parseFloatFromLine(line, key string) float64 {
	idx := strings.Index(line, key)
	if idx == -1 {
		return -1
	}
	// Search backward for the number before "%"
	rest := line[:idx]
	// Find the last space-separated token that ends with a number
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return -1
	}
	lastField := fields[len(fields)-1]
	// Remove trailing comma if present
	lastField = strings.TrimRight(lastField, ",%")
	v, err := strconv.ParseFloat(lastField, 64)
	if err != nil {
		return -1
	}
	return v
}

// readPlatformRAMImpl reads RAM total and available via sysctl (macOS).
// hw.memsize = total RAM in bytes
// vm_stat gives page info (page size, free, active, inactive, speculative, wired, purgeable)
func readPlatformRAMImpl() (totalBytes, availableBytes uint64, ok bool) {
	// Total RAM
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, 0, false
	}
	memTotal, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || memTotal == 0 {
		return 0, 0, false
	}

	// Available RAM: parse vm_stat for free + inactive + speculative pages
	out, err = exec.Command("vm_stat").Output()
	if err != nil {
		// Fallback: use total as a rough estimate (will show 0% used)
		return memTotal, memTotal, true
	}

	// Parse page size from first line: "Mach Virtual Memory Statistics: (page size of 4096 bytes)"
	pageSize := uint64(4096) // default
	if idx := strings.Index(string(out), "page size of "); idx != -1 {
		rest := string(out)[idx+len("page size of "):]
		if end := strings.Index(rest, " "); end != -1 {
			if ps, err := strconv.ParseUint(rest[:end], 10, 64); err == nil {
				pageSize = ps
			}
		}
	}

	var freePages, inactivePages, speculativePages uint64
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimRight(line, ".")
		if strings.HasPrefix(line, "Pages free:") {
			v, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "Pages free:")), 10, 64)
			freePages = v
		} else if strings.HasPrefix(line, "Pages inactive:") {
			v, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "Pages inactive:")), 10, 64)
			inactivePages = v
		} else if strings.HasPrefix(line, "Pages speculative:") {
			v, _ := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "Pages speculative:")), 10, 64)
			speculativePages = v
		}
	}

	availableBytes = (freePages + inactivePages + speculativePages) * pageSize
	if availableBytes > memTotal {
		availableBytes = memTotal
	}

	return memTotal, availableBytes, true
}
