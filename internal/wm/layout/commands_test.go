package layout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testRegistry creates a Registry backed by a temp directory.
func testRegistry(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	return NewRegistry(dir)
}

// --- Save / Load / Delete / List ---

func TestSaveAndLoad(t *testing.T) {
	r := testRegistry(t)

	snap := &Layout{
		MonitorCount: 2,
		Windows: []WindowPlacement{
			{Workspace: "ws-1", X: 0, Y: 0, Width: 960, Height: 1080, SessionID: "s1"},
			{Workspace: "ws-2", X: 960, Y: 0, Width: 960, Height: 1080, SessionID: "s2"},
		},
	}
	r.CurrentLayout = func() *Layout { return snap }

	var loaded *Layout
	r.OnLoad = func(l *Layout) error {
		loaded = l
		return nil
	}

	if err := r.Run("save my-layout"); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists on disk.
	path := filepath.Join(r.StoreDir(), "my-layout.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("layout file not created: %v", err)
	}

	if err := r.Run("load my-layout"); err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("OnLoad was not called")
	}
	if loaded.Name != "my-layout" {
		t.Errorf("expected name 'my-layout', got %q", loaded.Name)
	}
	if len(loaded.Windows) != 2 {
		t.Errorf("expected 2 windows, got %d", len(loaded.Windows))
	}
	if loaded.MonitorCount != 2 {
		t.Errorf("expected monitor count 2, got %d", loaded.MonitorCount)
	}
}

func TestSave_DuplicateName(t *testing.T) {
	r := testRegistry(t)
	r.CurrentLayout = func() *Layout { return &Layout{} }

	if err := r.Run("save dup"); err != nil {
		t.Fatalf("first save: %v", err)
	}
	err := r.Run("save dup")
	if err == nil {
		t.Fatal("expected error on duplicate save")
	}
}

func TestLoad_NotFound(t *testing.T) {
	r := testRegistry(t)
	err := r.Run("load nonexistent")
	if err == nil {
		t.Fatal("expected error loading missing layout")
	}
}

func TestDelete(t *testing.T) {
	r := testRegistry(t)
	r.CurrentLayout = func() *Layout { return &Layout{} }

	if err := r.Run("save to-delete"); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := r.Run("delete to-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Verify file removed.
	path := filepath.Join(r.StoreDir(), "to-delete.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted, stat err: %v", err)
	}
}

func TestDelete_NotFound(t *testing.T) {
	r := testRegistry(t)
	err := r.Run("delete ghost")
	if err == nil {
		t.Fatal("expected error deleting missing layout")
	}
}

func TestList(t *testing.T) {
	r := testRegistry(t)
	r.CurrentLayout = func() *Layout {
		return &Layout{MonitorCount: 1, Windows: []WindowPlacement{{SessionID: "s1"}}}
	}

	// Empty list.
	results, err := r.ListLayouts()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 layouts, got %d", len(results))
	}

	// Save a few.
	for _, name := range []string{"beta", "alpha", "gamma"} {
		if err := r.Run("save " + name); err != nil {
			t.Fatalf("save %s: %v", name, err)
		}
	}

	results, err = r.ListLayouts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 layouts, got %d", len(results))
	}

	// Verify sorted order.
	if results[0].Name != "alpha" || results[1].Name != "beta" || results[2].Name != "gamma" {
		t.Errorf("expected sorted [alpha beta gamma], got [%s %s %s]",
			results[0].Name, results[1].Name, results[2].Name)
	}

	// Verify metadata.
	for _, lr := range results {
		if lr.MonitorCount != 1 {
			t.Errorf("layout %s: expected monitor count 1, got %d", lr.Name, lr.MonitorCount)
		}
		if lr.WindowCount != 1 {
			t.Errorf("layout %s: expected window count 1, got %d", lr.Name, lr.WindowCount)
		}
	}
}

func TestListCommand(t *testing.T) {
	r := testRegistry(t)
	// "list" command should succeed even with empty directory.
	if err := r.Run("list"); err != nil {
		t.Fatalf("list command: %v", err)
	}
}

// --- Auto-arrange ---

func TestAutoArrange_SingleMonitor(t *testing.T) {
	lay := AutoArrange(1, 4)
	if lay.MonitorCount != 1 {
		t.Errorf("expected monitor count 1, got %d", lay.MonitorCount)
	}
	if len(lay.Windows) != 4 {
		t.Fatalf("expected 4 windows, got %d", len(lay.Windows))
	}

	// All windows on ws-1.
	for _, w := range lay.Windows {
		if w.Workspace != "ws-1" {
			t.Errorf("expected workspace ws-1, got %s", w.Workspace)
		}
		if w.Width <= 0 || w.Height <= 0 {
			t.Errorf("window has non-positive dimensions: %dx%d", w.Width, w.Height)
		}
	}
}

func TestAutoArrange_MultiMonitor(t *testing.T) {
	lay := AutoArrange(3, 6)
	if len(lay.Windows) != 6 {
		t.Fatalf("expected 6 windows, got %d", len(lay.Windows))
	}

	// Should distribute 2 per monitor.
	counts := make(map[string]int)
	for _, w := range lay.Windows {
		counts[w.Workspace]++
	}
	for ws, n := range counts {
		if n != 2 {
			t.Errorf("workspace %s: expected 2 windows, got %d", ws, n)
		}
	}
}

func TestAutoArrange_ZeroWindows(t *testing.T) {
	lay := AutoArrange(2, 0)
	if len(lay.Windows) != 0 {
		t.Errorf("expected 0 windows for 0 window count, got %d", len(lay.Windows))
	}
}

func TestAutoArrange_NoOverlap(t *testing.T) {
	lay := AutoArrange(1, 9)
	if len(lay.Windows) != 9 {
		t.Fatalf("expected 9 windows, got %d", len(lay.Windows))
	}

	// Check that no two windows overlap.
	for i := 0; i < len(lay.Windows); i++ {
		for j := i + 1; j < len(lay.Windows); j++ {
			a := lay.Windows[i]
			b := lay.Windows[j]
			if a.Workspace != b.Workspace {
				continue
			}
			overlapX := a.X < b.X+b.Width && b.X < a.X+a.Width
			overlapY := a.Y < b.Y+b.Height && b.Y < a.Y+a.Height
			if overlapX && overlapY {
				t.Errorf("windows %d and %d overlap: (%d,%d %dx%d) vs (%d,%d %dx%d)",
					i, j, a.X, a.Y, a.Width, a.Height, b.X, b.Y, b.Width, b.Height)
			}
		}
	}
}

func TestAutoCommand(t *testing.T) {
	r := testRegistry(t)
	r.CurrentLayout = func() *Layout {
		return &Layout{Windows: make([]WindowPlacement, 4)}
	}

	var applied *Layout
	r.OnLoad = func(l *Layout) error {
		applied = l
		return nil
	}

	if err := r.Run("auto 2"); err != nil {
		t.Fatalf("auto: %v", err)
	}
	if applied == nil {
		t.Fatal("OnLoad not called by auto")
	}
	if applied.MonitorCount != 2 {
		t.Errorf("expected 2 monitors, got %d", applied.MonitorCount)
	}
}

// --- Reset ---

func TestReset(t *testing.T) {
	r := testRegistry(t)
	var applied *Layout
	r.OnLoad = func(l *Layout) error {
		applied = l
		return nil
	}

	if err := r.Run("reset"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if applied == nil {
		t.Fatal("OnLoad not called by reset")
	}
	if applied.Name != "default" {
		t.Errorf("expected name 'default', got %q", applied.Name)
	}
	if len(applied.Windows) != 1 {
		t.Errorf("expected 1 window in default layout, got %d", len(applied.Windows))
	}
}

// --- JSON serialization ---

func TestLayoutJSONRoundTrip(t *testing.T) {
	original := Layout{
		Name: "test-layout",
		Windows: []WindowPlacement{
			{Workspace: "ws-1", X: 0, Y: 0, Width: 1920, Height: 1080, SessionID: "s1"},
			{Workspace: "ws-2", X: 100, Y: 200, Width: 800, Height: 600, SessionID: "s2"},
		},
		CreatedAt:    time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		MonitorCount: 2,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Layout
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Name != original.Name {
		t.Errorf("name mismatch: %q vs %q", decoded.Name, original.Name)
	}
	if decoded.MonitorCount != original.MonitorCount {
		t.Errorf("monitor count mismatch: %d vs %d", decoded.MonitorCount, original.MonitorCount)
	}
	if len(decoded.Windows) != len(original.Windows) {
		t.Fatalf("window count mismatch: %d vs %d", len(decoded.Windows), len(original.Windows))
	}
	for i, w := range decoded.Windows {
		o := original.Windows[i]
		if w != o {
			t.Errorf("window %d mismatch: %+v vs %+v", i, w, o)
		}
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("created_at mismatch: %v vs %v", decoded.CreatedAt, original.CreatedAt)
	}
}

// --- Command parsing ---

func TestRun_EmptyCommand(t *testing.T) {
	r := testRegistry(t)
	err := r.Run("")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	r := testRegistry(t)
	err := r.Run("bogus foo")
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestRun_MissingArgs(t *testing.T) {
	r := testRegistry(t)

	for _, cmd := range []string{"save", "load", "delete"} {
		if err := r.Run(cmd); err == nil {
			t.Errorf("expected error for %q with no args", cmd)
		}
	}
}

// --- Name validation ---

func TestValidateName(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		{"good-name", false},
		{"under_score", false},
		{"CamelCase", false},
		{"", true},
		{"has/slash", true},
		{"has\\backslash", true},
		{".", true},
		{"..", true},
	}

	for _, tc := range cases {
		err := validateName(tc.name)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateName(%q): err=%v, wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

// --- Registry accessors ---

func TestCommands_AllRegistered(t *testing.T) {
	r := testRegistry(t)
	cmds := r.Commands()

	expected := map[string]bool{
		"save": true, "load": true, "list": true,
		"delete": true, "auto": true, "reset": true,
	}

	if len(cmds) != len(expected) {
		t.Fatalf("expected %d commands, got %d", len(expected), len(cmds))
	}

	for _, c := range cmds {
		if !expected[c.Name] {
			t.Errorf("unexpected command %q", c.Name)
		}
		if c.Description == "" {
			t.Errorf("command %q has empty description", c.Name)
		}
	}
}

func TestCommands_SortedByName(t *testing.T) {
	r := testRegistry(t)
	cmds := r.Commands()
	for i := 1; i < len(cmds); i++ {
		if cmds[i].Name < cmds[i-1].Name {
			t.Errorf("commands not sorted: %q before %q", cmds[i-1].Name, cmds[i].Name)
		}
	}
}

func TestGet_Found(t *testing.T) {
	r := testRegistry(t)
	if cmd := r.Get("save"); cmd == nil {
		t.Error("expected to find 'save' command")
	}
}

func TestGet_NotFound(t *testing.T) {
	r := testRegistry(t)
	if cmd := r.Get("nope"); cmd != nil {
		t.Errorf("expected nil for unknown command, got %q", cmd.Name)
	}
}

// --- parseInt ---

func TestParseInt(t *testing.T) {
	cases := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"0", 0, false},
		{"1", 1, false},
		{"42", 42, false},
		{"-7", -7, false},
		{"+3", 3, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-", 0, true},
		{"+", 0, true},
	}
	for _, tc := range cases {
		got, err := parseInt(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseInt(%q): err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("parseInt(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// --- NewRegistry default store dir ---

func TestNewRegistry_DefaultStoreDir(t *testing.T) {
	r := NewRegistry("")
	if r.StoreDir() == "" {
		t.Error("expected non-empty default store dir")
	}
	// Should end with the expected suffix.
	suffix := filepath.Join(".config", "ralphglasses", "layouts")
	if !containsSuffix(r.StoreDir(), suffix) {
		t.Errorf("store dir %q does not end with %q", r.StoreDir(), suffix)
	}
}

func containsSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// --- Save without CurrentLayout callback ---

func TestSave_NilCurrentLayout(t *testing.T) {
	r := testRegistry(t)
	// CurrentLayout is nil by default; save should still produce a valid file.
	if err := r.Run("save empty-snap"); err != nil {
		t.Fatalf("save with nil CurrentLayout: %v", err)
	}

	// Load it back.
	var loaded *Layout
	r.OnLoad = func(l *Layout) error {
		loaded = l
		return nil
	}
	if err := r.Run("load empty-snap"); err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded == nil {
		t.Fatal("OnLoad not called")
	}
	if loaded.Name != "empty-snap" {
		t.Errorf("expected name 'empty-snap', got %q", loaded.Name)
	}
}
