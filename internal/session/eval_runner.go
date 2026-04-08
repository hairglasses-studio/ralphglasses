package session

import (
	"context"
	"sync"
	"time"
)

// EvalResult represents the outcome of running a prompt against a single provider.
type EvalResult struct {
	Provider Provider      `json:"provider"`
	Duration time.Duration `json:"duration"`
	CostUSD  float64       `json:"cost_usd"`
	Success  bool          `json:"success"`
	Error    string        `json:"error,omitempty"`
}

// EvalComparison aggregates the results of running the same prompt across multiple providers.
type EvalComparison struct {
	Prompt   string                  `json:"prompt"`
	RepoPath string                  `json:"repo_path"`
	Results  map[Provider]EvalResult `json:"results"`
}

// EvalRunner orchestrates concurrent evaluation tasks across multiple providers.
type EvalRunner struct {
	manager *Manager
}

// NewEvalRunner creates a new EvalRunner attached to the given session Manager.
func NewEvalRunner(manager *Manager) *EvalRunner {
	return &EvalRunner{
		manager: manager,
	}
}

// RunEval executes the provided prompt against Claude, Codex, and Gemini concurrently,
// returning a comparison of their performance (duration, cost, success rate).
func (r *EvalRunner) RunEval(ctx context.Context, repoPath, prompt string) *EvalComparison {
	providers := []Provider{ProviderClaude, ProviderCodex, ProviderGemini}

	comp := &EvalComparison{
		Prompt:   prompt,
		RepoPath: repoPath,
		Results:  make(map[Provider]EvalResult),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(prov Provider) {
			defer wg.Done()

			start := time.Now()

			opts := LaunchOptions{
				Provider: prov,
				RepoPath: repoPath,
				Prompt:   prompt,
			}

			sess, err := r.manager.Launch(ctx, opts)

			res := EvalResult{
				Provider: prov,
			}

			if err != nil {
				res.Duration = time.Since(start)
				res.Success = false
				res.Error = err.Error()
			} else {
				// Wait for session to finish
				waitErr := r.manager.waitForSession(ctx, sess)
				res.Duration = time.Since(start)

				sess.Lock()
				res.CostUSD = sess.SpentUSD
				if sess.Status == StatusCompleted {
					res.Success = true
				} else {
					res.Success = false
					if waitErr != nil {
						res.Error = waitErr.Error()
					} else if sess.Error != "" {
						res.Error = sess.Error
					} else {
						res.Error = sess.ExitReason
					}
				}
				sess.Unlock()
			}

			mu.Lock()
			comp.Results[prov] = res
			mu.Unlock()
		}(p)
	}

	wg.Wait()
	return comp
}
