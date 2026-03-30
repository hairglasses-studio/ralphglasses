package workflow

import (
	"context"
	"fmt"
	"time"
)

// DefaultTimeout is applied when a step has no explicit timeout.
const DefaultTimeout = 5 * time.Minute

// StepExecutor runs individual steps with timeout, retry, and output capture.
type StepExecutor struct {
	registry *StepRegistry
}

// NewStepExecutor creates a StepExecutor backed by the given registry.
func NewStepExecutor(reg *StepRegistry) *StepExecutor {
	return &StepExecutor{registry: reg}
}

// Execute runs a single step, honoring its timeout and retry policy.
// It returns a StepResult regardless of success or failure.
func (e *StepExecutor) Execute(ctx context.Context, step Step) StepResult {
	start := time.Now()
	result := StepResult{
		Name:   step.Name,
		Status: StepPending,
	}

	handler := e.registry.Lookup(step.Type)
	if handler == nil {
		result.Status = StepFailed
		result.Error = fmt.Sprintf("unknown step type %q", step.Type)
		result.Duration = time.Since(start)
		return result
	}

	timeout := step.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	maxAttempts := step.RetryCount + 1
	delay := step.RetryDelay
	if delay == 0 {
		delay = time.Second
	}

	var lastErr error
	var lastOutput string

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			result.Status = StepFailed
			result.Error = ctx.Err().Error()
			result.Duration = time.Since(start)
			result.Retries = attempt
			return result
		}

		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		output, err := handler(stepCtx, step)
		cancel()

		lastOutput = output
		lastErr = err

		if err == nil {
			result.Status = StepSucceeded
			result.Output = output
			result.Duration = time.Since(start)
			result.Retries = attempt
			return result
		}

		result.Retries = attempt + 1

		// Don't sleep after the last attempt.
		if attempt < maxAttempts-1 {
			select {
			case <-ctx.Done():
				result.Status = StepFailed
				result.Error = ctx.Err().Error()
				result.Duration = time.Since(start)
				return result
			case <-time.After(delay):
			}
		}
	}

	result.Status = StepFailed
	result.Output = lastOutput
	if lastErr != nil {
		result.Error = lastErr.Error()
	}
	result.Duration = time.Since(start)
	return result
}
