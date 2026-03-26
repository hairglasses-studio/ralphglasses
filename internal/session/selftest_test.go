package session

import (
	"os"
	"testing"
)

func TestRecursionGuard_NotSet(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv("RALPH_SELF_TEST")
	if err := RecursionGuard(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRecursionGuard_Set(t *testing.T) {
	os.Setenv("RALPH_SELF_TEST", "1")
	defer os.Unsetenv("RALPH_SELF_TEST")

	if err := RecursionGuard(); err == nil {
		t.Error("expected error when RALPH_SELF_TEST=1")
	}
}

func TestSetSelfTestEnv(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/tmp"}
	result := SetSelfTestEnv(env)
	if len(result) != 3 {
		t.Fatalf("expected 3 env vars, got %d", len(result))
	}
	if result[2] != "RALPH_SELF_TEST=1" {
		t.Errorf("last env = %q, want RALPH_SELF_TEST=1", result[2])
	}
}

func TestIsSelfTestTarget(t *testing.T) {
	// Non-existent path
	if IsSelfTestTarget("/nonexistent/path") {
		t.Error("expected false for nonexistent path")
	}

	// Path without go.mod
	dir := t.TempDir()
	if IsSelfTestTarget(dir) {
		t.Error("expected false for dir without go.mod")
	}

	// Path with wrong module
	os.WriteFile(dir+"/go.mod", []byte("module example.com/foo"), 0644)
	if IsSelfTestTarget(dir) {
		t.Error("expected false for wrong module")
	}
}
