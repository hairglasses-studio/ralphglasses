package envkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectShell(t *testing.T) {
	info := DetectShell()

	// $SHELL should be set on macOS/Linux
	if info.Shell == "" {
		t.Error("Shell should not be empty")
	}
	if info.Shell == "unknown" {
		t.Log("SHELL env var not set, got 'unknown' — acceptable in CI")
	}

	// Manager should always have a value
	if info.Manager == "" {
		t.Error("Manager should not be empty (expected at least 'none')")
	}
}

func TestDetectShellBash(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	info := DetectShell()
	if info.Shell != "bash" {
		t.Errorf("expected shell 'bash', got %q", info.Shell)
	}
}

func TestDetectShellFish(t *testing.T) {
	t.Setenv("SHELL", "/usr/local/bin/fish")
	info := DetectShell()
	if info.Shell != "fish" {
		t.Errorf("expected shell 'fish', got %q", info.Shell)
	}
}

func TestDetectShellEmpty(t *testing.T) {
	t.Setenv("SHELL", "")
	info := DetectShell()
	if info.Shell != "unknown" {
		t.Errorf("expected shell 'unknown' for empty SHELL, got %q", info.Shell)
	}
}

func TestDetectPluginManagerOhMyZsh(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".oh-my-zsh"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ensure $ZSH is not set so we test dir detection
	t.Setenv("ZSH", "")
	got := detectPluginManager(home)
	if got != "oh-my-zsh" {
		t.Errorf("expected 'oh-my-zsh', got %q", got)
	}
}

func TestDetectPluginManagerZshEnv(t *testing.T) {
	t.Setenv("ZSH", "/some/path")
	home := t.TempDir() // empty dir, no oh-my-zsh dir
	got := detectPluginManager(home)
	if got != "oh-my-zsh" {
		t.Errorf("expected 'oh-my-zsh' via $ZSH env, got %q", got)
	}
}

func TestDetectPluginManagerZinit(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".zinit"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSH", "")
	got := detectPluginManager(home)
	if got != "zinit" {
		t.Errorf("expected 'zinit', got %q", got)
	}
}

func TestDetectPluginManagerZinitLocal(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".local", "share", "zinit"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZSH", "")
	got := detectPluginManager(home)
	if got != "zinit" {
		t.Errorf("expected 'zinit', got %q", got)
	}
}

func TestDetectPluginManagerNone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZSH", "")
	got := detectPluginManager(home)
	if got != "none" {
		t.Errorf("expected 'none', got %q", got)
	}
}

func TestIsDirExists(t *testing.T) {
	dir := t.TempDir()
	if !isDir(dir) {
		t.Error("expected isDir to return true for existing directory")
	}
}

func TestIsDirNotExists(t *testing.T) {
	if isDir("/nonexistent/path/that/should/not/exist") {
		t.Error("expected isDir to return false for nonexistent path")
	}
}

func TestIsDirIsFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "testfile")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if isDir(f.Name()) {
		t.Error("expected isDir to return false for a regular file")
	}
}

func TestShellSummary(t *testing.T) {
	info := ShellInfo{
		Shell:   "zsh",
		Manager: "oh-my-zsh",
		RCFile:  "/home/test/.zshrc",
	}

	summary := info.ShellSummary()
	if summary == "" {
		t.Error("ShellSummary should not be empty")
	}

	// Verify all fields are included
	tests := []string{"zsh", "oh-my-zsh", "/home/test/.zshrc"}
	for _, want := range tests {
		if !contains(summary, want) {
			t.Errorf("summary should contain %q, got: %s", want, summary)
		}
	}
}

func TestShellSummaryNoManager(t *testing.T) {
	info := ShellInfo{
		Shell:   "bash",
		Manager: "none",
		RCFile:  "/home/test/.bashrc",
	}

	summary := info.ShellSummary()
	if contains(summary, "Plugin manager") {
		t.Error("summary should not mention plugin manager when it's 'none'")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
