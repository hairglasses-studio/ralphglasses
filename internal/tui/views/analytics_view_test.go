package views

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func sampleAnalyticsData() AnalyticsData {
	return AnalyticsData{
		SessionCounts: []SessionTimeBucket{
			{Time: time.Now().Add(-3 * time.Hour), Count: 2},
			{Time: time.Now().Add(-2 * time.Hour), Count: 5},
			{Time: time.Now().Add(-1 * time.Hour), Count: 3},
		},
		CostPerSession: []float64{0.50, 1.20, 0.80, 0.95},
		Providers: []ProviderShare{
			{Provider: "claude", Count: 10},
			{Provider: "gemini", Count: 5},
			{Provider: "codex", Count: 2},
		},
		Succeeded: 12,
		Failed:    3,
		Running:   2,
		Total:     17,
		TotalCost: 8.50,
	}
}

func TestRenderAnalytics_Title(t *testing.T) {
	out := RenderAnalytics(AnalyticsData{}, PanelSessionCount, TimeRange24h, 120, 40)
	if !strings.Contains(out, "Analytics Dashboard") {
		t.Fatal("output should contain title 'Analytics Dashboard'")
	}
}

func TestRenderAnalytics_TimeRangeLabels(t *testing.T) {
	out := RenderAnalytics(AnalyticsData{}, PanelSessionCount, TimeRange7d, 120, 40)
	for _, label := range []string{"1h", "24h", "7d", "30d"} {
		if !strings.Contains(out, label) {
			t.Errorf("output missing time range label %q", label)
		}
	}
}

func TestRenderAnalytics_StatBoxes(t *testing.T) {
	data := sampleAnalyticsData()
	out := RenderAnalytics(data, PanelSessionCount, TimeRange24h, 120, 40)

	if !strings.Contains(out, "17 total") {
		t.Error("output should show total session count")
	}
	if !strings.Contains(out, "$8.50") {
		t.Error("output should show total cost")
	}
	if !strings.Contains(out, "SUCCESS") {
		t.Error("output should show success rate header")
	}
}

func TestRenderAnalytics_SessionCountSparkline(t *testing.T) {
	data := sampleAnalyticsData()
	out := RenderAnalytics(data, PanelSessionCount, TimeRange24h, 120, 40)
	if !strings.Contains(out, "Session Count Over Time") {
		t.Error("output should contain session count panel header")
	}
	if !strings.Contains(out, "latest: 3") {
		t.Error("output should show latest session count")
	}
}

func TestRenderAnalytics_CostPerSession(t *testing.T) {
	data := sampleAnalyticsData()
	out := RenderAnalytics(data, PanelCostPerSession, TimeRange24h, 120, 40)
	if !strings.Contains(out, "Cost Per Session") {
		t.Error("output should contain cost panel header")
	}
	if !strings.Contains(out, "$0.9500 latest") {
		t.Error("output should show latest cost per session")
	}
}

func TestRenderAnalytics_ProviderDistribution(t *testing.T) {
	data := sampleAnalyticsData()
	out := RenderAnalytics(data, PanelProviderDist, TimeRange24h, 120, 40)
	if !strings.Contains(out, "Provider Distribution") {
		t.Error("output should contain provider distribution header")
	}
	if !strings.Contains(out, "claude") {
		t.Error("output should contain provider 'claude'")
	}
	if !strings.Contains(out, "gemini") {
		t.Error("output should contain provider 'gemini'")
	}
	if !strings.Contains(out, "codex") {
		t.Error("output should contain provider 'codex'")
	}
}

func TestRenderAnalytics_SuccessFailure(t *testing.T) {
	data := sampleAnalyticsData()
	out := RenderAnalytics(data, PanelSuccessRate, TimeRange24h, 120, 40)
	if !strings.Contains(out, "Success / Failure") {
		t.Error("output should contain success/failure panel header")
	}
	if !strings.Contains(out, "succeeded") {
		t.Error("output should show succeeded label")
	}
	if !strings.Contains(out, "failed") {
		t.Error("output should show failed label")
	}
	if !strings.Contains(out, "running") {
		t.Error("output should show running label")
	}
}

func TestRenderAnalytics_EmptyData(t *testing.T) {
	out := RenderAnalytics(AnalyticsData{}, PanelSessionCount, TimeRange24h, 120, 40)
	if !strings.Contains(out, "(no data)") {
		t.Error("empty data should show '(no data)' placeholder")
	}
	if !strings.Contains(out, "0 total") {
		t.Error("empty data should show 0 total sessions")
	}
}

func TestRenderAnalytics_HelpFooter(t *testing.T) {
	out := RenderAnalytics(AnalyticsData{}, PanelSessionCount, TimeRange24h, 120, 40)
	if !strings.Contains(out, "Tab:panel") {
		t.Error("output should contain help text for tab")
	}
	if !strings.Contains(out, "r:refresh") {
		t.Error("output should contain help text for refresh")
	}
}

func TestRenderAnalytics_ZeroDimensions(t *testing.T) {
	out := RenderAnalytics(sampleAnalyticsData(), PanelSessionCount, TimeRange24h, 0, 0)
	if out == "" {
		t.Error("should render even with zero dimensions")
	}
}

func TestRenderAnalytics_NarrowWidth(t *testing.T) {
	out := RenderAnalytics(sampleAnalyticsData(), PanelSessionCount, TimeRange24h, 30, 20)
	if !strings.Contains(out, "Analytics Dashboard") {
		t.Error("narrow render should still contain title")
	}
}

// --- AnalyticsView struct tests ---

func TestNewAnalyticsView(t *testing.T) {
	v := NewAnalyticsView()
	if v == nil {
		t.Fatal("NewAnalyticsView returned nil")
	}
	if v.Viewport == nil {
		t.Fatal("Viewport should not be nil")
	}
	if v.TimeRange() != TimeRange24h {
		t.Errorf("default time range = %v, want 24h", v.TimeRange())
	}
	if v.Panel() != PanelSessionCount {
		t.Errorf("default panel = %v, want SessionCount", v.Panel())
	}
}

func TestAnalyticsView_SetData(t *testing.T) {
	v := NewAnalyticsView()
	v.SetDimensions(120, 40)
	v.SetData(sampleAnalyticsData())

	out := v.Render()
	if !strings.Contains(out, "Analytics Dashboard") {
		t.Error("rendered view should contain title after SetData")
	}
	if !strings.Contains(out, "17 total") {
		t.Error("rendered view should show total sessions")
	}
}

func TestAnalyticsView_SetDimensions(t *testing.T) {
	v := NewAnalyticsView()
	v.SetDimensions(80, 24)

	// Render should not panic even without data.
	out := v.Render()
	if out == "" {
		// Viewport may be empty without data, that's acceptable.
		_ = out
	}
}

func TestAnalyticsView_HandleKey_Tab(t *testing.T) {
	v := NewAnalyticsView()
	v.SetDimensions(120, 40)

	if v.Panel() != PanelSessionCount {
		t.Fatal("initial panel should be SessionCount")
	}

	handled, _ := v.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if !handled {
		t.Fatal("tab should be handled")
	}
	if v.Panel() != PanelCostPerSession {
		t.Errorf("panel = %v, want CostPerSession after tab", v.Panel())
	}

	v.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if v.Panel() != PanelProviderDist {
		t.Errorf("panel = %v, want ProviderDist after second tab", v.Panel())
	}

	v.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if v.Panel() != PanelSuccessRate {
		t.Errorf("panel = %v, want SuccessRate after third tab", v.Panel())
	}

	// Wrap around
	v.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if v.Panel() != PanelSessionCount {
		t.Errorf("panel = %v, want SessionCount after wrap", v.Panel())
	}
}

func TestAnalyticsView_HandleKey_ShiftTab(t *testing.T) {
	v := NewAnalyticsView()
	v.SetDimensions(120, 40)

	// Reverse wrap
	handled, _ := v.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !handled {
		t.Fatal("shift+tab should be handled")
	}
	if v.Panel() != PanelSuccessRate {
		t.Errorf("panel = %v, want SuccessRate after shift+tab from first", v.Panel())
	}
}

func TestAnalyticsView_HandleKey_Refresh(t *testing.T) {
	v := NewAnalyticsView()
	handled, cmd := v.HandleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if !handled {
		t.Fatal("r should be handled")
	}
	if cmd == nil {
		t.Fatal("r should produce a cmd")
	}
	msg := cmd()
	if _, ok := msg.(AnalyticsRefreshMsg); !ok {
		t.Fatalf("expected AnalyticsRefreshMsg, got %T", msg)
	}
}

func TestAnalyticsView_HandleKey_TimeRange(t *testing.T) {
	v := NewAnalyticsView()
	v.SetDimensions(120, 40)

	// Default is 24h (index 1)
	if v.TimeRange() != TimeRange24h {
		t.Fatal("default should be 24h")
	}

	// ] moves forward
	handled, _ := v.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if !handled {
		t.Fatal("] should be handled")
	}
	if v.TimeRange() != TimeRange7d {
		t.Errorf("time range = %v, want 7d", v.TimeRange())
	}

	// [ moves backward
	handled, _ = v.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if !handled {
		t.Fatal("[ should be handled")
	}
	if v.TimeRange() != TimeRange24h {
		t.Errorf("time range = %v, want 24h", v.TimeRange())
	}

	// [ again
	v.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if v.TimeRange() != TimeRange1h {
		t.Errorf("time range = %v, want 1h", v.TimeRange())
	}

	// [ at min does not go below
	v.HandleKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if v.TimeRange() != TimeRange1h {
		t.Errorf("time range = %v, want 1h (clamped)", v.TimeRange())
	}

	// Go to max
	v.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	v.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	v.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if v.TimeRange() != TimeRange30d {
		t.Errorf("time range = %v, want 30d", v.TimeRange())
	}

	// ] at max does not go above
	v.HandleKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if v.TimeRange() != TimeRange30d {
		t.Errorf("time range = %v, want 30d (clamped)", v.TimeRange())
	}
}

func TestAnalyticsView_HandleKey_Unhandled(t *testing.T) {
	v := NewAnalyticsView()
	handled, cmd := v.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if handled {
		t.Error("x should not be handled")
	}
	if cmd != nil {
		t.Error("unhandled key should not produce a cmd")
	}
}

// --- TimeRange tests ---

func TestTimeRange_String(t *testing.T) {
	tests := []struct {
		tr   TimeRange
		want string
	}{
		{TimeRange1h, "1h"},
		{TimeRange24h, "24h"},
		{TimeRange7d, "7d"},
		{TimeRange30d, "30d"},
		{TimeRange(99), "?"},
	}
	for _, tt := range tests {
		if got := tt.tr.String(); got != tt.want {
			t.Errorf("TimeRange(%d).String() = %q, want %q", tt.tr, got, tt.want)
		}
	}
}

func TestTimeRange_Duration(t *testing.T) {
	if d := TimeRange1h.Duration(); d != time.Hour {
		t.Errorf("1h duration = %v", d)
	}
	if d := TimeRange24h.Duration(); d != 24*time.Hour {
		t.Errorf("24h duration = %v", d)
	}
	if d := TimeRange7d.Duration(); d != 7*24*time.Hour {
		t.Errorf("7d duration = %v", d)
	}
	if d := TimeRange30d.Duration(); d != 30*24*time.Hour {
		t.Errorf("30d duration = %v", d)
	}
	if d := TimeRange(99).Duration(); d != time.Hour {
		t.Errorf("unknown duration = %v, want 1h fallback", d)
	}
}

// --- AnalyticsPanel tests ---

func TestAnalyticsPanel_String(t *testing.T) {
	tests := []struct {
		p    AnalyticsPanel
		want string
	}{
		{PanelSessionCount, "Session Count"},
		{PanelCostPerSession, "Cost / Session"},
		{PanelProviderDist, "Provider Distribution"},
		{PanelSuccessRate, "Success / Failure"},
		{AnalyticsPanel(99), "?"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("AnalyticsPanel(%d).String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

// --- renderProviderBars edge cases ---

func TestRenderProviderBars_Empty(t *testing.T) {
	out := renderProviderBars(nil, 20)
	if out != "" {
		t.Errorf("empty providers should produce empty string, got %q", out)
	}
}

func TestRenderProviderBars_ZeroWidth(t *testing.T) {
	providers := []ProviderShare{{Provider: "claude", Count: 5}}
	out := renderProviderBars(providers, 0)
	if !strings.Contains(out, "claude") {
		t.Error("should still render provider name with zero width (uses fallback)")
	}
}

func TestRenderProviderBars_SingleProvider(t *testing.T) {
	providers := []ProviderShare{{Provider: "claude", Count: 10}}
	out := renderProviderBars(providers, 10)
	if !strings.Contains(out, "claude") {
		t.Error("should contain provider name")
	}
	if !strings.Contains(out, "10") {
		t.Error("should contain count")
	}
}

// --- renderSuccessFailure edge cases ---

func TestRenderSuccessFailure_ZeroTotal(t *testing.T) {
	out := renderSuccessFailure(AnalyticsData{}, 20)
	if !strings.Contains(out, "(no data)") {
		t.Error("zero total should show '(no data)'")
	}
}

func TestRenderSuccessFailure_WithData(t *testing.T) {
	data := AnalyticsData{Succeeded: 8, Failed: 2, Running: 1, Total: 11}
	out := renderSuccessFailure(data, 20)
	if !strings.Contains(out, "succeeded") {
		t.Error("should show succeeded")
	}
	if !strings.Contains(out, "failed") {
		t.Error("should show failed")
	}
}

// --- Active panel marker ---

func TestRenderAnalytics_ActivePanelMarker(t *testing.T) {
	data := sampleAnalyticsData()

	// When PanelProviderDist is active, the ">" marker should appear before its header.
	out := RenderAnalytics(data, PanelProviderDist, TimeRange24h, 120, 40)
	// The ">" marker is rendered with SelectedStyle, so look for the header text with ">" nearby.
	// Since lipgloss adds ANSI codes, we check the raw output contains ">" before "Provider Distribution".
	found := strings.Contains(out, "Provider Distribution")
	if !found {
		t.Fatal("output should contain Provider Distribution header")
	}
	// The marker is rendered a few chars before the header; just verify it appears somewhere.
	if !strings.Contains(out, ">") {
		t.Error("active panel should show '>' marker")
	}
}
