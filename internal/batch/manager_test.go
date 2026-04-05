package batch

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// mockClient implements Client for testing BatchManager and Scheduler.
// ---------------------------------------------------------------------------

type mockClient struct {
	mu            sync.Mutex
	submitCalls   [][]Request
	pollCalls     []string
	resultsCalls  []string
	cancelCalls   []string
	submitErr     error
	pollResult    *BatchStatus
	pollErr       error
	resultsResult []Result
	resultsErr    error
	provider      Provider
}

func newMockClient(p Provider) *mockClient {
	return &mockClient{provider: p}
}

func (m *mockClient) Submit(_ context.Context, requests []Request) (*BatchStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.submitCalls = append(m.submitCalls, requests)
	if m.submitErr != nil {
		return nil, m.submitErr
	}
	now := time.Now()
	return &BatchStatus{
		ID:        fmt.Sprintf("mock-batch-%d", len(m.submitCalls)),
		Provider:  m.provider,
		Status:    "processing",
		Total:     len(requests),
		CreatedAt: now,
	}, nil
}

func (m *mockClient) Poll(_ context.Context, batchID string) (*BatchStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pollCalls = append(m.pollCalls, batchID)
	if m.pollErr != nil {
		return nil, m.pollErr
	}
	if m.pollResult != nil {
		return m.pollResult, nil
	}
	return &BatchStatus{
		ID:     batchID,
		Status: "processing",
	}, nil
}

func (m *mockClient) Results(_ context.Context, batchID string) ([]Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resultsCalls = append(m.resultsCalls, batchID)
	if m.resultsErr != nil {
		return nil, m.resultsErr
	}
	return m.resultsResult, nil
}

func (m *mockClient) Cancel(_ context.Context, batchID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelCalls = append(m.cancelCalls, batchID)
	return nil
}

func (m *mockClient) Provider() Provider {
	return m.provider
}

func (m *mockClient) getSubmitCalls() [][]Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]Request, len(m.submitCalls))
	copy(out, m.submitCalls)
	return out
}

// ---------------------------------------------------------------------------
// BatchManager tests
// ---------------------------------------------------------------------------

func TestBatchManager_SubmitQueuesRequest(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{MaxBatchSize: 100, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()
	id, err := mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "hello", MaxTokens: 100},
		Priority: PriorityNormal,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty request ID")
	}

	// No API call should have been made yet.
	if len(mc.getSubmitCalls()) != 0 {
		t.Errorf("expected 0 submit calls, got %d", len(mc.getSubmitCalls()))
	}

	// Queue should have 1 item.
	if mgr.QueueLen() != 1 {
		t.Errorf("QueueLen = %d, want 1", mgr.QueueLen())
	}
}

func TestBatchManager_FlushSendsBatch(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{MaxBatchSize: 100, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()

	// Queue 3 requests.
	for i := range 3 {
		_, err := mgr.Submit(ctx, BatchManagerRequest{
			Request:  Request{UserPrompt: fmt.Sprintf("prompt-%d", i), MaxTokens: 100},
			Priority: PriorityNormal,
		})
		if err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	if mgr.QueueLen() != 3 {
		t.Fatalf("QueueLen = %d, want 3", mgr.QueueLen())
	}

	result, err := mgr.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil BatchManagerResult")
	}
	if result.RequestCount != 3 {
		t.Errorf("RequestCount = %d, want 3", result.RequestCount)
	}
	if result.BatchID == "" {
		t.Error("expected non-empty BatchID")
	}

	// Queue should be empty after flush.
	if mgr.QueueLen() != 0 {
		t.Errorf("QueueLen after flush = %d, want 0", mgr.QueueLen())
	}

	// Exactly one Submit call to the underlying client.
	calls := mc.getSubmitCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 submit call, got %d", len(calls))
	}
	if len(calls[0]) != 3 {
		t.Errorf("submit call had %d requests, want 3", len(calls[0]))
	}
}

func TestBatchManager_FlushEmptyQueueReturnsNil(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{MaxBatchSize: 100, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	result, err := mgr.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty queue, got %+v", result)
	}
}

func TestBatchManager_UrgentBypassesQueue(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderOpenAI)
	cfg := BatchManagerConfig{MaxBatchSize: 100, Provider: ProviderOpenAI}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()
	id, err := mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "urgent task", MaxTokens: 200},
		Priority: PriorityUrgent,
	})
	if err != nil {
		t.Fatalf("Submit urgent: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty request ID")
	}

	// Urgent should have called Submit immediately.
	calls := mc.getSubmitCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 immediate submit call, got %d", len(calls))
	}
	if len(calls[0]) != 1 {
		t.Errorf("urgent submit had %d requests, want 1", len(calls[0]))
	}
	if calls[0][0].UserPrompt != "urgent task" {
		t.Errorf("prompt = %q, want %q", calls[0][0].UserPrompt, "urgent task")
	}

	// Queue should remain empty.
	if mgr.QueueLen() != 0 {
		t.Errorf("QueueLen = %d, want 0 (urgent bypasses queue)", mgr.QueueLen())
	}
}

func TestBatchManager_FlushRespectsMaxBatchSize(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{MaxBatchSize: 2, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()

	// Queue 5 requests with MaxBatchSize=2.
	for i := range 5 {
		_, err := mgr.Submit(ctx, BatchManagerRequest{
			Request:  Request{UserPrompt: fmt.Sprintf("p-%d", i)},
			Priority: PriorityNormal,
		})
		if err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	// First flush should send 2 and re-queue 3.
	result, err := mgr.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush 1: %v", err)
	}
	if result.RequestCount != 2 {
		t.Errorf("Flush 1 RequestCount = %d, want 2", result.RequestCount)
	}
	if mgr.QueueLen() != 3 {
		t.Errorf("QueueLen after first flush = %d, want 3", mgr.QueueLen())
	}

	// Second flush sends 2, leaving 1.
	result, err = mgr.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush 2: %v", err)
	}
	if result.RequestCount != 2 {
		t.Errorf("Flush 2 RequestCount = %d, want 2", result.RequestCount)
	}
	if mgr.QueueLen() != 1 {
		t.Errorf("QueueLen after second flush = %d, want 1", mgr.QueueLen())
	}

	// Third flush sends the last 1.
	result, err = mgr.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush 3: %v", err)
	}
	if result.RequestCount != 1 {
		t.Errorf("Flush 3 RequestCount = %d, want 1", result.RequestCount)
	}
	if mgr.QueueLen() != 0 {
		t.Errorf("QueueLen after third flush = %d, want 0", mgr.QueueLen())
	}
}

func TestBatchManager_FlushRequeuesOnError(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	mc.submitErr = fmt.Errorf("api down")
	cfg := BatchManagerConfig{MaxBatchSize: 100, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()
	_, _ = mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "will fail"},
		Priority: PriorityNormal,
	})

	_, err := mgr.Flush(ctx)
	if err == nil {
		t.Fatal("expected error from Flush")
	}

	// Request should be re-queued.
	if mgr.QueueLen() != 1 {
		t.Errorf("QueueLen = %d, want 1 (re-queued after error)", mgr.QueueLen())
	}
}

func TestBatchManager_Poll(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	mc.pollResult = &BatchStatus{
		ID:        "batch-123",
		Status:    "completed",
		Completed: 5,
	}
	mc.resultsResult = []Result{
		{RequestID: "r1", Content: "answer1"},
		{RequestID: "r2", Content: "answer2"},
	}

	cfg := BatchManagerConfig{Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	status, err := mgr.Poll(context.Background(), "batch-123")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}

	if status.BatchID != "batch-123" {
		t.Errorf("BatchID = %s, want batch-123", status.BatchID)
	}
	if status.Status != "completed" {
		t.Errorf("Status = %s, want completed", status.Status)
	}
	if status.CompletedCount != 5 {
		t.Errorf("CompletedCount = %d, want 5", status.CompletedCount)
	}
	if len(status.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(status.Results))
	}
}

func TestBatchManager_PollIncomplete(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	mc.pollResult = &BatchStatus{
		ID:        "batch-456",
		Status:    "processing",
		Completed: 2,
	}

	cfg := BatchManagerConfig{Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	status, err := mgr.Poll(context.Background(), "batch-456")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}

	if status.Status != "processing" {
		t.Errorf("Status = %s, want processing", status.Status)
	}
	// Should not have fetched results since batch is not complete.
	mc.mu.Lock()
	resultsCalls := len(mc.resultsCalls)
	mc.mu.Unlock()
	if resultsCalls != 0 {
		t.Errorf("expected 0 Results calls for incomplete batch, got %d", resultsCalls)
	}
	if len(status.Results) != 0 {
		t.Errorf("expected no results for incomplete batch, got %d", len(status.Results))
	}
}

func TestBatchManager_SubmitAssignsID(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()

	// Submit a request without an explicit ID.
	_, err := mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "no-id"},
		Priority: PriorityNormal,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Flush and verify the request got an ID assigned.
	_, err = mgr.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}

	calls := mc.getSubmitCalls()
	if len(calls) != 1 || len(calls[0]) != 1 {
		t.Fatalf("expected 1 call with 1 request, got %d calls", len(calls))
	}
	if calls[0][0].ID == "" {
		t.Error("expected request to have auto-assigned ID")
	}
}

func TestBatchManager_SubmitPreservesExplicitID(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)

	ctx := context.Background()
	_, err := mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{ID: "my-custom-id", UserPrompt: "hello"},
		Priority: PriorityNormal,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	_, err = mgr.Flush(ctx)
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}

	calls := mc.getSubmitCalls()
	if calls[0][0].ID != "my-custom-id" {
		t.Errorf("request ID = %q, want my-custom-id", calls[0][0].ID)
	}
}

func TestBatchManager_DefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := BatchManagerConfig{}
	if cfg.maxBatchSize() != 100 {
		t.Errorf("default maxBatchSize = %d, want 100", cfg.maxBatchSize())
	}
	if cfg.flushInterval() != 5*time.Minute {
		t.Errorf("default flushInterval = %v, want 5m", cfg.flushInterval())
	}
}

// ---------------------------------------------------------------------------
// Scheduler tests
// ---------------------------------------------------------------------------

func TestScheduler_AutoFlushOnSizeThreshold(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{
		MaxBatchSize:  3,
		FlushInterval: 1 * time.Hour, // Long interval so only size triggers.
		Provider:      ProviderClaude,
	}
	mgr := NewBatchManager(cfg, mc)

	var flushMu sync.Mutex
	var flushResults []*BatchManagerResult
	sched := NewScheduler(mgr, cfg)
	sched.OnFlush = func(r *BatchManagerResult) {
		flushMu.Lock()
		flushResults = append(flushResults, r)
		flushMu.Unlock()
	}

	ctx := context.Background()
	sched.Start(ctx)
	defer sched.Stop()

	// Submit enough requests to trigger the size threshold.
	for i := range 3 {
		_, err := mgr.Submit(ctx, BatchManagerRequest{
			Request:  Request{UserPrompt: fmt.Sprintf("p-%d", i)},
			Priority: PriorityNormal,
		})
		if err != nil {
			t.Fatalf("Submit %d: %v", i, err)
		}
	}

	// Wait for the scheduler to notice and flush.
	deadline := time.After(5 * time.Second)
	for {
		flushMu.Lock()
		n := len(flushResults)
		flushMu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for auto-flush on size threshold")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	flushMu.Lock()
	defer flushMu.Unlock()
	if flushResults[0].RequestCount != 3 {
		t.Errorf("flush RequestCount = %d, want 3", flushResults[0].RequestCount)
	}
}

func TestScheduler_AutoFlushOnInterval(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{
		MaxBatchSize:  1000,                   // Large so size won't trigger.
		FlushInterval: 200 * time.Millisecond, // Short interval for test.
		Provider:      ProviderClaude,
	}
	mgr := NewBatchManager(cfg, mc)

	var flushMu sync.Mutex
	var flushResults []*BatchManagerResult
	sched := NewScheduler(mgr, cfg)
	sched.OnFlush = func(r *BatchManagerResult) {
		flushMu.Lock()
		flushResults = append(flushResults, r)
		flushMu.Unlock()
	}

	ctx := context.Background()

	// Queue a request before starting the scheduler.
	_, err := mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "interval-test"},
		Priority: PriorityNormal,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	sched.Start(ctx)
	defer sched.Stop()

	// Wait for the interval flush.
	deadline := time.After(5 * time.Second)
	for {
		flushMu.Lock()
		n := len(flushResults)
		flushMu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for auto-flush on interval")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	flushMu.Lock()
	defer flushMu.Unlock()
	if flushResults[0].RequestCount != 1 {
		t.Errorf("flush RequestCount = %d, want 1", flushResults[0].RequestCount)
	}
}

func TestScheduler_StopIsIdempotent(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{FlushInterval: time.Hour, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)
	sched := NewScheduler(mgr, cfg)

	sched.Start(context.Background())
	if !sched.Running() {
		t.Error("expected Running() = true after Start")
	}

	sched.Stop()
	if sched.Running() {
		t.Error("expected Running() = false after Stop")
	}

	// Second stop should not panic.
	sched.Stop()
}

func TestScheduler_StartIsIdempotent(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{FlushInterval: time.Hour, Provider: ProviderClaude}
	mgr := NewBatchManager(cfg, mc)
	sched := NewScheduler(mgr, cfg)

	ctx := context.Background()
	sched.Start(ctx)
	defer sched.Stop()

	// Second start should be a no-op.
	sched.Start(ctx)
	if !sched.Running() {
		t.Error("expected Running() = true")
	}
}

func TestScheduler_FlushErrorCallsOnError(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	mc.submitErr = fmt.Errorf("api down")
	cfg := BatchManagerConfig{
		MaxBatchSize:  1,
		FlushInterval: time.Hour,
		Provider:      ProviderClaude,
	}
	mgr := NewBatchManager(cfg, mc)

	var errMu sync.Mutex
	var capturedErrors []error
	sched := NewScheduler(mgr, cfg)
	sched.OnError = func(err error) {
		errMu.Lock()
		capturedErrors = append(capturedErrors, err)
		errMu.Unlock()
	}

	ctx := context.Background()

	// Queue a request so the scheduler has something to flush.
	_, _ = mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "fail"},
		Priority: PriorityNormal,
	})

	sched.Start(ctx)
	defer sched.Stop()

	// Wait for error to be captured.
	deadline := time.After(5 * time.Second)
	for {
		errMu.Lock()
		n := len(capturedErrors)
		errMu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for OnError callback")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	errMu.Lock()
	defer errMu.Unlock()
	if capturedErrors[0] == nil {
		t.Error("expected non-nil error")
	}
}

func TestScheduler_StopDrainsQueue(t *testing.T) {
	t.Parallel()

	mc := newMockClient(ProviderClaude)
	cfg := BatchManagerConfig{
		MaxBatchSize:  1000,
		FlushInterval: time.Hour, // Long interval; only drain-on-stop triggers.
		Provider:      ProviderClaude,
	}
	mgr := NewBatchManager(cfg, mc)

	var flushMu sync.Mutex
	var flushResults []*BatchManagerResult
	sched := NewScheduler(mgr, cfg)
	sched.OnFlush = func(r *BatchManagerResult) {
		flushMu.Lock()
		flushResults = append(flushResults, r)
		flushMu.Unlock()
	}

	ctx := context.Background()

	// Queue a request.
	_, _ = mgr.Submit(ctx, BatchManagerRequest{
		Request:  Request{UserPrompt: "drain-me"},
		Priority: PriorityNormal,
	})

	sched.Start(ctx)
	// Stop should drain the queue.
	sched.Stop()

	flushMu.Lock()
	defer flushMu.Unlock()
	if len(flushResults) != 1 {
		t.Errorf("expected 1 flush on stop (drain), got %d", len(flushResults))
	}
}
