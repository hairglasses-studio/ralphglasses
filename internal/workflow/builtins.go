package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// StepHandler is the function signature for executing a step type.
// It receives the step definition and should return captured output or an error.
type StepHandler func(ctx context.Context, step Step) (output string, err error)

// StepRegistry maps step type names to their handlers.
type StepRegistry struct {
	mu       sync.RWMutex
	handlers map[string]StepHandler
}

// NewStepRegistry creates a registry pre-loaded with built-in step types.
func NewStepRegistry() *StepRegistry {
	r := &StepRegistry{
		handlers: make(map[string]StepHandler),
	}
	r.registerBuiltins()
	return r
}

// Register adds or replaces a handler for the given step type.
func (r *StepRegistry) Register(stepType string, handler StepHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[stepType] = handler
}

// Lookup returns the handler for a step type, or nil if not registered.
func (r *StepRegistry) Lookup(stepType string) StepHandler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[stepType]
}

// Types returns all registered step type names.
func (r *StepRegistry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		out = append(out, k)
	}
	return out
}

func (r *StepRegistry) registerBuiltins() {
	r.handlers["shell_exec"] = shellExec
	r.handlers["git_commit"] = gitCommit
	r.handlers["test_run"] = testRun
	r.handlers["deploy"] = deploy
}

// shellExec runs a shell command via /bin/sh -c and captures combined output.
func shellExec(ctx context.Context, step Step) (string, error) {
	if step.Command == "" {
		return "", fmt.Errorf("shell_exec: command is required")
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", step.Command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// gitCommit stages and commits with a message from params.
func gitCommit(ctx context.Context, step Step) (string, error) {
	msg := step.Params["message"]
	if msg == "" {
		msg = step.Name
	}
	paths := step.Params["paths"]
	if paths == "" {
		paths = "."
	}

	var buf bytes.Buffer

	// Stage
	addArgs := append([]string{"add"}, strings.Fields(paths)...)
	add := exec.CommandContext(ctx, "git", addArgs...)
	add.Stdout = &buf
	add.Stderr = &buf
	if err := add.Run(); err != nil {
		return buf.String(), fmt.Errorf("git add: %w", err)
	}

	// Commit
	commit := exec.CommandContext(ctx, "git", "commit", "-m", msg)
	commit.Stdout = &buf
	commit.Stderr = &buf
	if err := commit.Run(); err != nil {
		return buf.String(), fmt.Errorf("git commit: %w", err)
	}

	return buf.String(), nil
}

// testRun executes `go test` for specified packages.
func testRun(ctx context.Context, step Step) (string, error) {
	pkg := step.Params["package"]
	if pkg == "" {
		pkg = "./..."
	}
	args := []string{"test", "-v", pkg}
	if step.Params["race"] == "true" {
		args = append(args, "-race")
	}
	if step.Params["count"] != "" {
		args = append(args, "-count="+step.Params["count"])
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// deploy is a placeholder that logs the deployment target.
// Real implementations would be registered by the caller.
func deploy(ctx context.Context, step Step) (string, error) {
	target := step.Params["target"]
	if target == "" {
		return "", fmt.Errorf("deploy: target param is required")
	}
	// Placeholder: real deploy logic would be injected via Registry.
	return fmt.Sprintf("deploy to %q: OK (dry-run)", target), nil
}
