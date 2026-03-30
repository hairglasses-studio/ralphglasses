package views

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func newTestForecastView() ForecastView {
	v := NewForecastView()
	v.SetDimensions(100, 40)
	return v
}

func sampleForecastData() ForecastData {
	return ForecastData{
		TotalBudget:    100.0,
		CurrentSpend:   42.50,
		ActiveSessions: 5,
		StartTime:      time.Now().Add(-2 * time.Hour),
		Providers: []ProviderCost{
			{Provider: "claude", CurrentSpend: 30.00, SessionCount: 3},
			{Provider: "gemini", CurrentSpend: 10.00, SessionCount: 4},
			{Provider: "codex", CurrentSpend: 2.50, SessionCount: 1},
		},
		HourlySpends: []HourlySpend{
			{Hour: time.Now().Add(-4 * time.Hour), Amount: 5.00},
			{Hour: time.Now().Add(-3 * time.Hour), Amount: 8.50},
			{Hour: time.Now().Add(-2 * time.Hour), Amount: 12.00},
			{Hour: time.Now().Add(-1 * time.Hour), Amount: 10.00},
			{Hour: time.Now(), Amount: 7.00},
		},
	}
}

func TestNewForecastView(t *testing.T) {
	v := NewForecastView()
	if v.CurrentForecastRange() != ForecastRange1H {
		t.Errorf("expected default ForecastRange1H, got %v", v.CurrentForecastRange())
	}
	if v.Data().TotalBudget != 0 {
		t.Errorf("expected zero budget, got %f", v.Data().TotalBudget)
	}
}

func TestSetData(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	v.SetData(data)

	if v.Data().TotalBudget != 100.0 {
		t.Errorf("expected budget 100, got %f", v.Data().TotalBudget)
	}
	if v.Data().CurrentSpend != 42.50 {
		t.Errorf("expected spend 42.50, got %f", v.Data().CurrentSpend)
	}
	if len(v.Data().Providers) != 3 {
		t.Errorf("expected 3 providers, got %d", len(v.Data().Providers))
	}
}

func TestSetDimensions(t *testing.T) {
	v := NewForecastView()
	v.SetDimensions(120, 50)
	if v.width != 120 || v.height != 50 {
		t.Errorf("expected 120x50, got %dx%d", v.width, v.height)
	}
}

func TestBudgetRemaining(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{TotalBudget: 100, CurrentSpend: 40})
	if got := v.BudgetRemaining(); got != 60 {
		t.Errorf("expected 60, got %f", got)
	}
}

func TestBudgetRemaining_NegativeClamped(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{TotalBudget: 10, CurrentSpend: 50})
	if got := v.BudgetRemaining(); got != 0 {
		t.Errorf("expected 0 when over budget, got %f", got)
	}
}

func TestCostPerSession(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{CurrentSpend: 20.0, ActiveSessions: 4})
	if got := v.CostPerSession(); got != 5.0 {
		t.Errorf("expected 5.0, got %f", got)
	}
}

func TestCostPerSession_ZeroSessions(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{CurrentSpend: 20.0, ActiveSessions: 0})
	if got := v.CostPerSession(); got != 0 {
		t.Errorf("expected 0 for zero sessions, got %f", got)
	}
}

func TestBudgetPercent(t *testing.T) {
	tests := []struct {
		name    string
		budget  float64
		spent   float64
		want    float64
	}{
		{"half", 100, 50, 50},
		{"full", 100, 100, 100},
		{"over", 100, 150, 100},
		{"zero budget", 0, 50, 0},
		{"no spend", 100, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestForecastView()
			v.SetData(ForecastData{TotalBudget: tt.budget, CurrentSpend: tt.spent})
			got := v.BudgetPercent()
			if got != tt.want {
				t.Errorf("BudgetPercent(%f/%f) = %f, want %f", tt.spent, tt.budget, got, tt.want)
			}
		})
	}
}

func TestBudgetAlertLevel(t *testing.T) {
	tests := []struct {
		name  string
		spent float64
		want  string
	}{
		{"below 50", 30, ""},
		{"at 50", 50, "50"},
		{"at 75", 75, "75"},
		{"at 90", 90, "90"},
		{"at 100", 100, "100"},
		{"over 100", 120, "100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newTestForecastView()
			v.SetData(ForecastData{TotalBudget: 100, CurrentSpend: tt.spent})
			got := v.BudgetAlertLevel()
			if got != tt.want {
				t.Errorf("BudgetAlertLevel(spent=%f) = %q, want %q", tt.spent, got, tt.want)
			}
		})
	}
}

func TestProjectedSpend_NoElapsed(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{CurrentSpend: 10.0})
	// No StartTime set — elapsed is 0, should return current spend
	if got := v.ProjectedSpend(); got != 10.0 {
		t.Errorf("expected 10.0 with no elapsed time, got %f", got)
	}
}

func TestCycleForecastRange(t *testing.T) {
	v := newTestForecastView()
	expected := []ForecastRange{ForecastRange4H, ForecastRange12H, ForecastRange24H, ForecastRange1H}
	for _, want := range expected {
		v, _ = v.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
		if v.CurrentForecastRange() != want {
			t.Errorf("expected %v, got %v", want, v.CurrentForecastRange())
		}
	}
}

func TestForecastRangeString(t *testing.T) {
	tests := []struct {
		tr   ForecastRange
		want string
	}{
		{ForecastRange1H, "1h"},
		{ForecastRange4H, "4h"},
		{ForecastRange12H, "12h"},
		{ForecastRange24H, "24h"},
	}
	for _, tt := range tests {
		if got := tt.tr.String(); got != tt.want {
			t.Errorf("ForecastRange(%d).String() = %q, want %q", tt.tr, got, tt.want)
		}
	}
}

func TestForecastRangeHours(t *testing.T) {
	tests := []struct {
		tr   ForecastRange
		want float64
	}{
		{ForecastRange1H, 1},
		{ForecastRange4H, 4},
		{ForecastRange12H, 12},
		{ForecastRange24H, 24},
	}
	for _, tt := range tests {
		if got := tt.tr.Hours(); got != tt.want {
			t.Errorf("ForecastRange(%d).Hours() = %f, want %f", tt.tr, got, tt.want)
		}
	}
}

func TestRenderContainsTitle(t *testing.T) {
	v := newTestForecastView()
	v.SetData(sampleForecastData())
	output := v.Render()
	if !strings.Contains(output, "Cost Forecast") {
		t.Error("render should contain title 'Cost Forecast'")
	}
}

func TestRenderContainsSpend(t *testing.T) {
	v := newTestForecastView()
	v.SetData(sampleForecastData())
	output := v.Render()
	if !strings.Contains(output, "42.50") {
		t.Error("render should contain current spend")
	}
}

func TestRenderContainsProviders(t *testing.T) {
	v := newTestForecastView()
	v.SetData(sampleForecastData())
	output := v.Render()
	if !strings.Contains(output, "claude") {
		t.Error("render should contain provider 'claude'")
	}
	if !strings.Contains(output, "gemini") {
		t.Error("render should contain provider 'gemini'")
	}
	if !strings.Contains(output, "codex") {
		t.Error("render should contain provider 'codex'")
	}
}

func TestRenderContainsHelpLine(t *testing.T) {
	v := newTestForecastView()
	output := v.Render()
	if !strings.Contains(output, "r:refresh") {
		t.Error("render should contain help text")
	}
	if !strings.Contains(output, "t:time range") {
		t.Error("render should contain time range hint")
	}
}

func TestRenderBudgetAlert50(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{TotalBudget: 100, CurrentSpend: 55})
	output := v.Render()
	if !strings.Contains(output, "55.0%") {
		t.Error("should show 55% alert")
	}
}

func TestRenderBudgetAlert90(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{TotalBudget: 100, CurrentSpend: 92})
	output := v.Render()
	if !strings.Contains(output, "CRITICAL") {
		t.Error("90%+ should show CRITICAL alert")
	}
}

func TestRenderBudgetAlert100(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{TotalBudget: 100, CurrentSpend: 105})
	output := v.Render()
	if !strings.Contains(output, "EXCEEDED") {
		t.Error("100%+ should show EXCEEDED alert")
	}
}

func TestRenderNoAlertBelow50(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{TotalBudget: 100, CurrentSpend: 20})
	output := v.Render()
	if strings.Contains(output, "Budget Alerts") {
		t.Error("below 50% should not show budget alerts section")
	}
}

func TestRenderNoSpendData(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{})
	output := v.Render()
	if !strings.Contains(output, "No spend data") {
		t.Error("should show 'No spend data' when empty")
	}
}

func TestSparklineFromValues(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	result := SparklineFromValues(values)
	if len([]rune(result)) != 8 {
		t.Errorf("sparkline should have 8 chars, got %d", len([]rune(result)))
	}
	// First should be lowest bar, last should be highest
	runes := []rune(result)
	if runes[0] != '\u2581' {
		t.Errorf("first bar should be lowest, got %c", runes[0])
	}
	if runes[7] != '\u2588' {
		t.Errorf("last bar should be highest, got %c", runes[7])
	}
}

func TestSparklineFromValues_AllEqual(t *testing.T) {
	values := []float64{5, 5, 5, 5}
	result := SparklineFromValues(values)
	runes := []rune(result)
	if len(runes) != 4 {
		t.Errorf("expected 4 chars, got %d", len(runes))
	}
	// All should be mid-height
	for i, r := range runes {
		if r != '\u2584' {
			t.Errorf("bar %d should be mid-height \\u2584, got %c", i, r)
		}
	}
}

func TestSparklineFromValues_Empty(t *testing.T) {
	result := SparklineFromValues(nil)
	if result != "" {
		t.Errorf("empty input should return empty string, got %q", result)
	}
}

func TestForecastWindowSizeMsg(t *testing.T) {
	v := NewForecastView()
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if v.width != 120 || v.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", v.width, v.height)
	}
}

func TestForecastInit_ReturnsNil(t *testing.T) {
	v := NewForecastView()
	cmd := v.Init()
	if cmd != nil {
		t.Error("Init() should return nil cmd")
	}
}

func TestViewMatchesRender(t *testing.T) {
	v := newTestForecastView()
	v.SetData(sampleForecastData())
	if v.View() != v.Render() {
		t.Error("View() should return same content as Render()")
	}
}

func TestGenerateRecommendations_NoBudget(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{CurrentSpend: 10.0})
	recs := v.generateRecommendations()
	found := false
	for _, r := range recs {
		if strings.Contains(r, "No budget set") {
			found = true
		}
	}
	if !found {
		t.Error("should recommend setting a budget when none is configured")
	}
}

func TestGenerateRecommendations_GeminiSuggestion(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{
		CurrentSpend: 50.0,
		Providers: []ProviderCost{
			{Provider: "claude", CurrentSpend: 50.0, SessionCount: 6},
		},
	})
	recs := v.generateRecommendations()
	found := false
	for _, r := range recs {
		if strings.Contains(r, "Gemini") {
			found = true
		}
	}
	if !found {
		t.Error("should suggest Gemini when only Claude is used with high cost")
	}
}

func TestRenderRecommendations_InOutput(t *testing.T) {
	v := newTestForecastView()
	v.SetData(ForecastData{
		CurrentSpend: 50.0,
		Providers: []ProviderCost{
			{Provider: "claude", CurrentSpend: 50.0, SessionCount: 6},
		},
	})
	output := v.Render()
	if !strings.Contains(output, "Recommendations") {
		t.Error("render should contain Recommendations section")
	}
}

func TestEffectiveWidth_Minimum(t *testing.T) {
	v := NewForecastView()
	v.SetDimensions(20, 10)
	if got := v.effectiveWidth(); got != 80 {
		t.Errorf("expected minimum width 80, got %d", got)
	}
}

func TestEffectiveWidth_Normal(t *testing.T) {
	v := NewForecastView()
	v.SetDimensions(120, 10)
	if got := v.effectiveWidth(); got != 120 {
		t.Errorf("expected width 120, got %d", got)
	}
}

func TestRefreshKey(t *testing.T) {
	v := newTestForecastView()
	var cmd tea.Cmd
	v, cmd = v.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Error("pressing 'r' should return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(ForecastRefreshMsg); !ok {
		t.Errorf("expected ForecastRefreshMsg, got %T", msg)
	}
}
