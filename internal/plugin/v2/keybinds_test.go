package v2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeybindRegistry_Register_and_Lookup(t *testing.T) {
	r := NewKeybindRegistry()

	if err := r.Register("global", "ctrl+p", "command-palette", "core"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	action, ok := r.Lookup("global", "ctrl+p")
	if !ok {
		t.Fatal("Lookup returned false for registered keybind")
	}
	if action != "command-palette" {
		t.Errorf("action = %q, want %q", action, "command-palette")
	}
}

func TestKeybindRegistry_Lookup_Missing(t *testing.T) {
	r := NewKeybindRegistry()

	_, ok := r.Lookup("global", "ctrl+z")
	if ok {
		t.Error("Lookup should return false for unregistered keybind")
	}
}

func TestKeybindRegistry_DuplicateDetection(t *testing.T) {
	r := NewKeybindRegistry()

	if err := r.Register("overview", "g", "goto-top", "plugin-a"); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	err := r.Register("overview", "g", "goto-bottom", "plugin-b")
	if err == nil {
		t.Fatal("expected error for duplicate scope+key")
	}
	if !strings.Contains(err.Error(), "plugin-a") {
		t.Errorf("error should mention original plugin, got: %v", err)
	}
}

func TestKeybindRegistry_SameScopeDifferentKeys(t *testing.T) {
	r := NewKeybindRegistry()

	if err := r.Register("detail", "j", "scroll-down", "nav"); err != nil {
		t.Fatalf("Register j: %v", err)
	}
	if err := r.Register("detail", "k", "scroll-up", "nav"); err != nil {
		t.Fatalf("Register k: %v", err)
	}

	action, ok := r.Lookup("detail", "j")
	if !ok || action != "scroll-down" {
		t.Errorf("j lookup: action=%q ok=%v", action, ok)
	}
	action, ok = r.Lookup("detail", "k")
	if !ok || action != "scroll-up" {
		t.Errorf("k lookup: action=%q ok=%v", action, ok)
	}
}

func TestKeybindRegistry_SameKeyDifferentScopes(t *testing.T) {
	r := NewKeybindRegistry()

	if err := r.Register("overview", "enter", "open-detail", "core"); err != nil {
		t.Fatalf("Register overview: %v", err)
	}
	if err := r.Register("detail", "enter", "expand-section", "core"); err != nil {
		t.Fatalf("Register detail: %v", err)
	}

	action, _ := r.Lookup("overview", "enter")
	if action != "open-detail" {
		t.Errorf("overview enter = %q", action)
	}
	action, _ = r.Lookup("detail", "enter")
	if action != "expand-section" {
		t.Errorf("detail enter = %q", action)
	}
}

func TestKeybindRegistry_AllForScope(t *testing.T) {
	r := NewKeybindRegistry()

	_ = r.Register("global", "ctrl+q", "quit", "core")
	_ = r.Register("global", "ctrl+r", "refresh", "core")
	_ = r.Register("detail", "d", "delete", "admin")

	entries := r.AllForScope("global")
	if len(entries) != 2 {
		t.Fatalf("AllForScope(global) len = %d, want 2", len(entries))
	}
	// Should be sorted by key.
	if entries[0].Key != "ctrl+q" {
		t.Errorf("entries[0].Key = %q, want ctrl+q", entries[0].Key)
	}
	if entries[1].Key != "ctrl+r" {
		t.Errorf("entries[1].Key = %q, want ctrl+r", entries[1].Key)
	}
}

func TestKeybindRegistry_AllForScope_Empty(t *testing.T) {
	r := NewKeybindRegistry()
	entries := r.AllForScope("nonexistent")
	if len(entries) != 0 {
		t.Errorf("AllForScope(nonexistent) len = %d, want 0", len(entries))
	}
}

func TestKeybindRegistry_All(t *testing.T) {
	r := NewKeybindRegistry()

	_ = r.Register("overview", "b", "back", "nav")
	_ = r.Register("global", "q", "quit", "core")
	_ = r.Register("global", "?", "help", "core")

	entries := r.All()
	if len(entries) != 3 {
		t.Fatalf("All() len = %d, want 3", len(entries))
	}
	// Sorted by scope, then key.
	if entries[0].Scope != "global" || entries[0].Key != "?" {
		t.Errorf("entries[0] = {%q, %q}, want {global, ?}", entries[0].Scope, entries[0].Key)
	}
	if entries[1].Scope != "global" || entries[1].Key != "q" {
		t.Errorf("entries[1] = {%q, %q}, want {global, q}", entries[1].Scope, entries[1].Key)
	}
	if entries[2].Scope != "overview" || entries[2].Key != "b" {
		t.Errorf("entries[2] = {%q, %q}, want {overview, b}", entries[2].Scope, entries[2].Key)
	}
}

func TestRegisterKeybinds_Success(t *testing.T) {
	reg := NewKeybindRegistry()
	plugins := []*PluginDef{
		{
			Name:    "plugin-a",
			Version: "1.0.0",
			Keybinds: []KeybindDef{
				{Key: "ctrl+a", Scope: "global", Action: "select-all"},
			},
		},
		{
			Name:    "plugin-b",
			Version: "1.0.0",
			Keybinds: []KeybindDef{
				{Key: "ctrl+b", Scope: "global", Action: "build"},
				{Key: "d", Scope: "detail", Action: "diff"},
			},
		},
	}

	if err := RegisterKeybinds(reg, plugins); err != nil {
		t.Fatalf("RegisterKeybinds: %v", err)
	}

	action, ok := reg.Lookup("global", "ctrl+a")
	if !ok || action != "select-all" {
		t.Errorf("ctrl+a: action=%q ok=%v", action, ok)
	}
	action, ok = reg.Lookup("global", "ctrl+b")
	if !ok || action != "build" {
		t.Errorf("ctrl+b: action=%q ok=%v", action, ok)
	}
	action, ok = reg.Lookup("detail", "d")
	if !ok || action != "diff" {
		t.Errorf("d: action=%q ok=%v", action, ok)
	}
}

func TestRegisterKeybinds_Conflict(t *testing.T) {
	reg := NewKeybindRegistry()
	plugins := []*PluginDef{
		{
			Name:    "alpha",
			Version: "1.0.0",
			Keybinds: []KeybindDef{
				{Key: "x", Scope: "global", Action: "close"},
			},
		},
		{
			Name:    "beta",
			Version: "1.0.0",
			Keybinds: []KeybindDef{
				{Key: "x", Scope: "global", Action: "execute"},
			},
		},
	}

	err := RegisterKeybinds(reg, plugins)
	if err == nil {
		t.Fatal("expected error for conflicting keybinds")
	}
	if !strings.Contains(err.Error(), "beta") {
		t.Errorf("error should mention conflicting plugin, got: %v", err)
	}
}

func TestRegisterKeybinds_NilPlugins(t *testing.T) {
	reg := NewKeybindRegistry()
	if err := RegisterKeybinds(reg, nil); err != nil {
		t.Fatalf("RegisterKeybinds(nil): %v", err)
	}
}

func TestRegisterKeybinds_NoKeybinds(t *testing.T) {
	reg := NewKeybindRegistry()
	plugins := []*PluginDef{
		{Name: "empty", Version: "1.0.0"},
	}
	if err := RegisterKeybinds(reg, plugins); err != nil {
		t.Fatalf("RegisterKeybinds: %v", err)
	}
	if len(reg.All()) != 0 {
		t.Error("expected no keybinds registered")
	}
}

func TestValidate_KeybindMissingKey(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Keybinds: []KeybindDef{
			{Scope: "global", Action: "do-thing"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for keybind missing key")
	}
}

func TestValidate_KeybindMissingScope(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Keybinds: []KeybindDef{
			{Key: "g", Action: "goto"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for keybind missing scope")
	}
}

func TestValidate_KeybindMissingAction(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Keybinds: []KeybindDef{
			{Key: "g", Scope: "global"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for keybind missing action")
	}
}

func TestValidate_KeybindDuplicate(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Keybinds: []KeybindDef{
			{Key: "g", Scope: "global", Action: "goto-top"},
			{Key: "g", Scope: "global", Action: "goto-bottom"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for duplicate keybind within plugin")
	}
}

func TestValidate_KeybindValid(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Keybinds: []KeybindDef{
			{Key: "g", Scope: "global", Action: "goto-top"},
			{Key: "G", Scope: "global", Action: "goto-bottom"},
			{Key: "g", Scope: "detail", Action: "goto-top-detail"},
		},
	}
	if err := Validate(p); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestLoadPlugin_WithKeybinds(t *testing.T) {
	dir := t.TempDir()
	content := `
name: kb-plugin
version: "1.0.0"
commands:
  - name: greet
    run: echo hi
keybinds:
  - key: ctrl+g
    scope: global
    action: greet
  - key: d
    scope: detail
    action: echo diff
`
	path := filepath.Join(dir, "plugin.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPlugin(path)
	if err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	if len(p.Keybinds) != 2 {
		t.Fatalf("Keybinds len = %d, want 2", len(p.Keybinds))
	}
	if p.Keybinds[0].Key != "ctrl+g" {
		t.Errorf("Keybinds[0].Key = %q, want %q", p.Keybinds[0].Key, "ctrl+g")
	}
	if p.Keybinds[0].Scope != "global" {
		t.Errorf("Keybinds[0].Scope = %q, want %q", p.Keybinds[0].Scope, "global")
	}
	if p.Keybinds[0].Action != "greet" {
		t.Errorf("Keybinds[0].Action = %q, want %q", p.Keybinds[0].Action, "greet")
	}
	if p.Keybinds[1].Key != "d" {
		t.Errorf("Keybinds[1].Key = %q, want %q", p.Keybinds[1].Key, "d")
	}
}

func TestKeybindRegistry_ConcurrentAccess(t *testing.T) {
	r := NewKeybindRegistry()
	done := make(chan struct{})

	// Writer goroutine.
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			_ = r.Register("scope", string(rune('a'+i%26))+string(rune('0'+i/26)), "action", "plugin")
		}
	}()

	// Reader goroutines.
	for i := 0; i < 4; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				r.Lookup("scope", "a0")
				r.AllForScope("scope")
				r.All()
			}
		}()
	}

	<-done
}
