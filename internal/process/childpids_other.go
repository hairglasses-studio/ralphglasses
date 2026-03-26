//go:build !linux

package process

// collectChildPIDsFromProc is a no-op on non-Linux platforms where /proc is
// not available. Returns an empty (non-nil) slice.
func collectChildPIDsFromProc(_ int) []int {
	return []int{}
}
