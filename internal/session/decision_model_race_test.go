package session

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

func TestDecisionModelConcurrentTrainPredict(t *testing.T) {
	dm := NewDecisionModel()

	// Build 60 mock observations (above minSamples=50).
	observations := make([]LoopObservation, 60)
	baseTime := time.Now()
	for i := range observations {
		observations[i] = LoopObservation{
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			LoopID:          "loop-1",
			RepoName:        "test-repo",
			IterationNumber: i,
			WorkerProvider:   "claude",
			TaskType:         "coding",
			VerifyPassed:     i%3 != 0, // 2/3 pass
			TotalLatencyMs:  1000,
			WorkerLatencyMs: 500,
			Confidence:      0.7,
			DifficultyScore: 0.5,
			EpisodesUsed:    2,
		}
	}

	var wg sync.WaitGroup

	// 5 goroutines calling Predict with random features.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed+1)))
			for j := 0; j < 100; j++ {
				features := ConfidenceFeatures{
					TaskTypeHash:      rng.Float64(),
					ProviderID:        rng.Float64(),
					TurnRatio:         rng.Float64() * 3,
					HedgeCount:        rng.Float64(),
					VerifyPassed:      float64(rng.IntN(2)),
					ErrorFree:         float64(rng.IntN(2)),
					QuestionCount:     rng.Float64(),
					OutputLength:      rng.Float64(),
					DifficultyScore:   rng.Float64(),
					EpisodesAvailable: rng.Float64(),
				}
				score := dm.Predict(features)
				if score < 0 || score > 1 {
					t.Errorf("Predict returned %f, want [0,1]", score)
				}
			}
		}(i)
	}

	// 2 goroutines calling Train.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3; j++ {
				// Train may fail or succeed; we only care about no races.
				_ = dm.Train(observations)
			}
		}()
	}

	wg.Wait()

	// After concurrent access, Predict should still return valid values.
	score := dm.Predict(ConfidenceFeatures{
		VerifyPassed: 1.0,
		ErrorFree:    1.0,
		TurnRatio:    1.0,
	})
	if score < 0 || score > 1 {
		t.Errorf("final Predict returned %f, want [0,1]", score)
	}
}

func TestDecisionModelRace_HighContention(t *testing.T) {
	dm := NewDecisionModel()

	// Build 60 mock observations (above minSamples=50).
	observations := make([]LoopObservation, 60)
	baseTime := time.Now()
	for i := range observations {
		observations[i] = LoopObservation{
			Timestamp:       baseTime.Add(time.Duration(i) * time.Second),
			LoopID:          "loop-hc",
			RepoName:        "test-repo",
			IterationNumber: i,
			WorkerProvider:   "claude",
			TaskType:         "coding",
			VerifyPassed:     i%3 != 0,
			TotalLatencyMs:  1000,
			WorkerLatencyMs: 500,
			Confidence:      0.7,
			DifficultyScore: 0.5,
			EpisodesUsed:    2,
		}
	}

	var wg sync.WaitGroup

	// 20 goroutines calling Predict with random features.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed+1)))
			for j := 0; j < 100; j++ {
				features := ConfidenceFeatures{
					TaskTypeHash:      rng.Float64(),
					ProviderID:        rng.Float64(),
					TurnRatio:         rng.Float64() * 3,
					HedgeCount:        rng.Float64(),
					VerifyPassed:      float64(rng.IntN(2)),
					ErrorFree:         float64(rng.IntN(2)),
					QuestionCount:     rng.Float64(),
					OutputLength:      rng.Float64(),
					DifficultyScore:   rng.Float64(),
					EpisodesAvailable: rng.Float64(),
				}
				score := dm.Predict(features)
				if score < 0 || score > 1 {
					t.Errorf("Predict returned %f, want [0,1]", score)
				}
			}
		}(i)
	}

	// 5 goroutines calling Train concurrently.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_ = dm.Train(observations)
			}
		}()
	}

	wg.Wait()

	// After high contention, Predict should still return valid values.
	score := dm.Predict(ConfidenceFeatures{
		VerifyPassed: 1.0,
		ErrorFree:    1.0,
		TurnRatio:    1.0,
	})
	if score < 0 || score > 1 {
		t.Errorf("final Predict returned %f, want [0,1]", score)
	}
}
