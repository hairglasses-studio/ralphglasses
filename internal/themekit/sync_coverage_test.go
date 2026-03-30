package themekit

import (
	"errors"
	"sync"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// makeTestThemeSync creates a ThemeSync without loading a file, for unit tests.
func makeTestThemeSync() *ThemeSync {
	return &ThemeSync{
		themePath: "/fake/path/theme.yaml",
		appliers:  make(map[string]ThemeApplier),
	}
}

type fakeApplier struct {
	mu     sync.Mutex
	called int
	theme  styles.Theme
	err    error
}

func (f *fakeApplier) ApplyTheme(t styles.Theme) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.called++
	f.theme = t
	return f.err
}

func TestThemeSync_Register_Empty(t *testing.T) {
	ts := makeTestThemeSync()
	err := ts.Register("", &fakeApplier{})
	if err == nil {
		t.Error("Register with empty name should fail")
	}
}

func TestThemeSync_Register_NilApplier(t *testing.T) {
	ts := makeTestThemeSync()
	err := ts.Register("comp1", nil)
	if err == nil {
		t.Error("Register with nil applier should fail")
	}
}

func TestThemeSync_Register_Duplicate(t *testing.T) {
	ts := makeTestThemeSync()
	if err := ts.Register("comp1", &fakeApplier{}); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	err := ts.Register("comp1", &fakeApplier{})
	if err == nil {
		t.Error("duplicate Register should fail")
	}
}

func TestThemeSync_Register_Success(t *testing.T) {
	ts := makeTestThemeSync()
	err := ts.Register("comp1", &fakeApplier{})
	if err != nil {
		t.Errorf("Register() error = %v", err)
	}
}

func TestThemeSync_Unregister(t *testing.T) {
	ts := makeTestThemeSync()
	ts.Register("comp1", &fakeApplier{})
	ts.Unregister("comp1")

	// Re-registering after unregister should succeed.
	if err := ts.Register("comp1", &fakeApplier{}); err != nil {
		t.Errorf("re-Register after Unregister: %v", err)
	}
}

func TestThemeSync_Unregister_NonExistent(t *testing.T) {
	ts := makeTestThemeSync()
	// Should not panic.
	ts.Unregister("nonexistent")
}

func TestThemeSync_Current(t *testing.T) {
	ts := makeTestThemeSync()
	theme := styles.Theme{Name: "test-theme"}
	ts.current = theme

	got := ts.Current()
	if got.Name != "test-theme" {
		t.Errorf("Current().Name = %q, want test-theme", got.Name)
	}
}

func TestThemeSync_Apply_DistributesToAppliers(t *testing.T) {
	ts := makeTestThemeSync()
	a1 := &fakeApplier{}
	a2 := &fakeApplier{}
	ts.Register("c1", a1)
	ts.Register("c2", a2)

	theme := styles.Theme{Name: "applied"}
	err := ts.Apply(theme)
	if err != nil {
		t.Errorf("Apply() unexpected error: %v", err)
	}

	a1.mu.Lock()
	c1 := a1.called
	a1.mu.Unlock()
	a2.mu.Lock()
	c2 := a2.called
	a2.mu.Unlock()

	if c1 != 1 {
		t.Errorf("a1.called = %d, want 1", c1)
	}
	if c2 != 1 {
		t.Errorf("a2.called = %d, want 1", c2)
	}

	got := ts.Current()
	if got.Name != "applied" {
		t.Errorf("Current().Name = %q, want applied", got.Name)
	}
}

func TestThemeSync_Apply_ReturnsFirstError(t *testing.T) {
	ts := makeTestThemeSync()
	bad := &fakeApplier{err: errors.New("apply failed")}
	ts.Register("bad", bad)

	err := ts.Apply(styles.Theme{})
	if err == nil {
		t.Error("Apply() should return error from failing applier")
	}
}

func TestThemeSync_Export_JSON(t *testing.T) {
	ts := makeTestThemeSync()
	ts.current = styles.Theme{Name: "my-theme"}

	data, err := ts.Export("json")
	if err != nil {
		t.Fatalf("Export(json) error: %v", err)
	}
	if len(data) == 0 {
		t.Error("Export(json) returned empty data")
	}
	// Should be valid JSON containing the theme name.
	if !contains(string(data), "my-theme") {
		t.Errorf("exported JSON doesn't contain theme name: %s", data)
	}
}

func TestThemeSync_Export_YAML(t *testing.T) {
	ts := makeTestThemeSync()
	ts.current = styles.Theme{Name: "yaml-theme"}

	data, err := ts.Export("yaml")
	if err != nil {
		t.Fatalf("Export(yaml) error: %v", err)
	}
	if len(data) == 0 {
		t.Error("Export(yaml) returned empty data")
	}
}

func TestThemeSync_Export_YML(t *testing.T) {
	ts := makeTestThemeSync()
	_, err := ts.Export("yml")
	if err != nil {
		t.Fatalf("Export(yml) error: %v", err)
	}
}

func TestThemeSync_Export_UnknownFormat(t *testing.T) {
	ts := makeTestThemeSync()
	_, err := ts.Export("toml")
	if err == nil {
		t.Error("Export(unsupported format) should return error")
	}
}

func TestThemeSync_Import_JSON(t *testing.T) {
	ts := makeTestThemeSync()
	jsonData := []byte(`{"name":"imported-theme"}`)

	err := ts.Import(jsonData, "json")
	if err != nil {
		t.Fatalf("Import(json) error: %v", err)
	}
	if ts.Current().Name != "imported-theme" {
		t.Errorf("Current().Name = %q, want imported-theme", ts.Current().Name)
	}
}

func TestThemeSync_Import_YAML(t *testing.T) {
	ts := makeTestThemeSync()
	yamlData := []byte("name: yaml-imported\n")

	err := ts.Import(yamlData, "yaml")
	if err != nil {
		t.Fatalf("Import(yaml) error: %v", err)
	}
	if ts.Current().Name != "yaml-imported" {
		t.Errorf("Current().Name = %q, want yaml-imported", ts.Current().Name)
	}
}

func TestThemeSync_Import_InvalidJSON(t *testing.T) {
	ts := makeTestThemeSync()
	err := ts.Import([]byte("not json"), "json")
	if err == nil {
		t.Error("Import(invalid json) should return error")
	}
}

func TestThemeSync_Import_UnknownFormat(t *testing.T) {
	ts := makeTestThemeSync()
	err := ts.Import([]byte("data"), "xml")
	if err == nil {
		t.Error("Import(unsupported format) should return error")
	}
}

// contains is a helper (already in palette_test.go or similar; if not, define locally).
func contains(s, sub string) bool {
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
