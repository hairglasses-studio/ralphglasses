package workflow

import "testing"

func TestEvalCondition_Empty(t *testing.T) {
	if !EvalCondition("", nil) {
		t.Error("empty condition should be true")
	}
}

func TestEvalCondition_Always(t *testing.T) {
	if !EvalCondition("always", nil) {
		t.Error("'always' should be true")
	}
}

func TestEvalCondition_Never(t *testing.T) {
	if EvalCondition("never", nil) {
		t.Error("'never' should be false")
	}
}

func TestEvalCondition_StatusCheck(t *testing.T) {
	results := map[string]StepResult{
		"build": {Name: "build", Status: StepSucceeded, Output: "ok"},
		"test":  {Name: "test", Status: StepFailed, Error: "test error"},
	}

	tests := []struct {
		condition string
		want      bool
	}{
		{"build.status == succeeded", true},
		{"build.status == failed", false},
		{"test.status == failed", true},
		{"test.status != succeeded", true},
		{"build.status != succeeded", false},
		{"missing.status == succeeded", false},
	}

	for _, tc := range tests {
		t.Run(tc.condition, func(t *testing.T) {
			got := EvalCondition(tc.condition, results)
			if got != tc.want {
				t.Errorf("EvalCondition(%q) = %v, want %v", tc.condition, got, tc.want)
			}
		})
	}
}

func TestEvalCondition_OutputContains(t *testing.T) {
	results := map[string]StepResult{
		"build": {Name: "build", Status: StepSucceeded, Output: "compiled 42 packages"},
	}

	if !EvalCondition("build.output contains 42 packages", results) {
		t.Error("expected output contains match")
	}
	if EvalCondition("build.output contains error", results) {
		t.Error("expected output contains to not match")
	}
}

func TestEvalCondition_AND(t *testing.T) {
	results := map[string]StepResult{
		"a": {Status: StepSucceeded},
		"b": {Status: StepSucceeded},
		"c": {Status: StepFailed},
	}

	if !EvalCondition("a.status == succeeded && b.status == succeeded", results) {
		t.Error("both succeeded, AND should be true")
	}
	if EvalCondition("a.status == succeeded && c.status == succeeded", results) {
		t.Error("c failed, AND should be false")
	}
}

func TestEvalCondition_OR(t *testing.T) {
	results := map[string]StepResult{
		"a": {Status: StepFailed},
		"b": {Status: StepSucceeded},
	}

	if !EvalCondition("a.status == succeeded || b.status == succeeded", results) {
		t.Error("b succeeded, OR should be true")
	}
	if EvalCondition("a.status == succeeded || a.status == skipped", results) {
		t.Error("a failed, neither match, OR should be false")
	}
}

func TestEvalCondition_NumericComparison(t *testing.T) {
	results := map[string]StepResult{
		"build": {Status: StepSucceeded, Retries: 3},
	}

	if !EvalCondition("build.retries > 2", results) {
		t.Error("3 > 2 should be true")
	}
	if EvalCondition("build.retries < 2", results) {
		t.Error("3 < 2 should be false")
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("step.status == succeeded")
	if len(tokens) != 3 {
		t.Fatalf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
	if tokens[0] != "step.status" || tokens[1] != "==" || tokens[2] != "succeeded" {
		t.Errorf("unexpected tokens: %v", tokens)
	}
}
