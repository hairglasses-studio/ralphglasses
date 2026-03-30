package tmux

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// PaneInfo struct
// ---------------------------------------------------------------------------

func TestPaneInfo_Fields(t *testing.T) {
	p := PaneInfo{
		SessionName: "ralph",
		WindowIndex: 3,
		PaneID:      "%5",
	}
	if p.SessionName != "ralph" {
		t.Errorf("SessionName = %q, want %q", p.SessionName, "ralph")
	}
	if p.WindowIndex != 3 {
		t.Errorf("WindowIndex = %d, want 3", p.WindowIndex)
	}
	if p.PaneID != "%5" {
		t.Errorf("PaneID = %q, want %%5", p.PaneID)
	}
}

// ---------------------------------------------------------------------------
// CreateSessionPane — tmux not available
// ---------------------------------------------------------------------------

func TestCreateSessionPane_NoTmux(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	_, err := CreateSessionPane("ralph", "abc12345def")
	if err == nil {
		t.Fatal("expected error when tmux is not available")
	}
	if got := err.Error(); got != "tmux not available" {
		t.Errorf("error = %q, want %q", got, "tmux not available")
	}
}

// ---------------------------------------------------------------------------
// CreateSessionPane — short ID truncation
// ---------------------------------------------------------------------------

func TestCreateSessionPane_ShortID(t *testing.T) {
	// Create a fake tmux that records its arguments to a file.
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	script := `#!/bin/sh
echo "$@" >> ` + argsFile + `
case "$1" in
  has-session) exit 0 ;;
  new-window) echo "%42" ;;
  *) exit 0 ;;
esac
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	tests := []struct {
		name      string
		sessionID string
		wantShort string
	}{
		{"long ID truncated to 8", "abcdefghijklmnop", "abcdefgh"},
		{"exactly 8 chars", "12345678", "12345678"},
		{"short ID unchanged", "abc", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear args file.
			os.Remove(argsFile)

			pane, err := CreateSessionPane("ralph", tt.sessionID)
			if err != nil {
				t.Fatalf("CreateSessionPane error: %v", err)
			}
			if pane == nil {
				t.Fatal("returned nil PaneInfo")
			}
			if pane.SessionName != "ralph" {
				t.Errorf("SessionName = %q, want %q", pane.SessionName, "ralph")
			}
			if pane.PaneID != "%42" {
				t.Errorf("PaneID = %q, want %%42", pane.PaneID)
			}

			// Verify the args file contains the expected window name.
			argsData, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatalf("reading args file: %v", err)
			}
			wantWindowName := "ralph-" + tt.wantShort
			if !containsString(string(argsData), wantWindowName) {
				t.Errorf("tmux args %q don't contain window name %q", string(argsData), wantWindowName)
			}
		})
	}
}

// containsString is a simple substring check helper.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstring(s, substr))
}

func findSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// CreateSessionPane — EnsureSession failure
// ---------------------------------------------------------------------------

func TestCreateSessionPane_EnsureSessionFails(t *testing.T) {
	// Fake tmux where has-session and new-session both fail.
	dir := t.TempDir()
	script := `#!/bin/sh
exit 1
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	_, err := CreateSessionPane("ralph", "session-1")
	if err == nil {
		t.Fatal("expected error when EnsureSession fails")
	}
}

// ---------------------------------------------------------------------------
// CreateSessionPane — new-window failure
// ---------------------------------------------------------------------------

func TestCreateSessionPane_NewWindowFails(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  has-session) exit 0 ;;
  new-window) exit 1 ;;
  *) exit 0 ;;
esac
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	_, err := CreateSessionPane("ralph", "session-1")
	if err == nil {
		t.Fatal("expected error when new-window fails")
	}
}

// ---------------------------------------------------------------------------
// CloseSessionPane — command construction
// ---------------------------------------------------------------------------

func TestCloseSessionPane_NoTmux(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	err := CloseSessionPane("%5")
	if err == nil {
		t.Error("expected error when tmux is not available")
	}
}

func TestCloseSessionPane_Success(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
exit 0
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	if err := CloseSessionPane("%5"); err != nil {
		t.Errorf("CloseSessionPane error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SendToPane
// ---------------------------------------------------------------------------

func TestSendToPane_NoTmux(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	err := SendToPane("%0", "echo hello")
	if err == nil {
		t.Error("expected error when tmux is not available")
	}
}

func TestSendToPane_Success(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
exit 0
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	if err := SendToPane("%0", "echo hello"); err != nil {
		t.Errorf("SendToPane error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListPanes — parsing with fake tmux output
// ---------------------------------------------------------------------------

func TestListPanes_ParseOutput(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
printf "0\t%%1\n1\t%%2\n1\t%%3\n"
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	panes, err := ListPanes("ralph")
	if err != nil {
		t.Fatalf("ListPanes error: %v", err)
	}
	if len(panes) != 3 {
		t.Fatalf("got %d panes, want 3", len(panes))
	}

	tests := []struct {
		idx         int
		sessionName string
		windowIndex int
		paneID      string
	}{
		{0, "ralph", 0, "%1"},
		{1, "ralph", 1, "%2"},
		{2, "ralph", 1, "%3"},
	}
	for _, tt := range tests {
		p := panes[tt.idx]
		if p.SessionName != tt.sessionName {
			t.Errorf("pane[%d].SessionName = %q, want %q", tt.idx, p.SessionName, tt.sessionName)
		}
		if p.WindowIndex != tt.windowIndex {
			t.Errorf("pane[%d].WindowIndex = %d, want %d", tt.idx, p.WindowIndex, tt.windowIndex)
		}
		if p.PaneID != tt.paneID {
			t.Errorf("pane[%d].PaneID = %q, want %q", tt.idx, p.PaneID, tt.paneID)
		}
	}
}

// ---------------------------------------------------------------------------
// ListPanes — empty output
// ---------------------------------------------------------------------------

func TestListPanes_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	script := `#!/bin/sh
echo ""
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	panes, err := ListPanes("ralph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(panes) != 0 {
		t.Errorf("expected 0 panes from empty output, got %d", len(panes))
	}
}

// ---------------------------------------------------------------------------
// ListPanes — malformed lines are skipped
// ---------------------------------------------------------------------------

func TestListPanes_MalformedLines(t *testing.T) {
	dir := t.TempDir()
	// First line has only 1 field (skipped), second is valid.
	script := `#!/bin/sh
printf "incomplete\n2\t%%10\n"
`
	fakeTmux := filepath.Join(dir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir)
	defer os.Setenv("PATH", origPath)

	panes, err := ListPanes("ralph")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("expected 1 valid pane, got %d", len(panes))
	}
	if panes[0].WindowIndex != 2 {
		t.Errorf("WindowIndex = %d, want 2", panes[0].WindowIndex)
	}
	if panes[0].PaneID != "%10" {
		t.Errorf("PaneID = %q, want %%10", panes[0].PaneID)
	}
}

// ---------------------------------------------------------------------------
// ListPanes — error from tmux
// ---------------------------------------------------------------------------

func TestListPanes_Error(t *testing.T) {
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	defer os.Setenv("PATH", origPath)

	_, err := ListPanes("nonexistent")
	if err == nil {
		t.Error("expected error when tmux is not available")
	}
}
