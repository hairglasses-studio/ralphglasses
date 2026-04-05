package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CgroupLimits specifies resource limits for a session process cgroup.
type CgroupLimits struct {
	// MemoryMax is the hard memory limit in bytes. Zero means no limit.
	MemoryMax int64

	// CPUQuota is the CPU time quota in microseconds per CPUPeriod.
	// For example, 100000 with a period of 100000 gives 1 full CPU.
	// Zero means no limit.
	CPUQuota int64

	// CPUPeriod is the scheduling period in microseconds. Defaults to 100000 (100ms).
	CPUPeriod int64
}

// CgroupUsage reports current resource usage from a cgroup.
type CgroupUsage struct {
	// MemoryCurrent is the current memory usage in bytes.
	MemoryCurrent int64

	// CPUUsage is the total CPU usage in microseconds.
	CPUUsage int64
}

// defaultCgroupRoot is the cgroup v2 unified hierarchy mount point.
const defaultCgroupRoot = "/sys/fs/cgroup"

// cgroupRootOverride allows tests to redirect cgroup operations to a temp directory.
// When empty, defaultCgroupRoot is used.
var cgroupRootOverride string

// cgroupSupportedOverride allows tests to force the supported/unsupported state.
// nil means use real detection; non-nil pointer value is returned directly.
var cgroupSupportedOverride *bool

func cgroupRoot() string {
	if cgroupRootOverride != "" {
		return cgroupRootOverride
	}
	return defaultCgroupRoot
}

// cgroupBasePath is the parent cgroup under which session cgroups are created.
const cgroupBasePath = "ralphglasses.sessions"

// CgroupSupported returns true if cgroup v2 is available on this system.
// On non-Linux platforms this always returns false.
func CgroupSupported() bool {
	if cgroupSupportedOverride != nil {
		return *cgroupSupportedOverride
	}
	return cgroupSupportedPlatform()
}

// CgroupCreate creates a cgroup for the given session ID and applies the
// specified resource limits. The session ID should be unique per process
// (e.g., a repo path hash or UUID). Returns the cgroup path on success.
// On unsupported platforms this is a no-op that returns ("", nil).
func CgroupCreate(sessionID string, limits CgroupLimits) (string, error) {
	if !CgroupSupported() {
		return "", nil
	}

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
		if err := cgroupSetMemoryImpl(sessionID, limits.MemoryMax); err != nil {
			return cgPath, err
		}
	}

	if limits.CPUQuota > 0 {
		period := limits.CPUPeriod
		if period <= 0 {
			period = 100000
		}
		if err := cgroupSetCPUImpl(sessionID, limits.CPUQuota, period); err != nil {
			return cgPath, err
		}
	}

	return cgPath, nil
}

// CgroupSetMemory updates the memory limit for an existing session cgroup.
// On unsupported platforms this is a no-op.
func CgroupSetMemory(sessionID string, memoryMax int64) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupSetMemoryImpl(sessionID, memoryMax)
}

// CgroupSetCPU updates the CPU quota for an existing session cgroup.
// On unsupported platforms this is a no-op.
func CgroupSetCPU(sessionID string, quota, period int64) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupSetCPUImpl(sessionID, quota, period)
}

// CgroupUsageRead reads current resource usage from a session cgroup.
// On unsupported platforms this returns zero-valued usage with no error.
func CgroupUsageRead(sessionID string) (CgroupUsage, error) {
	if !CgroupSupported() {
		return CgroupUsage{}, nil
	}
	return cgroupUsageReadImpl(sessionID)
}

// CgroupAddPID adds a process to the session cgroup.
// On unsupported platforms this is a no-op.
func CgroupAddPID(sessionID string, pid int) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupAddPIDImpl(sessionID, pid)
}

// CgroupCleanup removes the cgroup for the given session ID.
// On unsupported platforms this is a no-op.
func CgroupCleanup(sessionID string) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupCleanupImpl(sessionID)
}

// --- Implementation (platform-independent file I/O) ---

// sessionCgroupPath returns the filesystem path for a session's cgroup.
func sessionCgroupPath(sessionID string) string {
	return filepath.Join(cgroupRoot(), cgroupBasePath, sessionID)
}

// cgroupSetMemoryImpl writes memory.max for the session cgroup.
func cgroupSetMemoryImpl(sessionID string, memoryMax int64) error {
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

// cgroupSetCPUImpl writes cpu.max for the session cgroup.
func cgroupSetCPUImpl(sessionID string, quota, period int64) error {
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

// cgroupUsageReadImpl reads memory.current and cpu.stat from the cgroup.
func cgroupUsageReadImpl(sessionID string) (CgroupUsage, error) {
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
	for line := range strings.SplitSeq(string(data), "\n") {
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

// cgroupAddPIDImpl writes a PID to the cgroup.procs file.
func cgroupAddPIDImpl(sessionID string, pid int) error {
	path := filepath.Join(sessionCgroupPath(sessionID), "cgroup.procs")
	if err := cgroupWriteFile(path, strconv.Itoa(pid)); err != nil {
		return fmt.Errorf("cgroup: add pid %d to %s: %w", pid, sessionID, err)
	}
	return nil
}

// cgroupCleanupImpl removes the session cgroup directory.
// The cgroup must have no running processes before removal.
func cgroupCleanupImpl(sessionID string) error {
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

// cgroupWriteFile is a small helper to write content to a cgroup control file.
func cgroupWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// cgroupReadFileInt reads a cgroup control file and parses it as an int64.
func cgroupReadFileInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("cgroup: read %s: %w", filepath.Base(path), err)
	}
	val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cgroup: parse %s: %w", filepath.Base(path), err)
	}
	return val, nil
}
