//go:build linux

package resource

import (
	"os/exec"
	"strconv"
	"strings"
)

// readLoadAverageSysctl reads the 1-minute load average via /proc/loadavg on Linux.
// This is a fallback; readLoadAverage in pressure.go already tries /proc/loadavg first.
func readLoadAverageSysctl() float64 {
	out, err := exec.Command("cat", "/proc/loadavg").Output()
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}

// readMemoryDarwin is a no-op on Linux; /proc/meminfo is used instead.
func readMemoryDarwin() (total, available uint64) {
	return 0, 0
}
