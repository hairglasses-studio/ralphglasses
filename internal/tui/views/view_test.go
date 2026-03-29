package views

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Compile-time interface checks.
var (
	_ View = (*ViewportView)(nil)
	_ View = (*RepoDetailView)(nil)
	_ View = (*HelpView)(nil)
	_ View = (*LoopHealthView)(nil)
	_ View = (*SessionDetailView)(nil)
	_ View = (*TeamDetailView)(nil)
	_ View = (*FleetView)(nil)
	_ View = (*DiffViewport)(nil)
	_ View = (*TimelineViewport)(nil)
	_ View = (*LoopDetailView)(nil)
	_ View = (*LoopControlView)(nil)
	_ View = (*ObservationViewport)(nil)
	_ View = (*RDCycleView)(nil)
)

func TestViewportView_SetContent(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 20)
	v.SetContent("hello world")
	out := v.Render()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected content in render, got %q", out)
	}
}

func TestViewportView_Render_Empty(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 20)
	// Should not panic with empty content
	_ = v.Render()
}

func TestViewportView_SetDimensions(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(100, 50)
	if v.width != 100 || v.height != 50 {
		t.Errorf("expected dimensions 100x50, got %dx%d", v.width, v.height)
	}
}

func TestViewportView_SetDimensions_ZeroHeight(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 0)
	// Should clamp to 1
	if v.vp.Height != 1 {
		t.Errorf("expected viewport height 1, got %d", v.vp.Height)
	}
}

func TestViewportView_ScrollUpDown(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 5)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	v.SetContent(strings.Join(lines, "\n"))
	// Scroll down then up — should not panic
	v.ScrollDown()
	v.ScrollDown()
	v.ScrollUp()
}

func TestViewportView_PageUpDown(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 5)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	v.SetContent(strings.Join(lines, "\n"))
	v.PageDown()
	v.PageUp()
}

func TestViewportView_GotoTopBottom(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 5)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	v.SetContent(strings.Join(lines, "\n"))

	v.GotoBottom()
	if !v.AtBottom() {
		t.Error("expected to be at bottom after GotoBottom")
	}

	v.GotoTop()
	if v.ScrollPercent() != 0 {
		t.Errorf("expected scroll percent 0 at top, got %f", v.ScrollPercent())
	}
}

func TestViewportView_AtBottom_Empty(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 20)
	// With no content, should be at bottom
	if !v.AtBottom() {
		t.Error("expected AtBottom to be true for empty content")
	}
}

func TestViewportView_ScrollPercent(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 5)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	v.SetContent(strings.Join(lines, "\n"))

	v.GotoTop()
	pct := v.ScrollPercent()
	if pct != 0 {
		t.Errorf("expected 0%% at top, got %f", pct)
	}

	v.GotoBottom()
	pct = v.ScrollPercent()
	if pct != 1.0 {
		t.Errorf("expected 100%% at bottom, got %f", pct)
	}
}

func TestRepoDetailView_Render(t *testing.T) {
	v := NewRepoDetailView()
	v.SetDimensions(80, 30)
	repo := &model.Repo{
		Name: "test-repo",
		Path: "/tmp/test-repo",
	}
	v.SetData(repo, nil)
	out := v.Render()
	if !strings.Contains(out, "test-repo") {
		t.Error("expected repo name in render output")
	}
}

func TestRepoDetailView_SetDimensions_Regenerates(t *testing.T) {
	v := NewRepoDetailView()
	repo := &model.Repo{
		Name: "test-repo",
		Path: "/tmp/test-repo",
	}
	v.SetData(repo, nil)
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render after SetDimensions")
	}
}

func TestHelpView_Render(t *testing.T) {
	v := NewHelpView()
	v.SetDimensions(80, 30)
	groups := []HelpGroup{
		{
			Name: "Navigation",
			Bindings: []key.Binding{
				key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "repos tab")),
			},
		},
	}
	v.SetData(groups)
	out := v.Render()
	if !strings.Contains(out, "Navigation") {
		t.Error("expected help group name in render output")
	}
}

func TestHelpView_Empty(t *testing.T) {
	v := NewHelpView()
	v.SetDimensions(80, 30)
	// No data set — should not panic
	out := v.Render()
	_ = out
}

func TestLoopHealthView_Render(t *testing.T) {
	v := NewLoopHealthView()
	v.SetDimensions(80, 30)
	data := LoopHealthData{
		RepoName: "test-repo",
		Observations: []session.LoopObservation{
			{
				IterationNumber: 1,
				Status:          "idle",
				TotalCostUSD:    0.05,
				TotalLatencyMs:  1200,
				VerifyPassed:    true,
			},
		},
	}
	v.SetData(data)
	out := v.Render()
	if !strings.Contains(out, "test-repo") {
		t.Error("expected repo name in render output")
	}
}

func TestLoopHealthView_NoData(t *testing.T) {
	v := NewLoopHealthView()
	v.SetDimensions(80, 30)
	v.SetData(LoopHealthData{RepoName: "empty"})
	out := v.Render()
	if !strings.Contains(out, "No loop observations") {
		t.Error("expected no-data message in render output")
	}
}

func TestViewportView_ScrollDown_Sequence(t *testing.T) {
	v := NewViewportView()
	v.SetDimensions(80, 3)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	v.SetContent(strings.Join(lines, "\n"))

	// Scroll down many times — should eventually reach bottom
	for i := 0; i < 25; i++ {
		v.ScrollDown()
	}
	if !v.AtBottom() {
		t.Error("expected to be at bottom after scrolling past content")
	}
}

func TestSessionDetailView_Render(t *testing.T) {
	v := NewSessionDetailView()
	v.SetDimensions(80, 30)
	s := &session.Session{
		ID:       "test-session-123",
		Provider: "claude",
		RepoName: "test-repo",
		Status:   session.StatusRunning,
	}
	v.SetData(s)
	out := v.Render()
	if !strings.Contains(out, "test-session-123") {
		t.Error("expected session ID in render output")
	}
}

func TestSessionDetailView_NilSession(t *testing.T) {
	v := NewSessionDetailView()
	v.SetDimensions(80, 30)
	// No data set — should not panic
	out := v.Render()
	_ = out
}

func TestSessionDetailView_SetDimensions_Regenerates(t *testing.T) {
	v := NewSessionDetailView()
	s := &session.Session{
		ID:       "test-session",
		Provider: "claude",
		RepoName: "test-repo",
		Status:   session.StatusRunning,
	}
	v.SetData(s)
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render after SetDimensions")
	}
}

func TestTeamDetailView_Render(t *testing.T) {
	v := NewTeamDetailView()
	v.SetDimensions(80, 30)
	team := &session.TeamStatus{
		Name:   "test-team",
		Status: session.StatusRunning,
		Tasks: []session.TeamTask{
			{Description: "task 1", Status: "completed"},
		},
	}
	v.SetData(team, nil)
	out := v.Render()
	if !strings.Contains(out, "test-team") {
		t.Error("expected team name in render output")
	}
}

func TestTeamDetailView_NilTeam(t *testing.T) {
	v := NewTeamDetailView()
	v.SetDimensions(80, 30)
	// No data set — should not panic
	out := v.Render()
	_ = out
}

func TestFleetView_Render(t *testing.T) {
	v := NewFleetView()
	v.SetDimensions(120, 40)
	data := FleetData{
		TotalRepos:    3,
		RunningLoops:  1,
		TotalSessions: 2,
	}
	v.SetData(data)
	out := v.Render()
	if !strings.Contains(out, "Fleet Dashboard") {
		t.Error("expected fleet dashboard title in render output")
	}
}

func TestFleetView_Empty(t *testing.T) {
	v := NewFleetView()
	v.SetDimensions(80, 30)
	v.SetData(FleetData{})
	out := v.Render()
	if !strings.Contains(out, "Fleet") {
		t.Error("expected fleet title in render output")
	}
}

func TestDiffViewport_Render(t *testing.T) {
	v := NewDiffViewport()
	v.SetDimensions(80, 30)
	// No data set — should not panic
	out := v.Render()
	_ = out
}

func TestDiffViewport_SetDimensions(t *testing.T) {
	v := NewDiffViewport()
	v.SetDimensions(120, 40)
	// Should not panic with no data
	out := v.Render()
	_ = out
}

func TestTimelineViewport_Render(t *testing.T) {
	v := NewTimelineViewport()
	v.SetDimensions(80, 30)
	entries := []TimelineEntry{
		{
			ID:       "sess-1",
			Provider: "claude",
			Status:   "running",
		},
	}
	v.SetData(entries, "test-repo")
	out := v.Render()
	if !strings.Contains(out, "test-repo") {
		t.Error("expected repo name in render output")
	}
}

func TestTimelineViewport_Empty(t *testing.T) {
	v := NewTimelineViewport()
	v.SetDimensions(80, 30)
	// No data set — should not panic
	out := v.Render()
	_ = out
}

func TestLoopDetailView_Render(t *testing.T) {
	v := NewLoopDetailView()
	v.SetDimensions(80, 30)
	// No data set — should not panic and should show placeholder
	out := v.Render()
	if !strings.Contains(out, "No loop selected") {
		t.Error("expected no-loop message in render output")
	}
}

func TestLoopDetailView_SetDimensions(t *testing.T) {
	v := NewLoopDetailView()
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render after SetDimensions")
	}
}

func TestLoopControlView_Render(t *testing.T) {
	v := NewLoopControlView()
	v.SetDimensions(80, 30)
	v.SetData(nil, 0)
	out := v.Render()
	if !strings.Contains(out, "Loop Control Panel") {
		t.Error("expected loop control title in render output")
	}
}

func TestLoopControlView_WithData(t *testing.T) {
	v := NewLoopControlView()
	v.SetDimensions(100, 40)
	data := []LoopControlData{
		{
			ID:        "loop-123",
			RepoName:  "test-repo",
			Status:    "running",
			IterCount: 5,
		},
	}
	v.SetData(data, 0)
	out := v.Render()
	if !strings.Contains(out, "test-repo") {
		t.Error("expected repo name in render output")
	}
}

func TestObservationViewport_Render(t *testing.T) {
	v := NewObservationViewport()
	v.SetDimensions(80, 30)
	data := ObservationViewData{
		RepoName: "test-repo",
	}
	v.SetData(data)
	out := v.Render()
	if !strings.Contains(out, "test-repo") {
		t.Error("expected repo name in render output")
	}
}

func TestObservationViewport_WithData(t *testing.T) {
	v := NewObservationViewport()
	v.SetDimensions(100, 40)
	data := ObservationViewData{
		RepoName: "test-repo",
		Observations: []session.LoopObservation{
			{
				IterationNumber: 1,
				TotalCostUSD:    0.05,
				TotalLatencyMs:  1200,
				FilesChanged:    3,
			},
		},
	}
	v.SetData(data)
	out := v.Render()
	if !strings.Contains(out, "Observation") {
		t.Error("expected observation title in render output")
	}
}

func TestRDCycleView_Render_Empty(t *testing.T) {
	v := NewRDCycleView()
	v.SetDimensions(80, 30)
	v.SetCycles(nil)
	out := v.Render()
	if !strings.Contains(out, "R&D Cycle Dashboard") {
		t.Error("expected R&D cycle dashboard title in render output")
	}
	if !strings.Contains(out, "No R&D cycles") {
		t.Error("expected no-cycles message in render output")
	}
}

func TestRDCycleView_Render_WithActiveCycle(t *testing.T) {
	v := NewRDCycleView()
	v.SetDimensions(120, 40)
	cycles := []*session.CycleRun{
		{
			ID:        "cycle-1",
			Name:      "test-cycle",
			Phase:     session.CycleExecuting,
			Objective: "Improve coverage",
			Tasks: []session.CycleTask{
				{Title: "Add unit tests", Source: "roadmap", Status: "done", LoopID: "loop-abc"},
				{Title: "Fix flaky test", Source: "finding", Status: "executing", LoopID: "loop-def"},
				{Title: "Lint cleanup", Source: "manual", Status: "pending"},
			},
			Findings: []session.CycleFinding{
				{ID: "f1", Description: "Coverage below 80%", Category: "quality", Severity: "warning"},
				{ID: "f2", Description: "Segfault in parallel tests", Category: "stability", Severity: "critical"},
			},
			Synthesis: &session.CycleSynthesis{
				Summary:       "Good progress on coverage",
				Accomplished:  []string{"Added 20 tests", "Fixed 3 bugs"},
				Remaining:     []string{"Lint cleanup"},
				NextObjective: "Reach 90% coverage",
				Patterns:      []string{"Test-first approach is faster"},
			},
		},
	}
	v.SetCycles(cycles)
	out := v.Render()
	if !strings.Contains(out, "test-cycle") {
		t.Error("expected cycle name in render output")
	}
	if !strings.Contains(out, "Improve coverage") {
		t.Error("expected objective in render output")
	}
	if !strings.Contains(out, "Add unit tests") {
		t.Error("expected task title in render output")
	}
	if !strings.Contains(out, "Coverage below 80%") {
		t.Error("expected finding description in render output")
	}
	if !strings.Contains(out, "Good progress") {
		t.Error("expected synthesis summary in render output")
	}
}

func TestRDCycleView_Render_CompletedCycles(t *testing.T) {
	v := NewRDCycleView()
	v.SetDimensions(100, 40)
	cycles := []*session.CycleRun{
		{ID: "c1", Name: "completed-cycle", Phase: session.CycleComplete, Tasks: []session.CycleTask{{Title: "t1", Status: "done"}}},
		{ID: "c2", Name: "failed-cycle", Phase: session.CycleFailed, Findings: []session.CycleFinding{{ID: "f1", Description: "crash"}}},
	}
	v.SetCycles(cycles)
	out := v.Render()
	if !strings.Contains(out, "Cycle History") {
		t.Error("expected cycle history section in render output")
	}
	if !strings.Contains(out, "completed-cycle") {
		t.Error("expected completed cycle name in render output")
	}
}

func TestRDCycleView_SetDimensions_Regenerates(t *testing.T) {
	v := NewRDCycleView()
	v.SetCycles([]*session.CycleRun{
		{ID: "c1", Name: "dim-test", Phase: session.CycleProposed, Objective: "test"},
	})
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render after SetDimensions")
	}
}
