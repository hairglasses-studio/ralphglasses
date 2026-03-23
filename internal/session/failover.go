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
	return FailoverChain{Providers: []Provider{ProviderClaude, ProviderGemini, ProviderCodex}}
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
		h := CheckProviderHealth(p)
		if !h.Healthy() {
			errs = append(errs, fmt.Sprintf("%s: %s", p, h.Error))
			continue
		}

		attempt := opts
		attempt.Provider = p

		s, err := m.Launch(ctx, attempt)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", p, err.Error()))
			continue
		}
		return s, p, nil
	}

	return nil, "", fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}
