//go:build !linux

package process

// cgroupSupportedPlatform returns false on non-Linux platforms where cgroup v2
// is not available.
func cgroupSupportedPlatform() bool {
	return false
}

// cgroupCreatePlatform is a no-op on non-Linux platforms.
func cgroupCreatePlatform(_ string, _ CgroupLimits) (string, error) {
	return "", nil
}

// cgroupSetMemoryPlatform is a no-op on non-Linux platforms.
func cgroupSetMemoryPlatform(_ string, _ int64) error {
	return nil
}

// cgroupSetCPUPlatform is a no-op on non-Linux platforms.
func cgroupSetCPUPlatform(_ string, _, _ int64) error {
	return nil
}

// cgroupUsageReadPlatform is a no-op on non-Linux platforms.
func cgroupUsageReadPlatform(_ string) (CgroupUsage, error) {
	return CgroupUsage{}, nil
}

// cgroupAddPIDPlatform is a no-op on non-Linux platforms.
func cgroupAddPIDPlatform(_ string, _ int) error {
	return nil
}

// cgroupCleanupPlatform is a no-op on non-Linux platforms.
func cgroupCleanupPlatform(_ string) error {
	return nil
}
