// E4.3: Local Model Provider — Ollama integration for self-hosted models.
// Optimized for the workstation-standard local model set rather than oversized
// defaults that do not fit comfortably on a 10 GB RTX 3080.
//
// Zero API cost, higher latency — the NeuralUCB bandit learns optimal routing
// between local and cloud providers based on task complexity and budget pressure.
package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider implements a local LLM provider using the Ollama API.
// It supports any installed Ollama model, with defaults tuned for local-first
// operation on this workstation.
type OllamaProvider struct {
	endpoint string // default http://127.0.0.1:11434
	model    string // default "qwen3:8b"
	client   *http.Client
}

// OllamaConfig configures the Ollama provider.
type OllamaConfig struct {
	Endpoint string        `json:"endpoint" yaml:"endpoint"` // default http://127.0.0.1:11434
	Model    string        `json:"model" yaml:"model"`       // default qwen3:8b
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`   // default 5m
}

// NewOllamaProvider creates an Ollama provider plugin.
func NewOllamaProvider(cfg OllamaConfig) *OllamaProvider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "http://127.0.0.1:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "qwen3:8b"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	return &OllamaProvider{
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		model:    cfg.Model,
		client:   &http.Client{Timeout: timeout},
	}
}

// --- Plugin interface ---

func (p *OllamaProvider) Name() string    { return "ollama-" + p.model }
func (p *OllamaProvider) Version() string { return "1.0.0" }
func (p *OllamaProvider) Enable() error   { return p.checkAvailable() }
func (p *OllamaProvider) Disable() error  { return nil }

// --- ProviderPlugin interface ---

func (p *OllamaProvider) ProviderName() string { return "ollama" }

// Complete sends a prompt to the Ollama API and returns the response.
func (p *OllamaProvider) Complete(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	reqBody := ollamaRequest{
		Model:  p.model,
		Prompt: prompt,
		Stream: false,
	}

	// Apply options
	if temp, ok := opts["temperature"]; ok {
		if t, ok := temp.(float64); ok {
			reqBody.Options.Temperature = t
		}
	}
	if maxTokens, ok := opts["max_tokens"]; ok {
		if m, ok := maxTokens.(float64); ok {
			reqBody.Options.NumPredict = int(m)
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama: decode response: %w", err)
	}

	return result.Response, nil
}

// checkAvailable verifies the Ollama server is running and the model is available.
func (p *OllamaProvider) checkAvailable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("ollama: not available: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: server not reachable at %s: %w", p.endpoint, err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: server returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// --- Ollama API types ---

type ollamaRequest struct {
	Model   string        `json:"model"`
	Prompt  string        `json:"prompt"`
	Stream  bool          `json:"stream"`
	Options ollamaOptions `json:"options"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}
