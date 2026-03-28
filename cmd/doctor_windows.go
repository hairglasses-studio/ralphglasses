//go:build windows

package cmd

func statfsFreeBytes(_ string) (uint64, error) {
	// Windows: return a safe placeholder value (100 GB).
	return 100 * 1024 * 1024 * 1024, nil
}
