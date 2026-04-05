package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestClassifySessionKillReason(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sess   *session.Session
		want   string
	}{
		{
			name: "empty_error_and_exit",
			sess: &session.Session{},
			want: "unknown",
		},
		{
			name: "oom",
			sess: &session.Session{Error: "Out of Memory"},
			want: "oom",
		},
		{
			name: "oom_keyword",
			sess: &session.Session{Error: "process OOM killed"},
			want: "oom",
		},
		{
			name: "timeout",
			sess: &session.Session{Error: "context deadline exceeded"},
			want: "timeout",
		},
		{
			name: "timeout_explicit",
			sess: &session.Session{Error: "request timeout after 30s"},
			want: "timeout",
		},
		{
			name: "signal_killed",
			sess: &session.Session{Error: "signal: killed"},
			want: "signal_killed",
		},
		{
			name: "sigkill",
			sess: &session.Session{ExitReason: "SIGKILL received"},
			want: "signal_killed",
		},
		{
			name: "budget_exceeded",
			sess: &session.Session{Error: "budget limit reached"},
			want: "budget_exceeded",
		},
		{
			name: "spend_limit",
			sess: &session.Session{Error: "spend limit exceeded"},
			want: "budget_exceeded",
		},
		{
			name: "rate_limited",
			sess: &session.Session{Error: "rate limit hit on API"},
			want: "rate_limited",
		},
		{
			name: "rate_limited_429",
			sess: &session.Session{Error: "HTTP 429 too many requests"},
			want: "rate_limited",
		},
		{
			name: "other_error",
			sess: &session.Session{Error: "something else happened"},
			want: "other",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifySessionKillReason(tt.sess)
			if got != tt.want {
				t.Errorf("classifySessionKillReason() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifySalvage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sess *session.Session
		want string
	}{
		{
			name: "no_output_no_turns",
			sess: &session.Session{LastOutput: "", TurnCount: 0},
			want: "no_useful_output",
		},
		{
			name: "nearly_complete",
			sess: &session.Session{TurnCount: 9, MaxTurns: 10, LastOutput: "some output"},
			want: "nearly_complete",
		},
		{
			name: "partial_by_keyword_commit",
			sess: &session.Session{TurnCount: 0, LastOutput: "Committed 3 files"},
			want: "partial_completion",
		},
		{
			name: "partial_by_keyword_fixed",
			sess: &session.Session{TurnCount: 0, LastOutput: "Fixed the bug in parser"},
			want: "partial_completion",
		},
		{
			name: "partial_by_keyword_pass",
			sess: &session.Session{TurnCount: 0, LastOutput: "All tests pass now"},
			want: "partial_completion",
		},
		{
			name: "partial_by_turns",
			sess: &session.Session{TurnCount: 3, LastOutput: "working on it"},
			want: "partial_completion",
		},
		{
			name: "no_output_with_turns_zero",
			sess: &session.Session{TurnCount: 0, LastOutput: "nothing relevant"},
			want: "no_useful_output",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := classifySalvage(tt.sess)
			if got != tt.want {
				t.Errorf("classifySalvage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildRecoveryPrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		sess       *session.Session
		assessment string
		wantParts  []string
	}{
		{
			name: "partial_completion",
			sess: &session.Session{
				ID:         "abc12345-full-id",
				RepoName:   "test-repo",
				Status:     session.StatusErrored,
				TurnCount:  5,
				SpentUSD:   1.50,
				Prompt:     "Fix the parser",
				LastOutput: "I was working on the parser refactoring",
			},
			assessment: "partial_completion",
			wantParts:  []string{"<context>", "abc12345", "test-repo", "Last output", "Fix the parser"},
		},
		{
			name: "no_useful_output",
			sess: &session.Session{
				ID:        "def67890-full-id",
				RepoName:  "other-repo",
				Status:    session.StatusInterrupted,
				TurnCount: 0,
				SpentUSD:  0.10,
				Prompt:    "Run tests",
			},
			assessment: "no_useful_output",
			wantParts:  []string{"<context>", "def67890", "other-repo", "Run tests"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := buildRecoveryPrompt(tt.sess, tt.assessment)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("buildRecoveryPrompt() missing %q in output:\n%s", part, got)
				}
			}
		})
	}
}

func TestScorePriority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sess *session.Session
		min  float64
		max  float64
	}{
		{
			name: "baseline",
			sess: &session.Session{SpentUSD: 0.10, TurnCount: 1},
			min:  0.4, max: 0.6,
		},
		{
			name: "high_cost",
			sess: &session.Session{SpentUSD: 5.0, TurnCount: 1},
			min:  0.6, max: 0.8,
		},
		{
			name: "many_turns",
			sess: &session.Session{SpentUSD: 0.10, TurnCount: 10},
			min:  0.6, max: 0.8,
		},
		{
			name: "transient_error",
			sess: &session.Session{SpentUSD: 0.10, TurnCount: 1, Error: "connection reset"},
			min:  0.5, max: 0.7,
		},
		{
			name: "all_factors",
			sess: &session.Session{SpentUSD: 5.0, TurnCount: 10, Error: "timeout"},
			min:  0.9, max: 1.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := scorePriority(tt.sess)
			if got < tt.min || got > tt.max {
				t.Errorf("scorePriority() = %f, want in [%f, %f]", got, tt.min, tt.max)
			}
		})
	}
}

func TestEstimateRetryCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		sess *session.Session
		want float64
	}{
		{
			name: "explicit_budget",
			sess: &session.Session{BudgetUSD: 3.0, SpentUSD: 1.0},
			want: 3.0,
		},
		{
			name: "low_spend_floor",
			sess: &session.Session{SpentUSD: 0.10},
			want: 0.50,
		},
		{
			name: "normal_spend",
			sess: &session.Session{SpentUSD: 2.0},
			want: 2.4,
		},
		{
			name: "high_spend_cap",
			sess: &session.Session{SpentUSD: 50.0},
			want: 10.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := estimateRetryCost(tt.sess)
			if got != tt.want {
				t.Errorf("estimateRetryCost() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestIsTransientError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  string
		want bool
	}{
		{name: "connection_reset", err: "connection reset by peer", want: true},
		{name: "timeout", err: "request Timeout", want: true},
		{name: "rate_limit", err: "rate limit exceeded", want: true},
		{name: "429", err: "HTTP 429", want: true},
		{name: "503", err: "503 Service Unavailable", want: true},
		{name: "permanent_error", err: "invalid configuration", want: false},
		{name: "empty", err: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTransientError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestTruncateStr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{name: "short_string", s: "hello", maxLen: 10, want: "hello"},
		{name: "exact_length", s: "hello", maxLen: 5, want: "hello"},
		{name: "truncated", s: "hello world", maxLen: 8, want: "hello..."},
		{name: "very_short_max", s: "hello world", maxLen: 3, want: "hel"},
		{name: "empty_string", s: "", maxLen: 5, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateStr(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestParseTimeParam(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		s               string
		defaultDuration time.Duration
		wantZero        bool
		wantErr         bool
	}{
		{
			name:            "empty_with_default",
			s:               "",
			defaultDuration: time.Hour,
			wantZero:        false,
		},
		{
			name:            "empty_no_default",
			s:               "",
			defaultDuration: 0,
			wantZero:        true,
		},
		{
			name: "rfc3339",
			s:    "2026-01-01T00:00:00Z",
		},
		{
			name: "days_format",
			s:    "3d",
		},
		{
			name: "go_duration",
			s:    "2h",
		},
		{
			name:    "invalid",
			s:       "not-a-time",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTimeParam(tt.s, tt.defaultDuration)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantZero && !got.IsZero() {
				t.Errorf("expected zero time, got %v", got)
			}
			if !tt.wantZero && tt.s != "" && got.IsZero() {
				t.Errorf("expected non-zero time for input %q", tt.s)
			}
		})
	}
}
