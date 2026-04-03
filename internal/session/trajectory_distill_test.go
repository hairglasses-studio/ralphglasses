package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seedEpisodicMemory creates an EpisodicMemory pre-loaded with episodes.
func seedEpisodicMemory(t *testing.T, dir string, episodes []Episode) *EpisodicMemory {
	t.Helper()
	em := NewEpisodicMemory(dir, 500, 5)
	em.mu.Lock()
	em.episodes = episodes
	em.mu.Unlock()
	return em
}

// seedReflexionStore creates a ReflexionStore pre-loaded with reflections.
func seedReflexionStore(t *testing.T, dir string, reflections []Reflection) *ReflexionStore {
	t.Helper()
	rs := NewReflexionStore(dir)
	rs.mu.Lock()
	rs.reflections = reflections
	rs.mu.Unlock()
	return rs
}

func TestDistill_FromEpisodes(t *testing.T) {
	dir := t.TempDir()

	episodes := []Episode{
		{
			Timestamp: time.Now().Add(-2 * time.Hour),
			TaskType:  "feature",
			TaskTitle: "add login endpoint",
			Prompt:    "add login endpoint",
			Provider:  "claude",
			Worked:    []string{"wrote tests first", "used table-driven tests"},
		},
		{
			Timestamp: time.Now().Add(-1 * time.Hour),
			TaskType:  "feature",
			TaskTitle: "add signup endpoint",
			Prompt:    "add signup endpoint",
			Provider:  "claude",
			Worked:    []string{"wrote tests first", "clean error handling"},
		},
		{
			Timestamp: time.Now(),
			TaskType:  "feature",
			TaskTitle: "add profile endpoint",
			Prompt:    "add profile endpoint",
			Provider:  "gemini",
			Worked:    []string{"wrote tests first", "used table-driven tests"},
		},
	}

	em := seedEpisodicMemory(t, dir, episodes)
	rs := NewReflexionStore(dir)
	td := NewTrajectoryDistiller(em, rs, dir)

	principles, err := td.Distill()
	if err != nil {
		t.Fatalf("Distill() error: %v", err)
	}

	if len(principles) == 0 {
		t.Fatal("expected at least one principle, got none")
	}

	// "wrote tests first" appears in all 3 episodes, should be distilled.
	found := false
	for _, p := range principles {
		if strings.Contains(strings.ToLower(p.Principle), "wrote tests first") {
			found = true
			if p.Source != "episode" {
				t.Errorf("expected source 'episode', got %q", p.Source)
			}
			if p.SourceCount < 2 {
				t.Errorf("expected source_count >= 2, got %d", p.SourceCount)
			}
			if p.Confidence <= 0 || p.Confidence > 1.0 {
				t.Errorf("expected confidence in (0,1], got %f", p.Confidence)
			}
			if !containsString(p.TaskTypes, "feature") {
				t.Errorf("expected task_types to contain 'feature', got %v", p.TaskTypes)
			}
			break
		}
	}
	if !found {
		t.Error("expected a principle about 'wrote tests first'")
	}
}

func TestDistill_FromReflexion(t *testing.T) {
	dir := t.TempDir()

	// Rules() requires >= 5 reflections and >= 3 with the same failure mode.
	reflections := []Reflection{
		{Timestamp: time.Now(), TaskTitle: "fix auth bug", FailureMode: "verify_failed", RootCause: "missing test assertion"},
		{Timestamp: time.Now(), TaskTitle: "fix auth bug", FailureMode: "verify_failed", RootCause: "missing test assertion"},
		{Timestamp: time.Now(), TaskTitle: "fix routing", FailureMode: "verify_failed", RootCause: "missing test assertion"},
		{Timestamp: time.Now(), TaskTitle: "add feature x", FailureMode: "worker_error", RootCause: "timeout"},
		{Timestamp: time.Now(), TaskTitle: "refactor code", FailureMode: "planner_error", RootCause: "parse failure"},
	}

	em := NewEpisodicMemory(dir, 500, 5)
	rs := seedReflexionStore(t, dir, reflections)
	td := NewTrajectoryDistiller(em, rs, dir)

	principles, err := td.Distill()
	if err != nil {
		t.Fatalf("Distill() error: %v", err)
	}

	// Should have at least one principle from the verify_failed rule.
	found := false
	for _, p := range principles {
		if p.Source == "reflexion" && strings.Contains(p.Principle, "verify_failed") {
			found = true
			if p.SourceCount < 3 {
				t.Errorf("expected reflexion source_count >= 3, got %d", p.SourceCount)
			}
			break
		}
	}
	if !found {
		t.Error("expected a reflexion-sourced principle about verify_failed")
	}
}

func TestDistill_MinimumEpisodes(t *testing.T) {
	dir := t.TempDir()

	// Single episode per task type: not enough to distill.
	episodes := []Episode{
		{
			Timestamp: time.Now(),
			TaskType:  "docs",
			TaskTitle: "write readme",
			Prompt:    "write readme",
			Provider:  "claude",
			Worked:    []string{"used examples"},
		},
	}

	em := seedEpisodicMemory(t, dir, episodes)
	rs := NewReflexionStore(dir)
	td := NewTrajectoryDistiller(em, rs, dir)

	principles, err := td.Distill()
	if err != nil {
		t.Fatalf("Distill() error: %v", err)
	}

	if len(principles) != 0 {
		t.Errorf("expected 0 principles from single episode, got %d", len(principles))
	}
}

func TestDistill_CrossTypePrinciples(t *testing.T) {
	dir := t.TempDir()

	// Same "worked" item across 3 different task types => universal principle.
	episodes := []Episode{
		{TaskType: "feature", Worked: []string{"incremental commits"}},
		{TaskType: "feature", Worked: []string{"incremental commits"}},
		{TaskType: "bug_fix", Worked: []string{"incremental commits"}},
		{TaskType: "bug_fix", Worked: []string{"incremental commits"}},
		{TaskType: "refactor", Worked: []string{"incremental commits"}},
		{TaskType: "refactor", Worked: []string{"incremental commits"}},
	}

	em := seedEpisodicMemory(t, dir, episodes)
	rs := NewReflexionStore(dir)
	td := NewTrajectoryDistiller(em, rs, dir)

	principles, err := td.Distill()
	if err != nil {
		t.Fatalf("Distill() error: %v", err)
	}

	found := false
	for _, p := range principles {
		if strings.Contains(p.Principle, "Universal principle") && strings.Contains(strings.ToLower(p.Principle), "incremental commits") {
			found = true
			if len(p.TaskTypes) < 3 {
				t.Errorf("expected universal principle to span 3+ task types, got %d", len(p.TaskTypes))
			}
			break
		}
	}
	if !found {
		t.Error("expected a universal principle about 'incremental commits'")
	}
}

func TestApplicable(t *testing.T) {
	dir := t.TempDir()
	td := NewTrajectoryDistiller(nil, nil, dir)

	td.mu.Lock()
	td.principles = []StrategicPrinciple{
		{ID: "1", Principle: "p1", Confidence: 0.9, TaskTypes: []string{"feature"}},
		{ID: "2", Principle: "p2", Confidence: 0.5, TaskTypes: []string{"bug_fix"}},
		{ID: "3", Principle: "p3", Confidence: 0.7, TaskTypes: []string{"feature", "refactor"}},
		{ID: "4", Principle: "p4", Confidence: 0.3, TaskTypes: []string{"general"}},
	}
	td.mu.Unlock()

	t.Run("filter by task type", func(t *testing.T) {
		got := td.Applicable("feature", 10)
		// Should match p1 (feature), p3 (feature+refactor), p4 (general).
		if len(got) != 3 {
			t.Fatalf("expected 3 applicable principles, got %d", len(got))
		}
		// Should be sorted by confidence descending.
		if got[0].ID != "1" {
			t.Errorf("expected highest confidence first (id=1), got %q", got[0].ID)
		}
		if got[1].ID != "3" {
			t.Errorf("expected second highest (id=3), got %q", got[1].ID)
		}
	})

	t.Run("limit results", func(t *testing.T) {
		got := td.Applicable("feature", 2)
		if len(got) != 2 {
			t.Fatalf("expected 2 principles with limit=2, got %d", len(got))
		}
	})

	t.Run("empty task type matches all", func(t *testing.T) {
		got := td.Applicable("", 10)
		if len(got) != 4 {
			t.Fatalf("expected 4 principles with empty taskType, got %d", len(got))
		}
	})

	t.Run("default limit", func(t *testing.T) {
		got := td.Applicable("feature", 0)
		// Default limit is 5; only 3 match feature.
		if len(got) != 3 {
			t.Fatalf("expected 3 with default limit, got %d", len(got))
		}
	})
}

func TestFormatForPrompt_Distiller(t *testing.T) {
	td := NewTrajectoryDistiller(nil, nil, "")

	t.Run("empty", func(t *testing.T) {
		got := td.FormatForPrompt(nil)
		if got != "" {
			t.Errorf("expected empty string for nil principles, got %q", got)
		}
	})

	t.Run("formatted output", func(t *testing.T) {
		principles := []StrategicPrinciple{
			{Principle: "always write tests first", Confidence: 0.95, SourceCount: 5},
			{Principle: "avoid verify_failed", Confidence: 0.5, SourceCount: 1},
		}
		got := td.FormatForPrompt(principles)

		if !strings.Contains(got, "Strategic Principles") {
			t.Error("expected header in output")
		}
		if !strings.Contains(got, "[HIGH]") {
			t.Error("expected [HIGH] confidence label for 0.95")
		}
		if !strings.Contains(got, "[LOW]") {
			t.Error("expected [LOW] confidence label for 0.5")
		}
		if !strings.Contains(got, "always write tests first") {
			t.Error("expected principle text in output")
		}
		if !strings.Contains(got, "_(observed 5x)_") {
			t.Error("expected observation count for source_count > 1")
		}
		// source_count == 1 should not get the observation suffix.
		if strings.Contains(got, "_(observed 1x)_") {
			t.Error("did not expect observation count for source_count == 1")
		}
	})
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Millisecond) // JSON loses sub-ms precision

	original := []StrategicPrinciple{
		{
			ID:          "test-1",
			Principle:   "always write tests",
			Source:      "episode",
			SourceCount: 4,
			Confidence:  0.8,
			TaskTypes:   []string{"feature", "bug_fix"},
			CreatedAt:   now,
			LastUsed:    now,
			UsageCount:  2,
		},
		{
			ID:          "test-2",
			Principle:   "avoid panics",
			Source:      "reflexion",
			SourceCount: 3,
			Confidence:  0.6,
			TaskTypes:   []string{"general"},
			CreatedAt:   now,
		},
	}

	// Save.
	td1 := NewTrajectoryDistiller(nil, nil, dir)
	td1.mu.Lock()
	td1.principles = original
	td1.mu.Unlock()

	if err := td1.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, principlesFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("saved file is empty")
	}

	// Verify valid JSON.
	var parsed []StrategicPrinciple
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 principles in file, got %d", len(parsed))
	}

	// Load into a fresh distiller.
	td2 := NewTrajectoryDistiller(nil, nil, dir)
	loaded := td2.Principles()

	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded principles, got %d", len(loaded))
	}

	if loaded[0].ID != "test-1" {
		t.Errorf("expected ID 'test-1', got %q", loaded[0].ID)
	}
	if loaded[0].Principle != "always write tests" {
		t.Errorf("expected principle text mismatch: %q", loaded[0].Principle)
	}
	if loaded[0].Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", loaded[0].Confidence)
	}
	if loaded[0].UsageCount != 2 {
		t.Errorf("expected usage_count 2, got %d", loaded[0].UsageCount)
	}
	if len(loaded[0].TaskTypes) != 2 {
		t.Errorf("expected 2 task types, got %d", len(loaded[0].TaskTypes))
	}
}

func TestSaveLoad_EmptyStateDir(t *testing.T) {
	td := NewTrajectoryDistiller(nil, nil, "")

	if err := td.Save(); err != nil {
		t.Errorf("Save() with empty stateDir should be no-op, got error: %v", err)
	}
	if err := td.Load(); err != nil {
		t.Errorf("Load() with empty stateDir should be no-op, got error: %v", err)
	}
}

func TestSaveLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	td := NewTrajectoryDistiller(nil, nil, dir)

	// No file exists yet, Load should succeed with no principles.
	if err := td.Load(); err != nil {
		t.Fatalf("Load() with no file should succeed, got error: %v", err)
	}
	if len(td.Principles()) != 0 {
		t.Errorf("expected 0 principles from missing file, got %d", len(td.Principles()))
	}
}

func TestMarkUsed(t *testing.T) {
	td := NewTrajectoryDistiller(nil, nil, "")
	td.mu.Lock()
	td.principles = []StrategicPrinciple{
		{ID: "abc", Principle: "test", UsageCount: 0},
		{ID: "def", Principle: "other", UsageCount: 5},
	}
	td.mu.Unlock()

	td.MarkUsed("abc")
	td.MarkUsed("abc")
	td.MarkUsed("nonexistent") // should be no-op

	td.mu.RLock()
	p0 := td.principles[0]
	p1 := td.principles[1]
	td.mu.RUnlock()

	if p0.UsageCount != 2 {
		t.Errorf("expected usage_count 2 for 'abc', got %d", p0.UsageCount)
	}
	if p0.LastUsed.IsZero() {
		t.Error("expected LastUsed to be set for 'abc'")
	}
	if p1.UsageCount != 5 {
		t.Errorf("expected usage_count 5 for 'def' (unchanged), got %d", p1.UsageCount)
	}
}

func TestMergePrinciples_Dedup(t *testing.T) {
	existing := []StrategicPrinciple{
		{
			ID:          "old-1",
			Principle:   "For feature tasks: wrote tests first",
			SourceCount: 3,
			Confidence:  0.6,
			TaskTypes:   []string{"feature"},
		},
	}

	fresh := []StrategicPrinciple{
		{
			// Nearly identical principle text — should merge, not duplicate.
			ID:          "new-1",
			Principle:   "For feature tasks: wrote tests first",
			SourceCount: 2,
			Confidence:  0.8,
			TaskTypes:   []string{"feature", "bug_fix"},
		},
		{
			// Different principle — should be added.
			ID:          "new-2",
			Principle:   "Universal principle: incremental commits",
			SourceCount: 1,
			Confidence:  0.5,
			TaskTypes:   []string{"general"},
		},
	}

	merged := mergePrinciples(existing, fresh)

	if len(merged) != 2 {
		t.Fatalf("expected 2 merged principles (1 deduped + 1 new), got %d", len(merged))
	}

	// The first should be the merged version of old-1.
	if merged[0].ID != "old-1" {
		t.Errorf("expected merged principle to keep original ID 'old-1', got %q", merged[0].ID)
	}
	if merged[0].SourceCount != 5 { // 3 + 2
		t.Errorf("expected merged source_count 5, got %d", merged[0].SourceCount)
	}
	if !containsString(merged[0].TaskTypes, "bug_fix") {
		t.Error("expected merged task types to include 'bug_fix'")
	}

	// The second should be the genuinely new one.
	if merged[1].ID != "new-2" {
		t.Errorf("expected second principle ID 'new-2', got %q", merged[1].ID)
	}
}

func TestConfidenceGrade(t *testing.T) {
	tests := []struct {
		confidence float64
		want       string
	}{
		{0.95, "HIGH"},
		{0.90, "HIGH"},
		{0.75, "MED"},
		{0.60, "MED"},
		{0.59, "LOW"},
		{0.0, "LOW"},
	}
	for _, tt := range tests {
		got := confidenceGrade(tt.confidence)
		if got != tt.want {
			t.Errorf("confidenceGrade(%f) = %q, want %q", tt.confidence, got, tt.want)
		}
	}
}

func TestClampConfidence(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{-0.5, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}
	for _, tt := range tests {
		got := clampConfidence(tt.input)
		if got != tt.want {
			t.Errorf("clampConfidence(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
