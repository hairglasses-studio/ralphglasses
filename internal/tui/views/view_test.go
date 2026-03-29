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
