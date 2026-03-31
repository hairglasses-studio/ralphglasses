package envkit

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DotfileSnapshot captures the contents of claudekit-managed config files.
type DotfileSnapshot struct {
	Files map[string]string `json:"files"` // relative path → content
}

// managedPaths returns the relative paths of config files managed by claudekit.
// The list is platform-aware: iTerm2 paths on macOS, Sway/Waybar paths on Linux.
func managedPaths() []string {
	paths := []string{
		".config/starship.toml",
		".config/ghostty/config",
		".config/bat/config",
		".config/delta/catppuccin.gitconfig",
	}
	switch runtime.GOOS {
	case "darwin":
		// iTerm2 dynamic profiles are macOS-only
		// (individual profile files are added via directory scan in Snapshot)
	case "linux":
		paths = append(paths,
			".config/sway/config",
			".config/waybar/config",
			".config/waybar/style.css",
		)
	}
	return paths
}

// itermDynamicProfilesDir returns the relative path to iTerm2 DynamicProfiles.
const itermDynamicProfilesRel = "Library/Application Support/iTerm2/DynamicProfiles"

// Snapshot captures current config files managed by claudekit.
// It reads each managed path relative to the user's home directory.
func Snapshot() (*DotfileSnapshot, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	snap := &DotfileSnapshot{
		Files: make(map[string]string),
	}

	// Capture standard managed paths
	for _, rel := range managedPaths() {
		abs := filepath.Join(home, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			continue // skip missing files
		}
		snap.Files[rel] = string(data)
	}

	// Capture iTerm2 DynamicProfiles claudekit files (macOS only)
	if runtime.GOOS == "darwin" {
		dpDir := filepath.Join(home, itermDynamicProfilesRel)
		entries, err := os.ReadDir(dpDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if !strings.HasPrefix(e.Name(), "claudekit") {
					continue
				}
				abs := filepath.Join(dpDir, e.Name())
				data, err := os.ReadFile(abs)
				if err != nil {
					continue
				}
				rel := filepath.Join(itermDynamicProfilesRel, e.Name())
				snap.Files[rel] = string(data)
			}
		}
	}

	return snap, nil
}

// SnapshotDir captures config files relative to a given base directory
// instead of the user's home directory. Useful for testing.
func SnapshotDir(baseDir string) (*DotfileSnapshot, error) {
	snap := &DotfileSnapshot{
		Files: make(map[string]string),
	}

	for _, rel := range managedPaths() {
		abs := filepath.Join(baseDir, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		snap.Files[rel] = string(data)
	}

	return snap, nil
}

// Restore restores a previously captured snapshot, writing files relative
// to the user's home directory.
func Restore(snap *DotfileSnapshot) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	return RestoreDir(snap, home)
}

// RestoreDir restores a snapshot relative to a given base directory.
// Useful for testing.
func RestoreDir(snap *DotfileSnapshot, baseDir string) error {
	for rel, content := range snap.Files {
		abs := filepath.Join(baseDir, rel)
		dir := filepath.Dir(abs)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", rel, err)
		}
	}
	return nil
}

// SnapshotSummary returns a human-readable summary of a snapshot.
func SnapshotSummary(snap *DotfileSnapshot) string {
	if len(snap.Files) == 0 {
		return "No managed config files found."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Captured %d config file(s):\n", len(snap.Files)))
	for rel := range snap.Files {
		b.WriteString(fmt.Sprintf("  %s\n", rel))
	}
	return b.String()
}
