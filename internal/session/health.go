package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProviderHealth holds the result of a health check for a single provider.
type ProviderHealth struct {
	Provider  Provider  `json:"provider"`
	Available bool      `json:"available"`  // binary found on PATH
	EnvOK     bool      `json:"env_ok"`     // required API key present
	Binary    string    `json:"binary"`
	Version   string    `json:"version,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
	LatencyMs int64     `json:"latency_ms,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// Healthy returns true if the provider is fully ready to use.
func (h ProviderHealth) Healthy() bool {
	return h.Available && h.EnvOK
}

// CheckProviderHealth runs a health check for the given provider.
// It verifies binary availability, API key presence, and queries --version
// to measure round-trip latency without making any API calls.
func CheckProviderHealth(p Provider) ProviderHealth {
	start := time.Now()
	h := ProviderHealth{
		Provider:  p,
		Binary:    providerBinary(p),
		CheckedAt: start,
	}

	if h.Binary == "" {
		h.Error = fmt.Sprintf("unknown provider: %q", p)
		return h
	}

	if _, err := exec.LookPath(h.Binary); err != nil {
		h.Error = fmt.Sprintf("%s not found on PATH", h.Binary)
		return h
	}
	h.Available = true

	if err := ValidateProviderEnv(p); err != nil {
		h.Error = err.Error()
	} else {
		h.EnvOK = true
	}

	// Query --version for latency measurement (no API call).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, h.Binary, "--version").Output()
	h.LatencyMs = time.Since(start).Milliseconds()
	if err == nil {
		h.Version = strings.TrimSpace(string(out))
	}

	return h
}

// CheckAllProviderHealth runs health checks for all known providers in parallel.
func CheckAllProviderHealth() map[Provider]ProviderHealth {
	providers := []Provider{ProviderClaude, ProviderGemini, ProviderCodex}
	type result struct {
		p Provider
		h ProviderHealth
	}
	ch := make(chan result, len(providers))

	for _, p := range providers {
		go func(p Provider) {
			ch <- result{p: p, h: CheckProviderHealth(p)}
		}(p)
	}

	out := make(map[Provider]ProviderHealth, len(providers))
	for range providers {
		r := <-ch
		out[r.p] = r.h
	}
	return out
}

// HealthyProviders returns providers that pass their health check.
func HealthyProviders(health map[Provider]ProviderHealth) []Provider {
	var out []Provider
	for p, h := range health {
		if h.Healthy() {
			out = append(out, p)
		}
	}
	return out
}
