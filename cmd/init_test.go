package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "init [directory]" {
			found = true
			break
		}
	}
	if !found {
		t.Error("init command not registered on rootCmd")
	}
}

func TestInitCmd_ShortDescription(t *testing.T) {
	if initCmd.Short == "" {
		t.Error("init command missing Short description")
	}
}

func TestInitCmd_LongDescription(t *testing.T) {
	if initCmd.Long == "" {
		t.Error("init command missing Long description")
	}
}

func TestInitCmd_Example(t *testing.T) {
	if initCmd.Example == "" {
		t.Error("init command missing Example")
	}
}

func TestInitCmd_Flags(t *testing.T) {
	f := initCmd.Flags()

	forceFlag := f.Lookup("force")
	if forceFlag == nil {
		t.Fatal("--force flag not registered")
	}

	minimalFlag := f.Lookup("minimal")
	if minimalFlag == nil {
		t.Fatal("--minimal flag not registered")
	}
}

func TestInit_CreatesRalphRC(t *testing.T) {
	dir := t.TempDir()

	// Reset flags
	initForce = false
	initMinimal = false

	err := runInit(initCmd, []string{dir})
	if err != nil {
		t.Fatalf("runInit: %v", err)
	}

	rcPath := filepath.Join(dir, ".ralphrc")
	data, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("read .ralphrc: %v", err)
	}

	content := string(data)

	// Should contain key configuration entries
	if !strings.Contains(content, "PROJECT_NAME=") {
		t.Error("generated .ralphrc missing PROJECT_NAME")
	}
	if !strings.Contains(content, "PROVIDER=") {
		t.Error("generated .ralphrc missing PROVIDER")
	}
	if !strings.Contains(content, "MAX_CALLS_PER_HOUR=") {
		t.Error("generated .ralphrc missing MAX_CALLS_PER_HOUR")
	}

	// Full template should have comments
	if !strings.Contains(content, "# ") {
		t.Error("generated .ralphrc missing comments")
	}
}

func TestInit_MinimalConfig(t *testing.T) {
	dir := t.TempDir()

	initForce = false
	initMinimal = true
	defer func() { initMinimal = false }()

	err := runInit(initCmd, []string{dir})
	if err != nil {
		t.Fatalf("runInit --minimal: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".ralphrc"))
	if err != nil {
		t.Fatalf("read .ralphrc: %v", err)
	}

	content := string(data)

	// Minimal should have required keys
	if !strings.Contains(content, "PROJECT_NAME=") {
		t.Error("minimal .ralphrc missing PROJECT_NAME")
	}
	if !strings.Contains(content, "PROVIDER=") {
		t.Error("minimal .ralphrc missing PROVIDER")
	}

	// Minimal should NOT have budget section comments
	if strings.Contains(content, "Circuit Breaker") {
		t.Error("minimal .ralphrc should not have Circuit Breaker section")
	}
}

func TestInit_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".ralphrc")

	// Create existing file
	if err := os.WriteFile(rcPath, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	initForce = false
	initMinimal = false

	err := runInit(initCmd, []string{dir})
	if err == nil {
		t.Fatal("expected error when .ralphrc already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists' message", err.Error())
	}

	// Original content should be preserved
	data, _ := os.ReadFile(rcPath)
	if string(data) != "existing" {
		t.Error("existing .ralphrc was modified without --force")
	}
}

func TestInit_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".ralphrc")

	// Create existing file
	if err := os.WriteFile(rcPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	initForce = true
	initMinimal = false
	defer func() { initForce = false }()

	err := runInit(initCmd, []string{dir})
	if err != nil {
		t.Fatalf("runInit --force: %v", err)
	}

	data, _ := os.ReadFile(rcPath)
	if string(data) == "old" {
		t.Error("--force did not overwrite existing .ralphrc")
	}
	if !strings.Contains(string(data), "PROJECT_NAME=") {
		t.Error("overwritten .ralphrc missing expected content")
	}
}

func TestInit_DefaultsToCurrentDir(t *testing.T) {
	// When no args given, should default to "."
	// We can't easily test the actual file creation in cwd,
	// but verify the logic flow
	dir := t.TempDir()

	// Change dir for this test
	old, _ := os.Getwd()
	defer os.Chdir(old) //nolint:errcheck
	os.Chdir(dir)       //nolint:errcheck

	initForce = false
	initMinimal = false

	err := runInit(initCmd, nil)
	if err != nil {
		t.Fatalf("runInit with no args: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".ralphrc")); err != nil {
		t.Error(".ralphrc not created in current directory")
	}
}

func TestMinimalRalphRC(t *testing.T) {
	content := minimalRalphRC()
	if !strings.Contains(content, "PROJECT_NAME=") {
		t.Error("minimal template missing PROJECT_NAME")
	}
	lines := strings.Split(strings.TrimSpace(content), "\n")
	// Should be concise
	if len(lines) > 10 {
		t.Errorf("minimal template has %d lines, expected <= 10", len(lines))
	}
}

func TestFullRalphRC(t *testing.T) {
	content := fullRalphRC()
	requiredKeys := []string{
		"PROJECT_NAME=",
		"PROVIDER=",
		"MAX_CALLS_PER_HOUR=",
	}
	for _, key := range requiredKeys {
		if !strings.Contains(content, key) {
			t.Errorf("full template missing %q", key)
		}
	}
	// Should have documentation comments
	if strings.Count(content, "#") < 10 {
		t.Error("full template should have many comment lines")
	}
}
