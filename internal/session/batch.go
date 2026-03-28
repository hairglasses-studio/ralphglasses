package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SessionResult holds the outcome of a completed batch session.
type SessionResult struct {
	SessionID  string        `json:"session_id"`
	Provider   Provider      `json:"provider"`
	Status     SessionStatus `json:"status"`
	SpentUSD   float64       `json:"spent_usd"`
	TurnCount  int           `json:"turn_count"`
	LastOutput string        `json:"last_output,omitempty"`
	Error      string        `json:"error,omitempty"`
	ExitReason string        `json:"exit_reason,omitempty"`
	Duration   time.Duration `json:"duration"`
	CollectedAt time.Time    `json:"collected_at"`
}

// BatchCollector tracks results from batch session executions.
// It is safe for concurrent use.
type BatchCollector struct {
	batchID       string
	callbackURL   string
	expectedCount int

	mu        sync.Mutex
	results   []SessionResult
	pollCursor int // index of next result to return from Poll
	completed bool
	httpClient *http.Client
}

// NewBatchCollector creates a new collector for the given batch.
func NewBatchCollector(batchID string) *BatchCollector {
	return &BatchCollector{
		batchID:    batchID,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// BatchID returns the batch identifier.
func (bc *BatchCollector) BatchID() string {
	return bc.batchID
}

// SetCallbackURL sets the webhook URL for result delivery on batch completion.
func (bc *BatchCollector) SetCallbackURL(url string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.callbackURL = url
}

// SetExpectedCount sets the total number of sessions expected in this batch.
// IsComplete returns true once this many results have been collected.
func (bc *BatchCollector) SetExpectedCount(n int) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.expectedCount = n
	bc.checkComplete()
}

// SetHTTPClient overrides the default HTTP client used for webhook callbacks.
// This is useful for testing.
func (bc *BatchCollector) SetHTTPClient(client *http.Client) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.httpClient = client
}

// AddResult records a session result. If the batch is now complete and a
// callback URL is set, the results are POSTed as JSON.
func (bc *BatchCollector) AddResult(sessionID string, result SessionResult) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	result.SessionID = sessionID
	if result.CollectedAt.IsZero() {
		result.CollectedAt = time.Now()
	}
	bc.results = append(bc.results, result)
	bc.checkComplete()
}

// Poll returns results collected since the last call to Poll.
// Returns nil if no new results are available.
func (bc *BatchCollector) Poll() []SessionResult {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.pollCursor >= len(bc.results) {
		return nil
	}

	newResults := make([]SessionResult, len(bc.results)-bc.pollCursor)
	copy(newResults, bc.results[bc.pollCursor:])
	bc.pollCursor = len(bc.results)
	return newResults
}

// Results returns all collected results.
func (bc *BatchCollector) Results() []SessionResult {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	out := make([]SessionResult, len(bc.results))
	copy(out, bc.results)
	return out
}

// IsComplete returns true when all expected sessions have reported results.
// Returns false if expected count has not been set (0).
func (bc *BatchCollector) IsComplete() bool {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.completed
}

// Count returns the number of results collected so far.
func (bc *BatchCollector) Count() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return len(bc.results)
}

// checkComplete checks if the batch is now complete and fires the callback
// if configured. Must be called with bc.mu held.
func (bc *BatchCollector) checkComplete() {
	if bc.expectedCount <= 0 || len(bc.results) < bc.expectedCount {
		return
	}
	if bc.completed {
		return
	}
	bc.completed = true

	if bc.callbackURL != "" {
		// Fire webhook in background to avoid blocking the caller.
		url := bc.callbackURL
		results := make([]SessionResult, len(bc.results))
		copy(results, bc.results)
		client := bc.httpClient
		batchID := bc.batchID

		go fireWebhook(client, url, batchID, results)
	}
}

// batchWebhookPayload is the JSON body sent to the callback URL.
type batchWebhookPayload struct {
	BatchID  string          `json:"batch_id"`
	Complete bool            `json:"complete"`
	Count    int             `json:"count"`
	Results  []SessionResult `json:"results"`
}

// fireWebhook POSTs batch results to the callback URL.
func fireWebhook(client *http.Client, url, batchID string, results []SessionResult) error {
	payload := batchWebhookPayload{
		BatchID:  batchID,
		Complete: true,
		Count:    len(results),
		Results:  results,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST to %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook POST to %s returned %d", url, resp.StatusCode)
	}
	return nil
}
