package envkit

import (
	"os"
	"path/filepath"
	"strings"
)

// ShellInfo describes the detected shell environment.
type ShellInfo struct {
	Shell   string `json:"shell"`   // "zsh", "bash", "fish"
	Manager string `json:"manager"` // "oh-my-zsh", "zinit", "none"
	RCFile  string `json:"rc_file"` // path to rc file
}

// DetectShell detects the current shell and plugin manager.
func DetectShell() ShellInfo {
	info := ShellInfo{
		Shell:   "unknown",
		Manager: "none",
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		return info
	}

	// Extract shell name from path (e.g., /bin/zsh → zsh)
	info.Shell = filepath.Base(shell)

	home, err := os.UserHomeDir()
	if err != nil {
		return info
	}

	// Set RC file based on shell
	switch info.Shell {
	case "zsh":
		info.RCFile = filepath.Join(home, ".zshrc")
	case "bash":
		info.RCFile = filepath.Join(home, ".bashrc")
	case "fish":
		info.RCFile = filepath.Join(home, ".config", "fish", "config.fish")
	}

	// Detect plugin manager
	info.Manager = detectPluginManager(home)

	return info
}

// detectPluginManager checks for known shell plugin managers.
func detectPluginManager(home string) string {
	// oh-my-zsh: $ZSH env var or ~/.oh-my-zsh/
	if os.Getenv("ZSH") != "" {
		return "oh-my-zsh"
	}
	if isDir(filepath.Join(home, ".oh-my-zsh")) {
		return "oh-my-zsh"
	}

	// zinit: ~/.zinit/ or ~/.local/share/zinit/
	if isDir(filepath.Join(home, ".zinit")) {
		return "zinit"
	}
	if isDir(filepath.Join(home, ".local", "share", "zinit")) {
		return "zinit"
	}

	return "none"
}

// isDir reports whether the path exists and is a directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// ShellSummary returns a human-readable summary of the shell info.
func (s ShellInfo) ShellSummary() string {
	var parts []string
	parts = append(parts, "Shell: "+s.Shell)
	if s.Manager != "none" {
		parts = append(parts, "Plugin manager: "+s.Manager)
	}
	if s.RCFile != "" {
		parts = append(parts, "RC file: "+s.RCFile)
	}
	return strings.Join(parts, "\n")
}
