package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/resource"
)

func setupValidRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755); err != nil {
		t.Fatal(err)
	}
	roadmap := "# Roadmap\n\n- [ ] First task\n- [x] Done task\n"
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte(roadmap), 0o644); err != nil {
		t.Fatal(err)
	}
	stubValidationEnvironment(t)
	return dir
}

func stubValidationEnvironment(t *testing.T) {
	t.Helper()
	oldClaude := validateClaudeLookPath
	oldTmux := validateTmuxLookPath
	oldTmuxList := validateTmuxList
	oldGitStatus := validateGitStatus
	oldResourceStatus := validateResourceStatus
	validateClaudeLookPath = func() error { return errors.New("missing") }
	validateTmuxLookPath = func() error { return errors.New("missing") }
	validateTmuxList = func() ([]byte, error) { return nil, errors.New("inactive") }
	validateGitStatus = func(string) ([]byte, error) { return nil, nil }
	validateResourceStatus = func(string) resource.Status {
		return resource.Status{DiskFreeBytes: 10 * 1024 * 1024 * 1024}
	}
	t.Cleanup(func() {
		validateClaudeLookPath = oldClaude
		validateTmuxLookPath = oldTmux
		validateTmuxList = oldTmuxList
		validateGitStatus = oldGitStatus
		validateResourceStatus = oldResourceStatus
	})
}

func TestValidateConfig_ValidRepo(t *testing.T) {
	dir := setupValidRepo(t)
	result := ValidateConfig(dir)
	if !result.OK() {
		t.Fatalf("expected OK, got errors: %v", result.Errors)
	}
}

func TestValidateConfig_MissingGit(t *testing.T) {
	dir := setupValidRepo(t)
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	result := ValidateConfig(dir)
	if result.OK() {
		t.Fatal("expected error for missing .git")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, ".git") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .git error, got: %v", result.Errors)
	}
}

func TestValidateConfig_MissingRoadmap(t *testing.T) {
	dir := setupValidRepo(t)
	if err := os.Remove(filepath.Join(dir, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	result := ValidateConfig(dir)
	if result.OK() {
		t.Fatal("expected error for missing ROADMAP.md")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "ROADMAP.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ROADMAP.md error, got: %v", result.Errors)
	}
}

func TestValidateConfig_AllItemsChecked(t *testing.T) {
	dir := setupValidRepo(t)
	roadmap := "# Roadmap\n\n- [x] Done task\n- [x] Another done\n"
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte(roadmap), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ValidateConfig(dir)
	if result.OK() {
		t.Fatal("expected error for all-checked ROADMAP")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "unchecked") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unchecked-items error, got: %v", result.Errors)
	}
}

func TestValidateConfig_WarningsDontPreventOK(t *testing.T) {
	dir := setupValidRepo(t)
	result := ValidateConfig(dir)
	if !result.OK() {
		t.Fatalf("expected OK despite warnings, got errors: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected at least one warning")
	}
}

func TestValidateConfig_LowDiskWarning(t *testing.T) {
	dir := setupValidRepo(t)
	validateResourceStatus = func(string) resource.Status {
		return resource.Status{DiskFreeBytes: 4 * 1024 * 1024 * 1024}
	}
	result := ValidateConfig(dir)
	if !result.OK() {
		t.Fatalf("expected OK despite low disk warning, got errors: %v", result.Errors)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "low disk space") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected low disk warning, got: %v", result.Warnings)
	}
}

func TestValidateConfig_TmuxInactiveWarning(t *testing.T) {
	dir := setupValidRepo(t)
	validateTmuxLookPath = func() error { return nil }
	validateTmuxList = func() ([]byte, error) { return nil, errors.New("not running") }
	result := ValidateConfig(dir)
	if !result.OK() {
		t.Fatalf("expected OK despite tmux warning, got errors: %v", result.Errors)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "tmux not active") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected tmux warning, got: %v", result.Warnings)
	}
}

func TestValidateConfig_NonexistentPath(t *testing.T) {
	stubValidationEnvironment(t)
	result := ValidateConfig("/tmp/definitely-does-not-exist-validate-test")
	if result.OK() {
		t.Fatal("expected error for nonexistent path")
	}
}
