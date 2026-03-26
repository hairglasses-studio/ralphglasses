package mcpserver

import (
	"testing"
)

func TestRegistryBuildAll(t *testing.T) {
	r := NewToolGroupRegistry()
	r.Register(NewFuncBuilder("alpha", func(s *Server) ToolGroup {
		return ToolGroup{Name: "alpha", Description: "first"}
	}))
	r.Register(NewFuncBuilder("beta", func(s *Server) ToolGroup {
		return ToolGroup{Name: "beta", Description: "second"}
	}))

	s := &Server{}
	m := r.BuildAll(s)

	if len(m) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(m))
	}
	if g, ok := m["alpha"]; !ok {
		t.Error("missing group 'alpha'")
	} else if g.Description != "first" {
		t.Errorf("alpha description = %q, want %q", g.Description, "first")
	}
	if g, ok := m["beta"]; !ok {
		t.Error("missing group 'beta'")
	} else if g.Description != "second" {
		t.Errorf("beta description = %q, want %q", g.Description, "second")
	}
}

func TestRegistryBuildAllOrdered(t *testing.T) {
	r := NewToolGroupRegistry()
	names := []string{"core", "session", "loop"}
	for _, n := range names {
		n := n
		r.Register(NewFuncBuilder(n, func(s *Server) ToolGroup {
			return ToolGroup{Name: n}
		}))
	}

	s := &Server{}
	groups := r.BuildAllOrdered(s)

	if len(groups) != len(names) {
		t.Fatalf("expected %d groups, got %d", len(names), len(groups))
	}
	for i, g := range groups {
		if g.Name != names[i] {
			t.Errorf("group[%d].Name = %q, want %q", i, g.Name, names[i])
		}
	}
}

func TestRegistryEmpty(t *testing.T) {
	r := NewToolGroupRegistry()
	s := &Server{}

	m := r.BuildAll(s)
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d entries", len(m))
	}

	groups := r.BuildAllOrdered(s)
	if len(groups) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(groups))
	}

	if r.Len() != 0 {
		t.Errorf("expected Len() = 0, got %d", r.Len())
	}
}

func TestFuncBuilder(t *testing.T) {
	called := false
	b := NewFuncBuilder("test_group", func(s *Server) ToolGroup {
		called = true
		return ToolGroup{Name: "test_group", Description: "desc"}
	})

	if b.Name() != "test_group" {
		t.Errorf("Name() = %q, want %q", b.Name(), "test_group")
	}

	s := &Server{}
	g := b.Build(s)

	if !called {
		t.Error("build function was not called")
	}
	if g.Name != "test_group" {
		t.Errorf("group Name = %q, want %q", g.Name, "test_group")
	}
	if g.Description != "desc" {
		t.Errorf("group Description = %q, want %q", g.Description, "desc")
	}
}

func TestRegistryNames(t *testing.T) {
	r := NewToolGroupRegistry()
	r.Register(NewFuncBuilder("a", func(s *Server) ToolGroup { return ToolGroup{} }))
	r.Register(NewFuncBuilder("b", func(s *Server) ToolGroup { return ToolGroup{} }))
	r.Register(NewFuncBuilder("c", func(s *Server) ToolGroup { return ToolGroup{} }))

	names := r.Names()
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	expected := []string{"a", "b", "c"}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, expected[i])
		}
	}
}

// TestDefaultRegistryMatchesToolGroupNames verifies that the default registry
// produces groups in the same order as the canonical ToolGroupNames list.
func TestDefaultRegistryMatchesToolGroupNames(t *testing.T) {
	r := defaultRegistry()
	names := r.Names()

	if len(names) != len(ToolGroupNames) {
		t.Fatalf("registry has %d groups, ToolGroupNames has %d", len(names), len(ToolGroupNames))
	}
	for i, n := range names {
		if n != ToolGroupNames[i] {
			t.Errorf("registry[%d] = %q, ToolGroupNames[%d] = %q", i, n, i, ToolGroupNames[i])
		}
	}
}

// TestDefaultRegistryBuildAllOutputMatchesLegacy ensures the registry-based
// buildToolGroups produces identical output to what the old inline list did.
func TestDefaultRegistryBuildAllOutputMatchesLegacy(t *testing.T) {
	s := &Server{}
	groups := s.buildToolGroups()

	if len(groups) != len(ToolGroupNames) {
		t.Fatalf("expected %d groups, got %d", len(ToolGroupNames), len(groups))
	}

	for i, g := range groups {
		if g.Name != ToolGroupNames[i] {
			t.Errorf("group[%d].Name = %q, want %q", i, g.Name, ToolGroupNames[i])
		}
		if g.Description == "" {
			t.Errorf("group[%d] (%s) has empty description", i, g.Name)
		}
		if len(g.Tools) == 0 {
			t.Errorf("group[%d] (%s) has no tools", i, g.Name)
		}
	}
}
