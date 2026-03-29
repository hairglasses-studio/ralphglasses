package batch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeminiCancel(t *testing.T) {
	t.Parallel()

	// Set up a test server that returns a valid Gemini batch response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiBatchResponse{
			Responses: []geminiInlineResponse{
				{
					Candidates: []geminiCandidate{
						{Content: geminiContent{Parts: []geminiPart{{Text: "hello"}}}},
					},
					UsageMetadata: &geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 3},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := NewClient(ProviderGemini, "test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()

	// Submit a batch to create stored results.
	status, err := c.Submit(ctx, []Request{{ID: "r1", UserPrompt: "hi"}})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Verify results are stored.
	results, err := c.Results(ctx, status.ID)
	if err != nil {
		t.Fatalf("Results before cancel: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Cancel removes stored results.
	if err := c.Cancel(ctx, status.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// After cancel, results should be gone.
	_, err = c.Results(ctx, status.ID)
	if err == nil {
		t.Fatal("expected error after cancel, got nil")
	}

	// Cancel on unknown batch ID is a no-op (no error).
	if err := c.Cancel(ctx, "nonexistent-batch"); err != nil {
		t.Fatalf("Cancel nonexistent: %v", err)
	}
}
