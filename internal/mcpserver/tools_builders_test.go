package mcpserver

import (
	"strings"
	"testing"
)

// builderSpec describes expectations for a single build*Group method.
type builderSpec struct {
	name        string                // expected group Name
	buildFn     func(s *Server) ToolGroup
	minTools    int  // minimum number of tools expected
	wantNonZero bool // always true; here for clarity
}

// allBuilderSpecs returns the canonical list of builder specs covering all 13
// namespaces.  The minTools counts are taken from the current source code and
// act as a regression guard — they must be updated when tools are added.
func allBuilderSpecs() []builderSpec {
	return []builderSpec{
		{"core", (*Server).buildCoreGroup, 10, true},
		{"session", (*Server).buildSessionGroup, 13, true},
		{"loop", (*Server).buildLoopGroup, 9, true},
		{"prompt", (*Server).buildPromptGroup, 8, true},
		{"fleet", (*Server).buildFleetGroup, 7, true},
		{"repo", (*Server).buildRepoGroup, 5, true},
		{"roadmap", (*Server).buildRoadmapGroup, 5, true},
		{"team", (*Server).buildTeamGroup, 6, true},
		{"awesome", (*Server).buildAwesomeGroup, 5, true},
		{"advanced", (*Server).buildAdvancedGroup, 23, true},
		{"eval", (*Server).buildEvalGroup, 4, true},
		{"fleet_h", (*Server).buildFleetHGroup, 4, true},
		{"observability", (*Server).buildObservabilityGroup, 12, true},
	}
}

// TestBuildGroupBasics runs table-driven tests for every build*Group method,
// verifying structural invariants: non-empty name, non-empty description,
// at least 1 tool, ralphglasses_ prefix, and non-nil handlers.
func TestBuildGroupBasics(t *testing.T) {
	srv, _ := setupTestServer(t)

	for _, spec := range allBuilderSpecs() {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			group := spec.buildFn(srv)

			// Non-empty name.
			if group.Name == "" {
				t.Fatal("group Name is empty")
			}
			if group.Name != spec.name {
				t.Fatalf("expected Name %q, got %q", spec.name, group.Name)
			}

			// Non-empty description.
			if group.Description == "" {
				t.Fatal("group Description is empty")
			}

			// At least minTools tools.
			if len(group.Tools) < spec.minTools {
				t.Fatalf("expected at least %d tools, got %d", spec.minTools, len(group.Tools))
			}

			// Every tool must have the ralphglasses_ prefix.
			for _, te := range group.Tools {
				toolName := te.Tool.Name
				if !strings.HasPrefix(toolName, "ralphglasses_") {
					t.Errorf("tool %q missing ralphglasses_ prefix", toolName)
				}
			}

			// Every tool handler must be non-nil.
			for _, te := range group.Tools {
				if te.Handler == nil {
					t.Errorf("tool %q has nil handler", te.Tool.Name)
				}
			}
		})
	}
}

// TestBuildGroupToolNamesUnique checks that no two tools within the same group
// share a name.
func TestBuildGroupToolNamesUnique(t *testing.T) {
	srv, _ := setupTestServer(t)

	for _, spec := range allBuilderSpecs() {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			group := spec.buildFn(srv)
			seen := make(map[string]struct{}, len(group.Tools))
			for _, te := range group.Tools {
				if _, dup := seen[te.Tool.Name]; dup {
					t.Errorf("duplicate tool name %q in group %q", te.Tool.Name, group.Name)
				}
				seen[te.Tool.Name] = struct{}{}
			}
		})
	}
}

// TestBuildGroupToolNamesGloballyUnique verifies that tool names are unique
// across all groups combined — a requirement for the MCP server registry.
func TestBuildGroupToolNamesGloballyUnique(t *testing.T) {
	srv, _ := setupTestServer(t)
	seen := make(map[string]string) // tool name -> group name

	for _, spec := range allBuilderSpecs() {
		group := spec.buildFn(srv)
		for _, te := range group.Tools {
			if prev, dup := seen[te.Tool.Name]; dup {
				t.Errorf("tool %q appears in both %q and %q", te.Tool.Name, prev, group.Name)
			}
			seen[te.Tool.Name] = group.Name
		}
	}
}

// TestDefaultRegistryReturnsAll13Groups verifies that defaultRegistry()
// registers exactly 13 builder entries matching ToolGroupNames.
func TestDefaultRegistryReturnsAll13Groups(t *testing.T) {
	reg := defaultRegistry()

	if reg.Len() != 13 {
		t.Fatalf("expected 13 registered groups, got %d", reg.Len())
	}

	names := reg.Names()
	if len(names) != len(ToolGroupNames) {
		t.Fatalf("registry names length %d != ToolGroupNames length %d", len(names), len(ToolGroupNames))
	}

	// The order must match ToolGroupNames.
	for i, want := range ToolGroupNames {
		if names[i] != want {
			t.Errorf("position %d: expected %q, got %q", i, want, names[i])
		}
	}
}

// TestDefaultRegistryBuildAll verifies that BuildAll produces a map keyed by
// every expected group name.
func TestDefaultRegistryBuildAll(t *testing.T) {
	srv, _ := setupTestServer(t)
	reg := defaultRegistry()
	groups := reg.BuildAll(srv)

	if len(groups) != 13 {
		t.Fatalf("expected 13 groups from BuildAll, got %d", len(groups))
	}

	for _, name := range ToolGroupNames {
		if _, ok := groups[name]; !ok {
			t.Errorf("group %q missing from BuildAll result", name)
		}
	}
}

// TestBuildToolGroupsTotalCount verifies that buildToolGroups returns the
// expected total number of tools across all groups.
func TestBuildToolGroupsTotalCount(t *testing.T) {
	srv, _ := setupTestServer(t)
	groups := srv.buildToolGroups()

	if len(groups) != 13 {
		t.Fatalf("expected 13 groups, got %d", len(groups))
	}

	// Count total tools.
	total := 0
	for _, g := range groups {
		total += len(g.Tools)
	}

	// The CLAUDE.md says 110 tools. Allow a small tolerance for additions.
	if total < 100 {
		t.Fatalf("expected at least 100 tools total, got %d", total)
	}
	t.Logf("total tool count: %d across %d groups", total, len(groups))
}

// TestBuildToolGroupsDescriptionsNonEmpty ensures every group returned by
// buildToolGroups has a non-empty description.
func TestBuildToolGroupsDescriptionsNonEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)
	groups := srv.buildToolGroups()

	for _, g := range groups {
		if g.Description == "" {
			t.Errorf("group %q has empty description", g.Name)
		}
	}
}

// TestBuildToolGroupsOrderMatchesToolGroupNames checks that the slice order
// from buildToolGroups matches the canonical ToolGroupNames ordering.
func TestBuildToolGroupsOrderMatchesToolGroupNames(t *testing.T) {
	srv, _ := setupTestServer(t)
	groups := srv.buildToolGroups()

	if len(groups) != len(ToolGroupNames) {
		t.Fatalf("group count %d != ToolGroupNames count %d", len(groups), len(ToolGroupNames))
	}

	for i, want := range ToolGroupNames {
		if groups[i].Name != want {
			t.Errorf("position %d: expected %q, got %q", i, want, groups[i].Name)
		}
	}
}

// TestBuildGroupToolDescriptionsNonEmpty checks every tool across all builders
// has a non-empty description.
func TestBuildGroupToolDescriptionsNonEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)

	for _, spec := range allBuilderSpecs() {
		group := spec.buildFn(srv)
		for _, te := range group.Tools {
			desc := te.Tool.Description
			if desc == "" {
				t.Errorf("tool %q in group %q has empty description", te.Tool.Name, group.Name)
			}
		}
	}
}
