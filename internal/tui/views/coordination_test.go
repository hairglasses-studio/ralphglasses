package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCoordinationModel_Init(t *testing.T) {
	m := NewCoordination()
	cmd := m.Init()
	if cmd != nil {
		t.Fatal("Init should return nil cmd")
	}
}

func TestCoordinationModel_View_Empty(t *testing.T) {
	m := NewCoordination()
	view := m.View()

	if !strings.Contains(view, "Coordination Dashboard") {
		t.Fatal("empty view should contain title")
	}
	if !strings.Contains(view, "No sessions") {
		t.Fatal("empty view should show 'No sessions'")
	}
	if !strings.Contains(view, "0 total") {
		t.Fatal("empty view should show 0 total nodes")
	}
}

func TestCoordinationModel_View_SingleSession(t *testing.T) {
	m := NewCoordination()
	m.SetNodes([]CoordinationNode{
		{
			ID:       "s1",
			Name:     "session-alpha",
			Provider: "claude",
			Status:   "running",
			Cost:     1.50,
			RepoPath: "/repo/a",
			Task:     "implement feature",
		},
	})

	view := m.View()

	if !strings.Contains(view, "1 total") {
		t.Fatalf("view should show 1 total node, got:\n%s", view)
	}
	if !strings.Contains(view, "session-alpha") {
		t.Fatal("view should contain session name")
	}
	if !strings.Contains(view, "claude") {
		t.Fatal("view should contain provider")
	}
	if !strings.Contains(view, "running") {
		t.Fatal("view should contain status")
	}
	if !strings.Contains(view, "$1.50") {
		t.Fatalf("view should contain cost '$1.50', got:\n%s", view)
	}
	if !strings.Contains(view, "implement feature") {
		t.Fatal("view should contain task")
	}
	// No contention for a single session.
	if strings.Contains(view, "Resource Contention") {
		t.Fatal("single session should not show contention")
	}
}

func TestCoordinationModel_View_MultipleSessions_WithDependencies(t *testing.T) {
	m := NewCoordination()
	m.SetNodes([]CoordinationNode{
		{
			ID:       "s1",
			Name:     "root-session",
			Provider: "claude",
			Status:   "running",
			Cost:     2.00,
			RepoPath: "/repo/a",
			Task:     "orchestrate",
		},
		{
			ID:        "s2",
			Name:      "worker-one",
			Provider:  "gemini",
			Status:    "waiting",
			Cost:      0.50,
			RepoPath:  "/repo/b",
			Task:      "implement API",
			DependsOn: []string{"s1"},
		},
		{
			ID:        "s3",
			Name:      "worker-two",
			Provider:  "codex",
			Status:    "error",
			Cost:      0.10,
			RepoPath:  "/repo/a",
			Task:      "write tests",
			DependsOn: []string{"s1"},
		},
	})

	view := m.View()

	if !strings.Contains(view, "3 total") {
		t.Fatalf("view should show 3 total nodes, got:\n%s", view)
	}
	if !strings.Contains(view, "root-session") {
		t.Fatal("view should contain root session")
	}
	if !strings.Contains(view, "worker-one") {
		t.Fatal("view should contain worker-one")
	}
	if !strings.Contains(view, "worker-two") {
		t.Fatal("view should contain worker-two")
	}

	// Dependency connector characters should appear for children.
	if !strings.Contains(view, "├──") {
		t.Fatal("view should contain dependency connector '├──'")
	}

	// Cost summary should include all.
	if !strings.Contains(view, "$2.60") {
		t.Fatalf("view should show total cost $2.60, got:\n%s", view)
	}

	// Resource contention: s1 (running) and s3 (error) both on /repo/a,
	// but s3 is "error" status, not active. Only running/waiting count.
	// s1=running on /repo/a, s3=error on /repo/a -> no contention.
	if strings.Contains(view, "Resource Contention") {
		t.Fatal("should not show contention when only one active session per repo")
	}
}

func TestCoordinationModel_View_ResourceContention(t *testing.T) {
	m := NewCoordination()
	m.SetNodes([]CoordinationNode{
		{
			ID:       "s1",
			Name:     "session-a",
			Provider: "claude",
			Status:   "running",
			RepoPath: "/repo/shared",
		},
		{
			ID:       "s2",
			Name:     "session-b",
			Provider: "gemini",
			Status:   "running",
			RepoPath: "/repo/shared",
		},
	})

	view := m.View()

	if !strings.Contains(view, "Resource Contention") {
		t.Fatalf("view should show contention when two sessions share a repo, got:\n%s", view)
	}
	if !strings.Contains(view, "/repo/shared") {
		t.Fatal("view should show the contended repo path")
	}
	if !strings.Contains(view, "s1") || !strings.Contains(view, "s2") {
		t.Fatal("view should list both contending session IDs")
	}
}

func TestCoordinationModel_KeyBindings(t *testing.T) {
	nodes := []CoordinationNode{
		{ID: "s1", Name: "a", Provider: "claude", Status: "running"},
		{ID: "s2", Name: "b", Provider: "gemini", Status: "waiting"},
		{ID: "s3", Name: "c", Provider: "codex", Status: "completed"},
	}

	t.Run("down moves cursor", func(t *testing.T) {
		m := NewCoordination()
		m.SetNodes(nodes)

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.cursor != 1 {
			t.Fatalf("cursor = %d, want 1", m.cursor)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.cursor != 2 {
			t.Fatalf("cursor = %d, want 2", m.cursor)
		}

		// Clamp at end.
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.cursor != 2 {
			t.Fatalf("cursor = %d, want 2 (clamped)", m.cursor)
		}
	})

	t.Run("up moves cursor", func(t *testing.T) {
		m := NewCoordination()
		m.SetNodes(nodes)
		m.cursor = 2

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if m.cursor != 1 {
			t.Fatalf("cursor = %d, want 1", m.cursor)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0", m.cursor)
		}

		// Clamp at start.
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0 (clamped)", m.cursor)
		}
	})

	t.Run("enter selects node", func(t *testing.T) {
		m := NewCoordination()
		m.SetNodes(nodes)
		m.cursor = 1

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("enter should produce a cmd")
		}
		msg := cmd()
		sel, ok := msg.(CoordinationSelectMsg)
		if !ok {
			t.Fatalf("expected CoordinationSelectMsg, got %T", msg)
		}
		if sel.NodeID != "s2" {
			t.Fatalf("NodeID = %q, want s2", sel.NodeID)
		}
	})

	t.Run("r triggers refresh", func(t *testing.T) {
		m := NewCoordination()
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		if cmd == nil {
			t.Fatal("r should produce a cmd")
		}
		msg := cmd()
		if _, ok := msg.(CoordinationRefreshMsg); !ok {
			t.Fatalf("expected CoordinationRefreshMsg, got %T", msg)
		}
	})

	t.Run("q quits", func(t *testing.T) {
		m := NewCoordination()
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if cmd == nil {
			t.Fatal("q should produce a quit cmd")
		}
	})
}

func TestCoordinationModel_SetNodes_CursorClamp(t *testing.T) {
	m := NewCoordination()

	m.SetNodes([]CoordinationNode{
		{ID: "s1"}, {ID: "s2"}, {ID: "s3"},
	})
	m.cursor = 2

	// Shrink to 1 node: cursor should clamp.
	m.SetNodes([]CoordinationNode{{ID: "s1"}})
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after shrink", m.cursor)
	}

	// Empty.
	m.SetNodes(nil)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 for empty nodes", m.cursor)
	}
}

func TestLayoutGraph_Empty(t *testing.T) {
	levels := LayoutGraph(nil)
	if len(levels) != 0 {
		t.Fatalf("expected 0 levels, got %d", len(levels))
	}
}

func TestLayoutGraph_SingleNode(t *testing.T) {
	nodes := []CoordinationNode{{ID: "a"}}
	levels := LayoutGraph(nodes)
	if len(levels) != 1 {
		t.Fatalf("expected 1 level, got %d", len(levels))
	}
	if len(levels[0]) != 1 || levels[0][0] != 0 {
		t.Fatalf("expected level 0 = [0], got %v", levels[0])
	}
}

func TestLayoutGraph_LinearChain(t *testing.T) {
	// a -> b -> c
	nodes := []CoordinationNode{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"b"}},
	}
	levels := LayoutGraph(nodes)
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d", len(levels))
	}
	if levels[0][0] != 0 {
		t.Fatalf("level 0 should be [0], got %v", levels[0])
	}
	if levels[1][0] != 1 {
		t.Fatalf("level 1 should be [1], got %v", levels[1])
	}
	if levels[2][0] != 2 {
		t.Fatalf("level 2 should be [2], got %v", levels[2])
	}
}

func TestLayoutGraph_Diamond(t *testing.T) {
	// a -> b, a -> c, b -> d, c -> d
	nodes := []CoordinationNode{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}
	levels := LayoutGraph(nodes)
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels for diamond, got %d", len(levels))
	}
	// Level 0: a
	if len(levels[0]) != 1 || levels[0][0] != 0 {
		t.Fatalf("level 0 should be [0], got %v", levels[0])
	}
	// Level 1: b, c (order may vary).
	if len(levels[1]) != 2 {
		t.Fatalf("level 1 should have 2 nodes, got %v", levels[1])
	}
	// Level 2: d
	if len(levels[2]) != 1 || levels[2][0] != 3 {
		t.Fatalf("level 2 should be [3], got %v", levels[2])
	}
}

func TestLayoutGraph_Cycle(t *testing.T) {
	// a -> b -> a (cycle, no roots with in-degree 0 after considering the cycle)
	// Actually a has in-degree from b, b has in-degree from a. Both have in-degree 1.
	nodes := []CoordinationNode{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}
	levels := LayoutGraph(nodes)
	// Both should appear in the remaining/cycle level.
	if len(levels) != 1 {
		t.Fatalf("expected 1 level for cycle, got %d", len(levels))
	}
	if len(levels[0]) != 2 {
		t.Fatalf("cycle level should have 2 nodes, got %v", levels[0])
	}
}

func TestCoordinationModel_WindowSize(t *testing.T) {
	m := NewCoordination()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 {
		t.Fatalf("width = %d, want 120", m.width)
	}
	if m.height != 40 {
		t.Fatalf("height = %d, want 40", m.height)
	}
}

func TestCoordinationModel_View_ClaimedTasks(t *testing.T) {
	m := NewCoordination()
	m.SetNodes([]CoordinationNode{
		{
			ID:           "s1",
			Name:         "orchestrator",
			Provider:     "claude",
			Status:       "running",
			ClaimedTasks: []string{"task-1", "task-2"},
		},
	})

	view := m.View()
	if !strings.Contains(view, "claims:") {
		t.Fatal("view should show claimed tasks")
	}
	if !strings.Contains(view, "task-1") {
		t.Fatal("view should show task-1")
	}
	if !strings.Contains(view, "task-2") {
		t.Fatal("view should show task-2")
	}
}

func TestCoordinationModel_View_LongNameTruncation(t *testing.T) {
	m := NewCoordination()
	m.SetNodes([]CoordinationNode{
		{
			ID:   "s1",
			Name: "very-long-session-name-that-exceeds-limit",
			Provider: "claude",
			Status:   "running",
		},
	})

	view := m.View()
	if strings.Contains(view, "very-long-session-name-that-exceeds-limit") {
		t.Fatal("long name should be truncated")
	}
	if !strings.Contains(view, "...") {
		t.Fatal("truncated name should end with '...'")
	}
}
