package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// helper: register a handler that records call order and name.
func echoRegistry() *StepRegistry {
	reg := NewStepRegistry()
	reg.Register("echo", func(ctx context.Context, step Step) (string, error) {
		return "echo:" + step.Name, nil
	})
	return reg
}

func TestLinearWorkflow(t *testing.T) {
	reg := NewStepRegistry()
	var order []string
	var mu sync.Mutex
	reg.Register("track", func(ctx context.Context, step Step) (string, error) {
		mu.Lock()
		order = append(order, step.Name)
		mu.Unlock()
		return step.Name, nil
	})

	wf := &Workflow{
		Name: "linear",
		Steps: []Step{
			{Name: "a", Type: "track"},
			{Name: "b", Type: "track", DependsOn: []string{"a"}},
			{Name: "c", Type: "track", DependsOn: []string{"b"}},
		},
	}

	eng := NewEngineWithRegistry(reg)
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StepSucceeded {
		t.Errorf("status = %v, want succeeded", res.Status)
	}
	if len(order) != 3 {
		t.Fatalf("executed %d steps, want 3", len(order))
	}
	// a must come before b, b before c.
	idxA, idxB, idxC := indexOf(order, "a"), indexOf(order, "b"), indexOf(order, "c")
	if idxA >= idxB || idxB >= idxC {
		t.Errorf("order = %v, want a < b < c", order)
	}
}

func TestParallelFanOut(t *testing.T) {
	reg := NewStepRegistry()
	var running atomic.Int32
	var maxPar atomic.Int32

	reg.Register("par", func(ctx context.Context, step Step) (string, error) {
		cur := running.Add(1)
		for {
			old := maxPar.Load()
			if cur > old {
				if maxPar.CompareAndSwap(old, cur) {
					break
				}
			} else {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		running.Add(-1)
		return step.Name, nil
	})

	// root -> {a, b, c} (parallel fan-out)
	wf := &Workflow{
		Name: "fanout",
		Steps: []Step{
			{Name: "root", Type: "par"},
			{Name: "a", Type: "par", DependsOn: []string{"root"}},
			{Name: "b", Type: "par", DependsOn: []string{"root"}},
			{Name: "c", Type: "par", DependsOn: []string{"root"}},
		},
	}

	eng := NewEngineWithRegistry(reg)
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StepSucceeded {
		t.Errorf("status = %v, want succeeded", res.Status)
	}
	// a, b, c should have run in parallel (max concurrency >= 2).
	if maxPar.Load() < 2 {
		t.Errorf("max parallelism = %d, want >= 2", maxPar.Load())
	}
}

func TestDependencyOrdering(t *testing.T) {
	reg := echoRegistry()

	// Diamond: a -> {b, c} -> d
	wf := &Workflow{
		Name: "diamond",
		Steps: []Step{
			{Name: "a", Type: "echo"},
			{Name: "b", Type: "echo", DependsOn: []string{"a"}},
			{Name: "c", Type: "echo", DependsOn: []string{"a"}},
			{Name: "d", Type: "echo", DependsOn: []string{"b", "c"}},
		},
	}

	eng := NewEngineWithRegistry(reg)
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StepSucceeded {
		t.Errorf("status = %v, want succeeded", res.Status)
	}
	for _, name := range []string{"a", "b", "c", "d"} {
		r, ok := res.Results[name]
		if !ok {
			t.Errorf("missing result for step %q", name)
			continue
		}
		if r.Status != StepSucceeded {
			t.Errorf("step %q status = %v, want succeeded", name, r.Status)
		}
	}
}

func TestStepFailureSkipsDownstream(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("fail", func(ctx context.Context, step Step) (string, error) {
		return "", fmt.Errorf("intentional failure")
	})
	reg.Register("ok", func(ctx context.Context, step Step) (string, error) {
		return "ok", nil
	})

	wf := &Workflow{
		Name: "fail-chain",
		Steps: []Step{
			{Name: "a", Type: "fail"},
			{Name: "b", Type: "ok", DependsOn: []string{"a"}},
		},
	}

	eng := NewEngineWithRegistry(reg)
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StepFailed {
		t.Errorf("workflow status = %v, want failed", res.Status)
	}
	if res.Results["a"].Status != StepFailed {
		t.Errorf("step a status = %v, want failed", res.Results["a"].Status)
	}
	if res.Results["b"].Status != StepSkipped {
		t.Errorf("step b status = %v, want skipped", res.Results["b"].Status)
	}
}

func TestStepRetry(t *testing.T) {
	reg := NewStepRegistry()
	var attempts atomic.Int32
	reg.Register("flaky", func(ctx context.Context, step Step) (string, error) {
		n := attempts.Add(1)
		if n < 3 {
			return "", fmt.Errorf("transient error #%d", n)
		}
		return "success on attempt 3", nil
	})

	wf := &Workflow{
		Name: "retry-test",
		Steps: []Step{
			{
				Name:       "flaky",
				Type:       "flaky",
				RetryCount: 3,
				RetryDelay: time.Millisecond,
			},
		},
	}

	eng := NewEngineWithRegistry(reg)
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StepSucceeded {
		t.Errorf("status = %v, want succeeded", res.Status)
	}
	r := res.Results["flaky"]
	if r.Retries != 2 {
		t.Errorf("retries = %d, want 2", r.Retries)
	}
}

func TestCycleDetection(t *testing.T) {
	wf := &Workflow{
		Name: "cycle",
		Steps: []Step{
			{Name: "a", Type: "echo", DependsOn: []string{"c"}},
			{Name: "b", Type: "echo", DependsOn: []string{"a"}},
			{Name: "c", Type: "echo", DependsOn: []string{"b"}},
		},
	}

	eng := NewEngine()
	_, err := eng.Run(context.Background(), wf)
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want cycle mentioned", err.Error())
	}
}

func TestContextCancellation(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("slow", func(ctx context.Context, step Step) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(10 * time.Second):
			return "done", nil
		}
	})

	wf := &Workflow{
		Name: "cancel",
		Steps: []Step{
			{Name: "slow", Type: "slow"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	eng := NewEngineWithRegistry(reg)
	res, err := eng.Run(ctx, wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Results["slow"].Status != StepFailed {
		t.Errorf("step status = %v, want failed", res.Results["slow"].Status)
	}
}

func TestYAMLParse(t *testing.T) {
	yaml := `
name: ci-pipeline
steps:
  - name: lint
    type: shell_exec
    command: "echo lint"
  - name: test
    type: shell_exec
    command: "echo test"
    depends_on: [lint]
    timeout: "30s"
    retry_count: 2
    retry_delay: "1s"
  - name: build
    type: shell_exec
    command: "echo build"
    depends_on: [test]
`
	wf, err := ParseBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if wf.Name != "ci-pipeline" {
		t.Errorf("name = %q, want ci-pipeline", wf.Name)
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(wf.Steps))
	}
	if wf.Steps[1].Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", wf.Steps[1].Timeout)
	}
	if wf.Steps[1].RetryCount != 2 {
		t.Errorf("retry_count = %d, want 2", wf.Steps[1].RetryCount)
	}
}

func TestShellExecBuiltin(t *testing.T) {
	wf := &Workflow{
		Name: "shell",
		Steps: []Step{
			{Name: "hello", Type: "shell_exec", Command: "echo hello-world"},
		},
	}

	eng := NewEngine()
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := res.Results["hello"].Output
	if !strings.Contains(out, "hello-world") {
		t.Errorf("output = %q, want to contain hello-world", out)
	}
}

func TestUnknownStepType(t *testing.T) {
	wf := &Workflow{
		Name: "unknown",
		Steps: []Step{
			{Name: "x", Type: "nonexistent"},
		},
	}

	eng := NewEngine()
	res, err := eng.Run(context.Background(), wf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Results["x"].Status != StepFailed {
		t.Errorf("status = %v, want failed", res.Results["x"].Status)
	}
}

func TestUnknownDependency(t *testing.T) {
	wf := &Workflow{
		Name: "bad-dep",
		Steps: []Step{
			{Name: "a", Type: "echo", DependsOn: []string{"missing"}},
		},
	}
	eng := NewEngine()
	_, err := eng.Run(context.Background(), wf)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
}

// indexOf returns the index of s in slice, or -1.
func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}
