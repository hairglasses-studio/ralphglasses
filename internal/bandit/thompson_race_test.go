package bandit

import (
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

func TestThompsonConcurrentSelectUpdate(t *testing.T) {
	arms := []Arm{
		{ID: "ultra-cheap", Provider: "gemini", Model: "gemini-3.1-flash-lite"},
		{ID: "worker", Provider: "gemini", Model: "gemini-3.1-flash"},
		{ID: "coding", Provider: "claude", Model: "claude-sonnet"},
		{ID: "reasoning", Provider: "claude", Model: "claude-opus"},
	}
	ts := NewThompsonSampling(arms, 0)

	var wg sync.WaitGroup

	// 20 goroutines calling Select
	for range 20 {
		wg.Go(func() {
			for range 50 {
				arm := ts.Select(nil)
				if arm.ID == "" {
					t.Error("Select returned empty arm")
				}
			}
		})
	}

	// 20 goroutines calling Update
	armIDs := []string{"ultra-cheap", "worker", "coding", "reasoning"}
	for i := range 20 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed+1)))
			for range 50 {
				ts.Update(Reward{
					ArmID:     armIDs[rng.IntN(len(armIDs))],
					Value:     rng.Float64(),
					Timestamp: time.Now(),
				})
			}
		}(i)
	}

	wg.Wait()

	// Verify stats are accessible after concurrent operations.
	stats := ts.ArmStats()
	if len(stats) != 4 {
		t.Errorf("expected 4 arm stats, got %d", len(stats))
	}
}

func TestUCB1ConcurrentSelectUpdate(t *testing.T) {
	arms := []Arm{
		{ID: "ultra-cheap", Provider: "gemini", Model: "gemini-3.1-flash-lite"},
		{ID: "worker", Provider: "gemini", Model: "gemini-3.1-flash"},
		{ID: "coding", Provider: "claude", Model: "claude-sonnet"},
		{ID: "reasoning", Provider: "claude", Model: "claude-opus"},
	}
	ucb := NewDiscountedUCB1(arms, 0.99)

	var wg sync.WaitGroup

	// 20 goroutines calling Select
	for range 20 {
		wg.Go(func() {
			for range 50 {
				arm := ucb.Select(nil)
				if arm.ID == "" {
					t.Error("Select returned empty arm")
				}
			}
		})
	}

	// 20 goroutines calling Update
	armIDs := []string{"ultra-cheap", "worker", "coding", "reasoning"}
	for i := range 20 {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed+1)))
			for range 50 {
				ucb.Update(Reward{
					ArmID:     armIDs[rng.IntN(len(armIDs))],
					Value:     rng.Float64(),
					Timestamp: time.Now(),
				})
			}
		}(i)
	}

	wg.Wait()

	stats := ucb.ArmStats()
	if len(stats) != 4 {
		t.Errorf("expected 4 arm stats, got %d", len(stats))
	}
}
