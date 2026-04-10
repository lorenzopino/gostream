//go:build linux

package collector

import (
	"os"
	"strconv"
	"strings"
)

// readPlatformCPUImpl reads CPU ticks from /proc/stat (Linux).
func readPlatformCPUImpl() (idle, total uint64, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return 0, 0, false
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
		if i == 4 {
			idle = v
		}
	}
	return idle, total, true
}

// readPlatformRAMImpl reads RAM info from /proc/meminfo (Linux).
func readPlatformRAMImpl() (totalBytes, availableBytes uint64, ok bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var memTotal, memAvail uint64
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		switch f[0] {
		case "MemTotal:":
			memTotal = v
		case "MemAvailable:":
			memAvail = v
		}
	}
	// Values are in kB
	return memTotal * 1024, memAvail * 1024, memTotal > 0
}
