//go:build e2e_live

package e2e

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestE2ELiveFire runs scenarios against real LLM providers.
// Requires: e2e_live build tag, ANTHROPIC_API_KEY set, real CLI binaries.
//
// Run with:
//
//	go test -tags e2e_live -run TestE2ELiveFire ./internal/e2e/ -v -timeout 30m
func TestE2ELiveFire(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Live-fire uses real providers — only run anchor scenarios to control cost
	anchors := []Scenario{
		TrivialFix(),
		FeatureAddition(),
	}

	for _, s := range anchors {
		s := s
		t.Run("live-"+s.Name, func(t *testing.T) {
			// Live tests are inherently sequential — don't parallelize
			_ = ctx
			t.Logf("scenario %s: category=%s budget=%.2f", s.Name, s.Category, s.Constraints.MaxCostUSD)
			// TODO: Wire up real Manager with real provider sessions
			// For now this is a placeholder that validates the build tag works
			t.Skip("live-fire not yet wired to real providers")
		})
	}
}
