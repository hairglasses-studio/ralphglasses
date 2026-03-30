package tmux

import (
	"os"
	"path/filepath"
	"strings"
)

// WSLMountPrefix is the default mount point for Windows drives under WSL.
const WSLMountPrefix = "/mnt/"

// IsWSLPath returns true if the path looks like a WSL-mounted Windows path.
func IsWSLPath(path string) bool {
	return strings.HasPrefix(path, WSLMountPrefix)
}

// ToWindowsPath converts a WSL path like /mnt/c/Users/foo to C:\Users\foo.
func ToWindowsPath(wslPath string) string {
	if !IsWSLPath(wslPath) {
		return wslPath
	}
	// Strip /mnt/ prefix
	rest := wslPath[len(WSLMountPrefix):]
	if len(rest) == 0 {
		return wslPath
	}
	// First char is the drive letter
	drive := strings.ToUpper(rest[:1])
	remainder := rest[1:]
	// Convert forward slashes to backslashes
	remainder = strings.ReplaceAll(remainder, "/", "\\")
	return drive + ":" + remainder
}

// ToWSLPath converts a Windows path like C:\Users\foo to /mnt/c/Users/foo.
func ToWSLPath(winPath string) string {
	if len(winPath) < 2 || winPath[1] != ':' {
		return winPath
	}
	drive := strings.ToLower(winPath[:1])
	remainder := winPath[2:]
	remainder = strings.ReplaceAll(remainder, "\\", "/")
	return filepath.Join(WSLMountPrefix, drive, remainder)
}

// WSLTmuxSocketPath returns the recommended tmux socket path for WSL.
// WSL's default /tmp can have permission issues; using a path under the
// user's home directory avoids those problems.
func WSLTmuxSocketPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".tmux-socket")
}
