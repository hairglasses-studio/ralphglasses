package e2e

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestAggregateSummary_Empty(t *testing.T) {
	s := AggregateSummary(nil)
	if s.TotalObservations != 0 {
		t.Errorf("TotalObservations = %d, want 0", s.TotalObservations)
	}
	if s.CrossScenario != nil {
		t.Error("CrossScenario should be nil for empty input")
	}
	if len(s.PerScenario) != 0 {
		t.Errorf("PerScenario should be empty, got %d entries", len(s.PerScenario))
	}
}

func TestAggregateSummary_Single(t *testing.T) {
	obs := []session.LoopObservation{
		{
			TaskTitle:    "fix-bug",
			TaskType:     "bugfix",
			TotalCostUSD: 0.05,
			TotalLatencyMs: 1500,
			FilesChanged: 3,
			LinesAdded:   20,
			Status:       "idle",
			VerifyPassed: true,
		},
	}

	s := AggregateSummary(obs)
	if s.TotalObservations != 1 {
		t.Errorf("TotalObservations = %d, want 1", s.TotalObservations)
	}

	stat, ok := s.PerScenario["fix-bug"]
	if !ok {
		t.Fatal("missing per-scenario entry for fix-bug")
	}
	if stat.Count != 1 {
		t.Errorf("Count = %d, want 1", stat.Count)
	}
	if stat.AvgCostUSD != 0.05 {
		t.Errorf("AvgCostUSD = %f, want 0.05", stat.AvgCostUSD)
	}
	if stat.AvgLatencyMs != 1500 {
		t.Errorf("AvgLatencyMs = %f, want 1500", stat.AvgLatencyMs)
	}
	if stat.CompletionRate != 1.0 {
		t.Errorf("CompletionRate = %f, want 1.0", stat.CompletionRate)
	}
	if stat.VerifyPassRate != 1.0 {
		t.Errorf("VerifyPassRate = %f, want 1.0", stat.VerifyPassRate)
	}
	if stat.AvgFilesChanged != 3.0 {
		t.Errorf("AvgFilesChanged = %f, want 3.0", stat.AvgFilesChanged)
	}
	if stat.AvgLinesAdded != 20.0 {
		t.Errorf("AvgLinesAdded = %f, want 20.0", stat.AvgLinesAdded)
	}

	if s.CrossScenario == nil {
		t.Fatal("CrossScenario should not be nil")
	}
	if s.CrossScenario.AvgCostUSD != 0.05 {
		t.Errorf("CrossScenario.AvgCostUSD = %f, want 0.05", s.CrossScenario.AvgCostUSD)
	}
	if s.CrossScenario.TaskTypeDist["bugfix"] != 1 {
		t.Errorf("TaskTypeDist[bugfix] = %d, want 1", s.CrossScenario.TaskTypeDist["bugfix"])
	}
}

func TestAggregateSummary_Multiple(t *testing.T) {
	obs := []session.LoopObservation{
		{
			TaskTitle:    "feature-a",
			TaskType:     "feature",
			TotalCostUSD: 0.10,
			TotalLatencyMs: 2000,
			FilesChanged: 4,
			LinesAdded:   40,
			Status:       "idle",
			VerifyPassed: true,
		},
		{
			TaskTitle:    "feature-a",
			TaskType:     "feature",
			TotalCostUSD: 0.20,
			TotalLatencyMs: 3000,
			FilesChanged: 6,
			LinesAdded:   60,
			Status:       "failed",
			Error:        "timeout",
		},
		{
			TaskTitle:    "bugfix-b",
			TaskType:     "bugfix",
			TotalCostUSD: 0.03,
			TotalLatencyMs: 500,
			FilesChanged: 1,
			LinesAdded:   5,
			Status:       "idle",
			VerifyPassed: true,
		},
	}

	s := AggregateSummary(obs)
	if s.TotalObservations != 3 {
		t.Errorf("TotalObservations = %d, want 3", s.TotalObservations)
	}

	fa := s.PerScenario["feature-a"]
	if fa == nil {
		t.Fatal("missing per-scenario entry for feature-a")
	}
	if fa.Count != 2 {
		t.Errorf("feature-a Count = %d, want 2", fa.Count)
	}
	// avg cost = (0.10 + 0.20) / 2 = 0.15
	if diff := fa.AvgCostUSD - 0.15; diff > 0.001 || diff < -0.001 {
		t.Errorf("feature-a AvgCostUSD = %f, want ~0.15", fa.AvgCostUSD)
	}
	// completion: 1 idle out of 2
	if fa.CompletionRate != 0.5 {
		t.Errorf("feature-a CompletionRate = %f, want 0.5", fa.CompletionRate)
	}
	// verify: first passes (VerifyPassed), second fails (status=failed, has error)
	if fa.VerifyPassRate != 0.5 {
		t.Errorf("feature-a VerifyPassRate = %f, want 0.5", fa.VerifyPassRate)
	}

	bb := s.PerScenario["bugfix-b"]
	if bb == nil {
		t.Fatal("missing per-scenario entry for bugfix-b")
	}
	if bb.Count != 1 {
		t.Errorf("bugfix-b Count = %d, want 1", bb.Count)
	}

	// Cross-scenario
	cs := s.CrossScenario
	if cs == nil {
		t.Fatal("CrossScenario should not be nil")
	}
	// avg cost = (0.10 + 0.20 + 0.03) / 3 = 0.11
	wantAvgCost := (0.10 + 0.20 + 0.03) / 3.0
	if diff := cs.AvgCostUSD - wantAvgCost; diff > 0.001 || diff < -0.001 {
		t.Errorf("CrossScenario.AvgCostUSD = %f, want ~%f", cs.AvgCostUSD, wantAvgCost)
	}
	// completion: 2 idle out of 3
	wantComp := 2.0 / 3.0
	if diff := cs.CompletionRate - wantComp; diff > 0.001 || diff < -0.001 {
		t.Errorf("CrossScenario.CompletionRate = %f, want ~%f", cs.CompletionRate, wantComp)
	}

	if cs.TaskTypeDist["feature"] != 2 {
		t.Errorf("TaskTypeDist[feature] = %d, want 2", cs.TaskTypeDist["feature"])
	}
	if cs.TaskTypeDist["bugfix"] != 1 {
		t.Errorf("TaskTypeDist[bugfix] = %d, want 1", cs.TaskTypeDist["bugfix"])
	}
}

func TestAggregateSummary_EmptyTaskTitle(t *testing.T) {
	obs := []session.LoopObservation{
		{TaskTitle: "", TotalCostUSD: 0.01, Status: "idle"},
	}
	s := AggregateSummary(obs)
	if _, ok := s.PerScenario["unknown"]; !ok {
		t.Error("expected empty TaskTitle to be grouped as 'unknown'")
	}
}

func TestAggregateSummary_VerifyPassRate_LenientFormula(t *testing.T) {
	// An observation that is NOT failed and has no error should count as verified
	// even when VerifyPassed is false
	obs := []session.LoopObservation{
		{TaskTitle: "test", Status: "idle", VerifyPassed: false, Error: ""},
	}
	s := AggregateSummary(obs)
	stat := s.PerScenario["test"]
	if stat.VerifyPassRate != 1.0 {
		t.Errorf("VerifyPassRate = %f, want 1.0 (lenient formula)", stat.VerifyPassRate)
	}
}

func TestAggregateSummary_EmptyTaskType(t *testing.T) {
	obs := []session.LoopObservation{
		{TaskTitle: "x", TaskType: ""},
	}
	s := AggregateSummary(obs)
	if len(s.CrossScenario.TaskTypeDist) != 0 {
		t.Errorf("TaskTypeDist should be empty when TaskType is empty, got %v", s.CrossScenario.TaskTypeDist)
	}
}

func TestFormatMarkdown_NonEmpty(t *testing.T) {
	obs := []session.LoopObservation{
		{TaskTitle: "scenario-a", TotalCostUSD: 0.1, TotalLatencyMs: 1000, Status: "idle", VerifyPassed: true},
	}
	s := AggregateSummary(obs)
	md := FormatMarkdown(s)
	if md == "" {
		t.Fatal("FormatMarkdown returned empty string")
	}
	if len(md) < 50 {
		t.Errorf("FormatMarkdown output too short: %d chars", len(md))
	}
}

func TestFormatMarkdown_Empty(t *testing.T) {
	s := AggregateSummary(nil)
	md := FormatMarkdown(s)
	if md == "" {
		t.Fatal("FormatMarkdown returned empty for empty summary")
	}
}

// Suppress unused import warning.
var _ = time.Now
