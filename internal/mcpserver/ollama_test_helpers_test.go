package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newReadyOllamaInventoryServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": "code-primary"},
				{"name": "devstral-small-2"},
				{"name": "code-fast"},
				{"name": "qwen2.5-coder:7b"},
				{"name": "code-heavy"},
				{"name": "devstral-2"},
				{"name": "code-long"},
				{"name": "qwen3-coder-next"},
				{"name": "nomic-embed-text:v1.5"},
			},
		})
	}))
}

func setReadyOllamaEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("OLLAMA_BASE_URL", baseURL)
	t.Setenv("OLLAMA_CHAT_MODEL", "code-primary")
	t.Setenv("OLLAMA_FAST_MODEL", "code-fast")
	t.Setenv("OLLAMA_CODE_MODEL", "code-primary")
	t.Setenv("OLLAMA_HEAVY_CODE_MODEL", "code-heavy")
	t.Setenv("OLLAMA_HIGH_CONTEXT_CODE_MODEL", "code-long")
	t.Setenv("OLLAMA_EMBED_MODEL", "nomic-embed-text:v1.5")
}
