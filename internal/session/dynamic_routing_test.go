package session

import (
	"strings"
	"sync"
	"testing"
)

var testProviders = []ProviderInfo{
	{Name: "claude", Model: "opus", CostPerToken: 0.015, MaxConcurrent: 5, Available: true},
	{Name: "gemini", Model: "pro", CostPerToken: 0.002, MaxConcurrent: 10, Available: true},
	{Name: "codex", Model: "o3", CostPerToken: 0.006, MaxConcurrent: 3, Available: true},
}

func TestSmartRouter_Decide_CheapestByDefault(t *testing.T) {
	sr := NewSmartRouter(testProviders)

	dec := sr.Decide("implement a new dashboard", 5.0)
	if dec.Provider != "gemini" {
		t.Errorf("expected cheapest provider gemini, got %q", dec.Provider)
	}
	if dec.MaxCost != 5.0 {
		t.Errorf("max_cost = %f, want 5.0", dec.MaxCost)
	}
	if !strings.Contains(dec.Reason, "cheapest") {
		t.Errorf("reason should mention cheapest, got %q", dec.Reason)
	}
}

func TestSmartRouter_Decide_WithRoutingRule(t *testing.T) {
	sr := NewSmartRouter(testProviders)
	sr.SetRouteConfig(RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeBugFix, PreferredProvider: "claude", MaxCost: 1.0, Priority: 1},
		},
	})

	dec := sr.Decide("fix the login crash", 5.0)
	if dec.Provider != "claude" {
		t.Errorf("expected rule-preferred claude, got %q", dec.Provider)
	}
	if !strings.Contains(dec.Reason, "rule match") {
		t.Errorf("reason should mention rule match, got %q", dec.Reason)
	}
	// Budget is higher than rule max, so rule max should apply.
	if dec.MaxCost != 1.0 {
		t.Errorf("max_cost = %f, want 1.0 (rule limit)", dec.MaxCost)
	}
}

func TestSmartRouter_Decide_BudgetLowerThanRule(t *testing.T) {
	sr := NewSmartRouter(testProviders)
	sr.SetRouteConfig(RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeBugFix, PreferredProvider: "claude", MaxCost: 5.0, Priority: 1},
		},
	})

	dec := sr.Decide("fix parsing bug", 0.50)
	if dec.Provider != "claude" {
		t.Errorf("expected claude, got %q", dec.Provider)
	}
	if dec.MaxCost != 0.50 {
		t.Errorf("max_cost = %f, want 0.50 (budget cap)", dec.MaxCost)
	}
}

func TestSmartRouter_Decide_NoProviders(t *testing.T) {
	sr := NewSmartRouter(nil)
	dec := sr.Decide("do something", 1.0)
	if dec.Provider != "" {
		t.Errorf("expected empty provider, got %q", dec.Provider)
	}
	if !strings.Contains(dec.Reason, "no providers") {
		t.Errorf("reason = %q, want mention of no providers", dec.Reason)
	}
}

func TestSmartRouter_Decide_UnavailablePreferred(t *testing.T) {
	providers := []ProviderInfo{
		{Name: "claude", CostPerToken: 0.015, Available: false},
		{Name: "gemini", CostPerToken: 0.002, Available: true},
	}
	sr := NewSmartRouter(providers)
	sr.SetRouteConfig(RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeBugFix, PreferredProvider: "claude", MaxCost: 1.0, Priority: 1},
		},
	})

	dec := sr.Decide("fix the crash", 5.0)
	// Claude is unavailable, should fall back to gemini.
	if dec.Provider != "gemini" {
		t.Errorf("expected fallback to gemini, got %q", dec.Provider)
	}
}

func TestSmartRouter_Decide_NoBudget(t *testing.T) {
	sr := NewSmartRouter(testProviders)
	dec := sr.Decide("implement webhooks", 0)
	if dec.Provider != "gemini" {
		t.Errorf("expected cheapest provider, got %q", dec.Provider)
	}
	if dec.MaxCost != 10.0 {
		t.Errorf("expected default max_cost=10.0, got %f", dec.MaxCost)
	}
}

func TestProviderPool_SetAvailable(t *testing.T) {
	pool := NewProviderPool(testProviders)

	pool.SetAvailable("claude", false)
	avail := pool.Available()

	for _, p := range avail {
		if p.Name == "claude" {
			t.Error("claude should not be available after SetAvailable(false)")
		}
	}
	if len(avail) != 2 {
		t.Errorf("expected 2 available providers, got %d", len(avail))
	}
}

func TestSmartRouter_ConcurrentDecide(t *testing.T) {
	sr := NewSmartRouter(testProviders)
	sr.SetRouteConfig(RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeBugFix, PreferredProvider: "claude", MaxCost: 1.0, Priority: 1},
		},
	})

	var wg sync.WaitGroup
	descriptions := []string{
		"fix login bug",
		"add new feature",
		"refactor the auth module",
		"write tests for router",
		"update documentation",
		"research caching approaches",
	}

	for range 50 {
		for _, desc := range descriptions {
			wg.Add(1)
			go func(d string) {
				defer wg.Done()
				dec := sr.Decide(d, 5.0)
				if dec.Provider == "" {
					t.Errorf("got empty provider for %q", d)
				}
			}(desc)
		}
	}
	wg.Wait()
}

func TestRoutingDecision_Fields(t *testing.T) {
	dec := RoutingDecision{
		Provider: "claude",
		Model:    "opus",
		MaxCost:  2.5,
		Reason:   "test",
	}
	if dec.Provider != "claude" || dec.Model != "opus" || dec.MaxCost != 2.5 {
		t.Errorf("unexpected fields: %+v", dec)
	}
}
