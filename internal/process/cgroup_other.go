//go:build !linux

package process

// cgroupSupportedPlatform returns false on non-Linux platforms where cgroup v2
// is not available.
func cgroupSupportedPlatform() bool {
	return false
}
