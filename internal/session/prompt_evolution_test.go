package session

import (
	"encoding/json"
	"testing"
)

func TestDefaultPromptEvolutionConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultPromptEvolutionConfig()
	if cfg.TournamentSize != 3 {
		t.Fatalf("expected TournamentSize=3, got %d", cfg.TournamentSize)
	}
	if cfg.MutationRate != 0.2 {
		t.Fatalf("expected MutationRate=0.2, got %f", cfg.MutationRate)
	}
	if cfg.MaxVariants != 10 {
		t.Fatalf("expected MaxVariants=10, got %d", cfg.MaxVariants)
	}
}

func TestNewPromptEvolution_Defaults(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(PromptEvolutionConfig{})
	if pe.tournamentSize != 3 {
		t.Fatalf("expected default tournamentSize=3, got %d", pe.tournamentSize)
	}
	if pe.mutationRate != 0.2 {
		t.Fatalf("expected default mutationRate=0.2, got %f", pe.mutationRate)
	}
}

func TestPromptEvolution_AddVariant(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())

	id := pe.AddVariant("feature", "implement the feature")
	if id == "" {
		t.Fatal("expected non-empty variant ID")
	}

	if pe.VariantCount() != 1 {
		t.Fatalf("expected 1 variant, got %d", pe.VariantCount())
	}
}

func TestPromptEvolution_AddVariant_PopulationCap(t *testing.T) {
	t.Parallel()
	cfg := DefaultPromptEvolutionConfig()
	cfg.MaxVariants = 3
	pe := NewPromptEvolution(cfg)

	for range 5 {
		pe.AddVariant("test", "variant text")
	}
	if pe.VariantCount() > 3 {
		t.Fatalf("expected max 3 variants, got %d", pe.VariantCount())
	}
}

func TestPromptEvolution_RecordTrial(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	id := pe.AddVariant("fix", "fix the bug")

	pe.RecordTrial(PromptTrialResult{
		VariantID: id,
		TaskType:  "fix",
		Success:   true,
		CostUSD:   0.10,
		DurSec:    5.0,
		Quality:   0.9,
	})

	// Check that fitness was updated.
	lb := pe.Leaderboard("fix", 1)
	if len(lb) != 1 {
		t.Fatalf("expected 1 leaderboard entry, got %d", len(lb))
	}
	if lb[0].Trials != 1 {
		t.Fatalf("expected 1 trial, got %d", lb[0].Trials)
	}
	if lb[0].Successes != 1 {
		t.Fatalf("expected 1 success, got %d", lb[0].Successes)
	}
}

func TestPromptEvolution_RecordTrial_UnknownTaskType(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	// Should not panic.
	pe.RecordTrial(PromptTrialResult{VariantID: "nonexistent", TaskType: "unknown"})
}

func TestPromptEvolution_RecordTrial_UnknownVariant(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	pe.AddVariant("fix", "fix it")
	// Record for a variant that doesn't exist in this population.
	pe.RecordTrial(PromptTrialResult{VariantID: "wrong-id", TaskType: "fix"})
	// Should not crash; variant count unchanged.
	if pe.VariantCount() != 1 {
		t.Fatalf("expected 1 variant, got %d", pe.VariantCount())
	}
}

func TestPromptEvolution_SelectBest_Empty(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	template, id := pe.SelectBest("nonexistent")
	if template != "" || id != "" {
		t.Fatalf("expected empty for nonexistent, got template=%q id=%q", template, id)
	}
}

func TestPromptEvolution_SelectBest_SingleVariant(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	id := pe.AddVariant("test", "the template")
	template, gotID := pe.SelectBest("test")
	if template != "the template" {
		t.Fatalf("expected 'the template', got %q", template)
	}
	if gotID != id {
		t.Fatalf("expected ID %q, got %q", id, gotID)
	}
}

func TestPromptEvolution_SelectBest_MultipleVariants(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	pe.AddVariant("test", "variant A")
	id2 := pe.AddVariant("test", "variant B")

	// Record trials to make variant B clearly better.
	for range 10 {
		pe.RecordTrial(PromptTrialResult{VariantID: id2, TaskType: "test", Success: true, CostUSD: 0.01})
	}

	template, _ := pe.SelectBest("test")
	if template == "" {
		t.Fatal("expected non-empty template")
	}
}

func TestPromptEvolution_Mutate_NoVariants(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	id := pe.Mutate("nonexistent")
	if id != "" {
		t.Fatalf("expected empty for nonexistent, got %q", id)
	}
}

func TestPromptEvolution_Mutate_WithVariants(t *testing.T) {
	t.Parallel()
	cfg := DefaultPromptEvolutionConfig()
	cfg.MutationRate = 1.0 // always mutate
	pe := NewPromptEvolution(cfg)
	pe.AddVariant("fix", "implement the fix carefully. ensure tests pass. add docs.")

	id := pe.Mutate("fix")
	if id == "" {
		t.Fatal("expected mutation to produce a new variant")
	}
	if pe.VariantCount() != 2 {
		t.Fatalf("expected 2 variants after mutation, got %d", pe.VariantCount())
	}
}

func TestPromptEvolution_Leaderboard_Empty(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	lb := pe.Leaderboard("nonexistent", 5)
	if lb != nil {
		t.Fatalf("expected nil leaderboard, got %v", lb)
	}
}

func TestPromptEvolution_Leaderboard_Sorted(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	id1 := pe.AddVariant("test", "bad variant")
	id2 := pe.AddVariant("test", "good variant")

	// Make id2 clearly better.
	for range 10 {
		pe.RecordTrial(PromptTrialResult{VariantID: id2, TaskType: "test", Success: true, CostUSD: 0.01})
		pe.RecordTrial(PromptTrialResult{VariantID: id1, TaskType: "test", Success: false, CostUSD: 1.0})
	}

	lb := pe.Leaderboard("test", 2)
	if len(lb) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(lb))
	}
	if lb[0].ID != id2 {
		t.Fatalf("expected best variant first, got %q", lb[0].ID)
	}
}

func TestPromptEvolution_Leaderboard_LimitN(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	for range 5 {
		pe.AddVariant("test", "variant")
	}
	lb := pe.Leaderboard("test", 2)
	if len(lb) != 2 {
		t.Fatalf("expected 2 entries (limited), got %d", len(lb))
	}
}

func TestPromptEvolution_PopulationSnapshot(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	pe.AddVariant("fix", "fix it")
	pe.AddVariant("test", "test it")

	snap := pe.PopulationSnapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 populations, got %d", len(snap))
	}
	if snap["fix"] == nil || snap["test"] == nil {
		t.Fatal("expected both populations in snapshot")
	}
}

func TestPromptEvolution_VariantCount(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	pe.AddVariant("a", "v1")
	pe.AddVariant("a", "v2")
	pe.AddVariant("b", "v3")
	if pe.VariantCount() != 3 {
		t.Fatalf("expected 3, got %d", pe.VariantCount())
	}
}

func TestPromptEvolution_MarshalJSON(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	pe.AddVariant("fix", "fix the bug")

	data, err := pe.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["fix"]; !ok {
		t.Fatal("expected 'fix' key in JSON")
	}
}

func TestPromptEvolution_ComputeFitness_ZeroTrials(t *testing.T) {
	t.Parallel()
	pe := NewPromptEvolution(DefaultPromptEvolutionConfig())
	v := &PromptVariant{Trials: 0}
	pe.mu.Lock()
	fitness := pe.computeFitness(v)
	pe.mu.Unlock()
	if fitness != 0.5 {
		t.Fatalf("expected 0.5 for untested, got %f", fitness)
	}
}

func TestPromptEvolution_MutationStrategies(t *testing.T) {
	t.Parallel()
	cfg := DefaultPromptEvolutionConfig()
	cfg.MutationRate = 1.0
	pe := NewPromptEvolution(cfg)

	template := "implement the fix carefully. ensure all tests pass. add documentation."

	// Run multiple mutations to exercise different strategies.
	for range 20 {
		pe.mu.Lock()
		mutType, mutated := pe.applyMutation(template)
		pe.mu.Unlock()
		if mutType == "" {
			t.Fatal("expected non-empty mutation type")
		}
		if mutated == "" {
			t.Fatal("expected non-empty mutated template")
		}
	}
}
