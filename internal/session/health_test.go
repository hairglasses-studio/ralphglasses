package session

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestCheckProviderHealthUnknown(t *testing.T) {
	h := CheckProviderHealth(Provider("unknown"))
	if h.Available {
		t.Error("unknown provider should not be available")
	}
	if h.Error == "" {
		t.Error("expected error for unknown provider")
	}
}

func TestCheckProviderHealthBinaryMissing(t *testing.T) {
	// gemini/codex binaries are unlikely to be on PATH in CI.
	// We just verify the struct fields are consistent.
	h := CheckProviderHealth(ProviderGemini)
	if h.Binary != "gemini" {
		t.Errorf("binary = %q, want %q", h.Binary, "gemini")
	}
	if h.Provider != ProviderGemini {
		t.Errorf("provider = %q, want %q", h.Provider, ProviderGemini)
	}
	// If not available, error must be set.
	if !h.Available && h.Error == "" {
		t.Error("unavailable provider should have error set")
	}
}

func TestProviderHealthHealthy(t *testing.T) {
	healthy := ProviderHealth{Available: true, EnvOK: true}
	if !healthy.Healthy() {
		t.Error("Available+EnvOK should be Healthy")
	}
	notHealthy := ProviderHealth{Available: true, EnvOK: false}
	if notHealthy.Healthy() {
		t.Error("missing EnvOK should not be Healthy")
	}
}

func TestHealthyProviders(t *testing.T) {
	health := map[Provider]ProviderHealth{
		ProviderClaude: {Provider: ProviderClaude, Available: true, EnvOK: true},
		ProviderGemini: {Provider: ProviderGemini, Available: false},
		ProviderCodex:  {Provider: ProviderCodex, Available: true, EnvOK: false},
	}
	healthy := HealthyProviders(health)
	if len(healthy) != 1 {
		t.Fatalf("expected 1 healthy provider, got %d: %v", len(healthy), healthy)
	}
	if healthy[0] != ProviderClaude {
		t.Errorf("expected claude, got %q", healthy[0])
	}
}

func TestCheckAllProviderHealthReturnsAllProviders(t *testing.T) {
	health := CheckAllProviderHealth()
	for _, p := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex, ProviderAntigravity} {
		if _, ok := health[p]; !ok {
			t.Errorf("missing health entry for provider %q", p)
		}
	}
}




func TestHealthChecker_DetectsUnavailable(t *testing.T) {
	bus := events.NewBus(100)
	// Use a provider whose binary is very unlikely to exist on PATH.
	hc := NewHealthChecker(bus, time.Second, Provider("nonexistent_provider_xyz"))
	result := hc.CheckAll()
	if result[Provider("nonexistent_provider_xyz")] {
		t.Error("expected nonexistent provider to be unhealthy")
	}
	if hc.IsHealthy(Provider("nonexistent_provider_xyz")) {
		t.Error("IsHealthy should return false for nonexistent provider")
	}
}

func TestHealthChecker_PublishesOnStateChange(t *testing.T) {
	bus := events.NewBus(100)
	sub := bus.Subscribe("test-health")

	// Use a provider that won't be on PATH (guaranteed unhealthy).
	hc := NewHealthChecker(bus, 50*time.Millisecond, Provider("fake_provider_abc"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		hc.Start(ctx)
		close(done)
	}()

	// Wait for the initial check event (transition from unknown → unhealthy).
	select {
	case ev := <-sub:
		if ev.Type != events.ProviderHealthChanged {
			t.Errorf("expected provider.health event, got %q", ev.Type)
		}
		if ev.Data["healthy"] != false {
			t.Error("expected healthy=false for fake provider")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first health event")
	}

	// Subsequent ticks should NOT publish (no state change: still unhealthy).
	// Wait for at least 2 tick intervals and drain.
	time.Sleep(150 * time.Millisecond)
	select {
	case ev := <-sub:
		t.Errorf("unexpected second event (should only publish on state change): %+v", ev)
	default:
		// Good: no duplicate event.
	}

	cancel()
	<-done
}

func TestNewHealthChecker_DefaultInterval(t *testing.T) {
	hc := NewHealthChecker(nil, 0)
	if hc.interval != 30*time.Second {
		t.Errorf("expected 30s default, got %v", hc.interval)
	}
	hc2 := NewHealthChecker(nil, -1)
	if hc2.interval != 30*time.Second {
		t.Errorf("expected 30s default for negative, got %v", hc2.interval)
	}
}

func TestHealthChecker_CheckAll(t *testing.T) {
	bus := events.NewBus(100)
	hc := NewHealthChecker(bus, time.Minute, ProviderClaude, ProviderGemini)
	result := hc.CheckAll()
	if len(result) != 2 {
		t.Errorf("expected 2 providers in result, got %d", len(result))
	}
	// Verify IsHealthy reflects CheckAll results.
	for p, healthy := range result {
		if hc.IsHealthy(p) != healthy {
			t.Errorf("IsHealthy(%s) = %v, want %v", p, hc.IsHealthy(p), healthy)
		}
	}
}
