package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDefaultFailoverChain(t *testing.T) {
	t.Parallel()

	chain := DefaultFailoverChain()

	if len(chain.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(chain.Providers))
	}

	want := []Provider{ProviderCodex, ProviderGemini, ProviderClaude}
	for i, p := range chain.Providers {
		if p != want[i] {
			t.Errorf("provider[%d] = %s, want %s", i, p, want[i])
		}
	}
}

func TestDefaultFailoverChain_Immutable(t *testing.T) {
	t.Parallel()

	// Ensure successive calls return fresh slices (no shared mutation).
	c1 := DefaultFailoverChain()
	c2 := DefaultFailoverChain()

	c1.Providers[0] = "mutated"
	if c2.Providers[0] == "mutated" {
		t.Fatal("DefaultFailoverChain shares underlying slice between calls")
	}
}

func TestLaunchWithFailover_EmptyChain(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	ctx := context.Background()
	chain := FailoverChain{Providers: []Provider{}}

	_, _, err := mgr.LaunchWithFailover(ctx, LaunchOptions{Prompt: "test"}, chain)
	if err == nil {
		t.Fatal("expected error for empty chain")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected error containing 'empty', got: %s", err)
	}
}

func TestLaunchWithFailover_NilProviders(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	ctx := context.Background()
	chain := FailoverChain{Providers: nil}

	_, _, err := mgr.LaunchWithFailover(ctx, LaunchOptions{Prompt: "test"}, chain)
	if err == nil {
		t.Fatal("expected error for nil providers")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected error containing 'empty', got: %s", err)
	}
}

func TestLaunchWithFailover_AllProvidersFail(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	// Inject a health check that always fails.
	mgr.SetHealthCheckForTesting(func(p Provider) ProviderHealth {
		return ProviderHealth{
			Provider:  p,
			Available: false,
			EnvOK:     false,
			Binary:    string(p),
			CheckedAt: time.Now(),
			Error:     fmt.Sprintf("%s not found on PATH", p),
		}
	})

	ctx := context.Background()
	chain := DefaultFailoverChain()

	_, _, err := mgr.LaunchWithFailover(ctx, LaunchOptions{Prompt: "test"}, chain)
	if err == nil {
		t.Fatal("expected error when all providers fail health check")
	}
	if !strings.Contains(err.Error(), "all providers failed") {
		t.Fatalf("expected 'all providers failed' in error, got: %s", err)
	}
	// Each provider's error should appear in the combined message.
	for _, p := range chain.Providers {
		if !strings.Contains(err.Error(), string(p)) {
			t.Errorf("expected provider %q mentioned in error, got: %s", p, err)
		}
	}
}

func TestLaunchWithFailover_FirstHealthyWins(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	// Only gemini passes health check.
	mgr.SetHealthCheckForTesting(func(p Provider) ProviderHealth {
		h := ProviderHealth{
			Provider:  p,
			Binary:    string(p),
			CheckedAt: time.Now(),
		}
		if p == ProviderGemini {
			h.Available = true
			h.EnvOK = true
		} else {
			h.Error = fmt.Sprintf("%s unavailable", p)
		}
		return h
	})

	// Mock Launch to succeed without real binary.
	mgr.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			return &Session{
				ID:       "test-session",
				Provider: opts.Provider,
				Status:   StatusRunning,
			}, nil
		},
		nil,
	)

	ctx := context.Background()
	chain := DefaultFailoverChain() // claude, gemini, codex

	sess, provider, err := mgr.LaunchWithFailover(ctx, LaunchOptions{Prompt: "test"}, chain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != ProviderGemini {
		t.Fatalf("expected gemini (first healthy), got %s", provider)
	}
	if sess.Provider != ProviderGemini {
		t.Fatalf("session provider = %s, want gemini", sess.Provider)
	}
}

func TestLaunchWithFailover_HealthyButLaunchFails(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	// All providers pass health check.
	mgr.SetHealthCheckForTesting(func(p Provider) ProviderHealth {
		return ProviderHealth{
			Provider:  p,
			Available: true,
			EnvOK:     true,
			Binary:    string(p),
			CheckedAt: time.Now(),
		}
	})

	callOrder := []Provider{}

	// With Codex first in the failover chain, the first attempt succeeds.
	mgr.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			callOrder = append(callOrder, opts.Provider)
			if opts.Provider == ProviderCodex {
				return &Session{
					ID:       "codex-session",
					Provider: ProviderCodex,
					Status:   StatusRunning,
				}, nil
			}
			return nil, fmt.Errorf("launch failed for %s", opts.Provider)
		},
		nil,
	)

	ctx := context.Background()
	chain := DefaultFailoverChain()

	sess, provider, err := mgr.LaunchWithFailover(ctx, LaunchOptions{Prompt: "test"}, chain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != ProviderCodex {
		t.Fatalf("expected codex, got %s", provider)
	}
	if sess.ID != "codex-session" {
		t.Fatalf("wrong session ID: %s", sess.ID)
	}

	// Verify the default Codex-first ordering short-circuits after the first success.
	if len(callOrder) != 1 {
		t.Fatalf("expected 1 launch attempt, got %d", len(callOrder))
	}
	if callOrder[0] != ProviderCodex {
		t.Fatalf("unexpected call order: %v", callOrder)
	}
}

func TestLaunchWithFailover_SingleProvider(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	mgr.SetHealthCheckForTesting(func(p Provider) ProviderHealth {
		return ProviderHealth{
			Provider:  p,
			Available: true,
			EnvOK:     true,
			Binary:    string(p),
			CheckedAt: time.Now(),
		}
	})

	mgr.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			return &Session{
				ID:       "solo-session",
				Provider: opts.Provider,
				Status:   StatusRunning,
			}, nil
		},
		nil,
	)

	ctx := context.Background()
	chain := FailoverChain{Providers: []Provider{ProviderClaude}}

	_, provider, err := mgr.LaunchWithFailover(ctx, LaunchOptions{Prompt: "test"}, chain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider != ProviderClaude {
		t.Fatalf("expected claude, got %s", provider)
	}
}

func TestLaunchWithSmartFailover_NoOptimizer(t *testing.T) {
	t.Parallel()

	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	// All health checks fail — we just want to verify it uses DefaultFailoverChain.
	mgr.SetHealthCheckForTesting(func(p Provider) ProviderHealth {
		return ProviderHealth{
			Provider:  p,
			Binary:    string(p),
			CheckedAt: time.Now(),
			Error:     "unavailable",
		}
	})

	ctx := context.Background()
	_, _, err := mgr.LaunchWithSmartFailover(ctx, LaunchOptions{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should mention all three default providers.
	for _, p := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex} {
		if !strings.Contains(err.Error(), string(p)) {
			t.Errorf("expected %q in error (default chain used), got: %s", p, err)
		}
	}
}
