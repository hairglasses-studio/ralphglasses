package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.yml")

	original := &AliasFile{
		Version: 1,
		Aliases: map[string]*AliasDef{
			"gs": {Command: "git status", Description: "Show git status"},
			"gl": {Command: "git log --oneline", Description: "Short git log", Args: []string{"-n", "10"}},
		},
	}

	if err := SaveAliasFile(path, original); err != nil {
		t.Fatalf("SaveAliasFile: %v", err)
	}

	loaded, err := LoadAliasFile(path)
	if err != nil {
		t.Fatalf("LoadAliasFile: %v", err)
	}

	if loaded.Version != 1 {
		t.Errorf("version = %d, want 1", loaded.Version)
	}
	if len(loaded.Aliases) != 2 {
		t.Errorf("len(aliases) = %d, want 2", len(loaded.Aliases))
	}

	gs, ok := loaded.Aliases["gs"]
	if !ok {
		t.Fatal("alias 'gs' not found after round-trip")
	}
	if gs.Command != "git status" {
		t.Errorf("gs.Command = %q, want %q", gs.Command, "git status")
	}

	gl, ok := loaded.Aliases["gl"]
	if !ok {
		t.Fatal("alias 'gl' not found after round-trip")
	}
	if len(gl.Args) != 2 || gl.Args[0] != "-n" || gl.Args[1] != "10" {
		t.Errorf("gl.Args = %v, want [-n 10]", gl.Args)
	}
}

func TestResolveKnownAlias(t *testing.T) {
	r := NewAliasRegistry()
	if err := r.Register("gs", "git status", "Show git status"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	def, ok := r.Resolve("gs")
	if !ok {
		t.Fatal("Resolve('gs') returned false, want true")
	}
	if def.Command != "git status" {
		t.Errorf("Command = %q, want %q", def.Command, "git status")
	}
}

func TestResolveUnknownAlias(t *testing.T) {
	r := NewAliasRegistry()

	_, ok := r.Resolve("nonexistent")
	if ok {
		t.Error("Resolve('nonexistent') returned true, want false")
	}
}

func TestDuplicateDetection(t *testing.T) {
	r := NewAliasRegistry()
	if err := r.Register("gs", "git status", ""); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	err := r.Register("gs", "git stash", "")
	if err == nil {
		t.Fatal("duplicate Register should return error")
	}
}

func TestValidationEmptyName(t *testing.T) {
	r := NewAliasRegistry()
	err := r.Register("", "git status", "")
	if err == nil {
		t.Fatal("Register with empty name should return error")
	}
}

func TestValidationEmptyCommand(t *testing.T) {
	r := NewAliasRegistry()
	err := r.Register("gs", "", "")
	if err == nil {
		t.Fatal("Register with empty command should return error")
	}
}

func TestCircularAlias(t *testing.T) {
	r := NewAliasRegistry()
	err := r.Register("gs", "gs --all", "circular")
	if err == nil {
		t.Fatal("Register with circular alias should return error")
	}
}

func TestAll(t *testing.T) {
	r := NewAliasRegistry()
	_ = r.Register("gs", "git status", "status")
	_ = r.Register("gl", "git log", "log")
	_ = r.Register("gd", "git diff", "diff")

	all := r.All()
	if len(all) != 3 {
		t.Errorf("All() returned %d aliases, want 3", len(all))
	}

	for _, name := range []string{"gs", "gl", "gd"} {
		if _, ok := all[name]; !ok {
			t.Errorf("All() missing alias %q", name)
		}
	}
}

func TestDefaultAliasPath(t *testing.T) {
	path := DefaultAliasPath()
	if !filepath.IsAbs(path) && path != filepath.Join(".config", "ralphglasses", "aliases.yml") {
		// Should contain the expected suffix regardless of home dir
		t.Logf("path = %s", path)
	}
	if !containsSuffix(path, filepath.Join("ralphglasses", "aliases.yml")) {
		t.Errorf("DefaultAliasPath() = %q, want suffix %q", path, filepath.Join("ralphglasses", "aliases.yml"))
	}
}

func containsSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestLoadInvalidVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(path, []byte("version: 99\naliases: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAliasFile(path)
	if err == nil {
		t.Fatal("LoadAliasFile with version 99 should return error")
	}
}

func TestLoadEmptyCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	content := "version: 1\naliases:\n  bad:\n    command: \"\"\n    description: empty cmd\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAliasFile(path)
	if err == nil {
		t.Fatal("LoadAliasFile with empty command should return error")
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "aliases.yml")

	af := &AliasFile{
		Version: 1,
		Aliases: map[string]*AliasDef{
			"t": {Command: "go test ./...", Description: "run tests"},
		},
	}

	if err := SaveAliasFile(path, af); err != nil {
		t.Fatalf("SaveAliasFile: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
