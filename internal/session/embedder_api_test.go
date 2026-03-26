package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// mockEmbedder is a test helper that counts calls and returns a fixed vector.
type mockEmbedder struct {
	calls int64
	vec   []float64
	err   error
}

func (m *mockEmbedder) Embed(_ string) ([]float64, error) {
	atomic.AddInt64(&m.calls, 1)
	return m.vec, m.err
}

func TestCachingEmbedder_CachesResults(t *testing.T) {
	inner := &mockEmbedder{vec: []float64{0.1, 0.2, 0.3}}
	ce := NewCachingEmbedder(inner)

	// First call should hit the inner embedder.
	v1, err := ce.Embed("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v1) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(v1))
	}

	// Second call with same text should return cached result.
	v2, err := ce.Embed("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v2) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(v2))
	}

	calls := atomic.LoadInt64(&inner.calls)
	if calls != 1 {
		t.Errorf("expected inner embedder called once, got %d", calls)
	}
}

func TestCachingEmbedder_DifferentTexts(t *testing.T) {
	inner := &mockEmbedder{vec: []float64{0.5, 0.5}}
	ce := NewCachingEmbedder(inner)

	_, _ = ce.Embed("text one")
	_, _ = ce.Embed("text two")
	_, _ = ce.Embed("text three")

	calls := atomic.LoadInt64(&inner.calls)
	if calls != 3 {
		t.Errorf("expected 3 inner calls for 3 different texts, got %d", calls)
	}
}

func TestCachingEmbedder_PropagatesErrors(t *testing.T) {
	inner := &mockEmbedder{err: fmt.Errorf("api down")}
	ce := NewCachingEmbedder(inner)

	_, err := ce.Embed("anything")
	if err == nil {
		t.Fatal("expected error from inner embedder")
	}

	// Error results should not be cached — a second call should try again.
	calls := atomic.LoadInt64(&inner.calls)
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}

	_, _ = ce.Embed("anything")
	calls = atomic.LoadInt64(&inner.calls)
	if calls != 2 {
		t.Errorf("expected 2 calls (error not cached), got %d", calls)
	}
}

func TestOpenAIEmbedder_RequestFormat(t *testing.T) {
	var gotReq openAIEmbeddingRequest
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "bad", 500)
			return
		}
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Errorf("unmarshal body: %v", err)
			http.Error(w, "bad", 500)
			return
		}

		resp := openAIEmbeddingResponse{
			Data: []openAIEmbeddingData{
				{Embedding: []float64{0.1, 0.2, 0.3}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewOpenAIEmbedder("test-key-123")
	e.endpoint = server.URL // Override to use test server.

	vec, err := e.Embed("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request format.
	if gotReq.Model != "text-embedding-3-small" {
		t.Errorf("expected model 'text-embedding-3-small', got %q", gotReq.Model)
	}
	if gotReq.Input != "hello world" {
		t.Errorf("expected input 'hello world', got %q", gotReq.Input)
	}
	if gotAuth != "Bearer test-key-123" {
		t.Errorf("expected auth 'Bearer test-key-123', got %q", gotAuth)
	}

	// Verify response parsing.
	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[1] != 0.2 || vec[2] != 0.3 {
		t.Errorf("unexpected vector: %v", vec)
	}
}

func TestOllamaEmbedder_RequestFormat(t *testing.T) {
	var gotReq ollamaEmbeddingRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ollama should not have Authorization header.
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no auth header for ollama, got %q", auth)
		}

		// Verify path.
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("expected path /api/embeddings, got %q", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "bad", 500)
			return
		}
		if err := json.Unmarshal(body, &gotReq); err != nil {
			t.Errorf("unmarshal body: %v", err)
			http.Error(w, "bad", 500)
			return
		}

		resp := ollamaEmbeddingResponse{
			Embedding: []float64{0.4, 0.5, 0.6, 0.7},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// NewOllamaEmbedder appends /api/embeddings, so use server URL as base.
	e := NewOllamaEmbedder("unused")
	e.endpoint = server.URL + "/api/embeddings" // Override to use test server.

	vec, err := e.Embed("test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify request format.
	if gotReq.Model != "nomic-embed-text" {
		t.Errorf("expected model 'nomic-embed-text', got %q", gotReq.Model)
	}
	if gotReq.Prompt != "test prompt" {
		t.Errorf("expected prompt 'test prompt', got %q", gotReq.Prompt)
	}

	// Verify response parsing.
	if len(vec) != 4 {
		t.Fatalf("expected 4 dimensions, got %d", len(vec))
	}
	if vec[0] != 0.4 || vec[3] != 0.7 {
		t.Errorf("unexpected vector: %v", vec)
	}
}

func TestOpenAIEmbedder_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	e := NewOpenAIEmbedder("bad-key")
	e.endpoint = server.URL

	_, err := e.Embed("test")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestOllamaEmbedder_EmptyEmbedding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaEmbeddingResponse{Embedding: nil}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewOllamaEmbedder("unused")
	e.endpoint = server.URL + "/api/embeddings"

	_, err := e.Embed("test")
	if err == nil {
		t.Fatal("expected error for empty embedding")
	}
}
