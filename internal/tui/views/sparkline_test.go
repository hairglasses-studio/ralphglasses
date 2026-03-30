package views

import (
	"strings"
	"testing"
	"time"
)

// ---------- SparklineModel ----------

func TestSparklineModel_Render_Empty(t *testing.T) {
	s := NewSparklineModel(nil)
	result := s.Render()
	if !strings.Contains(result, "no data") {
		t.Errorf("empty sparkline should show '(no data)', got %q", result)
	}
}

func TestSparklineModel_Render_SingleValue(t *testing.T) {
	s := NewSparklineModel([]float64{42})
	result := s.Render()
	// Single value => all equal => mid-height bar
	if len(stripANSI(result)) == 0 {
		t.Error("single value sparkline should render something")
	}
}

func TestSparklineModel_Render_Ascending(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	s := NewSparklineModel(data)
	result := stripANSI(s.Render())
	runes := []rune(result)
	if len(runes) != 8 {
		t.Fatalf("expected 8 chars, got %d: %q", len(runes), result)
	}
	// First should be lowest, last should be highest
	if runes[0] != '\u2581' {
		t.Errorf("first bar should be lowest (\\u2581), got %c", runes[0])
	}
	if runes[7] != '\u2588' {
		t.Errorf("last bar should be highest (\\u2588), got %c", runes[7])
	}
}

func TestSparklineModel_Render_AllEqual(t *testing.T) {
	data := []float64{5, 5, 5, 5}
	s := NewSparklineModel(data)
	result := stripANSI(s.Render())
	runes := []rune(result)
	for i, r := range runes {
		if r != '\u2584' {
			t.Errorf("bar %d should be mid-height (\\u2584), got %c", i, r)
		}
	}
}

func TestSparklineModel_Resample(t *testing.T) {
	data := make([]float64, 100)
	for i := range data {
		data[i] = float64(i)
	}
	s := NewSparklineModel(data)
	s.SetWidth(10)
	result := stripANSI(s.Render())
	runes := []rune(result)
	if len(runes) != 10 {
		t.Errorf("expected 10 chars after resampling, got %d", len(runes))
	}
}

func TestSparklineModel_Resample_WidthZero(t *testing.T) {
	data := []float64{1, 2, 3}
	s := NewSparklineModel(data)
	// Width 0 means no resampling
	result := stripANSI(s.Render())
	if len([]rune(result)) != 3 {
		t.Errorf("expected 3 chars with no resampling, got %d", len([]rune(result)))
	}
}

func TestSparklineModel_AnomalyMarkers(t *testing.T) {
	data := []float64{1, 2, 10, 3, 4} // index 2 is the anomaly
	s := NewSparklineModel(data)
	s.SetAnomalies([]AnomalyMarker{
		{Index: 2, ZScore: 3.5, Message: "spike"},
	})
	result := s.Render()
	// The anomaly should be highlighted — verify it renders without panic
	if len(result) == 0 {
		t.Error("sparkline with anomaly should produce output")
	}
	// The output should contain the highest bar character for the spike
	if !strings.Contains(stripANSI(result), string('\u2588')) {
		t.Error("anomaly at peak should contain highest bar character")
	}
}

func TestSparklineModel_AnomalyOutOfBounds(t *testing.T) {
	data := []float64{1, 2, 3}
	s := NewSparklineModel(data)
	s.SetAnomalies([]AnomalyMarker{
		{Index: -1, ZScore: 5.0},
		{Index: 99, ZScore: 5.0},
	})
	// Should not panic
	result := s.Render()
	if len(result) == 0 {
		t.Error("should render normally with out-of-bounds anomalies")
	}
}

// ---------- TrendIndicator ----------

func TestTrendIndicator_Rising(t *testing.T) {
	for _, trend := range []string{"accelerating", "increasing"} {
		result := TrendIndicator(trend)
		stripped := stripANSI(result)
		if !strings.Contains(stripped, "rising") {
			t.Errorf("TrendIndicator(%q) should contain 'rising', got %q", trend, stripped)
		}
		if !strings.Contains(stripped, "\u2191") {
			t.Errorf("TrendIndicator(%q) should contain up arrow", trend)
		}
	}
}

func TestTrendIndicator_Falling(t *testing.T) {
	for _, trend := range []string{"decelerating", "decreasing"} {
		result := TrendIndicator(trend)
		stripped := stripANSI(result)
		if !strings.Contains(stripped, "falling") {
			t.Errorf("TrendIndicator(%q) should contain 'falling', got %q", trend, stripped)
		}
		if !strings.Contains(stripped, "\u2193") {
			t.Errorf("TrendIndicator(%q) should contain down arrow", trend)
		}
	}
}

func TestTrendIndicator_Stable(t *testing.T) {
	for _, trend := range []string{"stable", "", "unknown"} {
		result := TrendIndicator(trend)
		stripped := stripANSI(result)
		if !strings.Contains(stripped, "stable") {
			t.Errorf("TrendIndicator(%q) should contain 'stable', got %q", trend, stripped)
		}
		if !strings.Contains(stripped, "\u2192") {
			t.Errorf("TrendIndicator(%q) should contain right arrow", trend)
		}
	}
}

// ---------- BudgetProjectionBar ----------

func TestBudgetProjectionBar_Zero(t *testing.T) {
	result := BudgetProjectionBar(0, 20)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "0.0%") {
		t.Errorf("0%% bar should show '0.0%%', got %q", stripped)
	}
}

func TestBudgetProjectionBar_Half(t *testing.T) {
	result := BudgetProjectionBar(50, 20)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "50.0%") {
		t.Errorf("50%% bar should show '50.0%%', got %q", stripped)
	}
	// Should have a mix of filled and empty chars
	filledCount := strings.Count(stripped, "\u2588")
	emptyCount := strings.Count(stripped, "\u2591")
	if filledCount == 0 || emptyCount == 0 {
		t.Errorf("50%% bar should have both filled and empty sections, filled=%d empty=%d",
			filledCount, emptyCount)
	}
}

func TestBudgetProjectionBar_Full(t *testing.T) {
	result := BudgetProjectionBar(100, 20)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "100.0%") {
		t.Errorf("100%% bar should show '100.0%%', got %q", stripped)
	}
}

func TestBudgetProjectionBar_Over100(t *testing.T) {
	result := BudgetProjectionBar(120, 20)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "120.0%") {
		t.Errorf("over-budget bar should show '120.0%%', got %q", stripped)
	}
}

func TestBudgetProjectionBar_MinWidth(t *testing.T) {
	// Width < 10 should be bumped to 20
	result := BudgetProjectionBar(50, 5)
	stripped := stripANSI(result)
	if len(stripped) == 0 {
		t.Error("small width bar should still produce output")
	}
}

// ---------- ExhaustionETA ----------

func TestExhaustionETA_Nil(t *testing.T) {
	result := ExhaustionETA(nil)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "N/A") {
		t.Errorf("nil exhaustion should show 'N/A', got %q", stripped)
	}
}

func TestExhaustionETA_Past(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	result := ExhaustionETA(&past)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "EXHAUSTED") {
		t.Errorf("past exhaustion should show 'EXHAUSTED', got %q", stripped)
	}
}

func TestExhaustionETA_Future_Hours(t *testing.T) {
	future := time.Now().Add(3*time.Hour + 30*time.Minute)
	result := ExhaustionETA(&future)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "3h") {
		t.Errorf("3.5h ETA should contain '3h', got %q", stripped)
	}
}

func TestExhaustionETA_Future_Days(t *testing.T) {
	future := time.Now().Add(49 * time.Hour) // 49h to avoid boundary issues
	result := ExhaustionETA(&future)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "2d") {
		t.Errorf("49h ETA should contain '2d', got %q", stripped)
	}
}

func TestExhaustionETA_Future_Minutes(t *testing.T) {
	future := time.Now().Add(15 * time.Minute)
	result := ExhaustionETA(&future)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "m") {
		t.Errorf("15m ETA should contain 'm', got %q", stripped)
	}
}

func TestExhaustionETA_Future_LessThanMinute(t *testing.T) {
	future := time.Now().Add(30 * time.Second)
	result := ExhaustionETA(&future)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "< 1m") {
		t.Errorf("30s ETA should show '< 1m', got %q", stripped)
	}
}

// ---------- formatETADuration ----------

func TestFormatETADuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "< 1m"},
		{5 * time.Minute, "5m"},
		{90 * time.Minute, "1h 30m"},
		{25 * time.Hour, "1d 1h"},
		{48*time.Hour + 30*time.Minute, "2d 0h"},
	}
	for _, tt := range tests {
		got := formatETADuration(tt.d)
		if got != tt.want {
			t.Errorf("formatETADuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

// ---------- barColorStyle ----------

func TestBarColorStyle_Levels(t *testing.T) {
	// Just ensure it doesn't panic across all levels
	for i := 0; i <= 7; i++ {
		s := barColorStyle(i)
		result := s.Render("X")
		if len(result) == 0 {
			t.Errorf("barColorStyle(%d) should produce output", i)
		}
	}
}

// ---------- Enhanced forecast view integration ----------

func TestRenderContainsBudgetProjection(t *testing.T) {
	v := newTestForecastView()
	v.SetData(sampleForecastData())
	output := v.Render()
	if !strings.Contains(output, "Budget Projection") {
		t.Error("render should contain 'Budget Projection' section")
	}
}

func TestRenderContainsBurnRate(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	data.BurnRatePerHour = 5.25
	data.Trend = "accelerating"
	v.SetData(data)
	output := v.Render()
	if !strings.Contains(output, "Burn Rate") {
		t.Error("render should contain 'Burn Rate' section")
	}
	if !strings.Contains(output, "5.2500") {
		t.Error("render should contain burn rate value")
	}
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "rising") {
		t.Error("render should contain trend indicator 'rising' for accelerating trend")
	}
}

func TestRenderContainsExhaustionETA(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	future := time.Now().Add(6 * time.Hour)
	data.ExhaustionTime = &future
	v.SetData(data)
	output := v.Render()
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "ETA") {
		t.Error("render should contain 'ETA'")
	}
	if !strings.Contains(stripped, "h") {
		t.Error("render should contain hours in ETA")
	}
}

func TestRenderContainsAnomalyCount(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	data.Anomalies = []ForecastAnomaly{
		{BucketIndex: 2, ZScore: 3.5, ActualUSD: 12.0, ExpectedUSD: 7.5},
	}
	v.SetData(data)
	output := v.Render()
	if !strings.Contains(output, "anomalies: 1") {
		t.Error("render should show anomaly count in legend")
	}
}

func TestRenderNoExhaustionTime(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	data.ExhaustionTime = nil
	v.SetData(data)
	output := v.Render()
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "N/A") {
		t.Error("nil exhaustion time should show 'N/A'")
	}
}

func TestRenderStableTrend(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	data.Trend = "stable"
	v.SetData(data)
	output := v.Render()
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "stable") {
		t.Error("render should show 'stable' trend")
	}
}

func TestRenderDeceleratingTrend(t *testing.T) {
	v := newTestForecastView()
	data := sampleForecastData()
	data.Trend = "decelerating"
	v.SetData(data)
	output := v.Render()
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "falling") {
		t.Error("render should show 'falling' for decelerating trend")
	}
}

// ---------- helpers ----------

// stripANSI removes ANSI escape sequences for testing text content.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
