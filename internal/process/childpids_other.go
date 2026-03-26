//go:build !linux

package process

// CollectChildPIDs is a no-op stub on non-Linux platforms.
// On Linux, /proc enumeration is used to find processes sharing the same pgid.
func CollectChildPIDs(pid int) []int {
	return nil
}
