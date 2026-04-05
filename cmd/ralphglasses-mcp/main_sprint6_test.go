package main

import (
	"os"
	"testing"
)

func TestResolveScanPath_Sprint6_ExpandsTildeFromEnv(t *testing.T) {
	old := os.Getenv("RALPHGLASSES_SCAN_PATH")
	defer func() {
		if old != "" {
			os.Setenv("RALPHGLASSES_SCAN_PATH", old)
		} else {
			os.Unsetenv("RALPHGLASSES_SCAN_PATH")
		}
	}()

	os.Setenv("RALPHGLASSES_SCAN_PATH", "~/test-path")
	sp := resolveScanPath()
	if len(sp) > 0 && sp[0] == '~' {
		t.Errorf("resolveScanPath should expand ~, got %q", sp)
	}
}

func TestResolveScanPath_Sprint6_AbsolutePath(t *testing.T) {
	old := os.Getenv("RALPHGLASSES_SCAN_PATH")
	defer func() {
		if old != "" {
			os.Setenv("RALPHGLASSES_SCAN_PATH", old)
		} else {
			os.Unsetenv("RALPHGLASSES_SCAN_PATH")
		}
	}()

	os.Setenv("RALPHGLASSES_SCAN_PATH", "/custom/absolute/path")
	sp := resolveScanPath()
	if sp != "/custom/absolute/path" {
		t.Errorf("resolveScanPath = %q, want %q", sp, "/custom/absolute/path")
	}
}

func TestSetup_Sprint6_ReturnsValidServer(t *testing.T) {
	dir := t.TempDir()
	srv, cleanup, err := setup(dir)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer cleanup()

	if srv == nil {
		t.Error("setup should return non-nil server")
	}
}

func TestSetup_Sprint6_CleanupSafe(t *testing.T) {
	dir := t.TempDir()
	_, cleanup, err := setup(dir)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	// Multiple calls should be safe
	cleanup()
	cleanup()
}

func TestSetup_Sprint6_RepeatedSetupTeardown(t *testing.T) {
	dir := t.TempDir()
	for i := range 3 {
		srv, cleanup, err := setup(dir)
		if err != nil {
			t.Fatalf("setup call %d failed: %v", i, err)
		}
		if srv == nil {
			t.Errorf("setup call %d returned nil server", i)
		}
		cleanup()
	}
}

func TestResolveScanPath_Sprint6_EmptyEnvUsesDefault(t *testing.T) {
	old := os.Getenv("RALPHGLASSES_SCAN_PATH")
	defer func() {
		if old != "" {
			os.Setenv("RALPHGLASSES_SCAN_PATH", old)
		} else {
			os.Unsetenv("RALPHGLASSES_SCAN_PATH")
		}
	}()

	os.Unsetenv("RALPHGLASSES_SCAN_PATH")
	sp := resolveScanPath()
	if sp == "" {
		t.Error("resolveScanPath should return non-empty default when env is unset")
	}
	// Default should be expanded (no ~)
	if len(sp) > 0 && sp[0] == '~' {
		t.Errorf("default path should be expanded, got %q", sp)
	}
}
