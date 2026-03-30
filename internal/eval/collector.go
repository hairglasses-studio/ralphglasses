package eval

import (
	"fmt"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// MetricSample holds a single metric observation from a completed session or loop iteration.
type MetricSample struct {
	Timestamp  time.Time     `json:"timestamp"`
	SessionID  string        `json:"session_id,omitempty"`
	LoopID     string        `json:"loop_id,omitempty"`
	Provider   string        `json:"provider"`
	Model      string        `json:"model,omitempty"`
	LatencyMs  int64         `json:"latency_ms"`
	CostUSD    float64       `json:"cost_usd"`
	Quality    float64       `json:"quality"`    // confidence or quality score
	Success    bool          `json:"success"`    // derived from status
	Turns      int           `json:"turns"`      // turn count if available
	Duration   time.Duration `json:"duration"`   // wall-clock duration
	RepoName   string        `json:"repo_name,omitempty"`
	SourceType string        `json:"source_type"` // "session" or "loop_iteration"
}

// MetricCollector subscribes to session.ended and loop.iterated events on the
// event bus, extracts metrics, and groups them by provider+model key for
// Bayesian A/B comparison.
type MetricCollector struct {
	bus  *events.Bus
	subID string

	mu      sync.RWMutex
	samples map[string][]MetricSample // key: providerKey(provider, model)
	stopped bool
}

// NewMetricCollector creates a MetricCollector that subscribes to session.ended
// and loop.iterated events on the given bus. Returns nil if bus is nil.
func NewMetricCollector(bus *events.Bus) *MetricCollector {
	if bus == nil {
		return nil
	}
	mc := &MetricCollector{
		bus:     bus,
		subID:   fmt.Sprintf("metric-collector-%d", time.Now().UnixNano()),
		samples: make(map[string][]MetricSample),
	}
	mc.start()
	return mc
}

// providerKey returns the grouping key for a provider+model combination.
// If model is empty, only the provider is used.
func providerKey(provider, model string) string {
	if model == "" {
		return provider
	}
	return provider + "/" + model
}

// start subscribes to the event bus and begins collecting metrics in the background.
func (mc *MetricCollector) start() {
	ch := mc.bus.SubscribeFiltered(mc.subID, events.SessionEnded, events.LoopIterated)
	go mc.consumeLoop(ch)
}

// Stop unsubscribes from the event bus and stops the collector.
func (mc *MetricCollector) Stop() {
	mc.mu.Lock()
	mc.stopped = true
	mc.mu.Unlock()
	mc.bus.Unsubscribe(mc.subID)
}

// consumeLoop reads events from the channel until it is closed.
func (mc *MetricCollector) consumeLoop(ch <-chan events.Event) {
	for ev := range ch {
		mc.mu.RLock()
		stopped := mc.stopped
		mc.mu.RUnlock()
		if stopped {
			return
		}

		switch ev.Type {
		case events.SessionEnded:
			mc.handleSessionEnded(ev)
		case events.LoopIterated:
			mc.handleLoopIterated(ev)
		}
	}
}

// handleSessionEnded extracts metrics from a session.ended event.
func (mc *MetricCollector) handleSessionEnded(ev events.Event) {
	provider := ev.Provider
	if provider == "" {
		return
	}

	sample := MetricSample{
		Timestamp:  ev.Timestamp,
		SessionID:  ev.SessionID,
		Provider:   provider,
		RepoName:   ev.RepoName,
		SourceType: "session",
	}

	// Extract data fields
	if d := ev.Data; d != nil {
		if v, ok := d["spent_usd"].(float64); ok {
			sample.CostUSD = v
		}
		if v, ok := d["turns"].(float64); ok {
			sample.Turns = int(v)
		}
		// int is also possible if published in-process without JSON round-trip
		if v, ok := d["turns"].(int); ok {
			sample.Turns = v
		}
		if v, ok := d["status"].(string); ok {
			sample.Success = v == "completed" || v == "done"
		}
	}

	key := providerKey(provider, "")
	mc.addSample(key, sample)
}

// handleLoopIterated extracts metrics from a loop.iterated event.
func (mc *MetricCollector) handleLoopIterated(ev events.Event) {
	d := ev.Data
	if d == nil {
		return
	}

	// The loop.iterated event may not have Provider on the event envelope;
	// try to extract from data or fall back to event-level provider.
	provider := ev.Provider
	if provider == "" {
		if v, ok := d["worker_provider"].(string); ok {
			provider = v
		}
	}
	if provider == "" {
		return
	}

	sample := MetricSample{
		Timestamp:  ev.Timestamp,
		Provider:   provider,
		RepoName:   ev.RepoName,
		SourceType: "loop_iteration",
	}

	if v, ok := d["loop_id"].(string); ok {
		sample.LoopID = v
	}
	if v, ok := d["cost_usd"].(float64); ok {
		sample.CostUSD = v
	}
	if v, ok := d["latency_ms"].(float64); ok {
		sample.LatencyMs = int64(v)
		sample.Duration = time.Duration(int64(v)) * time.Millisecond
	}
	if v, ok := d["latency_ms"].(int64); ok {
		sample.LatencyMs = v
		sample.Duration = time.Duration(v) * time.Millisecond
	}
	if v, ok := d["status"].(string); ok {
		sample.Success = v == "completed" || v == "done" || v == "passed"
	}

	key := providerKey(provider, "")
	mc.addSample(key, sample)
}

// addSample appends a sample to the given provider key.
func (mc *MetricCollector) addSample(key string, s MetricSample) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.samples[key] = append(mc.samples[key], s)
}

// Results returns all collected samples for the given provider key.
// The key can be a bare provider name (e.g. "claude") or a provider/model
// combination (e.g. "claude/claude-sonnet-4-6").
func (mc *MetricCollector) Results(provider string) []MetricSample {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	out := make([]MetricSample, len(mc.samples[provider]))
	copy(out, mc.samples[provider])
	return out
}

// AllProviders returns all provider keys that have collected samples.
func (mc *MetricCollector) AllProviders() []string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	keys := make([]string, 0, len(mc.samples))
	for k := range mc.samples {
		keys = append(keys, k)
	}
	return keys
}

// TotalSamples returns the total number of collected samples across all providers.
func (mc *MetricCollector) TotalSamples() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	total := 0
	for _, s := range mc.samples {
		total += len(s)
	}
	return total
}

// CompareProviders performs a Bayesian A/B comparison between two provider keys.
// It constructs LoopObservation values from the collected samples and delegates
// to the existing CompareAB function.
//
// Returns an error if either provider has zero samples.
func (mc *MetricCollector) CompareProviders(provA, provB string) (*ABTestResult, error) {
	mc.mu.RLock()
	samplesA := mc.samples[provA]
	samplesB := mc.samples[provB]
	mc.mu.RUnlock()

	if len(samplesA) == 0 {
		return nil, fmt.Errorf("no samples for provider %q", provA)
	}
	if len(samplesB) == 0 {
		return nil, fmt.Errorf("no samples for provider %q", provB)
	}

	// Convert MetricSamples to LoopObservations for the Bayesian comparison.
	obsA := samplesToObservations(samplesA)
	obsB := samplesToObservations(samplesB)

	result := CompareAB(obsA, obsB, func(o session.LoopObservation) bool {
		return o.VerifyPassed
	})
	return &result, nil
}

// samplesToObservations converts MetricSamples into LoopObservations for
// compatibility with the existing Bayesian comparison functions.
func samplesToObservations(samples []MetricSample) []session.LoopObservation {
	obs := make([]session.LoopObservation, len(samples))
	for i, s := range samples {
		obs[i] = session.LoopObservation{
			Timestamp:      s.Timestamp,
			LoopID:         s.LoopID,
			RepoName:       s.RepoName,
			TotalCostUSD:   s.CostUSD,
			TotalLatencyMs: s.LatencyMs,
			WorkerProvider: s.Provider,
			VerifyPassed:   s.Success,
			Confidence:     s.Quality,
			Status:         statusFromSuccess(s.Success),
		}
	}
	return obs
}

func statusFromSuccess(success bool) string {
	if success {
		return "completed"
	}
	return "failed"
}
