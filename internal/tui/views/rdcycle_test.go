package views

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewRDCycleView(t *testing.T) {
	v := NewRDCycleView()
	if v == nil {
		t.Fatal("NewRDCycleView returned nil")
	}
	if v.Viewport == nil {
		t.Fatal("Viewport should be initialized")
	}
}

func TestRDCycleView_ImplementsView(t *testing.T) {
	var _ View = (*RDCycleView)(nil)
}

func TestRenderRDCycleDashboard_NoCycles(t *testing.T) {
	out := RenderRDCycleDashboard(nil, 80, 30)
	if !strings.Contains(out, "R&D Cycle Dashboard") {
		t.Error("expected dashboard title")
	}
	if !strings.Contains(out, "No R&D cycles found") {
		t.Error("expected no-cycles message")
	}
	if !strings.Contains(out, "cycle_plan") {
		t.Error("expected cycle_plan hint")
	}
}

func TestRenderRDCycleDashboard_EmptySlice(t *testing.T) {
	out := RenderRDCycleDashboard([]*session.CycleRun{}, 80, 30)
	if !strings.Contains(out, "No R&D cycles found") {
		t.Error("expected no-cycles message for empty slice")
	}
}

func TestRenderRDCycleDashboard_ActiveCycleWithAllPanels(t *testing.T) {
	cycles := []*session.CycleRun{
		{
			ID:              "cycle-abc",
			Name:            "coverage-push",
			Phase:           session.CycleExecuting,
			Objective:       "Reach 90% coverage",
			SuccessCriteria: []string{"Coverage >= 90%", "No regressions"},
			Tasks: []session.CycleTask{
				{Title: "Write unit tests", Source: "roadmap", Status: "done", LoopID: "loop-111"},
				{Title: "Fix flaky CI", Source: "finding", Status: "executing", LoopID: "loop-222"},
				{Title: "Lint pass", Source: "manual", Status: "pending"},
			},
			Findings: []session.CycleFinding{
				{ID: "f1", Description: "Flaky test in session pkg", Category: "stability", Severity: "critical"},
				{ID: "f2", Description: "Missing edge case", Category: "quality", Severity: "warning"},
				{ID: "f3", Description: "New helper discovered", Category: "quality", Severity: "info"},
			},
			Synthesis: &session.CycleSynthesis{
				Summary:       "Strong progress on test coverage",
				Accomplished:  []string{"Added 25 tests", "Fixed 2 flaky tests"},
				Remaining:     []string{"Lint cleanup", "Doc updates"},
				NextObjective: "Ship v2.0",
				Patterns:      []string{"TDD yields fewer regressions"},
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 120, 50)

	// Active cycle header
	for _, want := range []string{
		"Active Cycle",
		"coverage-push",
		"executing",
		"Reach 90% coverage",
		"Coverage >= 90%",
		"No regressions",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in active cycle output", want)
		}
	}

	// Progress bar shows task counts
	if !strings.Contains(out, "1/3") {
		t.Error("expected progress 1/3 tasks")
	}

	// Task table
	for _, want := range []string{"Write unit tests", "Fix flaky CI", "Lint pass"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing task %q", want)
		}
	}

	// Findings
	for _, want := range []string{"Flaky test in session pkg", "Missing edge case", "New helper discovered", "stability", "quality"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing finding content %q", want)
		}
	}

	// Synthesis
	for _, want := range []string{
		"Strong progress",
		"Added 25 tests",
		"Lint cleanup",
		"Ship v2.0",
		"TDD yields fewer regressions",
		"Accomplished",
		"Remaining",
		"Next Objective",
		"Patterns",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing synthesis content %q", want)
		}
	}

	// Cycle history
	if !strings.Contains(out, "Cycle History") {
		t.Error("expected cycle history section")
	}

	// Help bar
	if !strings.Contains(out, "j/k") {
		t.Error("expected help bar with scroll keys")
	}
}

func TestRenderRDCycleDashboard_OnlyCompletedCycles(t *testing.T) {
	cycles := []*session.CycleRun{
		{ID: "c1", Name: "done-cycle", Phase: session.CycleComplete, Tasks: []session.CycleTask{{Title: "t1", Status: "done"}}},
		{ID: "c2", Name: "fail-cycle", Phase: session.CycleFailed, Error: "timeout"},
	}

	out := RenderRDCycleDashboard(cycles, 100, 40)

	// No active cycle panel (all are terminal)
	if strings.Contains(out, "Active Cycle") {
		t.Error("should not show active cycle when all cycles are terminal")
	}

	// History should show both
	if !strings.Contains(out, "done-cycle") {
		t.Error("expected completed cycle in history")
	}
	if !strings.Contains(out, "fail-cycle") {
		t.Error("expected failed cycle in history")
	}
}

func TestRenderRDCycleDashboard_ActiveCycleNoTasks(t *testing.T) {
	cycles := []*session.CycleRun{
		{ID: "c1", Name: "empty-tasks", Phase: session.CycleProposed, Objective: "Plan phase"},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if !strings.Contains(out, "No tasks defined") {
		t.Error("expected 'No tasks defined' for cycle with no tasks")
	}
}

func TestRenderRDCycleDashboard_ActiveCycleWithError(t *testing.T) {
	cycles := []*session.CycleRun{
		{ID: "c1", Name: "err-cycle", Phase: session.CycleObserving, Error: "budget exceeded"},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if !strings.Contains(out, "budget exceeded") {
		t.Error("expected error message in output")
	}
}

func TestRenderRDCycleDashboard_FindingsWithEmptyCategory(t *testing.T) {
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "cat-test",
			Phase: session.CycleExecuting,
			Findings: []session.CycleFinding{
				{ID: "f1", Description: "uncategorized issue", Category: "", Severity: "info"},
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if !strings.Contains(out, "general") {
		t.Error("expected empty category to default to 'general'")
	}
	if !strings.Contains(out, "uncategorized issue") {
		t.Error("expected finding description")
	}
}

func TestRenderRDCycleDashboard_CycleHistoryTruncatesTo5(t *testing.T) {
	var cycles []*session.CycleRun
	for i := 0; i < 8; i++ {
		cycles = append(cycles, &session.CycleRun{
			ID:    "c" + string(rune('0'+i)),
			Name:  "cycle-" + string(rune('A'+i)),
			Phase: session.CycleComplete,
		})
	}

	out := RenderRDCycleDashboard(cycles, 100, 40)
	// First 5 should be present
	for i := 0; i < 5; i++ {
		name := "cycle-" + string(rune('A'+i))
		if !strings.Contains(out, name) {
			t.Errorf("expected %q in truncated history", name)
		}
	}
	// 6th, 7th, 8th should NOT be present
	for i := 5; i < 8; i++ {
		name := "cycle-" + string(rune('A'+i))
		if strings.Contains(out, name) {
			t.Errorf("did not expect %q beyond 5-cycle limit", name)
		}
	}
}

func TestRenderRDCycleDashboard_LongCycleNameTruncated(t *testing.T) {
	longName := strings.Repeat("x", 30)
	cycles := []*session.CycleRun{
		{ID: "c1", Name: longName, Phase: session.CycleComplete},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	// The history renderer truncates names > 22 chars
	if strings.Contains(out, longName) {
		t.Error("expected long name to be truncated in history")
	}
}

func TestRenderRDCycleDashboard_TaskTitleTruncation(t *testing.T) {
	longTitle := strings.Repeat("T", 60)
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "trunc-test",
			Phase: session.CycleExecuting,
			Tasks: []session.CycleTask{
				{Title: longTitle, Status: "pending"},
			},
		},
	}

	// With width <= 100, max title is 30
	out := RenderRDCycleDashboard(cycles, 80, 30)
	if strings.Contains(out, longTitle) {
		t.Error("expected task title to be truncated at width 80")
	}

	// With width > 100, max title is 50
	out2 := RenderRDCycleDashboard(cycles, 120, 30)
	if strings.Contains(out2, longTitle) {
		t.Error("expected task title to be truncated at width 120")
	}
}

func TestRenderRDCycleDashboard_TaskSourceAndLoopIDDefaults(t *testing.T) {
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "defaults",
			Phase: session.CycleExecuting,
			Tasks: []session.CycleTask{
				{Title: "no-source-no-loop", Status: "pending", Source: "", LoopID: ""},
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	// Both source and loopID default to "-"
	if !strings.Contains(out, "-") {
		t.Error("expected dash defaults for empty source/loopID")
	}
}

func TestTaskStatusIcon_AllStatuses(t *testing.T) {
	cases := []struct {
		status string
	}{
		{"pending"},
		{"executing"},
		{"done"},
		{"failed"},
		{"unknown"},
		{""},
	}
	for _, tc := range cases {
		icon := taskStatusIcon(tc.status)
		if icon == "" {
			t.Errorf("taskStatusIcon(%q) returned empty string", tc.status)
		}
	}
}

func TestSeverityIcon_AllSeverities(t *testing.T) {
	cases := []struct {
		severity string
	}{
		{"critical"},
		{"warning"},
		{"info"},
		{"unknown"},
		{""},
	}
	for _, tc := range cases {
		icon := severityIcon(tc.severity)
		if icon == "" {
			t.Errorf("severityIcon(%q) returned empty string", tc.severity)
		}
	}
}

func TestPhaseDisplayStyle_AllPhases(t *testing.T) {
	phases := []session.CyclePhase{
		session.CycleComplete,
		session.CycleFailed,
		session.CycleExecuting,
		session.CycleProposed,
		session.CycleBaselining,
		session.CycleObserving,
		session.CycleSynthesizing,
		"unknown",
	}
	for _, p := range phases {
		style := phaseDisplayStyle(p)
		// Just verify it returns a usable style (can render without panic)
		_ = style.Render("test")
	}
}

func TestRDCycleView_SetCycles_Regenerates(t *testing.T) {
	v := NewRDCycleView()
	v.SetDimensions(80, 30)

	// Set empty, then set with data
	v.SetCycles(nil)
	out1 := v.Render()

	v.SetCycles([]*session.CycleRun{
		{ID: "c1", Name: "regen-test", Phase: session.CycleExecuting, Objective: "test regen"},
	})
	out2 := v.Render()

	if out1 == out2 {
		t.Error("expected different output after SetCycles with data")
	}
	if !strings.Contains(out2, "regen-test") {
		t.Error("expected cycle name after SetCycles")
	}
}

func TestRDCycleView_SetDimensions_UpdatesViewport(t *testing.T) {
	v := NewRDCycleView()
	v.SetDimensions(80, 30)
	if v.width != 80 || v.height != 30 {
		t.Errorf("expected 80x30, got %dx%d", v.width, v.height)
	}

	v.SetDimensions(120, 50)
	if v.width != 120 || v.height != 50 {
		t.Errorf("expected 120x50 after update, got %dx%d", v.width, v.height)
	}
}

func TestRenderRDCycleDashboard_SynthesisPartialFields(t *testing.T) {
	// Synthesis with only summary, no other fields
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "partial-synth",
			Phase: session.CycleExecuting,
			Synthesis: &session.CycleSynthesis{
				Summary: "Only summary provided",
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if !strings.Contains(out, "Only summary provided") {
		t.Error("expected partial synthesis summary")
	}
}

func TestRenderRDCycleDashboard_NoFindingsNoSynthesis(t *testing.T) {
	cycles := []*session.CycleRun{
		{
			ID:        "c1",
			Name:      "minimal",
			Phase:     session.CycleExecuting,
			Objective: "minimal cycle",
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if !strings.Contains(out, "Active Cycle") {
		t.Error("expected active cycle header")
	}
	// Should not contain Synthesis header when no synthesis data
	if strings.Contains(out, "Synthesis") {
		t.Error("should not show Synthesis section when nil")
	}
}

func TestRenderRDCycleDashboard_SuccessCriteria(t *testing.T) {
	cycles := []*session.CycleRun{
		{
			ID:              "c1",
			Name:            "criteria-test",
			Phase:           session.CycleExecuting,
			SuccessCriteria: []string{"Pass all tests", "Coverage > 85%"},
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if !strings.Contains(out, "Pass all tests") {
		t.Error("expected success criteria")
	}
	if !strings.Contains(out, "Coverage > 85%") {
		t.Error("expected second success criterion")
	}
}

func TestRenderRDCycleDashboard_LoopIDTruncation(t *testing.T) {
	longLoopID := "loop-abcdefghij123456"
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "loopid-test",
			Phase: session.CycleExecuting,
			Tasks: []session.CycleTask{
				{Title: "task1", Status: "done", LoopID: longLoopID},
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	// LoopID > 8 chars should be truncated to first 8
	if strings.Contains(out, longLoopID) {
		t.Error("expected loopID to be truncated")
	}
	if !strings.Contains(out, longLoopID[:8]) {
		t.Error("expected first 8 chars of loopID")
	}
}

func TestRenderRDCycleDashboard_SourceTruncation(t *testing.T) {
	longSource := "verylongsource"
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "source-test",
			Phase: session.CycleExecuting,
			Tasks: []session.CycleTask{
				{Title: "task1", Status: "pending", Source: longSource},
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 80, 30)
	if strings.Contains(out, longSource) {
		t.Error("expected source to be truncated")
	}
	if !strings.Contains(out, longSource[:8]) {
		t.Error("expected first 8 chars of source")
	}
}

func TestRenderRDCycleDashboard_MultipleFindingCategories(t *testing.T) {
	cycles := []*session.CycleRun{
		{
			ID:    "c1",
			Name:  "multi-cat",
			Phase: session.CycleExecuting,
			Findings: []session.CycleFinding{
				{ID: "f1", Description: "bug A", Category: "bugs", Severity: "critical"},
				{ID: "f2", Description: "perf issue", Category: "performance", Severity: "warning"},
				{ID: "f3", Description: "bug B", Category: "bugs", Severity: "info"},
			},
		},
	}

	out := RenderRDCycleDashboard(cycles, 100, 40)
	if !strings.Contains(out, "bugs") {
		t.Error("expected 'bugs' category")
	}
	if !strings.Contains(out, "performance") {
		t.Error("expected 'performance' category")
	}
	if !strings.Contains(out, "bug A") {
		t.Error("expected first finding in bugs category")
	}
	if !strings.Contains(out, "bug B") {
		t.Error("expected second finding in bugs category")
	}
}

func TestRenderRDCycleDashboard_FirstNonTerminalIsActive(t *testing.T) {
	// First cycle is complete, second is executing -- second should be active
	cycles := []*session.CycleRun{
		{ID: "c1", Name: "done-one", Phase: session.CycleComplete},
		{ID: "c2", Name: "active-one", Phase: session.CycleExecuting, Objective: "this is active"},
	}

	out := RenderRDCycleDashboard(cycles, 100, 40)
	if !strings.Contains(out, "Active Cycle") {
		t.Error("expected active cycle section")
	}
	if !strings.Contains(out, "this is active") {
		t.Error("expected the executing cycle to be shown as active")
	}
}
