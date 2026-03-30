package session

import (
	"testing"
)

func TestDefaultAdaptiveDepthConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultAdaptiveDepthConfig()
	if cfg.BaseDepth != DefaultDepth {
		t.Fatalf("expected BaseDepth=%d, got %d", DefaultDepth, cfg.BaseDepth)
	}
	if cfg.MinAdaptiveDepth != MinDepth {
		t.Fatalf("expected MinAdaptiveDepth=%d, got %d", MinDepth, cfg.MinAdaptiveDepth)
	}
	if cfg.MaxAdaptiveDepth != MaxDepth {
		t.Fatalf("expected MaxAdaptiveDepth=%d, got %d", MaxDepth, cfg.MaxAdaptiveDepth)
	}
	if cfg.QuickWinThreshold != 0.4 {
		t.Fatalf("expected QuickWinThreshold=0.4, got %f", cfg.QuickWinThreshold)
	}
}

func TestNewAdaptiveDepth(t *testing.T) {
	t.Parallel()
	cfg := DefaultAdaptiveDepthConfig()
	ad := NewAdaptiveDepth(cfg)
	if ad == nil {
		t.Fatal("expected non-nil AdaptiveDepth")
	}
	if len(ad.history) != 0 {
		t.Fatalf("expected empty history, got %d entries", len(ad.history))
	}
}

func TestAdaptiveDepth_RecommendDepth_NoHistory(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	tests := []struct {
		name       string
		complexity ComplexityLevel
		wantMin    int
		wantMax    int
	}{
		{"low complexity", ComplexityLow, 3, 50},
		{"medium complexity", ComplexityMedium, 3, 50},
		{"high complexity", ComplexityHigh, 3, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depth := ad.RecommendDepth("unknown-pattern", tt.complexity)
			if depth < tt.wantMin || depth > tt.wantMax {
				t.Fatalf("RecommendDepth=%d out of range [%d, %d]", depth, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestAdaptiveDepth_RecommendDepth_WithHistory(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	// Record a quick win to adjust depth down.
	ad.RecordSignal("fast-task", ProgressSignal{
		Iteration:    2,
		TotalDepth:   10,
		ProgressRate: 0.9,
		Completed:    true,
	})

	depth := ad.RecommendDepth("fast-task", ComplexityMedium)
	if depth >= DefaultDepth {
		t.Fatalf("expected reduced depth, got %d (default=%d)", depth, DefaultDepth)
	}
}

func TestAdaptiveDepth_RecordSignal_QuickWin(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	ad.RecordSignal("qw", ProgressSignal{
		Iteration:    2,
		TotalDepth:   10,
		ProgressRate: 0.8,
		Completed:    true,
	})

	stats := ad.Stats("qw")
	if stats.QuickWins != 1 {
		t.Fatalf("expected 1 quick win, got %d", stats.QuickWins)
	}
	if stats.AdjustedDepth == 0 {
		t.Fatal("expected non-zero adjusted depth after quick win")
	}
}

func TestAdaptiveDepth_RecordSignal_Stall(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	// Record a stall signal: low progress past midpoint.
	ad.RecordSignal("stall", ProgressSignal{
		Iteration:    8,
		TotalDepth:   10,
		ProgressRate: 0.05,
		Completed:    false,
	})

	stats := ad.Stats("stall")
	if stats.Stalls != 1 {
		t.Fatalf("expected 1 stall, got %d", stats.Stalls)
	}
	// Depth should have been adjusted up.
	if stats.AdjustedDepth <= DefaultDepth {
		t.Fatalf("expected increased depth, got %d (default=%d)", stats.AdjustedDepth, DefaultDepth)
	}
}

func TestAdaptiveDepth_RecordSignal_ErrorNearExhaustion(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	ad.RecordSignal("err", ProgressSignal{
		Iteration:    9,
		TotalDepth:   10,
		ProgressRate: 0.5,
		Errored:      true,
	})

	depth := ad.CurrentDepth("err")
	if depth <= DefaultDepth {
		t.Fatalf("expected increased depth after error near exhaustion, got %d", depth)
	}
}

func TestAdaptiveDepth_RecordSignal_Completion(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	// Complete at 80% — not a quick win (threshold is 0.4).
	ad.RecordSignal("normal", ProgressSignal{
		Iteration:    8,
		TotalDepth:   10,
		ProgressRate: 0.5,
		Completed:    true,
	})

	stats := ad.Stats("normal")
	if stats.Completions != 1 {
		t.Fatalf("expected 1 completion, got %d", stats.Completions)
	}
	if stats.QuickWins != 0 {
		t.Fatalf("expected 0 quick wins, got %d", stats.QuickWins)
	}
}

func TestAdaptiveDepth_CurrentDepth_UnknownPattern(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())
	if d := ad.CurrentDepth("nonexistent"); d != 0 {
		t.Fatalf("expected 0 for unknown pattern, got %d", d)
	}
}

func TestAdaptiveDepth_Stats_UnknownPattern(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())
	stats := ad.Stats("nope")
	if stats.SignalCount != 0 || stats.AdjustedDepth != 0 {
		t.Fatalf("expected zero stats for unknown pattern: %+v", stats)
	}
}

func TestAdaptiveDepth_Stats_AvgProgressRate(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	ad.RecordSignal("avg", ProgressSignal{Iteration: 5, TotalDepth: 10, ProgressRate: 0.6})
	ad.RecordSignal("avg", ProgressSignal{Iteration: 6, TotalDepth: 10, ProgressRate: 0.8})

	stats := ad.Stats("avg")
	if stats.SignalCount != 2 {
		t.Fatalf("expected 2 signals, got %d", stats.SignalCount)
	}
	expected := 0.7
	if stats.AvgProgressRate < expected-0.01 || stats.AvgProgressRate > expected+0.01 {
		t.Fatalf("expected avg progress ~%.2f, got %.2f", expected, stats.AvgProgressRate)
	}
}

func TestAdaptiveDepth_Reset(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())
	ad.RecordSignal("pattern", ProgressSignal{Iteration: 2, TotalDepth: 10, Completed: true, ProgressRate: 0.9})

	if ad.CurrentDepth("pattern") == 0 {
		t.Fatal("expected non-zero depth before reset")
	}

	ad.Reset()
	if ad.CurrentDepth("pattern") != 0 {
		t.Fatal("expected zero depth after reset")
	}
}

func TestAdaptiveDepth_SignalWindowCap(t *testing.T) {
	t.Parallel()
	ad := NewAdaptiveDepth(DefaultAdaptiveDepthConfig())

	// Record 25 signals — should be capped at 20.
	for i := 0; i < 25; i++ {
		ad.RecordSignal("cap", ProgressSignal{
			Iteration:    5,
			TotalDepth:   10,
			ProgressRate: 0.5,
		})
	}
	stats := ad.Stats("cap")
	if stats.SignalCount != 20 {
		t.Fatalf("expected 20 signals (capped), got %d", stats.SignalCount)
	}
}

func TestAdaptiveDepth_ClampBounds(t *testing.T) {
	t.Parallel()
	cfg := DefaultAdaptiveDepthConfig()
	cfg.DepthDecrement = 100 // extreme decrement to test floor
	ad := NewAdaptiveDepth(cfg)

	ad.RecordSignal("floor", ProgressSignal{
		Iteration:    1,
		TotalDepth:   10,
		ProgressRate: 0.9,
		Completed:    true,
	})

	depth := ad.CurrentDepth("floor")
	if depth < cfg.MinAdaptiveDepth {
		t.Fatalf("depth %d below min %d", depth, cfg.MinAdaptiveDepth)
	}
}
