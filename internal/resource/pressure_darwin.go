//go:build darwin

package resource

import (
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// readLoadAverageSysctl reads the 1-minute load average via sysctl on Darwin.
func readLoadAverageSysctl() float64 {
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return 0
	}
	// Output format: "{ 1.23 4.56 7.89 }"
	s := strings.Trim(string(out), "{ }\n")
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}

// readMemoryDarwin reads total and available memory on macOS using sysctl
// for total memory and host_statistics64 for free/inactive pages.
func readMemoryDarwin() (total, available uint64) {
	// Total physical memory.
	totalMem, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, 0
	}
	total = totalMem

	// Page size.
	pageSize, err := unix.SysctlUint32("vm.pagesize")
	if err != nil {
		pageSize = 4096 // safe default
	}

	// Use vm_stat command to get page counts — avoids CGo for host_statistics64.
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return total, total / 2 // conservative fallback
	}

	var freePages, inactivePages, speculativePages uint64
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(strings.TrimRight(parts[1], "."))
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case "Pages free":
			freePages = val
		case "Pages inactive":
			inactivePages = val
		case "Pages speculative":
			speculativePages = val
		}
	}

	// Available = free + inactive + speculative (similar to Activity Monitor).
	available = (freePages + inactivePages + speculativePages) * uint64(pageSize)

	// Ensure we don't report more available than total due to rounding or speculative pages.
	if available > total {
		available = total
	}

	return total, available
}
