package session

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// APIEmbedder calls a remote embedding API (OpenAI) to produce vectors.
type APIEmbedder struct {
	provider string // "openai"
	endpoint string // API endpoint URL
	model    string // embedding model name
	client   *http.Client
	apiKey   string
}

// NewOpenAIEmbedder creates an embedder that calls OpenAI's text-embedding-3-small model.
func NewOpenAIEmbedder(apiKey string) *APIEmbedder {
	return &APIEmbedder{
		provider: "openai",
		endpoint: "https://api.openai.com/v1/embeddings",
		model:    "text-embedding-3-small",
		client:   &http.Client{Timeout: 30 * time.Second},
		apiKey:   apiKey,
	}
}

// Embed produces a vector embedding for the given text by calling the configured API.
func (e *APIEmbedder) Embed(text string) ([]float64, error) {
	var reqBody []byte
	var err error

	switch e.provider {
	case "openai":
		reqBody, err = json.Marshal(openAIEmbeddingRequest{
			Model: e.model,
			Input: text,
		})
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", e.provider)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, e.endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding API call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	switch e.provider {
	case "openai":
		var result openAIEmbeddingResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("unmarshal openai response: %w", err)
		}
		if len(result.Data) == 0 {
			return nil, fmt.Errorf("openai returned no embeddings")
		}
		return result.Data[0].Embedding, nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", e.provider)
	}
}

// OpenAI request/response types.
type openAIEmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []openAIEmbeddingData `json:"data"`
}

type openAIEmbeddingData struct {
	Embedding []float64 `json:"embedding"`
}

// CachingEmbedder wraps an Embedder and caches results keyed by text SHA-256 hash.
type CachingEmbedder struct {
	inner Embedder
	cache map[string][]float64
	mu    sync.RWMutex
}

// NewCachingEmbedder wraps an existing embedder with an in-memory cache.
func NewCachingEmbedder(inner Embedder) *CachingEmbedder {
	return &CachingEmbedder{
		inner: inner,
		cache: make(map[string][]float64),
	}
}

// Embed returns a cached embedding if available, otherwise delegates to the inner embedder.
func (c *CachingEmbedder) Embed(text string) ([]float64, error) {
	key := hashText(text)

	c.mu.RLock()
	if vec, ok := c.cache[key]; ok {
		c.mu.RUnlock()
		return vec, nil
	}
	c.mu.RUnlock()

	vec, err := c.inner.Embed(text)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.cache[key] = vec
	c.mu.Unlock()

	return vec, nil
}

// hashText returns a hex-encoded SHA-256 hash of the input text.
func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
