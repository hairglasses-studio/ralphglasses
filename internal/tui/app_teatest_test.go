package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// frozenTime is the fixed "now" for golden file determinism.
var frozenTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func init() {
	// Freeze the clock so formatAgo produces deterministic output in golden files.
	components.NowFunc = func() time.Time { return frozenTime }
}

// newTestModel creates a Model with deterministic state for golden file tests.
func newTestModel(t *testing.T) Model {
	t.Helper()
	m := NewModel("", nil) // empty path skips scanRepos for deterministic golden files
	m.Width = 120
	m.Height = 40
	m.Table.Width = 120
	m.StatusBar.Width = 120
	m.LastRefresh = frozenTime.Add(-5 * time.Minute) // always "5m" ago
	return m
}

// newTestModelWithRepos creates a test model pre-loaded with mock repos.
func newTestModelWithRepos(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m.Repos = []*model.Repo{
		{Name: "ralphglasses", Path: "/tmp/ralphglasses", Status: &model.LoopStatus{Status: "running"}},
		{Name: "mcpkit", Path: "/tmp/mcpkit", Status: &model.LoopStatus{Status: "idle"}},
		{Name: "hg-mcp", Path: "/tmp/hg-mcp", Status: &model.LoopStatus{Status: "completed"}},
	}
	m.updateTable()
	return m
}

// newTestModelWithRepoDetail creates a deterministic nested repo-detail view.
func newTestModelWithRepoDetail(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m.Repos = []*model.Repo{
		{
			Name:  "ralphglasses",
			Path:  "/tmp/ralphglasses",
			HasRC: true,
			Status: &model.LoopStatus{
				Status:          "running",
				LoopCount:       17,
				CallsMadeThisHr: 23,
				MaxCallsPerHour: 120,
				Model:           "gpt-5.4",
				SessionSpendUSD: 12.50,
				BudgetStatus:    "ok",
				LastAction:      "planner",
				Timestamp:       frozenTime.Add(-2 * time.Minute),
			},
			Circuit: &model.CircuitBreakerState{
				State:                        "CLOSED",
				LastChange:                   frozenTime.Add(-15 * time.Minute),
				ConsecutiveNoProgress:        1,
				ConsecutiveSameError:         0,
				ConsecutivePermissionDenials: 0,
				TotalOpens:                   0,
			},
			Progress: &model.Progress{
				Iteration:    17,
				CompletedIDs: []string{"QW-12", "WM-1"},
				Status:       "running",
			},
			Config: &model.RalphConfig{
				Values: map[string]string{
					"PROJECT_NAME":         "ralphglasses",
					"PROVIDER":             "codex",
					"RALPH_SESSION_BUDGET": "100",
				},
			},
		},
	}
	m.Sel.RepoIdx = 0
	m.Nav.CurrentView = ViewRepoDetail
	m.Nav.ActiveTab = 0
	m.Nav.Breadcrumb = components.Breadcrumb{Parts: []string{"Repos", "ralphglasses"}}
	return m
}

// newTestModelWithFleetView creates a deterministic nested fleet dashboard view.
func newTestModelWithFleetView(t *testing.T) Model {
	t.Helper()
	m := newTestModelWithRepos(t)
	m.Nav.CurrentView = ViewFleet
	m.Nav.ActiveTab = 3
	m.Nav.Breadcrumb = components.Breadcrumb{Parts: []string{"Fleet"}}
	m.Repos[0].Circuit = &model.CircuitBreakerState{State: "OPEN", Reason: "budget gate"}

	mgr := session.NewManager()
	s := &session.Session{
		ID:         "session-123456",
		Provider:   session.ProviderCodex,
		RepoPath:   "/tmp/ralphglasses",
		RepoName:   "ralphglasses",
		Status:     session.StatusRunning,
		SpentUSD:   2.50,
		BudgetUSD:  10,
		TurnCount:  4,
		LaunchedAt: frozenTime.Add(-30 * time.Minute),
	}
	s.CostHistory = []float64{0.25, 0.75, 1.25, 2.50}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	return m
}

// keyPressMsg constructs a v2 key press message for a single printable character.
func keyPressMsg(ch rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: ch, Text: string(ch)}
}

// testProgram runs model m with the given messages sent before quit, capturing output.
// Returns (finalModel, output).
func testProgram(t *testing.T, m Model, width, height int, msgs ...tea.Msg) (Model, []byte) {
	t.Helper()
	var out bytes.Buffer

	p := tea.NewProgram(m,
		tea.WithInput(bytes.NewBuffer(nil)),
		tea.WithOutput(&out),
		tea.WithoutSignals(),
		tea.WithWindowSize(width, height),
	)

	go func() {
		for _, msg := range msgs {
			p.Send(msg)
		}
	}()

	final, err := p.Run()
	if err != nil {
		t.Fatalf("program failed: %v", err)
	}
	fm, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	return fm, out.Bytes()
}

// normalizeGoldenViewSnapshot snapshots the final rendered view instead of the
// raw terminal transcript, which can legitimately vary across Bubble Tea
// renderer passes and race-enabled CI jobs.
func normalizeGoldenViewSnapshot(view string) string {
	view = strings.ReplaceAll(view, "\r\n", "\n")
	view = components.StripAnsi(view)
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func requireViewFitsWidth(t *testing.T, view string, width int) {
	t.Helper()
	for idx, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width %d exceeds budget %d: %q", idx+1, got, width, line)
		}
	}
}

// --- Golden file snapshot tests ---

func TestTeatest_OverviewEmpty(t *testing.T) {
	m := newTestModel(t)
	fm, _ := testProgram(t, m, 120, 40, keyPressMsg('q'))
	golden.RequireEqual(t, normalizeGoldenViewSnapshot(fm.View().Content))
}

func TestTeatest_OverviewWithRepos(t *testing.T) {
	m := newTestModelWithRepos(t)
	fm, _ := testProgram(t, m, 120, 40, keyPressMsg('q'))
	golden.RequireEqual(t, normalizeGoldenViewSnapshot(fm.View().Content))
}

func TestTeatest_HelpView(t *testing.T) {
	m := newTestModel(t)
	m.Nav.CurrentView = ViewHelp
	m.Nav.Breadcrumb.Push("Help")
	fm, _ := testProgram(t, m, 120, 40, keyPressMsg('q'))
	golden.RequireEqual(t, normalizeGoldenViewSnapshot(fm.View().Content))
}

func TestTeatest_RepoDetailView(t *testing.T) {
	m := newTestModelWithRepoDetail(t)
	fm, _ := testProgram(t, m, 84, 28, keyPressMsg('q'))
	view := normalizeGoldenViewSnapshot(fm.View().Content)
	requireViewFitsWidth(t, view, 84)
	golden.RequireEqual(t, view)
}

func TestTeatest_FleetView(t *testing.T) {
	m := newTestModelWithFleetView(t)
	fm, _ := testProgram(t, m, 84, 28, keyPressMsg('q'))
	view := normalizeGoldenViewSnapshot(fm.View().Content)
	requireViewFitsWidth(t, view, 84)
	golden.RequireEqual(t, view)
}

func TestTeatest_SmallTerminal(t *testing.T) {
	m := NewModel(t.TempDir(), nil)
	m.Width = 2
	m.Height = 2
	fm, _ := testProgram(t, m, 2, 2, keyPressMsg('q'))
	golden.RequireEqual(t, normalizeGoldenViewSnapshot(fm.View().Content))
}

// --- Interactive flow tests ---

func TestTeatest_NavigateToHelp(t *testing.T) {
	m := newTestModel(t)
	fm, _ := testProgram(t, m, 120, 40,
		keyPressMsg('?'),
		keyPressMsg('q'),
	)
	if fm.Nav.CurrentView != ViewHelp {
		t.Errorf("expected ViewHelp, got %d", fm.Nav.CurrentView)
	}
}

func TestTeatest_TabSwitching(t *testing.T) {
	m := newTestModel(t)
	fm, _ := testProgram(t, m, 120, 40,
		keyPressMsg('2'),
		keyPressMsg('3'),
		keyPressMsg('q'),
	)
	if fm.Nav.ActiveTab != 2 {
		t.Errorf("expected tab 2 (Teams), got %d", fm.Nav.ActiveTab)
	}
}

func TestTeatest_WindowResize(t *testing.T) {
	m := newTestModel(t)
	// Test resize via direct Update to avoid v2 Send timing races.
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	rm := resized.(Model)
	if rm.Width != 200 || rm.Height != 60 {
		t.Errorf("expected 200x60, got %dx%d", rm.Width, rm.Height)
	}
}

func TestNestedViewsRespectWidthBudgetAfterResize(t *testing.T) {
	tests := []struct {
		name  string
		model Model
	}{
		{name: "repo-detail", model: newTestModelWithRepoDetail(t)},
		{name: "fleet", model: newTestModelWithFleetView(t)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resized, _ := tt.model.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
			rm := resized.(Model)
			view := normalizeGoldenViewSnapshot(rm.View().Content)
			requireViewFitsWidth(t, view, 72)
		})
	}
}

// requiredFinalOutput is kept for backward compatibility with golden tests
// that may call it via RequireEqualOutput.
func RequireEqualOutput(t *testing.T, out []byte) {
	t.Helper()
	golden.RequireEqual(t, normalizeGoldenViewSnapshot(string(out)))
}
