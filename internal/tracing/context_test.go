package tracing

import (
	"context"
	"testing"
	"time"
)

func TestWithToolName_Roundtrip(t *testing.T) {
	t.Parallel()
	ctx := WithToolName(context.Background(), "ralphglasses_session_launch")
	got := ToolNameFromContext(ctx)
	if got != "ralphglasses_session_launch" {
		t.Errorf("ToolNameFromContext = %q, want ralphglasses_session_launch", got)
	}
}

func TestToolNameFromContext_Empty(t *testing.T) {
	t.Parallel()
	got := ToolNameFromContext(context.Background())
	if got != "" {
		t.Errorf("ToolNameFromContext on bare context = %q, want empty", got)
	}
}

func TestWithRepo_Roundtrip(t *testing.T) {
	t.Parallel()
	ctx := WithRepo(context.Background(), "ralphglasses")
	got := RepoFromContext(ctx)
	if got != "ralphglasses" {
		t.Errorf("RepoFromContext = %q, want ralphglasses", got)
	}
}

func TestRepoFromContext_Empty(t *testing.T) {
	t.Parallel()
	got := RepoFromContext(context.Background())
	if got != "" {
		t.Errorf("RepoFromContext on bare context = %q, want empty", got)
	}
}

func TestWithRequestStart_Roundtrip(t *testing.T) {
	t.Parallel()
	now := time.Now()
	ctx := WithRequestStart(context.Background(), now)
	got := RequestStartFromContext(ctx)
	if !got.Equal(now) {
		t.Errorf("RequestStartFromContext = %v, want %v", got, now)
	}
}

func TestRequestStartFromContext_Empty(t *testing.T) {
	t.Parallel()
	got := RequestStartFromContext(context.Background())
	if !got.IsZero() {
		t.Errorf("RequestStartFromContext on bare context = %v, want zero", got)
	}
}

func TestRequestLogger_WithAllFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = WithToolName(ctx, "test_tool")
	ctx = WithRepo(ctx, "test_repo")
	ctx = WithTraceID(ctx, "abc123")
	ctx = WithRequestStart(ctx, time.Now().Add(-100*time.Millisecond))

	// Should not panic and return a non-nil logger.
	logger := RequestLogger(ctx)
	if logger == nil {
		t.Error("RequestLogger returned nil")
	}
}

func TestRequestLogger_EmptyContext(t *testing.T) {
	t.Parallel()
	logger := RequestLogger(context.Background())
	if logger == nil {
		t.Error("RequestLogger returned nil for empty context")
	}
}

func TestContextValues_Independent(t *testing.T) {
	t.Parallel()
	// Verify setting one context key does not affect others.
	ctx := WithToolName(context.Background(), "tool1")
	if RepoFromContext(ctx) != "" {
		t.Error("repo should be empty when only tool is set")
	}
	if TraceIDFromContext(ctx) != "" {
		t.Error("trace ID should be empty when only tool is set")
	}
}

func TestContextValues_Stacking(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = WithToolName(ctx, "tool1")
	ctx = WithRepo(ctx, "repo1")
	ctx = WithTraceID(ctx, "trace1")

	if ToolNameFromContext(ctx) != "tool1" {
		t.Error("tool name lost after stacking")
	}
	if RepoFromContext(ctx) != "repo1" {
		t.Error("repo lost after stacking")
	}
	if TraceIDFromContext(ctx) != "trace1" {
		t.Error("trace ID lost after stacking")
	}
}
