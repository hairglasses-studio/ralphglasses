package e2e

import (
	"testing"
)

func TestDedupeResults_Nil(t *testing.T) {
	// Should not panic on nil input.
	DedupeResults(nil)
}

func TestDedupeResults_Empty(t *testing.T) {
	report := &GateReport{Results: []GateResult{}}
	DedupeResults(report)
	if len(report.Results) != 0 {
		t.Errorf("DedupeResults empty: len = %d, want 0", len(report.Results))
	}
}

func TestDedupeResults_NoDuplicates(t *testing.T) {
	report := &GateReport{
		Results: []GateResult{
			{Metric: "cost", Verdict: VerdictPass},
			{Metric: "latency", Verdict: VerdictPass},
		},
	}
	DedupeResults(report)
	if len(report.Results) != 2 {
		t.Errorf("DedupeResults no dups: len = %d, want 2", len(report.Results))
	}
}

func TestDedupeResults_WithDuplicates(t *testing.T) {
	report := &GateReport{
		Results: []GateResult{
			{Metric: "cost", Verdict: VerdictPass},
			{Metric: "latency", Verdict: VerdictFail},
			{Metric: "cost", Verdict: VerdictFail}, // duplicate
			{Metric: "error_rate", Verdict: VerdictWarn},
			{Metric: "latency", Verdict: VerdictPass}, // duplicate
		},
	}
	DedupeResults(report)
	if len(report.Results) != 3 {
		t.Errorf("DedupeResults with dups: len = %d, want 3", len(report.Results))
	}
	// First occurrence should be kept.
	if report.Results[0].Verdict != VerdictPass {
		t.Errorf("cost verdict = %v, want pass (first occurrence)", report.Results[0].Verdict)
	}
	if report.Results[1].Verdict != VerdictFail {
		t.Errorf("latency verdict = %v, want fail (first occurrence)", report.Results[1].Verdict)
	}
}

func TestDedupeResults_SingleEntry(t *testing.T) {
	report := &GateReport{
		Results: []GateResult{
			{Metric: "cost", Verdict: VerdictPass},
		},
	}
	DedupeResults(report)
	if len(report.Results) != 1 {
		t.Errorf("DedupeResults single entry: len = %d, want 1", len(report.Results))
	}
}
