package batch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Priority controls whether a request is queued for batch or sent immediately.
type Priority int

const (
	// PriorityNormal queues the request for the next batch flush.
	PriorityNormal Priority = iota
	// PriorityUrgent bypasses the batch queue and sends immediately.
	PriorityUrgent
)

// BatchManagerConfig configures a BatchManager.
type BatchManagerConfig struct {
	// MaxBatchSize is the maximum number of requests per batch flush.
	// Defaults to 100 if zero.
	MaxBatchSize int

	// FlushInterval is how often the scheduler auto-flushes the queue.
	// Defaults to 5 minutes if zero.
	FlushInterval time.Duration

	// Provider identifies which batch API to use.
	Provider Provider
}

func (c *BatchManagerConfig) maxBatchSize() int {
	if c.MaxBatchSize > 0 {
		return c.MaxBatchSize
	}
	return 100
}

func (c *BatchManagerConfig) flushInterval() time.Duration {
	if c.FlushInterval > 0 {
		return c.FlushInterval
	}
	return 5 * time.Minute
}

// BatchManagerRequest wraps a batch Request with priority and tracking metadata.
type BatchManagerRequest struct {
	Request  Request
	Priority Priority
}

// BatchManagerResult holds the outcome of a Flush call.
type BatchManagerResult struct {
	BatchID                 string
	RequestCount            int
	EstimatedCompletionTime time.Time
}

// BatchManagerStatus holds polling results for a submitted batch.
type BatchManagerStatus struct {
	BatchID        string
	Status         string
	CompletedCount int
	Results        []Result
}

// BatchManager queues non-urgent requests and flushes them as provider batches.
// Urgent requests bypass the queue and are submitted immediately as single-item batches.
type BatchManager struct {
	cfg    BatchManagerConfig
	client Client

	mu    sync.Mutex
	queue []pendingRequest
	seq   atomic.Int64
}

type pendingRequest struct {
	id  string
	req Request
}

// NewBatchManager creates a BatchManager backed by the given Client.
func NewBatchManager(cfg BatchManagerConfig, client Client) *BatchManager {
	return &BatchManager{
		cfg:    cfg,
		client: client,
	}
}

// Submit adds a request to the batch queue (PriorityNormal) or sends it
// immediately (PriorityUrgent). Returns a unique request ID for tracking.
func (m *BatchManager) Submit(ctx context.Context, bmr BatchManagerRequest) (string, error) {
	id := m.nextID()

	// Ensure the request has an ID for correlation.
	req := bmr.Request
	if req.ID == "" {
		req.ID = id
	}

	if bmr.Priority == PriorityUrgent {
		// Send immediately as a single-item batch.
		_, err := m.client.Submit(ctx, []Request{req})
		if err != nil {
			return "", fmt.Errorf("batch manager: urgent submit: %w", err)
		}
		return id, nil
	}

	// Queue for the next batch flush.
	m.mu.Lock()
	m.queue = append(m.queue, pendingRequest{id: id, req: req})
	m.mu.Unlock()

	return id, nil
}

// QueueLen returns the number of requests currently queued for the next flush.
func (m *BatchManager) QueueLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue)
}

// Flush sends all queued requests as a single batch to the provider.
// Returns nil if the queue is empty.
func (m *BatchManager) Flush(ctx context.Context) (*BatchManagerResult, error) {
	m.mu.Lock()
	if len(m.queue) == 0 {
		m.mu.Unlock()
		return nil, nil
	}

	// Drain the queue.
	pending := m.queue
	m.queue = nil
	m.mu.Unlock()

	// Split into chunks of maxBatchSize if needed.
	maxSize := m.cfg.maxBatchSize()
	requests := make([]Request, len(pending))
	for i, p := range pending {
		requests[i] = p.req
	}

	// For simplicity, flush the first maxBatchSize items;
	// re-queue any overflow.
	if len(requests) > maxSize {
		overflow := requests[maxSize:]
		requests = requests[:maxSize]

		m.mu.Lock()
		for _, r := range overflow {
			m.queue = append(m.queue, pendingRequest{req: r})
		}
		m.mu.Unlock()
	}

	status, err := m.client.Submit(ctx, requests)
	if err != nil {
		// Re-queue on failure so requests are not lost.
		m.mu.Lock()
		requeue := make([]pendingRequest, len(requests))
		for i, r := range requests {
			requeue[i] = pendingRequest{req: r}
		}
		m.queue = append(requeue, m.queue...)
		m.mu.Unlock()
		return nil, fmt.Errorf("batch manager: flush: %w", err)
	}

	return &BatchManagerResult{
		BatchID:                 status.ID,
		RequestCount:            len(requests),
		EstimatedCompletionTime: time.Now().Add(24 * time.Hour),
	}, nil
}

// Poll checks the status of a previously flushed batch.
func (m *BatchManager) Poll(ctx context.Context, batchID string) (*BatchManagerStatus, error) {
	status, err := m.client.Poll(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("batch manager: poll: %w", err)
	}

	bms := &BatchManagerStatus{
		BatchID:        status.ID,
		Status:         status.Status,
		CompletedCount: status.Completed,
	}

	// If completed, also fetch results.
	if status.Status == "completed" {
		results, err := m.client.Results(ctx, batchID)
		if err != nil {
			return nil, fmt.Errorf("batch manager: fetch results: %w", err)
		}
		bms.Results = results
	}

	return bms, nil
}

func (m *BatchManager) nextID() string {
	n := m.seq.Add(1)
	return fmt.Sprintf("bmr-%d-%d", time.Now().UnixNano(), n)
}
