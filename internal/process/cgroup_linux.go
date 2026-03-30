//go:build linux

package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroupSupportedPlatform checks for cgroup v2 unified hierarchy on Linux.
func cgroupSupportedPlatform() bool {
	// cgroup v2 is indicated by the cgroup.controllers file at the root.
	_, err := os.Stat(filepath.Join(cgroupRoot(), "cgroup.controllers"))
	return err == nil
}

// cgroupCreatePlatform creates a session cgroup directory and applies limits.
func cgroupCreatePlatform(sessionID string, limits CgroupLimits) (string, error) {
	cgPath := sessionCgroupPath(sessionID)

	// Ensure the parent directory exists.
	parentPath := filepath.Join(cgroupRoot(), cgroupBasePath)
	if err := os.MkdirAll(parentPath, 0755); err != nil {
		return "", fmt.Errorf("cgroup: create parent %s: %w", cgroupBasePath, err)
	}

	// Enable controllers in the parent so children can use them.
	if err := enableControllers(parentPath); err != nil {
		return "", fmt.Errorf("cgroup: enable controllers: %w", err)
	}

	if err := os.MkdirAll(cgPath, 0755); err != nil {
		return "", fmt.Errorf("cgroup: create %s/%s: %w", cgroupBasePath, sessionID, err)
	}

	if limits.MemoryMax > 0 {
		if err := cgroupSetMemoryPlatform(sessionID, limits.MemoryMax); err != nil {
			return cgPath, err
		}
	}

	if limits.CPUQuota > 0 {
		period := limits.CPUPeriod
		if period <= 0 {
			period = 100000
		}
		if err := cgroupSetCPUPlatform(sessionID, limits.CPUQuota, period); err != nil {
			return cgPath, err
		}
	}

	return cgPath, nil
}

// cgroupSetMemoryPlatform writes memory.max for the session cgroup.
func cgroupSetMemoryPlatform(sessionID string, memoryMax int64) error {
	path := filepath.Join(sessionCgroupPath(sessionID), "memory.max")
	content := "max"
	if memoryMax > 0 {
		content = strconv.FormatInt(memoryMax, 10)
	}
	if err := cgroupWriteFile(path, content); err != nil {
		return fmt.Errorf("cgroup: set memory.max for %s: %w", sessionID, err)
	}
	return nil
}

// cgroupSetCPUPlatform writes cpu.max for the session cgroup.
func cgroupSetCPUPlatform(sessionID string, quota, period int64) error {
	if period <= 0 {
		period = 100000
	}
	path := filepath.Join(sessionCgroupPath(sessionID), "cpu.max")
	content := fmt.Sprintf("%d %d", quota, period)
	if err := cgroupWriteFile(path, content); err != nil {
		return fmt.Errorf("cgroup: set cpu.max for %s: %w", sessionID, err)
	}
	return nil
}

// cgroupUsageReadPlatform reads memory.current and cpu.stat from the cgroup.
func cgroupUsageReadPlatform(sessionID string) (CgroupUsage, error) {
	cgPath := sessionCgroupPath(sessionID)
	var usage CgroupUsage

	memCur, err := cgroupReadFileInt(filepath.Join(cgPath, "memory.current"))
	if err != nil {
		return usage, fmt.Errorf("cgroup: read usage for %s: %w", sessionID, err)
	}
	usage.MemoryCurrent = memCur

	// cpu.stat contains "usage_usec <value>" among other lines.
	cpuStatPath := filepath.Join(cgPath, "cpu.stat")
	data, err := os.ReadFile(cpuStatPath)
	if err != nil {
		return usage, fmt.Errorf("cgroup: read cpu.stat for %s: %w", sessionID, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "usage_usec ") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				val, parseErr := strconv.ParseInt(parts[1], 10, 64)
				if parseErr == nil {
					usage.CPUUsage = val
				}
			}
			break
		}
	}

	return usage, nil
}

// cgroupAddPIDPlatform writes a PID to the cgroup.procs file.
func cgroupAddPIDPlatform(sessionID string, pid int) error {
	path := filepath.Join(sessionCgroupPath(sessionID), "cgroup.procs")
	if err := cgroupWriteFile(path, strconv.Itoa(pid)); err != nil {
		return fmt.Errorf("cgroup: add pid %d to %s: %w", pid, sessionID, err)
	}
	return nil
}

// cgroupCleanupPlatform removes the session cgroup directory.
// The cgroup must have no running processes before removal.
func cgroupCleanupPlatform(sessionID string) error {
	cgPath := sessionCgroupPath(sessionID)
	if err := os.Remove(cgPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cgroup: cleanup %s: %w", sessionID, err)
	}
	return nil
}

// enableControllers reads available controllers from cgroup.controllers and
// enables them in cgroup.subtree_control so child cgroups can use them.
func enableControllers(parentPath string) error {
	controllersPath := filepath.Join(parentPath, "cgroup.controllers")
	data, err := os.ReadFile(controllersPath)
	if err != nil {
		// If we can't read controllers, the cgroup may not exist yet or
		// we lack permissions. This is not fatal — limits may just not apply.
		return nil
	}

	controllers := strings.Fields(strings.TrimSpace(string(data)))
	if len(controllers) == 0 {
		return nil
	}

	// Write "+controller" entries to subtree_control.
	var enables []string
	for _, c := range controllers {
		enables = append(enables, "+"+c)
	}
	subtreePath := filepath.Join(parentPath, "cgroup.subtree_control")
	return cgroupWriteFile(subtreePath, strings.Join(enables, " "))
}
