package session

import (
	"strings"
	"testing"
	"time"
)

func TestNewWeeklyReportGenerator_Default(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(0)
	if g.WindowDays != 7 {
		t.Fatalf("expected 7, got %d", g.WindowDays)
	}
}

func TestNewWeeklyReportGenerator_Custom(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(14)
	if g.WindowDays != 14 {
		t.Fatalf("expected 14, got %d", g.WindowDays)
	}
}

func TestWeeklyReport_EmptySessions(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	report := g.Generate(ReportInput{Sessions: nil, Now: now})
	if report.Summary.TotalSessions != 0 {
		t.Fatalf("expected 0 sessions, got %d", report.Summary.TotalSessions)
	}
	if report.Markdown == "" {
		t.Fatal("expected non-empty markdown")
	}
}

func TestWeeklyReport_WithSessions(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Provider: "claude", TaskType: "feature", Status: "completed", CostUSD: 0.50, DurSec: 30, TurnCount: 5, StartedAt: now.Add(-time.Hour)},
		{ID: "s2", Provider: "claude", TaskType: "fix", Status: "completed", CostUSD: 0.30, DurSec: 20, TurnCount: 3, StartedAt: now.Add(-2 * time.Hour)},
		{ID: "s3", Provider: "gemini", TaskType: "feature", Status: "errored", CostUSD: 1.00, DurSec: 60, TurnCount: 10, StartedAt: now.Add(-3 * time.Hour)},
		{ID: "s4", Provider: "gemini", TaskType: "fix", Status: "stopped", CostUSD: 0.10, DurSec: 5, TurnCount: 1, StartedAt: now.Add(-4 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})

	// Summary checks.
	if report.Summary.TotalSessions != 4 {
		t.Fatalf("expected 4 sessions, got %d", report.Summary.TotalSessions)
	}
	if report.Summary.Completed != 2 {
		t.Fatalf("expected 2 completed, got %d", report.Summary.Completed)
	}
	if report.Summary.Errored != 1 {
		t.Fatalf("expected 1 errored, got %d", report.Summary.Errored)
	}
	if report.Summary.Stopped != 1 {
		t.Fatalf("expected 1 stopped, got %d", report.Summary.Stopped)
	}
	if report.Summary.SuccessRate != 0.5 {
		t.Fatalf("expected 50%% success rate, got %f", report.Summary.SuccessRate)
	}

	// Provider stats.
	if len(report.ByProvider) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(report.ByProvider))
	}

	// Task type stats.
	if len(report.ByTaskType) != 2 {
		t.Fatalf("expected 2 task types, got %d", len(report.ByTaskType))
	}

	// Markdown should contain table headers.
	if !strings.Contains(report.Markdown, "## Summary") {
		t.Fatal("expected markdown to contain Summary section")
	}
	if !strings.Contains(report.Markdown, "## By Provider") {
		t.Fatal("expected markdown to contain By Provider section")
	}
}

func TestWeeklyReport_WindowFiltering(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "recent", Status: "completed", StartedAt: now.Add(-time.Hour)},
		{ID: "old", Status: "completed", StartedAt: now.AddDate(0, 0, -30)}, // outside window
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	if report.Summary.TotalSessions != 1 {
		t.Fatalf("expected 1 session (filtered), got %d", report.Summary.TotalSessions)
	}
}

func TestWeeklyReport_Anomalies_HighErrorRate(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Status: "errored", StartedAt: now.Add(-time.Hour)},
		{ID: "s2", Status: "errored", StartedAt: now.Add(-2 * time.Hour)},
		{ID: "s3", Status: "completed", StartedAt: now.Add(-3 * time.Hour)},
		{ID: "s4", Status: "errored", StartedAt: now.Add(-4 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	foundErrorAnomaly := false
	for _, a := range report.Anomalies {
		if a.Category == "error_rate" {
			foundErrorAnomaly = true
			break
		}
	}
	if !foundErrorAnomaly {
		t.Fatal("expected error_rate anomaly")
	}
}

func TestWeeklyReport_Anomalies_CriticalErrorRate(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Status: "errored", StartedAt: now.Add(-time.Hour)},
		{ID: "s2", Status: "errored", StartedAt: now.Add(-2 * time.Hour)},
		{ID: "s3", Status: "errored", StartedAt: now.Add(-3 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	for _, a := range report.Anomalies {
		if a.Category == "error_rate" && a.Severity != "critical" {
			t.Fatalf("expected critical severity for 100%% error rate, got %s", a.Severity)
		}
	}
}

func TestWeeklyReport_Anomalies_CostSpike(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Status: "completed", CostUSD: 0.10, StartedAt: now.Add(-time.Hour)},
		{ID: "s2", Status: "completed", CostUSD: 0.10, StartedAt: now.Add(-2 * time.Hour)},
		{ID: "s3", Status: "completed", CostUSD: 0.10, StartedAt: now.Add(-3 * time.Hour)},
		{ID: "spike", Status: "completed", CostUSD: 5.00, StartedAt: now.Add(-4 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	foundCostSpike := false
	for _, a := range report.Anomalies {
		if a.Category == "cost_spike" {
			foundCostSpike = true
			break
		}
	}
	if !foundCostSpike {
		t.Fatal("expected cost_spike anomaly")
	}
}

func TestWeeklyReport_Anomalies_Duration(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Status: "completed", DurSec: 10, StartedAt: now.Add(-time.Hour)},
		{ID: "s2", Status: "completed", DurSec: 12, StartedAt: now.Add(-2 * time.Hour)},
		{ID: "s3", Status: "completed", DurSec: 11, StartedAt: now.Add(-3 * time.Hour)},
		{ID: "s4", Status: "completed", DurSec: 10, StartedAt: now.Add(-4 * time.Hour)},
		{ID: "s5", Status: "completed", DurSec: 13, StartedAt: now.Add(-5 * time.Hour)},
		{ID: "s6", Status: "completed", DurSec: 10, StartedAt: now.Add(-6 * time.Hour)},
		{ID: "s7", Status: "completed", DurSec: 11, StartedAt: now.Add(-7 * time.Hour)},
		{ID: "s8", Status: "completed", DurSec: 12, StartedAt: now.Add(-8 * time.Hour)},
		{ID: "slow", Status: "completed", DurSec: 1000, StartedAt: now.Add(-9 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	foundDuration := false
	for _, a := range report.Anomalies {
		if a.Category == "duration" {
			foundDuration = true
			break
		}
	}
	if !foundDuration {
		t.Fatal("expected duration anomaly")
	}
}

func TestWeeklyReport_Recommendations_LowSuccessRate(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Status: "errored", StartedAt: now.Add(-time.Hour)},
		{ID: "s2", Status: "errored", StartedAt: now.Add(-2 * time.Hour)},
		{ID: "s3", Status: "completed", StartedAt: now.Add(-3 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	foundRec := false
	for _, r := range report.Recommendations {
		if strings.Contains(r, "Success rate") {
			foundRec = true
			break
		}
	}
	if !foundRec {
		t.Fatal("expected success rate recommendation")
	}
}

func TestWeeklyReport_Recommendations_ProviderSwitch(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		// Claude: 100% success.
		{ID: "c1", Provider: "claude", Status: "completed", StartedAt: now.Add(-time.Hour)},
		{ID: "c2", Provider: "claude", Status: "completed", StartedAt: now.Add(-2 * time.Hour)},
		{ID: "c3", Provider: "claude", Status: "completed", StartedAt: now.Add(-3 * time.Hour)},
		// Gemini: 0% success.
		{ID: "g1", Provider: "gemini", Status: "errored", StartedAt: now.Add(-4 * time.Hour)},
		{ID: "g2", Provider: "gemini", Status: "errored", StartedAt: now.Add(-5 * time.Hour)},
		{ID: "g3", Provider: "gemini", Status: "errored", StartedAt: now.Add(-6 * time.Hour)},
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	foundSwitch := false
	for _, r := range report.Recommendations {
		if strings.Contains(r, "routing") || strings.Contains(r, "Consider") {
			foundSwitch = true
			break
		}
	}
	if !foundSwitch {
		t.Fatal("expected provider switch recommendation")
	}
}

func TestWeeklyReport_Recommendations_HighCost(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := make([]SessionSummary, 0, 5)
	for i := range 5 {
		sessions = append(sessions, SessionSummary{
			ID:        "s",
			Status:    "completed",
			CostUSD:   5.0,
			StartedAt: now.Add(-time.Duration(i+1) * time.Hour),
		})
	}

	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	foundCost := false
	for _, r := range report.Recommendations {
		if strings.Contains(r, "cost") {
			foundCost = true
			break
		}
	}
	if !foundCost {
		t.Fatal("expected high cost recommendation")
	}
}

func TestWeeklyReport_EmptyProvider(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", Provider: "", Status: "completed", StartedAt: now.Add(-time.Hour)},
	}
	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	if len(report.ByProvider) != 1 || report.ByProvider[0].Provider != "unknown" {
		t.Fatal("expected provider to be 'unknown'")
	}
}

func TestWeeklyReport_EmptyTaskType(t *testing.T) {
	t.Parallel()
	g := NewWeeklyReportGenerator(7)
	now := time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)

	sessions := []SessionSummary{
		{ID: "s1", TaskType: "", Status: "completed", StartedAt: now.Add(-time.Hour)},
	}
	report := g.Generate(ReportInput{Sessions: sessions, Now: now})
	if len(report.ByTaskType) != 1 || report.ByTaskType[0].TaskType != "unclassified" {
		t.Fatal("expected task type to be 'unclassified'")
	}
}

func TestMeanStddev(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		vals    []float64
		wantMu  float64
		wantSig float64
	}{
		{"empty", nil, 0, 0},
		{"single", []float64{5.0}, 5.0, 0.0},
		{"uniform", []float64{2, 2, 2}, 2.0, 0.0},
		{"varied", []float64{2, 4, 4, 4, 5, 5, 7, 9}, 5.0, 2.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu, sig := meanStddev(tt.vals)
			if diff := mu - tt.wantMu; diff < -0.01 || diff > 0.01 {
				t.Fatalf("mean: expected %f, got %f", tt.wantMu, mu)
			}
			if diff := sig - tt.wantSig; diff < -0.01 || diff > 0.01 {
				t.Fatalf("stddev: expected %f, got %f", tt.wantSig, sig)
			}
		})
	}
}
