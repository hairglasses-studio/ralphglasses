package builtins

import (
	"testing"
)

func TestLoadBuiltins_NoError(t *testing.T) {
	plugins, err := LoadBuiltins()
	if err != nil {
		t.Fatalf("LoadBuiltins() error: %v", err)
	}
	if len(plugins) == 0 {
		t.Fatal("LoadBuiltins() returned no plugins")
	}
}

func TestLoadBuiltins_AllHaveRequiredFields(t *testing.T) {
	plugins, err := LoadBuiltins()
	if err != nil {
		t.Fatalf("LoadBuiltins() error: %v", err)
	}

	for _, p := range plugins {
		t.Run(p.Name, func(t *testing.T) {
			if p.Name == "" {
				t.Error("plugin has empty name")
			}
			if p.Version == "" {
				t.Errorf("plugin %q has empty version", p.Name)
			}
			if len(p.Commands) == 0 {
				t.Errorf("plugin %q has no commands", p.Name)
			}
		})
	}
}

func TestLoadBuiltins_SternLogs(t *testing.T) {
	plugins, err := LoadBuiltins()
	if err != nil {
		t.Fatalf("LoadBuiltins() error: %v", err)
	}

	var found bool
	for _, p := range plugins {
		if p.Name == "stern-logs" {
			found = true
			if len(p.Commands) != 1 {
				t.Errorf("stern-logs: expected 1 command, got %d", len(p.Commands))
			}
			if p.Config["namespace"] == nil {
				t.Error("stern-logs: missing 'namespace' config")
			}
			if p.Config["tail_lines"] == nil {
				t.Error("stern-logs: missing 'tail_lines' config")
			}
		}
	}
	if !found {
		t.Error("stern-logs plugin not found")
	}
}

func TestLoadBuiltins_GhPr(t *testing.T) {
	plugins, err := LoadBuiltins()
	if err != nil {
		t.Fatalf("LoadBuiltins() error: %v", err)
	}

	var found bool
	for _, p := range plugins {
		if p.Name == "gh-pr" {
			found = true
			if len(p.Commands) != 3 {
				t.Errorf("gh-pr: expected 3 commands, got %d", len(p.Commands))
			}
			if p.Config["default_base"] == nil {
				t.Error("gh-pr: missing 'default_base' config")
			}
		}
	}
	if !found {
		t.Error("gh-pr plugin not found")
	}
}

func TestLoadBuiltins_SessionCost(t *testing.T) {
	plugins, err := LoadBuiltins()
	if err != nil {
		t.Fatalf("LoadBuiltins() error: %v", err)
	}

	var found bool
	for _, p := range plugins {
		if p.Name == "session-cost" {
			found = true
			if len(p.Commands) != 2 {
				t.Errorf("session-cost: expected 2 commands, got %d", len(p.Commands))
			}
			if len(p.Hooks) != 1 {
				t.Errorf("session-cost: expected 1 hook, got %d", len(p.Hooks))
			}
			if len(p.Hooks) > 0 && p.Hooks[0].Event != "session.ended" {
				t.Errorf("session-cost: hook event = %q, want %q", p.Hooks[0].Event, "session.ended")
			}
		}
	}
	if !found {
		t.Error("session-cost plugin not found")
	}
}

func TestBuiltinNames_Count(t *testing.T) {
	names := BuiltinNames()
	if len(names) != 3 {
		t.Errorf("BuiltinNames() returned %d names, want 3: %v", len(names), names)
	}
}

func TestBuiltinNames_Expected(t *testing.T) {
	names := BuiltinNames()
	expected := map[string]bool{
		"gh-pr":        false,
		"session-cost": false,
		"stern-logs":   false,
	}

	for _, n := range names {
		if _, ok := expected[n]; ok {
			expected[n] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected builtin %q not found in BuiltinNames()", name)
		}
	}
}
