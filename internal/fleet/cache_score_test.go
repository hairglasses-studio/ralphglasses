package fleet

import (
	"fmt"
	"testing"
)

func TestCacheBoostNoHistory(t *testing.T) {
	cs := NewCacheScorer()
	boost := cs.CacheBoost("worker-1", "some prompt text")
	if boost != 1.0 {
		t.Errorf("expected 1.0 for unknown worker, got %f", boost)
	}
}

func TestCacheBoostMatch(t *testing.T) {
	cs := NewCacheScorer()
	prompt := "implement a REST API handler for user authentication"
	cs.RecordPrompt("worker-1", prompt)

	boost := cs.CacheBoost("worker-1", prompt)
	if boost != 2.0 {
		t.Errorf("expected 2.0 for matching prompt, got %f", boost)
	}
}

func TestCacheBoostMismatch(t *testing.T) {
	cs := NewCacheScorer()
	cs.RecordPrompt("worker-1", "implement a REST API handler")

	boost := cs.CacheBoost("worker-1", "refactor the database layer")
	if boost != 1.0 {
		t.Errorf("expected 1.0 for non-matching prompt, got %f", boost)
	}
}

func TestCacheBoostDifferentWorker(t *testing.T) {
	cs := NewCacheScorer()
	cs.RecordPrompt("worker-1", "implement a REST API handler")

	boost := cs.CacheBoost("worker-2", "implement a REST API handler")
	if boost != 1.0 {
		t.Errorf("expected 1.0 for different worker, got %f", boost)
	}
}

func TestEviction(t *testing.T) {
	cs := NewCacheScorer()
	// Record 10 unique prompts for a worker.
	for i := range maxRecentPrompts {
		cs.RecordPrompt("worker-1", fmt.Sprintf("prompt-%d", i))
	}

	// The first prompt should still be present (it's at index 0 of a full ring).
	boost := cs.CacheBoost("worker-1", "prompt-0")
	if boost != 2.0 {
		t.Errorf("expected prompt-0 still present before eviction, got %f", boost)
	}

	// Record one more to trigger eviction of prompt-0.
	cs.RecordPrompt("worker-1", "prompt-new")

	boost = cs.CacheBoost("worker-1", "prompt-0")
	if boost != 1.0 {
		t.Errorf("expected prompt-0 evicted after overflow, got %f", boost)
	}

	// The newest prompt should be present.
	boost = cs.CacheBoost("worker-1", "prompt-new")
	if boost != 2.0 {
		t.Errorf("expected prompt-new present, got %f", boost)
	}

	// prompt-1 (second oldest) should still be present.
	boost = cs.CacheBoost("worker-1", "prompt-1")
	if boost != 2.0 {
		t.Errorf("expected prompt-1 still present, got %f", boost)
	}
}

func TestCacheBoostPrefixOnly(t *testing.T) {
	cs := NewCacheScorer()
	// Two prompts with same first 500 chars but different suffixes should match.
	base := make([]byte, 500)
	for i := range base {
		base[i] = 'a'
	}
	prompt1 := string(base) + " suffix one"
	prompt2 := string(base) + " suffix two"

	cs.RecordPrompt("worker-1", prompt1)
	boost := cs.CacheBoost("worker-1", prompt2)
	if boost != 2.0 {
		t.Errorf("expected 2.0 for same-prefix prompts, got %f", boost)
	}
}
