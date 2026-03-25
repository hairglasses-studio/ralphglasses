package session

import (
	"strings"
	"testing"
)

func TestConvergence(t *testing.T) {
	tests := []struct {
		name       string
		iterations []LoopIteration
		wantConv   bool
		wantReason string // substring match
	}{
		{
			name:       "no iterations",
			iterations: nil,
			wantConv:   false,
		},
		{
			name:       "1 iteration",
			iterations: []LoopIteration{{Status: "idle"}},
			wantConv:   false,
		},
		{
			name: "2 completed iterations with no changes",
			iterations: []LoopIteration{
				{Status: "idle", Acceptance: &AcceptanceResult{}},
				{Status: "idle", Acceptance: &AcceptanceResult{}},
			},
			wantConv:   true,
			wantReason: "no changes produced",
		},
		{
			name: "2 completed iterations with nil acceptance",
			iterations: []LoopIteration{
				{Status: "idle"},
				{Status: "idle"},
			},
			wantConv:   true,
			wantReason: "no changes produced",
		},
		{
			name: "2 completed iterations WITH changes",
			iterations: []LoopIteration{
				{Status: "idle", Acceptance: &AcceptanceResult{SafePaths: []string{"docs/foo.md"}}},
				{Status: "idle", Acceptance: &AcceptanceResult{ReviewPaths: []string{"internal/session/bar.go"}}},
			},
			wantConv: false,
		},
		{
			name: "3 failed iterations with same error",
			iterations: []LoopIteration{
				{Status: "failed", Error: "compile error: undefined foo"},
				{Status: "failed", Error: "compile error: undefined foo"},
				{Status: "failed", Error: "compile error: undefined foo"},
			},
			wantConv:   true,
			wantReason: "repeating error",
		},
		{
			name: "3 failed iterations with different errors",
			iterations: []LoopIteration{
				{Status: "failed", Error: "error A"},
				{Status: "failed", Error: "error B"},
				{Status: "failed", Error: "error C"},
			},
			wantConv: false,
		},
		{
			name: "3 iterations with same task title",
			iterations: []LoopIteration{
				{Status: "idle", Task: LoopTask{Title: "fix linter"}, Acceptance: &AcceptanceResult{SafePaths: []string{"x"}}},
				{Status: "idle", Task: LoopTask{Title: "fix linter"}, Acceptance: &AcceptanceResult{SafePaths: []string{"y"}}},
				{Status: "idle", Task: LoopTask{Title: "fix linter"}, Acceptance: &AcceptanceResult{SafePaths: []string{"z"}}},
			},
			wantConv:   true,
			wantReason: "same task repeated 3 times",
		},
		{
			name: "3 iterations with different task titles",
			iterations: []LoopIteration{
				{Status: "idle", Task: LoopTask{Title: "task A"}, Acceptance: &AcceptanceResult{SafePaths: []string{"x"}}},
				{Status: "idle", Task: LoopTask{Title: "task B"}, Acceptance: &AcceptanceResult{SafePaths: []string{"y"}}},
				{Status: "idle", Task: LoopTask{Title: "task C"}, Acceptance: &AcceptanceResult{SafePaths: []string{"z"}}},
			},
			wantConv: false,
		},
		{
			name: "mixed: 2 failures then 1 success",
			iterations: []LoopIteration{
				{Status: "failed", Error: "same error"},
				{Status: "failed", Error: "same error"},
				{Status: "idle", Acceptance: &AcceptanceResult{SafePaths: []string{"ok.go"}}},
			},
			wantConv: false,
		},
		{
			name: "error prefix truncation: same prefix over 100 chars",
			iterations: []LoopIteration{
				{Status: "failed", Error: strings.Repeat("x", 120) + "aaa"},
				{Status: "failed", Error: strings.Repeat("x", 120) + "bbb"},
				{Status: "failed", Error: strings.Repeat("x", 120) + "ccc"},
			},
			wantConv:   true,
			wantReason: "repeating error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotConv, gotReason := detectConvergence(tt.iterations)
			if gotConv != tt.wantConv {
				t.Errorf("detectConvergence() converged = %v, want %v", gotConv, tt.wantConv)
			}
			if tt.wantReason != "" && !strings.Contains(gotReason, tt.wantReason) {
				t.Errorf("detectConvergence() reason = %q, want substring %q", gotReason, tt.wantReason)
			}
			if !tt.wantConv && gotReason != "" {
				t.Errorf("detectConvergence() reason = %q, want empty for no convergence", gotReason)
			}
		})
	}
}
