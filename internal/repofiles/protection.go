package repofiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RequiredPaths lists paths that must exist for a ralph loop to function.
// If any are missing, the loop should not proceed.
var RequiredPaths = []string{
	".ralph",
	".ralphrc",
}

// ValidateIntegrity checks that all required ralph files exist in the repo.
// Returns an error listing missing paths if any are absent.
func ValidateIntegrity(repoPath string) error {
	var missing []string
	for _, p := range RequiredPaths {
		if _, err := os.Stat(filepath.Join(repoPath, p)); err != nil {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing critical ralph files: %s", strings.Join(missing, ", "))
	}
	return nil
}
