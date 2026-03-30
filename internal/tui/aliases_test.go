package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func tempAliasPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "aliases.json")
}

func TestAliasStore_SetGetDelete(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}

	// Set and Get a user alias.
	if err := s.Set("st", "start"); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get("st")
	if !ok || got != "start" {
		t.Errorf("Get(st) = %q, %v; want %q, true", got, ok, "start")
	}

	// Overwrite existing alias.
	if err := s.Set("st", "stop"); err != nil {
		t.Fatal(err)
	}
	got, ok = s.Get("st")
	if !ok || got != "stop" {
		t.Errorf("Get(st) after overwrite = %q, %v; want %q, true", got, ok, "stop")
	}

	// Delete.
	if err := s.Delete("st"); err != nil {
		t.Fatal(err)
	}
	_, ok = s.Get("st")
	if ok {
		t.Error("Get(st) after delete should return false")
	}
}

func TestAliasStore_List(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("deploy", "fleet submit"); err != nil {
		t.Fatal(err)
	}
	all := s.List()

	// Must contain built-in aliases.
	for k, v := range builtinCommandAliases {
		if all[k] != v {
			t.Errorf("List()[%q] = %q, want %q", k, all[k], v)
		}
	}
	// Must contain user alias.
	if all["deploy"] != "fleet submit" {
		t.Errorf("List()[deploy] = %q, want %q", all["deploy"], "fleet submit")
	}
}

func TestAliasStore_BuiltinAliases(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}

	// Built-ins should be resolvable.
	for alias, cmd := range builtinCommandAliases {
		got, ok := s.Get(alias)
		if !ok || got != cmd {
			t.Errorf("Get(%q) = %q, %v; want %q, true", alias, got, ok, cmd)
		}
	}
}

func TestAliasStore_Resolve(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("st", "start"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"st mesmer", "start mesmer"},
		{"st", "start"},
		{"q", "quit"},
		{"h", "help"},
		{"s", "status"},
		{"unknown", "unknown"},
		{"", ""},
		{"  st  mesmer  ", "start mesmer"},
	}
	for _, tt := range tests {
		got := s.Resolve(tt.input)
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAliasStore_ConflictBuiltinAlias(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}

	// Cannot redefine a built-in alias.
	if err := s.Set("q", "something"); err == nil {
		t.Error("Set(q) should fail for built-in alias")
	}
	if err := s.Set("h", "something"); err == nil {
		t.Error("Set(h) should fail for built-in alias")
	}
}

func TestAliasStore_ConflictBuiltinCommand(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}

	// Cannot alias a built-in command name.
	if err := s.Set("quit", "exit"); err == nil {
		t.Error("Set(quit) should fail for built-in command")
	}
	if err := s.Set("start", "begin"); err == nil {
		t.Error("Set(start) should fail for built-in command")
	}
}

func TestAliasStore_DeleteBuiltinFails(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("q"); err == nil {
		t.Error("Delete(q) should fail for built-in alias")
	}
}

func TestAliasStore_Persistence(t *testing.T) {
	path := tempAliasPath(t)

	// Create store and set aliases.
	s1, err := NewAliasStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Set("deploy", "fleet submit"); err != nil {
		t.Fatal(err)
	}
	if err := s1.Set("ll", "logs"); err != nil {
		t.Fatal(err)
	}
	if err := s1.Save(); err != nil {
		t.Fatal(err)
	}

	// Verify JSON on disk.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ondisk map[string]string
	if err := json.Unmarshal(data, &ondisk); err != nil {
		t.Fatalf("invalid JSON on disk: %v", err)
	}
	if ondisk["deploy"] != "fleet submit" {
		t.Errorf("disk[deploy] = %q, want %q", ondisk["deploy"], "fleet submit")
	}

	// Load a fresh store from the same path.
	s2, err := NewAliasStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("deploy")
	if !ok || got != "fleet submit" {
		t.Errorf("round-trip Get(deploy) = %q, %v; want %q, true", got, ok, "fleet submit")
	}
	got, ok = s2.Get("ll")
	if !ok || got != "logs" {
		t.Errorf("round-trip Get(ll) = %q, %v; want %q, true", got, ok, "logs")
	}
}

func TestAliasStore_EmptyInputValidation(t *testing.T) {
	s, err := NewAliasStore(tempAliasPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set("", "something"); err == nil {
		t.Error("Set with empty alias should fail")
	}
	if err := s.Set("x", ""); err == nil {
		t.Error("Set with empty command should fail")
	}
}

func TestNewAliasStore_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "aliases.json")
	s, err := NewAliasStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// Should still work with built-ins.
	got, ok := s.Get("q")
	if !ok || got != "quit" {
		t.Errorf("Get(q) on missing file = %q, %v; want %q, true", got, ok, "quit")
	}
}

func TestNewAliasStore_InvalidJSON(t *testing.T) {
	path := tempAliasPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewAliasStore(path)
	if err == nil {
		t.Error("NewAliasStore should fail on invalid JSON")
	}
}
