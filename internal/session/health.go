package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// ProviderHealth holds the result of a health check for a single provider.
type ProviderHealth struct {
	Provider  Provider  `json:"provider"`
	Available bool      `json:"available"` // binary found on PATH
	EnvOK     bool      `json:"env_ok"`    // required API key present
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

func checkOllamaRuntimeHealth(fetcher func(context.Context, time.Duration) ([]string, error)) error {
	models, err := fetcher(context.Background(), 5*time.Second)
	if err != nil {
		return err
	}

	model := resolveOllamaCodeModel()
	if ollamaModelInstalledExact(model, models) {
		return nil
	}
	if sourceModel := ollamaAliasSourceModel(model); sourceModel != "" && ollamaModelInstalledExact(sourceModel, models) {
		return fmt.Errorf("ollama model %q is not installed at %s; backing model %q is present but the managed alias is missing; run `~/hairglasses-studio/dotfiles/scripts/hg-ollama-sync-aliases.sh`",
			model, resolveOllamaBaseURL(), sourceModel)
	}
	return fmt.Errorf("ollama model %q is not installed at %s; pull the backing model with `ollama pull %s` or sync aliases with `~/hairglasses-studio/dotfiles/scripts/hg-ollama-sync-aliases.sh`",
		model, resolveOllamaBaseURL(), ollamaPullHintModel(model))
}

// CheckProviderHealth runs a health check for the given provider.
// It verifies binary availability, required env, and version probe latency.
// Ollama also checks that the configured local coding lane is installed.
func CheckProviderHealth(p Provider) ProviderHealth {
	p = normalizeSessionProvider(p)
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
	if p == ProviderOllama {
		if err := checkOllamaRuntimeHealth(fetchOllamaModels); err != nil {
			h.EnvOK = false
			h.Error = err.Error()
			h.LatencyMs = time.Since(start).Milliseconds()
			return h
		}
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
	providers := []Provider{ProviderClaude, ProviderOllama, ProviderGemini, ProviderCodex, ProviderAntigravity}
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

// HealthChecker runs periodic provider health checks.
// It publishes events.ProviderHealthChanged only on state transitions
// (healthy→unhealthy or unhealthy→healthy).
type HealthChecker struct {
	providers []Provider
	bus       *events.Bus
	interval  time.Duration
	lastState map[Provider]bool // true = healthy
	mu        sync.Mutex
}

// NewHealthChecker creates a HealthChecker that periodically checks the given providers.
func NewHealthChecker(bus *events.Bus, interval time.Duration, providers ...Provider) *HealthChecker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &HealthChecker{
		providers: providers,
		bus:       bus,
		interval:  interval,
		lastState: make(map[Provider]bool),
	}
}

// Start begins the background health check loop. Stops when ctx is cancelled.
func (h *HealthChecker) Start(ctx context.Context) {
	// Run an initial check immediately.
	h.tick()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.tick()
		}
	}
}

// tick performs one round of health checks and publishes events on state changes.
func (h *HealthChecker) tick() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, p := range h.providers {
		healthy := h.checkOne(p)
		prev, known := h.lastState[p]
		if !known || prev != healthy {
			h.lastState[p] = healthy
			if h.bus != nil {
				h.bus.Publish(events.Event{
					Type:     events.ProviderHealthChanged,
					Provider: string(p),
					Data: map[string]any{
						"healthy":  healthy,
						"provider": string(p),
					},
				})
			}
		}
	}
}

// checkOne returns true if the provider is ready for launch.
func (h *HealthChecker) checkOne(p Provider) bool {
	if err := ValidateProvider(p); err != nil {
		return false
	}
	if err := ValidateProviderEnv(p); err != nil {
		return false
	}
	if normalizeSessionProvider(p) == ProviderOllama {
		return checkOllamaRuntimeHealth(fetchOllamaModels) == nil
	}
	return true
}

// CheckAll checks all providers and returns their current health status.
func (h *HealthChecker) CheckAll() map[Provider]bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	result := make(map[Provider]bool, len(h.providers))
	for _, p := range h.providers {
		healthy := h.checkOne(p)
		result[p] = healthy
		h.lastState[p] = healthy
	}
	return result
}

// IsHealthy returns the last known health status for a provider.
func (h *HealthChecker) IsHealthy(p Provider) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastState[p]
}
