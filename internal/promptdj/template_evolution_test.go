package promptdj

import "testing"

func TestTemplateEvolution_Lifecycle(t *testing.T) {
	te := NewTemplateEvolution()

	// Add template
	te.AddTemplate("code_pattern_1", "Write a {{LANGUAGE}} function that {{TASK}}", "code")

	// Record trials
	te.RecordTrial("code_pattern_1", true, 0.10, 75)
	te.RecordTrial("code_pattern_1", true, 0.08, 80)
	te.RecordTrial("code_pattern_1", false, 0.20, 50)
	te.RecordTrial("code_pattern_1", true, 0.12, 70)
	te.RecordTrial("code_pattern_1", true, 0.09, 85)

	// Check fitness computed
	best, ok := te.SelectBest("code")
	if !ok {
		t.Fatal("expected to find best template for code")
	}
	if best.Trials != 5 {
		t.Errorf("expected 5 trials, got %d", best.Trials)
	}
	if best.Fitness <= 0 {
		t.Errorf("expected positive fitness, got %.3f", best.Fitness)
	}
}

func TestTemplateEvolution_AutoPromote(t *testing.T) {
	te := NewTemplateEvolution()
	te.AddTemplate("high_performer", "Test template", "analysis")

	// Record 5 successful trials (should trigger promotion)
	for i := 0; i < 5; i++ {
		te.RecordTrial("high_performer", true, 0.05, 90)
	}

	best, ok := te.SelectBest("analysis")
	if !ok {
		t.Fatal("expected to find template")
	}
	if best.Status != "promoted" {
		t.Errorf("expected promoted status after 5 successes, got %s (fitness=%.3f)", best.Status, best.Fitness)
	}
}

func TestTemplateEvolution_AutoDemote(t *testing.T) {
	te := NewTemplateEvolution()
	te.AddTemplate("low_performer", "Bad template", "workflow")

	// Record 5 failures (should trigger demotion)
	for i := 0; i < 5; i++ {
		te.RecordTrial("low_performer", false, 0.50, 20)
	}

	_, ok := te.SelectBest("workflow")
	if ok {
		t.Error("demoted template should not be returned by SelectBest")
	}
}

func TestTemplateEvolution_Leaderboard(t *testing.T) {
	te := NewTemplateEvolution()
	te.AddTemplate("t1", "Template 1", "code")
	te.AddTemplate("t2", "Template 2", "code")

	te.RecordTrial("t1", true, 0.1, 80)
	te.RecordTrial("t2", false, 0.5, 30)

	board := te.Leaderboard("code", 10)
	if len(board) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(board))
	}
	if board[0].Name != "t1" {
		t.Error("expected t1 to be ranked first (higher fitness)")
	}
}

func TestTemplateEvolution_Stats(t *testing.T) {
	te := NewTemplateEvolution()
	te.AddTemplate("s1", "Template", "code")
	te.RecordTrial("s1", true, 0.1, 80)

	stats := te.Stats()
	if stats["total_templates"].(int) != 1 {
		t.Error("expected 1 template")
	}
	if stats["total_trials"].(int) != 1 {
		t.Error("expected 1 trial")
	}
}

func TestTemplateEvolution_History(t *testing.T) {
	te := NewTemplateEvolution()
	te.AddTemplate("h1", "Template", "code")
	te.RecordTrial("h1", true, 0.1, 80)

	history := te.History(10)
	if len(history) < 2 {
		t.Errorf("expected at least 2 events (created + trial), got %d", len(history))
	}
}
