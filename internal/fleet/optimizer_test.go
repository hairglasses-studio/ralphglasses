package fleet

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewOptimizer(t *testing.T) {
	opt := NewOptimizer(nil)
	if opt == nil {
		t.Fatal("expected non-nil optimizer")
	}

	// Default weights: Claude=1.0, Gemini=1.0, Codex=1.2 (cost-efficient default)
	wantWeights := map[session.Provider]float64{
		session.ProviderClaude: 1.0,
		session.ProviderGemini: 1.0,
		session.ProviderCodex:  1.2,
	}
	for p, want := range wantWeights {
		if w := opt.ProviderWeight(p); w != want {
			t.Errorf("default weight for %s: got %f, want %f", p, w, want)
		}
	}
}

func TestOptimizer_ProviderWeightUnknown(t *testing.T) {
	opt := NewOptimizer(nil)
	if w := opt.ProviderWeight("unknown"); w != 1.0 {
		t.Errorf("unknown provider weight: got %f, want 1.0", w)
	}
}

func TestOptimizer_RepoWeight(t *testing.T) {
	opt := NewOptimizer(nil)

	// Default weight for unknown repo
	if w := opt.RepoWeight("unknown-repo"); w != 1.0 {
		t.Errorf("unknown repo weight: got %f, want 1.0", w)
	}
}

func TestOptimizer_UpdateWeightsNilFeedback(t *testing.T) {
	opt := NewOptimizer(nil)
	// Should not panic with nil feedback
	opt.UpdateWeights()
}

func TestOptimizer_UpdateWeightsWithFeedback(t *testing.T) {
	fa := session.NewFeedbackAnalyzer("", 3)

	// Ingest enough samples to trigger weight updates
	for range 5 {
		fa.Ingest([]session.JournalEntry{
			{
				Provider:   string(session.ProviderClaude),
				SpentUSD:   0.50,
				TurnCount:  10,
				ExitReason: "success",
			},
		})
	}

	opt := NewOptimizer(fa)
	opt.UpdateWeights()

	// Weight should have been adjusted from default
	w := opt.ProviderWeight(session.ProviderClaude)
	if w == 0 {
		t.Error("claude weight should be non-zero after update")
	}
}

func TestOptimizer_SetBanditStats(t *testing.T) {
	fa := session.NewFeedbackAnalyzer("", 3)

	// Ingest samples
	for range 5 {
		fa.Ingest([]session.JournalEntry{
			{
				Provider:   string(session.ProviderClaude),
				SpentUSD:   0.50,
				TurnCount:  10,
				ExitReason: "success",
			},
		})
	}

	opt := NewOptimizer(fa)

	opt.SetBanditStats(func() map[string]float64 {
		return map[string]float64{
			string(session.ProviderClaude): 0.8,
			string(session.ProviderGemini): 0.6,
		}
	})

	opt.UpdateWeights()

	// Should not panic, weights should be reasonable
	w := opt.ProviderWeight(session.ProviderClaude)
	if w <= 0 {
		t.Errorf("claude weight should be positive, got %f", w)
	}
}

func TestOptimizer_IngestCrossWorkerJournals(t *testing.T) {
	fa := session.NewFeedbackAnalyzer("", 3)
	opt := NewOptimizer(fa)

	entries := make([]session.JournalEntry, 5)
	for i := range entries {
		entries[i] = session.JournalEntry{
			Provider:   string(session.ProviderGemini),
			SpentUSD:   0.10,
			TurnCount:  5,
			ExitReason: "success",
		}
	}

	opt.IngestCrossWorkerJournals(entries)

	w := opt.ProviderWeight(session.ProviderGemini)
	if w <= 0 {
		t.Errorf("gemini weight should be positive, got %f", w)
	}
}

func TestOptimizer_IngestCrossWorkerJournalsNilFeedback(t *testing.T) {
	opt := NewOptimizer(nil)
	// Should not panic
	opt.IngestCrossWorkerJournals(nil)
	opt.IngestCrossWorkerJournals([]session.JournalEntry{{Provider: "claude"}})
}

func TestOptimizer_IngestCrossWorkerJournalsEmpty(t *testing.T) {
	fa := session.NewFeedbackAnalyzer("", 3)
	opt := NewOptimizer(fa)
	// Should not panic with empty slice
	opt.IngestCrossWorkerJournals(nil)
}

func TestOptimizer_Summary(t *testing.T) {
	opt := NewOptimizer(nil)
	summary := opt.Summary()

	if _, ok := summary["provider_weights"]; !ok {
		t.Error("summary missing provider_weights")
	}
	if _, ok := summary["repo_weights"]; !ok {
		t.Error("summary missing repo_weights")
	}
	if _, ok := summary["last_optimized"]; !ok {
		t.Error("summary missing last_optimized")
	}

	pw, _ := summary["provider_weights"].(map[string]float64)
	if len(pw) != 3 {
		t.Errorf("expected 3 provider weights, got %d", len(pw))
	}
}
