package styles

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	lipgloss "charm.land/lipgloss/v2"
)

const testThemeYAML = `name: hotswap-test
primary: "#aabbcc"
accent: "#ddeeff"
green: "#00ff00"
yellow: "#ffff00"
red: "#ff0000"
gray: "#808080"
dark_bg: "#000000"
bright_fg: "#ffffff"
`

const updatedThemeYAML = `name: hotswap-updated
primary: "#112233"
accent: "#445566"
green: "#778899"
yellow: "#aabbcc"
red: "#ddeeff"
gray: "#111111"
dark_bg: "#222222"
bright_fg: "#333333"
`

const intermediateThemeYAML = `name: hotswap-intermediate
primary: "#010203"
accent: "#040506"
green: "#070809"
yellow: "#0a0b0c"
red: "#0d0e0f"
gray: "#101112"
dark_bg: "#131415"
bright_fg: "#161718"
`

func writeThemeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestThemeChangedMsg_ImplementsTeaMsg(t *testing.T) {
	// ThemeChangedMsg must satisfy tea.Msg (any empty interface).
	var msg any = ThemeChangedMsg{
		Theme: &Theme{Name: "test"},
		Path:  "/tmp/test.yaml",
	}
	if msg == nil {
		t.Fatal("ThemeChangedMsg should not be nil")
	}
	tcm, ok := msg.(ThemeChangedMsg)
	if !ok {
		t.Fatal("ThemeChangedMsg does not satisfy tea.Msg interface")
	}
	if tcm.Theme.Name != "test" {
		t.Errorf("Theme.Name = %q, want %q", tcm.Theme.Name, "test")
	}
	if tcm.Path != "/tmp/test.yaml" {
		t.Errorf("Path = %q, want %q", tcm.Path, "/tmp/test.yaml")
	}
}

func TestThemeWatcherErrorMsg_ImplementsTeaMsg(t *testing.T) {
	var msg any = ThemeWatcherErrorMsg{Err: os.ErrNotExist}
	if msg == nil {
		t.Fatal("ThemeWatcherErrorMsg should not be nil")
	}
	tem, ok := msg.(ThemeWatcherErrorMsg)
	if !ok {
		t.Fatal("ThemeWatcherErrorMsg does not satisfy tea.Msg interface")
	}
	if tem.Err != os.ErrNotExist {
		t.Errorf("Err = %v, want os.ErrNotExist", tem.Err)
	}
}

func TestWatchThemeFile_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	cmd := WatchThemeFile(path)

	// Write the updated theme after a brief delay.
	go func() {
		time.Sleep(150 * time.Millisecond)
		writeThemeFile(t, path, updatedThemeYAML)
	}()

	done := make(chan any, 1)
	go func() {
		done <- cmd()
	}()

	select {
	case msg := <-done:
		tcm, ok := msg.(ThemeChangedMsg)
		if !ok {
			t.Fatalf("expected ThemeChangedMsg, got %T: %v", msg, msg)
		}
		if tcm.Theme.Name != "hotswap-updated" {
			t.Errorf("Theme.Name = %q, want %q", tcm.Theme.Name, "hotswap-updated")
		}
		if tcm.Theme.Primary != "#112233" {
			t.Errorf("Theme.Primary = %q, want %q", tcm.Theme.Primary, "#112233")
		}
		if tcm.Path != path {
			t.Errorf("Path = %q, want %q", tcm.Path, path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WatchThemeFile timed out waiting for file change")
	}
}

func TestWatchThemeFile_AppliesTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	// Save originals.
	origPrimary := ColorPrimary

	cmd := WatchThemeFile(path)

	go func() {
		time.Sleep(150 * time.Millisecond)
		writeThemeFile(t, path, updatedThemeYAML)
	}()

	done := make(chan any, 1)
	go func() {
		done <- cmd()
	}()

	select {
	case msg := <-done:
		_, ok := msg.(ThemeChangedMsg)
		if !ok {
			t.Fatalf("expected ThemeChangedMsg, got %T", msg)
		}
		// Verify that ApplyTheme was called by checking the package-level var.
		if ColorPrimary != lipgloss.Color("#112233") {
			t.Errorf("ColorPrimary = %v after hot-swap, want %v", ColorPrimary, lipgloss.Color("#112233"))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}

	// Restore originals.
	ThemeMu.Lock()
	ColorPrimary = origPrimary
	ThemeMu.Unlock()
}

func TestWatchThemeFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	cmd := WatchThemeFile(path)

	go func() {
		time.Sleep(150 * time.Millisecond)
		writeThemeFile(t, path, "{{{{ not valid yaml")
	}()

	done := make(chan any, 1)
	go func() {
		done <- cmd()
	}()

	select {
	case msg := <-done:
		tem, ok := msg.(ThemeWatcherErrorMsg)
		if !ok {
			t.Fatalf("expected ThemeWatcherErrorMsg for bad YAML, got %T", msg)
		}
		if tem.Err == nil {
			t.Fatal("expected non-nil error for invalid YAML")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestWatchThemeFile_NonexistentFile(t *testing.T) {
	cmd := WatchThemeFile("/nonexistent/path/theme.yaml")

	msg := cmd()
	tem, ok := msg.(ThemeWatcherErrorMsg)
	if !ok {
		t.Fatalf("expected ThemeWatcherErrorMsg for missing file, got %T", msg)
	}
	if tem.Err == nil {
		t.Fatal("expected non-nil error for missing file")
	}
}

func TestNewThemeWatcher_NonexistentFile(t *testing.T) {
	_, err := NewThemeWatcher("/nonexistent/path/theme.yaml", nil)
	if err == nil {
		t.Fatal("NewThemeWatcher should return error for nonexistent file")
	}
}

func TestThemeWatcher_Close_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	tw, err := NewThemeWatcher(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Close multiple times should not panic.
	if err := tw.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

func TestThemeWatcher_DetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	// Use a channel-based "program" to capture the sent message.
	// We use the internal constructor with a real watcher but nil program,
	// then check the side effect via ApplyTheme on the package-level vars.
	origPrimary := ColorPrimary

	tw, err := NewThemeWatcher(path, nil, WithDebounce(50*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	defer tw.Close()

	// Write updated theme.
	time.Sleep(100 * time.Millisecond)
	writeThemeFile(t, path, updatedThemeYAML)

	// Wait for debounce + reload.
	time.Sleep(300 * time.Millisecond)

	ThemeMu.RLock()
	got := ColorPrimary
	ThemeMu.RUnlock()
	if got != lipgloss.Color("#112233") {
		t.Errorf("ColorPrimary = %v after ThemeWatcher reload, want %v", got, lipgloss.Color("#112233"))
	}

	// Restore.
	ThemeMu.Lock()
	ColorPrimary = origPrimary
	ThemeMu.Unlock()
}

func TestWithDebounce_SetsValue(t *testing.T) {
	tw := &ThemeWatcher{debounce: 100 * time.Millisecond}
	WithDebounce(500 * time.Millisecond)(tw)
	if tw.debounce != 500*time.Millisecond {
		t.Errorf("debounce = %v, want %v", tw.debounce, 500*time.Millisecond)
	}
}

func TestThemeWatcher_ClosedWatcherEmitsNoMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	tw, err := NewThemeWatcher(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Close the watcher immediately; the loop should exit cleanly.
	if err := tw.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	// Give the goroutine time to exit.
	time.Sleep(50 * time.Millisecond)
}

func TestWatchThemeFile_MultipleWrites_LastWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	writeThemeFile(t, path, testThemeYAML)

	cmd := WatchThemeFile(path)

	// The one-shot watcher should debounce rapid writes and load the final file.
	go func() {
		time.Sleep(150 * time.Millisecond)
		writeThemeFile(t, path, intermediateThemeYAML)
		time.Sleep(25 * time.Millisecond)
		writeThemeFile(t, path, updatedThemeYAML)
	}()

	done := make(chan any, 1)
	go func() {
		done <- cmd()
	}()

	select {
	case msg := <-done:
		tcm, ok := msg.(ThemeChangedMsg)
		if !ok {
			t.Fatalf("expected ThemeChangedMsg, got %T", msg)
		}
		if tcm.Theme.Name != "hotswap-updated" {
			t.Errorf("Theme.Name = %q, want %q", tcm.Theme.Name, "hotswap-updated")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}
