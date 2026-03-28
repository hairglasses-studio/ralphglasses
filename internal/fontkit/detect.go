package fontkit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FontStatus describes which font families are installed and available.
type FontStatus struct {
	Installed    []FontFamily // Families found on disk
	NotInstalled []FontFamily // Families not found
	BrewAvail    bool         // Whether Homebrew is available
	BestFont     string       // Recommended font from fallback chain
}

// Detect scans the system for installed Monaspace/Monaspice fonts.
func Detect(ctx context.Context) (*FontStatus, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	fontDirs := []string{
		filepath.Join(home, "Library", "Fonts"),
		"/Library/Fonts",
	}

	status := &FontStatus{}

	for _, fam := range AllFamilies() {
		if fontOnDisk(fontDirs, fam) {
			status.Installed = append(status.Installed, fam)
		} else {
			status.NotInstalled = append(status.NotInstalled, fam)
		}
	}

	status.BrewAvail = brewAvailable(ctx)
	status.BestFont = bestAvailable(status.Installed)

	return status, nil
}

// fontOnDisk checks if any file matching the family's pattern exists in the font directories.
func fontOnDisk(dirs []string, fam FontFamily) bool {
	for _, dir := range dirs {
		matches, _ := filepath.Glob(filepath.Join(dir, fam.FilePattern))
		if len(matches) > 0 {
			return true
		}
		// Also check for .ttf variants
		ttfPattern := strings.Replace(fam.FilePattern, ".otf", ".ttf", 1)
		matches, _ = filepath.Glob(filepath.Join(dir, ttfPattern))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// brewAvailable checks if the brew command exists.
func brewAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// bestAvailable returns the best font name from the fallback chain
// based on what's installed.
func bestAvailable(installed []FontFamily) string {
	chain := FallbackChain()
	for _, fontName := range chain {
		for _, fam := range installed {
			if strings.HasPrefix(fontName, fam.PostScriptBase) {
				return fontName
			}
		}
	}
	// System fallback
	return "Menlo"
}
