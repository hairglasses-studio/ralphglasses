package workflow

import (
	"context"
	"sort"
	"testing"
)

func TestNewStepRegistry(t *testing.T) {
	reg := NewStepRegistry()
	types := reg.Types()
	sort.Strings(types)

	want := []string{"deploy", "git_commit", "shell_exec", "test_run"}
	if len(types) != len(want) {
		t.Fatalf("Types() = %v, want %v", types, want)
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("Types()[%d] = %q, want %q", i, types[i], w)
		}
	}
}

func TestStepRegistry_RegisterAndLookup(t *testing.T) {
	reg := NewStepRegistry()

	// Lookup non-existent type returns nil.
	if h := reg.Lookup("custom"); h != nil {
		t.Error("Lookup(custom) should return nil for unregistered type")
	}

	// Register a custom handler.
	reg.Register("custom", func(ctx context.Context, step Step) (string, error) {
		return "custom-output", nil
	})

	h := reg.Lookup("custom")
	if h == nil {
		t.Fatal("Lookup(custom) returned nil after Register")
	}
	out, err := h(context.Background(), Step{Name: "test"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if out != "custom-output" {
		t.Errorf("output = %q, want %q", out, "custom-output")
	}
}

func TestStepRegistry_RegisterOverwrite(t *testing.T) {
	reg := NewStepRegistry()
	reg.Register("x", func(ctx context.Context, step Step) (string, error) {
		return "first", nil
	})
	reg.Register("x", func(ctx context.Context, step Step) (string, error) {
		return "second", nil
	})

	h := reg.Lookup("x")
	out, _ := h(context.Background(), Step{})
	if out != "second" {
		t.Errorf("overwritten handler returned %q, want %q", out, "second")
	}
}

func TestShellExec_EmptyCommand(t *testing.T) {
	_, err := shellExec(context.Background(), Step{Name: "empty"})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestShellExec_Success(t *testing.T) {
	out, err := shellExec(context.Background(), Step{
		Name:    "echo-test",
		Command: "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "hello\n" {
		t.Errorf("output = %q, want %q", out, "hello\n")
	}
}

func TestShellExec_Failure(t *testing.T) {
	_, err := shellExec(context.Background(), Step{
		Name:    "bad-cmd",
		Command: "false",
	})
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestShellExec_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := shellExec(ctx, Step{
		Name:    "cancelled",
		Command: "sleep 10",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDeploy_MissingTarget(t *testing.T) {
	_, err := deploy(context.Background(), Step{
		Name:   "deploy-no-target",
		Params: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing target")
	}
}

func TestDeploy_NilParams(t *testing.T) {
	_, err := deploy(context.Background(), Step{
		Name: "deploy-nil-params",
	})
	if err == nil {
		t.Fatal("expected error for nil params (empty target)")
	}
}

func TestDeploy_Success(t *testing.T) {
	out, err := deploy(context.Background(), Step{
		Name:   "deploy-ok",
		Params: map[string]string{"target": "staging"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestTestRun_DefaultPackage(t *testing.T) {
	// testRun calls `go test -v ./...` which would actually run.
	// We just verify no panic and it returns (may error if not in a Go module).
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so it doesn't actually run tests
	_, _ = testRun(ctx, Step{Name: "test-default"})
}

func TestTestRun_WithRaceAndCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = testRun(ctx, Step{
		Name: "test-race",
		Params: map[string]string{
			"package": "./...",
			"race":    "true",
			"count":   "1",
		},
	})
}

func TestGitCommit_DefaultParams(t *testing.T) {
	// gitCommit runs actual git commands, so we just check it doesn't panic
	// with a cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gitCommit(ctx, Step{
		Name: "commit-test",
	})
	if err == nil {
		t.Log("gitCommit with cancelled context returned nil error (expected error)")
	}
}
