package fleet

import (
	"testing"
	"time"
)

func TestAllCategories(t *testing.T) {
	cats := AllCategories()
	if len(cats) == 0 {
		t.Fatal("expected non-empty categories")
	}
	// Verify known categories are present.
	found := make(map[TaskCategory]bool)
	for _, c := range cats {
		found[c] = true
	}
	expected := []TaskCategory{
		CategoryCodeGen, CategoryCodeReview, CategoryRefactor,
		CategoryTesting, CategoryDebug, CategoryDocs,
		CategoryResearch, CategoryGeneral,
	}
	for _, c := range expected {
		if !found[c] {
			t.Errorf("missing category %q from AllCategories", c)
		}
	}
}

func TestCategoryRecord_Total(t *testing.T) {
	tests := []struct {
		rec  CategoryRecord
		want int
	}{
		{CategoryRecord{Successes: 0, Failures: 0}, 0},
		{CategoryRecord{Successes: 3, Failures: 0}, 3},
		{CategoryRecord{Successes: 0, Failures: 2}, 2},
		{CategoryRecord{Successes: 5, Failures: 3}, 8},
	}
	for _, tt := range tests {
		got := tt.rec.Total()
		if got != tt.want {
			t.Errorf("Total() = %d, want %d (rec=%+v)", got, tt.want, tt.rec)
		}
	}
}

func TestCategoryRecord_SuccessRate(t *testing.T) {
	tests := []struct {
		rec  CategoryRecord
		want float64
	}{
		{CategoryRecord{Successes: 0, Failures: 0}, 0.0},
		{CategoryRecord{Successes: 4, Failures: 0}, 1.0},
		{CategoryRecord{Successes: 0, Failures: 4}, 0.0},
		{CategoryRecord{Successes: 3, Failures: 1}, 0.75},
	}
	for _, tt := range tests {
		got := tt.rec.SuccessRate()
		if got != tt.want {
			t.Errorf("SuccessRate() = %f, want %f (rec=%+v)", got, tt.want, tt.rec)
		}
	}
}

func TestCategoryRecord_AvgDuration(t *testing.T) {
	tests := []struct {
		rec  CategoryRecord
		want float64
	}{
		{CategoryRecord{Successes: 0, TotalTimeS: 0}, 0.0},
		{CategoryRecord{Successes: 4, TotalTimeS: 100.0}, 25.0},
		{CategoryRecord{Successes: 1, TotalTimeS: 60.0}, 60.0},
	}
	for _, tt := range tests {
		got := tt.rec.AvgDuration()
		if got != tt.want {
			t.Errorf("AvgDuration() = %f, want %f (rec=%+v)", got, tt.want, tt.rec)
		}
	}
}

func TestDefaultSpecializerConfig(t *testing.T) {
	cfg := DefaultSpecializerConfig()
	if cfg.ColdStartScore != 0.5 {
		t.Errorf("ColdStartScore = %f, want 0.5", cfg.ColdStartScore)
	}
	if cfg.MinSamplesForConfidence <= 0 {
		t.Errorf("MinSamplesForConfidence = %d, want > 0", cfg.MinSamplesForConfidence)
	}
	if cfg.RecencyDecayDays <= 0 {
		t.Errorf("RecencyDecayDays = %d, want > 0", cfg.RecencyDecayDays)
	}
}

func TestNewWorkerSpecializer_ColdStart(t *testing.T) {
	cfg := DefaultSpecializerConfig()
	ws := NewWorkerSpecializer(cfg)

	// Unknown worker should return cold start score.
	score := ws.Score("worker-1", CategoryCodeGen)
	if score != cfg.ColdStartScore {
		t.Errorf("cold start score = %f, want %f", score, cfg.ColdStartScore)
	}
}

func TestWorkerSpecializer_RecordAndScore(t *testing.T) {
	cfg := SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 2,
		RecencyDecayDays:        0, // disable recency decay for predictable test
		SpeedWeight:             0,
	}
	ws := NewWorkerSpecializer(cfg)

	// Record 2 successes and 0 failures → 100% success rate.
	ws.RecordOutcome("w1", CategoryCodeGen, true, 10.0)
	ws.RecordOutcome("w1", CategoryCodeGen, true, 20.0)

	score := ws.Score("w1", CategoryCodeGen)
	// At min confidence, score should blend toward 1.0.
	if score < 0.7 {
		t.Errorf("after 2 successes, score = %f, want > 0.7", score)
	}
}

func TestWorkerSpecializer_FailuresReduceScore(t *testing.T) {
	cfg := SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 2,
		RecencyDecayDays:        0,
		SpeedWeight:             0,
	}
	ws := NewWorkerSpecializer(cfg)

	ws.RecordOutcome("w1", CategoryCodeGen, false, 0)
	ws.RecordOutcome("w1", CategoryCodeGen, false, 0)

	score := ws.Score("w1", CategoryCodeGen)
	// 0% success rate → score below cold start.
	if score >= cfg.ColdStartScore {
		t.Errorf("after 2 failures, score = %f, want < %f", score, cfg.ColdStartScore)
	}
}

func TestWorkerSpecializer_BestWorker(t *testing.T) {
	cfg := SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 1,
		RecencyDecayDays:        0,
		SpeedWeight:             0,
	}
	ws := NewWorkerSpecializer(cfg)

	// w1 has 100% success rate, w2 has 0%.
	ws.RecordOutcome("w1", CategoryTesting, true, 10)
	ws.RecordOutcome("w2", CategoryTesting, false, 0)

	best := ws.BestWorker([]string{"w1", "w2"}, CategoryTesting)
	if best != "w1" {
		t.Errorf("BestWorker = %q, want w1", best)
	}
}

func TestWorkerSpecializer_BestWorkerEmpty(t *testing.T) {
	ws := NewWorkerSpecializer(DefaultSpecializerConfig())
	best := ws.BestWorker(nil, CategoryCodeGen)
	if best != "" {
		t.Errorf("BestWorker(nil) = %q, want empty", best)
	}
}

func TestWorkerSpecializer_RankWorkers(t *testing.T) {
	cfg := SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 1,
		RecencyDecayDays:        0,
		SpeedWeight:             0,
	}
	ws := NewWorkerSpecializer(cfg)

	ws.RecordOutcome("w1", CategoryDebug, true, 10)
	ws.RecordOutcome("w2", CategoryDebug, false, 0)
	ws.RecordOutcome("w3", CategoryDebug, true, 10)

	ranked := ws.RankWorkers([]string{"w1", "w2", "w3"}, CategoryDebug)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 ranked workers, got %d", len(ranked))
	}
	// w2 (0% success) should be last.
	if ranked[len(ranked)-1].WorkerID != "w2" {
		t.Errorf("expected w2 last, got %q", ranked[len(ranked)-1].WorkerID)
	}
	// Scores should be in descending order.
	for i := 1; i < len(ranked); i++ {
		if ranked[i].Score > ranked[i-1].Score {
			t.Errorf("rank[%d].Score (%f) > rank[%d].Score (%f)", i, ranked[i].Score, i-1, ranked[i-1].Score)
		}
	}
}

func TestWorkerSpecializer_WorkerStrengths(t *testing.T) {
	ws := NewWorkerSpecializer(DefaultSpecializerConfig())
	ws.RecordOutcome("w1", CategoryCodeGen, true, 5)
	ws.RecordOutcome("w1", CategoryCodeGen, true, 5)

	strengths := ws.WorkerStrengths("w1")
	if len(strengths) != len(AllCategories()) {
		t.Errorf("expected %d strengths, got %d", len(AllCategories()), len(strengths))
	}
	// Scores should be in descending order.
	for i := 1; i < len(strengths); i++ {
		if strengths[i].Score > strengths[i-1].Score {
			t.Errorf("strengths[%d].Score (%f) > strengths[%d].Score (%f)",
				i, strengths[i].Score, i-1, strengths[i-1].Score)
		}
	}
}

func TestWorkerSpecializer_GetRecord(t *testing.T) {
	ws := NewWorkerSpecializer(DefaultSpecializerConfig())

	// No record yet.
	rec := ws.GetRecord("w1", CategoryCodeGen)
	if rec != nil {
		t.Error("expected nil for unrecorded worker/category")
	}

	ws.RecordOutcome("w1", CategoryCodeGen, true, 30)
	ws.RecordOutcome("w1", CategoryCodeGen, false, 0)

	rec = ws.GetRecord("w1", CategoryCodeGen)
	if rec == nil {
		t.Fatal("expected non-nil record")
	}
	if rec.Successes != 1 {
		t.Errorf("Successes = %d, want 1", rec.Successes)
	}
	if rec.Failures != 1 {
		t.Errorf("Failures = %d, want 1", rec.Failures)
	}
	if rec.TotalTimeS != 30.0 {
		t.Errorf("TotalTimeS = %f, want 30.0", rec.TotalTimeS)
	}
}

func TestWorkerSpecializer_GetRecord_IsCopy(t *testing.T) {
	// Mutating the returned record should not affect the internal state.
	ws := NewWorkerSpecializer(DefaultSpecializerConfig())
	ws.RecordOutcome("w1", CategoryCodeGen, true, 10)

	rec := ws.GetRecord("w1", CategoryCodeGen)
	rec.Successes = 999

	rec2 := ws.GetRecord("w1", CategoryCodeGen)
	if rec2.Successes == 999 {
		t.Error("GetRecord should return a copy, not a reference")
	}
}

func TestWorkerSpecializer_Reset(t *testing.T) {
	ws := NewWorkerSpecializer(DefaultSpecializerConfig())
	ws.RecordOutcome("w1", CategoryCodeGen, true, 10)
	ws.RecordOutcome("w2", CategoryTesting, false, 0)

	ws.Reset()

	if ws.GetRecord("w1", CategoryCodeGen) != nil {
		t.Error("expected nil after Reset")
	}
	if ws.GetRecord("w2", CategoryTesting) != nil {
		t.Error("expected nil after Reset")
	}
}

func TestWorkerSpecializer_SpeedBonus(t *testing.T) {
	cfg := SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 1,
		RecencyDecayDays:        0,
		SpeedWeight:             0.3,
	}
	ws := NewWorkerSpecializer(cfg)

	// Record a slow worker (600s — well above the 300s baseline, so no speed bonus).
	ws.RecordOutcome("slow", CategoryCodeGen, true, 600)

	slowScore := ws.Score("slow", CategoryCodeGen)

	// Add a failure too so slow's success rate is lower.
	ws.RecordOutcome("slow", CategoryCodeGen, false, 0)
	ws.RecordOutcome("slow", CategoryCodeGen, false, 0)

	// Record a fast, always-succeeding worker (10s vs 300s baseline).
	ws.RecordOutcome("fast", CategoryCodeGen, true, 10)
	ws.RecordOutcome("fast", CategoryCodeGen, true, 10)

	fastScore := ws.Score("fast", CategoryCodeGen)
	slowScoreAfter := ws.Score("slow", CategoryCodeGen)

	// fast with high success + speed bonus should score higher than slow with failures.
	if fastScore <= slowScoreAfter {
		t.Errorf("fast worker (%f) should score higher than slow worker (%f)", fastScore, slowScoreAfter)
	}
	_ = slowScore
}

func TestWorkerSpecializer_RecencyDecay(t *testing.T) {
	cfg := SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 1,
		RecencyDecayDays:        7,
		SpeedWeight:             0,
	}
	ws := NewWorkerSpecializer(cfg)
	ws.RecordOutcome("w1", CategoryCodeGen, true, 10)

	// Score right after recording should be reasonably high.
	recentScore := ws.Score("w1", CategoryCodeGen)

	// Manually set LastUsed to 14 days ago (2 half-lives).
	ws.mu.Lock()
	key := workerKey{WorkerID: "w1", Category: CategoryCodeGen}
	ws.records[key].LastUsed = time.Now().Add(-14 * 24 * time.Hour)
	ws.mu.Unlock()

	oldScore := ws.Score("w1", CategoryCodeGen)

	if oldScore >= recentScore {
		t.Errorf("old score (%f) should be less than recent score (%f) due to recency decay", oldScore, recentScore)
	}
}
