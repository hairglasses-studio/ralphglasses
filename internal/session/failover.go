package session

import (
	"context"
	"fmt"
	"strings"
)

// FailoverChain defines an ordered list of providers to try on failure.
// The first provider is primary; subsequent providers are fallbacks.
type FailoverChain struct {
	Providers []Provider
}

// DefaultFailoverChain returns the standard provider preference order.
func DefaultFailoverChain() FailoverChain {
	return FailoverChain{Providers: []Provider{DefaultPrimaryProvider(), ProviderGemini, ProviderClaude}}
}

// LaunchWithFailover attempts to launch a session using the first healthy
// provider in the chain. Falls over to the next on validation or launch error.
// Returns the launched session and the provider that succeeded.
func (m *Manager) LaunchWithFailover(ctx context.Context, opts LaunchOptions, chain FailoverChain) (*Session, Provider, error) {
	if len(chain.Providers) == 0 {
		return nil, "", fmt.Errorf("failover chain is empty")
	}

	var errs []string
	for _, p := range chain.Providers {
		// Quick health pre-check: skip binary-missing or env-missing providers
		// without paying the cost of a failed launch attempt.
		h := m.checkHealth(p)
		if !h.Healthy() {
			errs = append(errs, fmt.Sprintf("%s: %s", p, h.Error))
			continue
		}

		attempt := opts
		attempt.Provider = p

		s, err := m.launchWorkflowSession(ctx, attempt)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", p, err.Error()))
			continue
		}
		return s, p, nil
	}

	return nil, "", fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}

// LaunchWithSmartFailover uses FeedbackAnalyzer profiles to build an optimized
// failover chain, then falls back to the default static chain. This replaces
// the static default ordering with data-driven provider selection.
func (m *Manager) LaunchWithSmartFailover(ctx context.Context, opts LaunchOptions) (*Session, Provider, error) {
	m.configMu.RLock()
	optimizer := m.optimizer
	m.configMu.RUnlock()

	var chain FailoverChain
	if optimizer != nil {
		chain = optimizer.BuildSmartFailoverChain(opts.Prompt)
	} else {
		chain = DefaultFailoverChain()
	}

	return m.LaunchWithFailover(ctx, opts, chain)
}
