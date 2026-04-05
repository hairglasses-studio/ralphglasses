package session

import (
	"math"
	"testing"
	"time"
)

func TestNewDecisionModel(t *testing.T) {
	dm := NewDecisionModel()
	if dm.trained {
		t.Fatal("new model should be untrained")
	}
	if dm.minSamples != 50 {
		t.Fatalf("expected minSamples=50, got %d", dm.minSamples)
	}
	if dm.bias != 0.0 {
		t.Fatalf("expected bias=0.0, got %f", dm.bias)
	}
	expected := [10]float64{0.0, 0.0, 0.20, -0.25, 0.30, 0.15, -0.10, 0.0, 0.0, 0.0}
	if dm.weights != expected {
		t.Fatalf("initial weights mismatch: got %v", dm.weights)
	}
}

func TestDecisionModelPredictUntrained(t *testing.T) {
	dm := NewDecisionModel()

	f := ConfidenceFeatures{
		VerifyPassed:  1.0,
		HedgeCount:    0.0,
		TurnRatio:     0.8,
		ErrorFree:     1.0,
		QuestionCount: 0.0,
	}

	got := dm.Predict(f)
	// Expected: 0.30*1 + 0.25*(1-0) + 0.20*0.8 + 0.15*1 + 0.10*(1-0)
	//         = 0.30 + 0.25 + 0.16 + 0.15 + 0.10 = 0.96
	want := 0.96
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("heuristic predict: got %f, want %f", got, want)
	}

	// Low confidence case.
	f2 := ConfidenceFeatures{
		VerifyPassed:  0.0,
		HedgeCount:    1.0,
		TurnRatio:     0.0,
		ErrorFree:     0.0,
		QuestionCount: 1.0,
	}
	got2 := dm.Predict(f2)
	// 0.30*0 + 0.25*0 + 0.20*0 + 0.15*0 + 0.10*0 = 0
	if got2 != 0.0 {
		t.Fatalf("low confidence heuristic: got %f, want 0.0", got2)
	}
}

func TestDecisionModelPredictTrained(t *testing.T) {
	dm := NewDecisionModel()
	dm.trained = true
	dm.weights = [10]float64{0, 0, 0, 0, 2.0, 0, 0, 0, 0, 0} // only VerifyPassed matters
	dm.bias = -1.0

	f := ConfidenceFeatures{VerifyPassed: 1.0}
	got := dm.Predict(f)
	// z = -1 + 2*1 = 1, sigmoid(1) ≈ 0.7310585
	want := 1.0 / (1.0 + math.Exp(-1.0))
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("trained predict: got %f, want %f", got, want)
	}

	f2 := ConfidenceFeatures{VerifyPassed: 0.0}
	got2 := dm.Predict(f2)
	// z = -1 + 0 = -1, sigmoid(-1) ≈ 0.2689
	want2 := 1.0 / (1.0 + math.Exp(1.0))
	if math.Abs(got2-want2) > 1e-6 {
		t.Fatalf("trained predict low: got %f, want %f", got2, want2)
	}
}

func TestDecisionModelTrainInsufficientData(t *testing.T) {
	dm := NewDecisionModel()
	obs := make([]LoopObservation, 10)
	err := dm.Train(obs)
	if err == nil {
		t.Fatal("expected error for insufficient data")
	}
}

func TestDecisionModelTrainAndPredict(t *testing.T) {
	dm := NewDecisionModel()

	obs := make([]LoopObservation, 100)
	for i := range 50 {
		// Passing observations: high confidence, no errors.
		obs[i] = LoopObservation{
			Timestamp:       time.Now(),
			VerifyPassed:    true,
			Confidence:      0.9,
			DifficultyScore: 0.2,
			WorkerProvider:  "claude",
			TaskType:        "refactor",
			TotalLatencyMs:  1000,
			WorkerLatencyMs: 800,
			EpisodesUsed:    3,
		}
	}
	for i := 50; i < 100; i++ {
		// Failing observations: low confidence, errors.
		obs[i] = LoopObservation{
			Timestamp:       time.Now(),
			VerifyPassed:    false,
			Confidence:      0.2,
			Error:           "build failed",
			DifficultyScore: 0.9,
			WorkerProvider:  "gemini",
			TaskType:        "new_feature",
			TotalLatencyMs:  5000,
			WorkerLatencyMs: 1000,
			EpisodesUsed:    0,
		}
	}

	if err := dm.Train(obs); err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if !dm.IsTrained() {
		t.Fatal("model should be trained after Train()")
	}

	// Predict on pass-like features.
	passFeatures := observationToFeatures(obs[0])
	failFeatures := observationToFeatures(obs[50])

	passScore := dm.Predict(passFeatures)
	failScore := dm.Predict(failFeatures)

	if passScore <= failScore {
		t.Fatalf("pass score (%f) should be > fail score (%f)", passScore, failScore)
	}
}

func TestDecisionModelCalibrate(t *testing.T) {
	dm := NewDecisionModel()
	dm.trained = true
	dm.weights = [10]float64{0, 0, 0, 0, 2.0, 0, 0, 0, 0, 0}
	dm.bias = 0.0

	obs := make([]LoopObservation, 60)
	for i := range 30 {
		obs[i] = LoopObservation{VerifyPassed: true, Confidence: 0.8}
	}
	for i := 30; i < 60; i++ {
		obs[i] = LoopObservation{VerifyPassed: false, Confidence: 0.3}
	}

	if err := dm.Calibrate(obs); err != nil {
		t.Fatalf("Calibrate failed: %v", err)
	}

	dm.mu.Lock()
	binCount := len(dm.calibration)
	dm.mu.Unlock()

	if binCount == 0 {
		t.Fatal("expected calibration bins to be created")
	}
}

func TestDecisionModelAdaptThreshold(t *testing.T) {
	dm := NewDecisionModel()

	// Create observations where threshold 0.6 works better than 0.7.
	// Passing observations with confidence around 0.65 (above 0.6, below 0.7).
	obs := make([]LoopObservation, 100)
	for i := range 60 {
		obs[i] = LoopObservation{
			VerifyPassed:    true,
			Confidence:      0.65,
			DifficultyScore: 0.4,
			WorkerProvider:  "claude",
			TaskType:        "fix",
		}
	}
	for i := 60; i < 100; i++ {
		obs[i] = LoopObservation{
			VerifyPassed:    false,
			Confidence:      0.2,
			Error:           "test failed",
			DifficultyScore: 0.8,
			WorkerProvider:  "claude",
			TaskType:        "fix",
		}
	}

	threshold := dm.AdaptThreshold(obs)
	// Threshold should be <= 0.7 since many passing observations have moderate scores.
	if threshold > 0.75 {
		t.Fatalf("expected threshold <= 0.75, got %f", threshold)
	}
	if threshold < 0.3 {
		t.Fatalf("expected threshold >= 0.3, got %f", threshold)
	}
}

func TestDecisionModelAdaptThresholdEmpty(t *testing.T) {
	dm := NewDecisionModel()
	threshold := dm.AdaptThreshold(nil)
	if threshold != 0.7 {
		t.Fatalf("expected default threshold 0.7, got %f", threshold)
	}
}

func TestDecisionModelObservationToFeatures(t *testing.T) {
	obs := LoopObservation{
		VerifyPassed:    true,
		Confidence:      0.8,
		Error:           "",
		DifficultyScore: 0.5,
		WorkerProvider:  "gemini",
		TaskType:        "refactor",
		TotalLatencyMs:  2000,
		WorkerLatencyMs: 1000,
		EpisodesUsed:    5,
	}

	f := observationToFeatures(obs)

	if f.VerifyPassed != 1.0 {
		t.Fatalf("VerifyPassed: got %f, want 1.0", f.VerifyPassed)
	}
	if f.ErrorFree != 1.0 {
		t.Fatalf("ErrorFree: got %f, want 1.0", f.ErrorFree)
	}
	if f.ProviderID != 0.33 {
		t.Fatalf("ProviderID: got %f, want 0.33", f.ProviderID)
	}
	if f.DifficultyScore != 0.5 {
		t.Fatalf("DifficultyScore: got %f, want 0.5", f.DifficultyScore)
	}
	if f.EpisodesAvailable != 0.5 {
		t.Fatalf("EpisodesAvailable: got %f, want 0.5", f.EpisodesAvailable)
	}
	// TurnRatio = 2000/1000 = 2.0
	if math.Abs(f.TurnRatio-2.0) > 1e-9 {
		t.Fatalf("TurnRatio: got %f, want 2.0", f.TurnRatio)
	}
	// HedgeCount should be 0 since Confidence (0.8) >= 0.5
	if f.HedgeCount != 0.0 {
		t.Fatalf("HedgeCount: got %f, want 0.0", f.HedgeCount)
	}
	// TaskTypeHash should be deterministic
	if f.TaskTypeHash != hashTaskType("refactor") {
		t.Fatalf("TaskTypeHash mismatch")
	}
}

func TestDecisionModelStats(t *testing.T) {
	dm := NewDecisionModel()
	stats := dm.Stats()

	if stats["trained"].(bool) != false {
		t.Fatal("expected trained=false in stats")
	}
	if stats["sample_count"].(int) != 0 {
		t.Fatal("expected sample_count=0")
	}
	w := stats["weights"].([]float64)
	if len(w) != 10 {
		t.Fatalf("expected 10 weights, got %d", len(w))
	}
}

func TestDecisionModelTrainExactlyMinSamples(t *testing.T) {
	dm := NewDecisionModel()

	// Exactly 50 observations (the minimum).
	obs := make([]LoopObservation, 50)
	for i := range 25 {
		obs[i] = LoopObservation{
			Timestamp:       time.Now(),
			VerifyPassed:    true,
			Confidence:      0.8,
			WorkerProvider:  "claude",
			TaskType:        "fix",
			TotalLatencyMs:  1000,
			WorkerLatencyMs: 500,
		}
	}
	for i := 25; i < 50; i++ {
		obs[i] = LoopObservation{
			Timestamp:       time.Now(),
			VerifyPassed:    false,
			Confidence:      0.3,
			Error:           "test failed",
			WorkerProvider:  "gemini",
			TaskType:        "refactor",
			TotalLatencyMs:  3000,
			WorkerLatencyMs: 1000,
		}
	}

	err := dm.Train(obs)
	if err != nil {
		t.Fatalf("Train with exactly minSamples should succeed, got: %v", err)
	}
	if !dm.IsTrained() {
		t.Fatal("model should be trained after Train()")
	}
}

func TestDecisionModelPredictClamped(t *testing.T) {
	dm := NewDecisionModel()
	dm.mu.Lock()
	dm.trained = true
	// Extreme weights to try to push output beyond [0,1].
	dm.weights = [10]float64{100, 100, 100, 100, 100, 100, 100, 100, 100, 100}
	dm.bias = 500
	dm.mu.Unlock()

	// Extreme high features.
	high := ConfidenceFeatures{
		TaskTypeHash:      1.0,
		ProviderID:        1.0,
		TurnRatio:         3.0,
		HedgeCount:        1.0,
		VerifyPassed:      1.0,
		ErrorFree:         1.0,
		QuestionCount:     1.0,
		OutputLength:      1.0,
		DifficultyScore:   1.0,
		EpisodesAvailable: 1.0,
	}
	score := dm.Predict(high)
	if score < 0 || score > 1 {
		t.Fatalf("prediction out of [0,1] range: %f", score)
	}

	// Extreme negative bias.
	dm.mu.Lock()
	dm.weights = [10]float64{-100, -100, -100, -100, -100, -100, -100, -100, -100, -100}
	dm.bias = -500
	dm.mu.Unlock()

	score2 := dm.Predict(high)
	if score2 < 0 || score2 > 1 {
		t.Fatalf("prediction out of [0,1] range with negative weights: %f", score2)
	}
}

func TestPredictConfidenceAdapter(t *testing.T) {
	dm := NewDecisionModel()

	// Basic call with reasonable inputs (untrained model uses heuristic).
	score := dm.PredictConfidence(5, 10, "the implementation looks correct", true)
	if score < 0 || score > 1 {
		t.Fatalf("PredictConfidence out of [0,1]: %f", score)
	}

	// Verify passed => should contribute positively.
	scorePassed := dm.PredictConfidence(5, 10, "done", true)
	scoreFailed := dm.PredictConfidence(5, 10, "done", false)
	if scorePassed <= scoreFailed {
		t.Errorf("passed score (%f) should be > failed score (%f)", scorePassed, scoreFailed)
	}

	// Hedge words should reduce confidence.
	scoreNoHedge := dm.PredictConfidence(5, 10, "completed successfully", true)
	scoreHedge := dm.PredictConfidence(5, 10, "maybe possibly might not sure perhaps uncertain", true)
	if scoreHedge >= scoreNoHedge {
		t.Errorf("hedged score (%f) should be < non-hedged score (%f)", scoreHedge, scoreNoHedge)
	}

	// Zero expected turns (avoid division by zero).
	scoreZeroExpected := dm.PredictConfidence(5, 0, "output", true)
	if scoreZeroExpected < 0 || scoreZeroExpected > 1 {
		t.Fatalf("PredictConfidence with 0 expectedTurns out of range: %f", scoreZeroExpected)
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		input, want float64
	}{
		{-1.0, 0.0},
		{-0.5, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
		{100.0, 1.0},
	}
	for _, tt := range tests {
		if got := clamp01(tt.input); got != tt.want {
			t.Errorf("clamp01(%f) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestProviderToFloat(t *testing.T) {
	tests := []struct {
		provider string
		want     float64
	}{
		{"claude", 0.0},
		{"gemini", 0.33},
		{"codex", 0.66},
		{"unknown", 0.0},
		{"", 0.0},
	}
	for _, tt := range tests {
		if got := providerToFloat(tt.provider); got != tt.want {
			t.Errorf("providerToFloat(%q) = %f, want %f", tt.provider, got, tt.want)
		}
	}
}
