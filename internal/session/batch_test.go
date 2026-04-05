package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewBatchCollector(t *testing.T) {
	bc := NewBatchCollector("batch-1")
	if bc.BatchID() != "batch-1" {
		t.Errorf("BatchID() = %q, want %q", bc.BatchID(), "batch-1")
	}
	if bc.Count() != 0 {
		t.Errorf("Count() = %d, want 0", bc.Count())
	}
	if bc.IsComplete() {
		t.Error("IsComplete() = true, want false for new collector")
	}
}

func TestBatchCollectorAddAndPoll(t *testing.T) {
	bc := NewBatchCollector("batch-2")
	bc.SetExpectedCount(3)

	// First poll should return nil (no results yet)
	if got := bc.Poll(); got != nil {
		t.Errorf("Poll() before any results = %v, want nil", got)
	}

	// Add two results
	bc.AddResult("sess-1", SessionResult{
		Provider: ProviderClaude,
		Status:   StatusCompleted,
		SpentUSD: 0.05,
	})
	bc.AddResult("sess-2", SessionResult{
		Provider: ProviderGemini,
		Status:   StatusCompleted,
		SpentUSD: 0.01,
	})

	// Poll should return both
	results := bc.Poll()
	if len(results) != 2 {
		t.Fatalf("Poll() returned %d results, want 2", len(results))
	}
	if results[0].SessionID != "sess-1" {
		t.Errorf("results[0].SessionID = %q, want %q", results[0].SessionID, "sess-1")
	}
	if results[1].SessionID != "sess-2" {
		t.Errorf("results[1].SessionID = %q, want %q", results[1].SessionID, "sess-2")
	}

	// Second poll should return nil (no new results)
	if got := bc.Poll(); got != nil {
		t.Errorf("Poll() after draining = %v, want nil", got)
	}

	// Not yet complete
	if bc.IsComplete() {
		t.Error("IsComplete() = true with 2/3 results")
	}

	// Add third result
	bc.AddResult("sess-3", SessionResult{
		Provider: ProviderCodex,
		Status:   StatusErrored,
		Error:    "timeout",
	})

	if !bc.IsComplete() {
		t.Error("IsComplete() = false with 3/3 results")
	}

	// Poll returns only the new result
	results = bc.Poll()
	if len(results) != 1 {
		t.Fatalf("Poll() after third result returned %d, want 1", len(results))
	}
	if results[0].SessionID != "sess-3" {
		t.Errorf("results[0].SessionID = %q, want %q", results[0].SessionID, "sess-3")
	}
}

func TestBatchCollectorResults(t *testing.T) {
	bc := NewBatchCollector("batch-3")
	bc.AddResult("a", SessionResult{Status: StatusCompleted})
	bc.AddResult("b", SessionResult{Status: StatusErrored})

	all := bc.Results()
	if len(all) != 2 {
		t.Fatalf("Results() returned %d, want 2", len(all))
	}

	// Results() should be a copy, not alias the internal slice
	all[0].SessionID = "mutated"
	original := bc.Results()
	if original[0].SessionID == "mutated" {
		t.Error("Results() returned a reference to internal slice, want a copy")
	}
}

func TestBatchCollectorWebhook(t *testing.T) {
	var received batchWebhookPayload
	var mu sync.Mutex
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != http.MethodPost {
			t.Errorf("webhook method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode webhook body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	bc := NewBatchCollector("webhook-batch")
	bc.SetCallbackURL(server.URL)
	bc.SetHTTPClient(server.Client())
	bc.SetExpectedCount(2)

	bc.AddResult("s1", SessionResult{Provider: ProviderClaude, Status: StatusCompleted, SpentUSD: 0.10})
	bc.AddResult("s2", SessionResult{Provider: ProviderGemini, Status: StatusCompleted, SpentUSD: 0.02})

	// Wait for webhook to fire (background goroutine)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("webhook was not called within timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	if received.BatchID != "webhook-batch" {
		t.Errorf("webhook batch_id = %q, want %q", received.BatchID, "webhook-batch")
	}
	if !received.Complete {
		t.Error("webhook complete = false, want true")
	}
	if received.Count != 2 {
		t.Errorf("webhook count = %d, want 2", received.Count)
	}
	if len(received.Results) != 2 {
		t.Fatalf("webhook results len = %d, want 2", len(received.Results))
	}
}

func TestBatchCollectorNoWebhookBeforeComplete(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	bc := NewBatchCollector("no-early-webhook")
	bc.SetCallbackURL(server.URL)
	bc.SetHTTPClient(server.Client())
	bc.SetExpectedCount(3)

	bc.AddResult("s1", SessionResult{Status: StatusCompleted})
	bc.AddResult("s2", SessionResult{Status: StatusCompleted})

	// Give a brief moment for any erroneous goroutine to fire
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("webhook should not be called before batch is complete")
	}
}

func TestBatchCollectorConcurrentAddResult(t *testing.T) {
	bc := NewBatchCollector("concurrent")
	bc.SetExpectedCount(100)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			bc.AddResult(
				fmt.Sprintf("sess-%d", id),
				SessionResult{Provider: ProviderClaude, Status: StatusCompleted},
			)
		}(i)
	}
	wg.Wait()

	if bc.Count() != 100 {
		t.Errorf("Count() = %d, want 100", bc.Count())
	}
	if !bc.IsComplete() {
		t.Error("IsComplete() = false after 100/100 results")
	}
}

func TestBatchCollectorNoExpectedCount(t *testing.T) {
	bc := NewBatchCollector("no-expected")
	bc.AddResult("s1", SessionResult{Status: StatusCompleted})
	bc.AddResult("s2", SessionResult{Status: StatusCompleted})

	// Without SetExpectedCount, IsComplete should always be false
	if bc.IsComplete() {
		t.Error("IsComplete() should be false when expected count is 0")
	}
}

func TestBatchCollectorSetExpectedCountAfterResults(t *testing.T) {
	bc := NewBatchCollector("late-expected")
	bc.AddResult("s1", SessionResult{Status: StatusCompleted})
	bc.AddResult("s2", SessionResult{Status: StatusCompleted})

	if bc.IsComplete() {
		t.Error("IsComplete() should be false before SetExpectedCount")
	}

	// Setting expected count to match existing results should complete immediately
	bc.SetExpectedCount(2)
	if !bc.IsComplete() {
		t.Error("IsComplete() should be true after SetExpectedCount(2) with 2 results")
	}
}

func TestBatchCollectorCollectedAtFilled(t *testing.T) {
	bc := NewBatchCollector("timestamp")
	bc.AddResult("s1", SessionResult{Status: StatusCompleted})

	results := bc.Results()
	if results[0].CollectedAt.IsZero() {
		t.Error("CollectedAt should be auto-filled when zero")
	}
}

func TestFireWebhookRetry(t *testing.T) {
	var mu sync.Mutex
	var attempts int
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()

		if current < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Third attempt succeeds
		var payload batchWebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode webhook body: %v", err)
		}
		if payload.Attempts != 3 {
			t.Errorf("payload.Attempts = %d, want 3", payload.Attempts)
		}
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer server.Close()

	bc := NewBatchCollector("retry-batch")
	bc.SetCallbackURL(server.URL)
	bc.SetHTTPClient(server.Client())
	bc.SetExpectedCount(1)

	bc.AddResult("s1", SessionResult{Provider: ProviderClaude, Status: StatusCompleted})

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("webhook retry did not succeed within timeout")
	}

	mu.Lock()
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	mu.Unlock()
}

func TestFireWebhookAllFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := fireWebhook(server.Client(), server.URL, "fail-batch", []SessionResult{
		{SessionID: "s1", Status: StatusCompleted},
	})
	if err == nil {
		t.Fatal("expected error when all attempts fail")
	}
	if !strings.Contains(err.Error(), "webhook failed after 3 attempts") {
		t.Errorf("error = %q, want 'webhook failed after 3 attempts'", err)
	}
}

func TestBatchCollectorNullArrayMarshal(t *testing.T) {
	bc := NewBatchCollector("null-test")
	results := bc.Results()
	data, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "null" {
		t.Error("Results() marshals as null, want []")
	}
	if string(data) != "[]" {
		t.Errorf("Results() marshals as %q, want []", string(data))
	}
}

func TestBatchCollectorPreservedCollectedAt(t *testing.T) {
	bc := NewBatchCollector("preserved-ts")
	custom := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	bc.AddResult("s1", SessionResult{Status: StatusCompleted, CollectedAt: custom})

	results := bc.Results()
	if !results[0].CollectedAt.Equal(custom) {
		t.Errorf("CollectedAt = %v, want %v", results[0].CollectedAt, custom)
	}
}
