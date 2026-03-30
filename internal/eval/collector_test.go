package eval

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestNewMetricCollector_NilBus(t *testing.T) {
	mc := NewMetricCollector(nil)
	if mc != nil {
		t.Fatal("expected nil collector for nil bus")
	}
}

func TestCollector_SessionEndedEvents(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	// Publish a few session.ended events for different providers.
	bus.Publish(events.Event{
		Type:      events.SessionEnded,
		Provider:  "claude",
		SessionID: "s1",
		RepoName:  "repo-a",
		Data: map[string]any{
			"status":    "completed",
			"spent_usd": 0.05,
			"turns":     12,
		},
	})
	bus.Publish(events.Event{
		Type:      events.SessionEnded,
		Provider:  "claude",
		SessionID: "s2",
		RepoName:  "repo-b",
		Data: map[string]any{
			"status":    "errored",
			"spent_usd": 0.02,
			"turns":     3,
		},
	})
	bus.Publish(events.Event{
		Type:      events.SessionEnded,
		Provider:  "gemini",
		SessionID: "s3",
		RepoName:  "repo-a",
		Data: map[string]any{
			"status":    "completed",
			"spent_usd": 0.01,
			"turns":     8,
		},
	})

	// Give the consumer goroutine time to process.
	time.Sleep(50 * time.Millisecond)

	claudeSamples := mc.Results("claude")
	if len(claudeSamples) != 2 {
		t.Fatalf("expected 2 claude samples, got %d", len(claudeSamples))
	}

	// First sample should be successful.
	if !claudeSamples[0].Success {
		t.Error("expected first claude sample to be successful")
	}
	if claudeSamples[0].CostUSD != 0.05 {
		t.Errorf("expected cost 0.05, got %f", claudeSamples[0].CostUSD)
	}
	if claudeSamples[0].Turns != 12 {
		t.Errorf("expected 12 turns, got %d", claudeSamples[0].Turns)
	}
	if claudeSamples[0].SourceType != "session" {
		t.Errorf("expected source_type 'session', got %q", claudeSamples[0].SourceType)
	}

	// Second sample should be a failure.
	if claudeSamples[1].Success {
		t.Error("expected second claude sample to be a failure")
	}

	geminiSamples := mc.Results("gemini")
	if len(geminiSamples) != 1 {
		t.Fatalf("expected 1 gemini sample, got %d", len(geminiSamples))
	}
	if !geminiSamples[0].Success {
		t.Error("expected gemini sample to be successful")
	}
}

func TestCollector_LoopIteratedEvents(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	bus.Publish(events.Event{
		Type:     events.LoopIterated,
		Provider: "openai",
		RepoName: "repo-x",
		Data: map[string]any{
			"loop_id":    "loop-1",
			"iteration":  1,
			"status":     "completed",
			"cost_usd":   0.03,
			"latency_ms": float64(1500),
		},
	})
	bus.Publish(events.Event{
		Type:     events.LoopIterated,
		Provider: "openai",
		RepoName: "repo-x",
		Data: map[string]any{
			"loop_id":    "loop-1",
			"iteration":  2,
			"status":     "failed",
			"cost_usd":   0.01,
			"latency_ms": float64(800),
		},
	})

	time.Sleep(50 * time.Millisecond)

	samples := mc.Results("openai")
	if len(samples) != 2 {
		t.Fatalf("expected 2 openai samples, got %d", len(samples))
	}

	if samples[0].LatencyMs != 1500 {
		t.Errorf("expected latency 1500, got %d", samples[0].LatencyMs)
	}
	if samples[0].SourceType != "loop_iteration" {
		t.Errorf("expected source_type 'loop_iteration', got %q", samples[0].SourceType)
	}
	if samples[0].LoopID != "loop-1" {
		t.Errorf("expected loop_id 'loop-1', got %q", samples[0].LoopID)
	}
	if !samples[0].Success {
		t.Error("expected first sample to be successful")
	}
	if samples[1].Success {
		t.Error("expected second sample to be a failure")
	}
}

func TestCollector_GroupsByProvider(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	providers := []string{"claude", "gemini", "openai", "claude", "gemini"}
	for i, p := range providers {
		bus.Publish(events.Event{
			Type:      events.SessionEnded,
			Provider:  p,
			SessionID: "s" + string(rune('0'+i)),
			Data: map[string]any{
				"status":    "completed",
				"spent_usd": 0.01 * float64(i+1),
			},
		})
	}

	time.Sleep(50 * time.Millisecond)

	allProviders := mc.AllProviders()
	if len(allProviders) != 3 {
		t.Fatalf("expected 3 provider keys, got %d: %v", len(allProviders), allProviders)
	}

	if len(mc.Results("claude")) != 2 {
		t.Errorf("expected 2 claude samples, got %d", len(mc.Results("claude")))
	}
	if len(mc.Results("gemini")) != 2 {
		t.Errorf("expected 2 gemini samples, got %d", len(mc.Results("gemini")))
	}
	if len(mc.Results("openai")) != 1 {
		t.Errorf("expected 1 openai sample, got %d", len(mc.Results("openai")))
	}

	if mc.TotalSamples() != 5 {
		t.Errorf("expected 5 total samples, got %d", mc.TotalSamples())
	}
}

func TestCollector_CompareProviders_SufficientSamples(t *testing.T) {
	bus := events.NewBus(1000)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	// Generate enough samples for a meaningful comparison.
	// Provider A: 80% success rate.
	for i := range 20 {
		status := "completed"
		if i%5 == 0 {
			status = "failed"
		}
		bus.Publish(events.Event{
			Type:      events.SessionEnded,
			Provider:  "claude",
			SessionID: "claude-" + string(rune('A'+i)),
			Data: map[string]any{
				"status":    status,
				"spent_usd": 0.05,
			},
		})
	}
	// Provider B: 50% success rate.
	for i := range 20 {
		status := "completed"
		if i%2 == 0 {
			status = "failed"
		}
		bus.Publish(events.Event{
			Type:      events.SessionEnded,
			Provider:  "gemini",
			SessionID: "gemini-" + string(rune('A'+i)),
			Data: map[string]any{
				"status":    status,
				"spent_usd": 0.02,
			},
		})
	}

	time.Sleep(100 * time.Millisecond)

	result, err := mc.CompareProviders("claude", "gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.SampleSizeA != 20 {
		t.Errorf("expected sample size A=20, got %d", result.SampleSizeA)
	}
	if result.SampleSizeB != 20 {
		t.Errorf("expected sample size B=20, got %d", result.SampleSizeB)
	}

	// With 80% vs 50% success rates and 20 samples each,
	// A should be better with reasonable probability.
	if result.ProbABetter < 0.5 {
		t.Errorf("expected P(A>B) > 0.5 given higher success rate, got %f", result.ProbABetter)
	}

	// Probabilities should sum to approximately 1.
	total := result.ProbABetter + result.ProbBBetter
	if total < 0.95 || total > 1.05 {
		t.Errorf("expected probabilities to sum to ~1.0, got %f", total)
	}
}

func TestCollector_CompareProviders_EmptyProvider(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	// Only publish for one provider.
	bus.Publish(events.Event{
		Type:     events.SessionEnded,
		Provider: "claude",
		Data: map[string]any{
			"status": "completed",
		},
	})

	time.Sleep(50 * time.Millisecond)

	_, err := mc.CompareProviders("claude", "gemini")
	if err == nil {
		t.Fatal("expected error for empty provider B")
	}
}

func TestCollector_CompareProviders_BothEmpty(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	_, err := mc.CompareProviders("claude", "gemini")
	if err == nil {
		t.Fatal("expected error when both providers are empty")
	}
}

func TestCollector_IgnoresEventsWithNoProvider(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	// Event with no provider should be ignored.
	bus.Publish(events.Event{
		Type: events.SessionEnded,
		Data: map[string]any{
			"status": "completed",
		},
	})
	bus.Publish(events.Event{
		Type: events.LoopIterated,
		Data: map[string]any{
			"status": "completed",
		},
	})

	time.Sleep(50 * time.Millisecond)

	if mc.TotalSamples() != 0 {
		t.Errorf("expected 0 samples for events without provider, got %d", mc.TotalSamples())
	}
}

func TestCollector_StopIdempotent(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)

	// Stop should be safe to call multiple times.
	mc.Stop()
	mc.Stop()

	// After stop, new events should not be collected.
	bus.Publish(events.Event{
		Type:     events.SessionEnded,
		Provider: "claude",
		Data: map[string]any{
			"status": "completed",
		},
	})

	time.Sleep(50 * time.Millisecond)

	if mc.TotalSamples() != 0 {
		t.Errorf("expected 0 samples after stop, got %d", mc.TotalSamples())
	}
}

func TestCollector_MixedEventTypes(t *testing.T) {
	bus := events.NewBus(100)
	mc := NewMetricCollector(bus)
	defer mc.Stop()

	// Mix session and loop events for the same provider.
	bus.Publish(events.Event{
		Type:      events.SessionEnded,
		Provider:  "claude",
		SessionID: "s1",
		Data: map[string]any{
			"status":    "completed",
			"spent_usd": 0.10,
		},
	})
	bus.Publish(events.Event{
		Type:     events.LoopIterated,
		Provider: "claude",
		Data: map[string]any{
			"loop_id":    "loop-1",
			"status":     "completed",
			"cost_usd":   0.03,
			"latency_ms": float64(2000),
		},
	})

	time.Sleep(50 * time.Millisecond)

	samples := mc.Results("claude")
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	// Verify both source types are present.
	sourceTypes := map[string]bool{}
	for _, s := range samples {
		sourceTypes[s.SourceType] = true
	}
	if !sourceTypes["session"] {
		t.Error("expected a 'session' source type sample")
	}
	if !sourceTypes["loop_iteration"] {
		t.Error("expected a 'loop_iteration' source type sample")
	}
}

func TestProviderKey(t *testing.T) {
	tests := []struct {
		provider, model, want string
	}{
		{"claude", "", "claude"},
		{"claude", "claude-sonnet-4-6", "claude/claude-sonnet-4-6"},
		{"gemini", "gemini-2.5-pro", "gemini/gemini-2.5-pro"},
	}
	for _, tt := range tests {
		got := providerKey(tt.provider, tt.model)
		if got != tt.want {
			t.Errorf("providerKey(%q, %q) = %q, want %q", tt.provider, tt.model, got, tt.want)
		}
	}
}

func TestSamplesToObservations(t *testing.T) {
	now := time.Now()
	samples := []MetricSample{
		{
			Timestamp: now,
			Provider:  "claude",
			LoopID:    "loop-1",
			CostUSD:   0.05,
			LatencyMs: 1500,
			Success:   true,
			Quality:   0.85,
			RepoName:  "repo-a",
		},
		{
			Timestamp: now.Add(time.Minute),
			Provider:  "claude",
			CostUSD:   0.02,
			LatencyMs: 800,
			Success:   false,
		},
	}

	obs := samplesToObservations(samples)
	if len(obs) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(obs))
	}

	if obs[0].WorkerProvider != "claude" {
		t.Errorf("expected provider 'claude', got %q", obs[0].WorkerProvider)
	}
	if obs[0].TotalCostUSD != 0.05 {
		t.Errorf("expected cost 0.05, got %f", obs[0].TotalCostUSD)
	}
	if !obs[0].VerifyPassed {
		t.Error("expected first observation to have VerifyPassed=true")
	}
	if obs[1].VerifyPassed {
		t.Error("expected second observation to have VerifyPassed=false")
	}
	if obs[1].Status != "failed" {
		t.Errorf("expected status 'failed', got %q", obs[1].Status)
	}
}
