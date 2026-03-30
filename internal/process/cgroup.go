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

// cgroupRoot is the cgroup v2 unified hierarchy mount point.
// Tests override this via the cgroupRootOverride variable.
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
	return cgroupCreatePlatform(sessionID, limits)
}

// CgroupSetMemory updates the memory limit for an existing session cgroup.
// On unsupported platforms this is a no-op.
func CgroupSetMemory(sessionID string, memoryMax int64) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupSetMemoryPlatform(sessionID, memoryMax)
}

// CgroupSetCPU updates the CPU quota for an existing session cgroup.
// On unsupported platforms this is a no-op.
func CgroupSetCPU(sessionID string, quota, period int64) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupSetCPUPlatform(sessionID, quota, period)
}

// CgroupUsageRead reads current resource usage from a session cgroup.
// On unsupported platforms this returns zero-valued usage with no error.
func CgroupUsageRead(sessionID string) (CgroupUsage, error) {
	if !CgroupSupported() {
		return CgroupUsage{}, nil
	}
	return cgroupUsageReadPlatform(sessionID)
}

// CgroupAddPID adds a process to the session cgroup.
// On unsupported platforms this is a no-op.
func CgroupAddPID(sessionID string, pid int) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupAddPIDPlatform(sessionID, pid)
}

// CgroupCleanup removes the cgroup for the given session ID.
// On unsupported platforms this is a no-op.
func CgroupCleanup(sessionID string) error {
	if !CgroupSupported() {
		return nil
	}
	return cgroupCleanupPlatform(sessionID)
}

// --- Shared helpers used by the Linux implementation and tests ---

// sessionCgroupPath returns the filesystem path for a session's cgroup.
func sessionCgroupPath(sessionID string) string {
	return filepath.Join(cgroupRoot(), cgroupBasePath, sessionID)
}

// writeFile is a small helper to write content to a cgroup control file.
func cgroupWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// readFileInt reads a cgroup control file and parses it as an int64.
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
