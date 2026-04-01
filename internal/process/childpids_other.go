//go:build !linux

package process

// CollectChildPIDsFromProc is a no-op on non-Linux platforms where /proc is
// not available. Returns an empty (non-nil) slice.
func CollectChildPIDsFromProc(_ int) []int {
	return []int{}
}
