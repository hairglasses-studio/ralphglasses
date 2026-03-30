package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHookChain_EmptyChain(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, time.Second)
	results := c.Run(context.Background())
	if results != nil {
		t.Errorf("expected nil results for empty chain, got %d", len(results))
	}
}

func TestHookChain_SingleHookSuccess(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{Name: "echo", Command: "echo hello"})
	results := c.Run(context.Background())

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Name != "echo" {
		t.Errorf("name = %q, want echo", r.Name)
	}
	if r.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", r.ExitCode)
	}
	if r.Err != nil {
		t.Errorf("unexpected error: %v", r.Err)
	}
	if got := strings.TrimSpace(r.Stdout); got != "hello" {
		t.Errorf("stdout = %q, want hello", got)
	}
	if r.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestHookChain_StdoutStderrCapture(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{
		Name:    "both",
		Command: "echo out && echo err >&2",
	})
	results := c.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if got := strings.TrimSpace(r.Stdout); got != "out" {
		t.Errorf("stdout = %q, want out", got)
	}
	if got := strings.TrimSpace(r.Stderr); got != "err" {
		t.Errorf("stderr = %q, want err", got)
	}
}

func TestHookChain_FailFast(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{Name: "ok", Command: "echo first"})
	c.Add(Hook{Name: "fail", Command: "exit 1"})
	c.Add(Hook{Name: "skipped", Command: "echo third"})

	results := c.Run(context.Background())
	if len(results) != 2 {
		t.Fatalf("fail-fast should stop after failure; got %d results, want 2", len(results))
	}
	if results[0].ExitCode != 0 {
		t.Error("first hook should succeed")
	}
	if results[1].ExitCode == 0 || results[1].Err == nil {
		t.Error("second hook should fail with exit code != 0")
	}
}

func TestHookChain_ContinueOnError(t *testing.T) {
	t.Parallel()
	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.Add(Hook{Name: "ok1", Command: "echo first"})
	c.Add(Hook{Name: "fail", Command: "exit 42"})
	c.Add(Hook{Name: "ok2", Command: "echo third"})

	results := c.Run(context.Background())
	if len(results) != 3 {
		t.Fatalf("continue-on-error should run all; got %d results, want 3", len(results))
	}
	if results[0].ExitCode != 0 {
		t.Errorf("hook 0: exit code = %d, want 0", results[0].ExitCode)
	}
	if results[1].ExitCode != 42 {
		t.Errorf("hook 1: exit code = %d, want 42", results[1].ExitCode)
	}
	if results[2].ExitCode != 0 {
		t.Errorf("hook 2: exit code = %d, want 0", results[2].ExitCode)
	}
	if got := strings.TrimSpace(results[2].Stdout); got != "third" {
		t.Errorf("hook 2 stdout = %q, want third", got)
	}
}

func TestHookChain_PerHookTimeout(t *testing.T) {
	t.Parallel()
	c := NewHookChain(ContinueOnError, 30*time.Second)
	c.Add(Hook{
		Name:    "slow",
		Command: "sleep 30",
		Timeout: 200 * time.Millisecond,
	})
	c.Add(Hook{Name: "fast", Command: "echo done"})

	start := time.Now()
	results := c.Run(context.Background())
	elapsed := time.Since(start)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// The slow hook should have been killed by its timeout.
	r := results[0]
	if r.Err == nil {
		t.Error("slow hook should have timed out")
	}
	if !strings.Contains(r.Err.Error(), "timed out") {
		t.Errorf("error should mention timeout, got: %v", r.Err)
	}
	if r.ExitCode == 0 {
		t.Error("timed-out hook should have non-zero exit code")
	}

	// The fast hook should still succeed.
	if results[1].ExitCode != 0 {
		t.Errorf("fast hook exit code = %d, want 0", results[1].ExitCode)
	}

	// Total time should be well under the 30s default.
	if elapsed > 5*time.Second {
		t.Errorf("chain took %s; per-hook timeout should have limited it", elapsed)
	}
}

func TestHookChain_DefaultTimeout(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 200*time.Millisecond)
	c.Add(Hook{
		Name:    "slow",
		Command: "sleep 30",
		// No per-hook timeout; should use the 200ms default.
	})

	start := time.Now()
	results := c.Run(context.Background())
	elapsed := time.Since(start)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("hook should have timed out via default timeout")
	}
	if elapsed > 5*time.Second {
		t.Errorf("default timeout not applied; took %s", elapsed)
	}
}

func TestHookChain_DefaultTimeoutFallback(t *testing.T) {
	t.Parallel()
	// Passing zero should fall back to 30s.
	c := NewHookChain(FailFast, 0)
	if c.defaultTimeout != 30*time.Second {
		t.Errorf("defaultTimeout = %s, want 30s", c.defaultTimeout)
	}
}

func TestHookChain_ParentContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.Add(Hook{Name: "a", Command: "echo a"})
	c.Add(Hook{Name: "b", Command: "echo b"})

	results := c.Run(ctx)

	// With a cancelled context, at least the first result should carry the error.
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestHookChain_WorkingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker.txt")

	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{
		Name:    "touch",
		Command: "touch marker.txt",
		Dir:     dir,
	})
	results := c.Run(context.Background())

	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("hook failed: %v", results)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker file not created in working dir: %v", err)
	}
}

func TestHookChain_EnvironmentVariables(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{
		Name:    "env",
		Command: "echo $HOOK_TEST_VAR",
		Env:     []string{"HOOK_TEST_VAR=hello_chain"},
	})
	results := c.Run(context.Background())

	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("hook failed: %v", results)
	}
	if got := strings.TrimSpace(results[0].Stdout); got != "hello_chain" {
		t.Errorf("stdout = %q, want hello_chain", got)
	}
}

func TestHookChain_Parallel(t *testing.T) {
	t.Parallel()
	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.SetParallel(true)

	// Three hooks that each sleep briefly then echo their name.
	// In parallel they should complete faster than in sequence.
	c.Add(Hook{Name: "a", Command: "sleep 0.1 && echo a"})
	c.Add(Hook{Name: "b", Command: "sleep 0.1 && echo b"})
	c.Add(Hook{Name: "c", Command: "sleep 0.1 && echo c"})

	start := time.Now()
	results := c.Run(context.Background())
	elapsed := time.Since(start)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Results should be in order (indexed by position).
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("hook %d (%s) failed: %v", i, r.Name, r.Err)
		}
	}

	// Parallel execution: total time should be closer to 100ms than 300ms.
	// Allow generous headroom for CI.
	if elapsed > 2*time.Second {
		t.Errorf("parallel chain took %s, expected well under 2s", elapsed)
	}
}

func TestHookChain_ParallelWithFailure(t *testing.T) {
	t.Parallel()
	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.SetParallel(true)
	c.Add(Hook{Name: "ok", Command: "echo ok"})
	c.Add(Hook{Name: "fail", Command: "exit 7"})
	c.Add(Hook{Name: "ok2", Command: "echo ok2"})

	results := c.Run(context.Background())
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].ExitCode != 0 {
		t.Errorf("hook 0: exit code = %d, want 0", results[0].ExitCode)
	}
	if results[1].ExitCode != 7 {
		t.Errorf("hook 1: exit code = %d, want 7", results[1].ExitCode)
	}
	if results[2].ExitCode != 0 {
		t.Errorf("hook 2: exit code = %d, want 0", results[2].ExitCode)
	}
}

func TestHookChain_ParallelWithTimeout(t *testing.T) {
	t.Parallel()
	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.SetParallel(true)
	c.Add(Hook{
		Name:    "slow",
		Command: "sleep 30",
		Timeout: 200 * time.Millisecond,
	})
	c.Add(Hook{Name: "fast", Command: "echo fast"})

	start := time.Now()
	results := c.Run(context.Background())
	elapsed := time.Since(start)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("slow hook should have timed out")
	}
	if results[1].ExitCode != 0 {
		t.Errorf("fast hook exit code = %d, want 0", results[1].ExitCode)
	}
	if elapsed > 5*time.Second {
		t.Errorf("chain took %s; timeout should have limited it", elapsed)
	}
}

func TestHookChain_OutputAggregation(t *testing.T) {
	t.Parallel()
	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.Add(Hook{Name: "h1", Command: "echo line1"})
	c.Add(Hook{Name: "h2", Command: "echo line2"})
	c.Add(Hook{Name: "h3", Command: "echo line3"})

	results := c.Run(context.Background())
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify each result has independent output.
	expected := []string{"line1", "line2", "line3"}
	for i, want := range expected {
		got := strings.TrimSpace(results[i].Stdout)
		if got != want {
			t.Errorf("results[%d].Stdout = %q, want %q", i, got, want)
		}
		if results[i].Stderr != "" {
			t.Errorf("results[%d].Stderr = %q, want empty", i, results[i].Stderr)
		}
	}
}

func TestHookChain_NonZeroExitCode(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{Name: "exit99", Command: "exit 99"})

	results := c.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ExitCode != 99 {
		t.Errorf("exit code = %d, want 99", results[0].ExitCode)
	}
	if results[0].Err == nil {
		t.Error("expected error for non-zero exit")
	}
}

func TestHookChain_SequentialOrdering(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outFile := filepath.Join(dir, "order.txt")

	c := NewHookChain(ContinueOnError, 5*time.Second)
	c.Add(Hook{Name: "first", Command: "echo first >> " + outFile})
	c.Add(Hook{Name: "second", Command: "echo second >> " + outFile})
	c.Add(Hook{Name: "third", Command: "echo third >> " + outFile})

	results := c.Run(context.Background())
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("hook %d failed: %v", i, r.Err)
		}
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read order file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{"first", "second", "third"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %v, want %v", lines, want)
	}
	for i, w := range want {
		if strings.TrimSpace(lines[i]) != w {
			t.Errorf("line %d = %q, want %q", i, lines[i], w)
		}
	}
}

func TestHookResult_DurationPositive(t *testing.T) {
	t.Parallel()
	c := NewHookChain(FailFast, 5*time.Second)
	c.Add(Hook{Name: "sleep", Command: "sleep 0.05"})
	results := c.Run(context.Background())
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Duration < 40*time.Millisecond {
		t.Errorf("duration = %s, expected at least 40ms", results[0].Duration)
	}
}
