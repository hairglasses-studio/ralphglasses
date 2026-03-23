package components

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
)

func TestGateVerdictBadge(t *testing.T) {
	tests := []struct {
		verdict  string
		contains string
	}{
		{"pass", "PASS"},
		{"warn", "WARN"},
		{"fail", "FAIL"},
		{"skip", "SKIP"},
	}
	for _, tt := range tests {
		t.Run(tt.verdict, func(t *testing.T) {
			badge := GateVerdictBadge(tt.verdict)
			if badge == "" {
				t.Fatal("badge is empty")
			}
			if !strings.Contains(badge, tt.contains) {
				t.Errorf("badge %q does not contain %q", badge, tt.contains)
			}
		})
	}
}

func TestGateVerdictRow(t *testing.T) {
	row := GateVerdictRow("cost", "warn", 30.5)
	if !strings.Contains(row, "WARN") {
		t.Error("row missing WARN badge")
	}
	if !strings.Contains(row, "cost") {
		t.Error("row missing metric name")
	}
	if !strings.Contains(row, "+30.5%") {
		t.Error("row missing delta")
	}
}

func TestGateReportSummary(t *testing.T) {
	results := []e2e.GateResult{
		{Verdict: e2e.VerdictPass},
		{Verdict: e2e.VerdictPass},
		{Verdict: e2e.VerdictWarn},
		{Verdict: e2e.VerdictFail},
	}
	summary := GateReportSummary(results)
	if !strings.Contains(summary, "2 pass") {
		t.Error("summary missing pass count")
	}
	if !strings.Contains(summary, "1 warn") {
		t.Error("summary missing warn count")
	}
	if !strings.Contains(summary, "1 fail") {
		t.Error("summary missing fail count")
	}
}

func TestGateReportSummaryEmpty(t *testing.T) {
	summary := GateReportSummary(nil)
	if !strings.Contains(summary, "no data") {
		t.Errorf("expected 'no data', got %q", summary)
	}
}

func TestHealthSparkline(t *testing.T) {
	data := []float64{0.1, 0.2, 0.5, 0.8, 1.2}
	result := HealthSparkline(data, 0.9, 10)
	if result == "" {
		t.Fatal("sparkline is empty")
	}
	// Should have some content for each data point
	// Exact rendering depends on terminal, just check non-empty
}

func TestHealthSparklineEmpty(t *testing.T) {
	if HealthSparkline(nil, 1.0, 10) != "" {
		t.Error("expected empty for nil data")
	}
	if HealthSparkline([]float64{1}, 1.0, 0) != "" {
		t.Error("expected empty for zero width")
	}
}
