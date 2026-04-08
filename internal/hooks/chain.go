package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// HookResult captures the outcome of a single hook execution.
type HookResult struct {
	Name     string        `json:"name"`
	ExitCode int           `json:"exit_code"`
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	Duration time.Duration `json:"duration"`
	Err      error         `json:"error,omitempty"`
}

// ChainMode controls how a HookChain handles failures.
type ChainMode int

const (
	// FailFast stops execution after the first hook failure.
	FailFast ChainMode = iota
	// ContinueOnError runs all hooks regardless of individual failures.
	ContinueOnError
)

// Hook defines a single executable unit in a chain.
type Hook struct {
	Name    string
	Command string
	Dir     string        // working directory
	Env     []string      // additional environment variables
	Timeout time.Duration // per-hook timeout; 0 means use chain default
}

// HookChain runs multiple hooks in sequence or in parallel with configurable
// error handling and per-hook timeouts.
type HookChain struct {
	hooks          []Hook
	mode           ChainMode
	defaultTimeout time.Duration
	parallel       bool
}

// NewHookChain creates a chain with the given mode and default per-hook timeout.
// If defaultTimeout is zero, 30 seconds is used.
func NewHookChain(mode ChainMode, defaultTimeout time.Duration) *HookChain {
	if defaultTimeout <= 0 {
		defaultTimeout = 30 * time.Second
	}
	return &HookChain{
		mode:           mode,
		defaultTimeout: defaultTimeout,
	}
}

// Add appends a hook to the chain.
func (c *HookChain) Add(h Hook) {
	c.hooks = append(c.hooks, h)
}

// SetParallel enables or disables parallel execution.
func (c *HookChain) SetParallel(p bool) {
	c.parallel = p
}

// Run executes the chain with the given parent context.
// Returns results for every hook that was attempted.
func (c *HookChain) Run(ctx context.Context) []HookResult {
	if len(c.hooks) == 0 {
		return nil
	}
	if c.parallel {
		return c.runParallel(ctx)
	}
	return c.runSequential(ctx)
}

func (c *HookChain) runSequential(ctx context.Context) []HookResult {
	results := make([]HookResult, 0, len(c.hooks))
	for _, h := range c.hooks {
		if ctx.Err() != nil {
			results = append(results, HookResult{
				Name: h.Name,
				Err:  ctx.Err(),
			})
			break
		}
		r := c.execHook(ctx, h)
		results = append(results, r)
		if c.mode == FailFast && r.Err != nil {
			break
		}
	}
	return results
}

func (c *HookChain) runParallel(ctx context.Context) []HookResult {
	results := make([]HookResult, len(c.hooks))
	var wg sync.WaitGroup
	wg.Add(len(c.hooks))
	for i, h := range c.hooks {
		go func(idx int, hook Hook) {
			defer wg.Done()
			results[idx] = c.execHook(ctx, hook)
		}(i, h)
	}
	wg.Wait()
	return results
}

func (c *HookChain) execHook(ctx context.Context, h Hook) HookResult {
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = c.defaultTimeout
	}

	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("sh", "-c", h.Command)
	setCommandProcessGroup(cmd)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if h.Dir != "" {
		cmd.Dir = h.Dir
	}
	if len(h.Env) > 0 {
		cmd.Env = append(cmd.Environ(), h.Env...)
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return HookResult{
			Name:     h.Name,
			ExitCode: -1,
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			Duration: time.Since(start),
			Err:      err,
		}
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	var err error
	timedOut := false
	select {
	case err = <-done:
	case <-hookCtx.Done():
		_ = killCommandProcessGroup(cmd)
		<-done
		if hookCtx.Err() == context.DeadlineExceeded {
			timedOut = true
		} else {
			err = hookCtx.Err()
		}
	}
	dur := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Non-exit errors (timeout, signal, etc.) use -1.
			exitCode = -1
		}
	}

	// Wrap context deadline exceeded for clarity.
	if timedOut {
		err = fmt.Errorf("hook %q timed out after %s: %w", h.Name, timeout, hookCtx.Err())
		if exitCode == 0 {
			exitCode = -1
		}
	}

	return HookResult{
		Name:     h.Name,
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: dur,
		Err:      err,
	}
}
