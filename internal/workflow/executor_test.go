package workflow

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestStepExecutor_UnknownType(t *testing.T) {
	reg := NewStepRegistry()
	exec := NewStepExecutor(reg)

	result := exec.Execute(context.Background(), Step{
		Name: "unknown",
		Type: "nonexistent",
	})
	if result.Status != StepFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error for unknown step type")
	}
	if result.Name != "unknown" {
		t.Errorf("name = %q, want %q", result.Name, "unknown")
	}
}

func TestStepExecutor_Success(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("ok", func(ctx context.Context, step Step) (string, error) {
		return "output-ok", nil
	})
	exec := NewStepExecutor(reg)

	result := exec.Execute(context.Background(), Step{
		Name: "good-step",
		Type: "ok",
	})
	if result.Status != StepSucceeded {
		t.Errorf("status = %v, want succeeded", result.Status)
	}
	if result.Output != "output-ok" {
		t.Errorf("output = %q, want %q", result.Output, "output-ok")
	}
	if result.Retries != 0 {
		t.Errorf("retries = %d, want 0", result.Retries)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestStepExecutor_Failure(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("fail", func(ctx context.Context, step Step) (string, error) {
		return "partial", fmt.Errorf("boom")
	})
	exec := NewStepExecutor(reg)

	result := exec.Execute(context.Background(), Step{
		Name: "bad-step",
		Type: "fail",
	})
	if result.Status != StepFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
	if result.Error != "boom" {
		t.Errorf("error = %q, want %q", result.Error, "boom")
	}
	if result.Output != "partial" {
		t.Errorf("output = %q, want %q", result.Output, "partial")
	}
}

func TestStepExecutor_RetrySuccess(t *testing.T) {
	reg := NewStepRegistry()
	var calls atomic.Int32
	reg.Register("flaky", func(ctx context.Context, step Step) (string, error) {
		n := calls.Add(1)
		if n < 3 {
			return "", fmt.Errorf("attempt %d failed", n)
		}
		return "ok", nil
	})
	exec := NewStepExecutor(reg)

	result := exec.Execute(context.Background(), Step{
		Name:       "retry-step",
		Type:       "flaky",
		RetryCount: 4,
		RetryDelay: time.Millisecond,
	})
	if result.Status != StepSucceeded {
		t.Errorf("status = %v, want succeeded", result.Status)
	}
	if result.Retries != 2 {
		t.Errorf("retries = %d, want 2", result.Retries)
	}
}

func TestStepExecutor_RetryExhausted(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("always-fail", func(ctx context.Context, step Step) (string, error) {
		return "fail-output", fmt.Errorf("persistent error")
	})
	exec := NewStepExecutor(reg)

	result := exec.Execute(context.Background(), Step{
		Name:       "exhaust",
		Type:       "always-fail",
		RetryCount: 2,
		RetryDelay: time.Millisecond,
	})
	if result.Status != StepFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
	if result.Error != "persistent error" {
		t.Errorf("error = %q, want %q", result.Error, "persistent error")
	}
	if result.Retries != 3 {
		t.Errorf("retries = %d, want 3 (1 initial + 2 retries)", result.Retries)
	}
}

func TestStepExecutor_DefaultTimeout(t *testing.T) {
	reg := NewStepRegistry()
	var gotDeadline bool
	reg.Register("check-deadline", func(ctx context.Context, step Step) (string, error) {
		_, gotDeadline = ctx.Deadline()
		return "ok", nil
	})
	exec := NewStepExecutor(reg)

	exec.Execute(context.Background(), Step{
		Name: "deadline-check",
		Type: "check-deadline",
	})
	if !gotDeadline {
		t.Error("expected context to have a deadline (DefaultTimeout)")
	}
}

func TestStepExecutor_CustomTimeout(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("slow", func(ctx context.Context, step Step) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	})
	exec := NewStepExecutor(reg)

	result := exec.Execute(context.Background(), Step{
		Name:    "timeout-step",
		Type:    "slow",
		Timeout: 50 * time.Millisecond,
	})
	if result.Status != StepFailed {
		t.Errorf("status = %v, want failed (timeout)", result.Status)
	}
}

func TestStepExecutor_ContextCancelledBeforeRun(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("noop", func(ctx context.Context, step Step) (string, error) {
		return "should-not-run", nil
	})
	exec := NewStepExecutor(reg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := exec.Execute(ctx, Step{
		Name: "cancelled",
		Type: "noop",
	})
	if result.Status != StepFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
}

func TestStepExecutor_ContextCancelledDuringRetryDelay(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("fail-once", func(ctx context.Context, step Step) (string, error) {
		return "", fmt.Errorf("fail")
	})
	exec := NewStepExecutor(reg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	result := exec.Execute(ctx, Step{
		Name:       "cancel-during-retry",
		Type:       "fail-once",
		RetryCount: 10,
		RetryDelay: 5 * time.Second, // long delay, context will cancel first
	})
	if result.Status != StepFailed {
		t.Errorf("status = %v, want failed", result.Status)
	}
}

func TestStepExecutor_DefaultRetryDelay(t *testing.T) {
	// Verify that with no RetryDelay set, it uses the 1s default.
	// We don't actually wait 1s; we just ensure the code path doesn't panic.
	reg := NewStepRegistry()
	var calls atomic.Int32
	reg.Register("quick-fail", func(ctx context.Context, step Step) (string, error) {
		if calls.Add(1) == 1 {
			return "", fmt.Errorf("first fail")
		}
		return "ok", nil
	})
	exec := NewStepExecutor(reg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// RetryCount=1, RetryDelay=0 (default 1s) — context should cancel before retry delay completes.
	result := exec.Execute(ctx, Step{
		Name:       "default-delay",
		Type:       "quick-fail",
		RetryCount: 1,
	})
	// Either failed due to context cancel during delay, or succeeded if delay was fast enough.
	// The important thing is no panic.
	_ = result
}
