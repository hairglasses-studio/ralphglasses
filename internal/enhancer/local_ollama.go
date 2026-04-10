package enhancer

import (
	"net/url"
	"os"
	"strings"
)

const defaultLocalOllamaBaseURL = "http://127.0.0.1:11434"
const defaultLocalOllamaChatModel = "qwen3:8b"

func resolveProviderBaseURL(cfg LLMConfig, defaultBaseURL string) string {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" && strings.EqualFold(cfg.APIKeyEnv, "OLLAMA_API_KEY") {
		baseURL = strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func isLocalOllamaBaseURL(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

func resolveLocalOllamaAPIKey(baseURL string) string {
	if !isLocalOllamaBaseURL(baseURL) {
		return ""
	}
	if apiKey := strings.TrimSpace(os.Getenv("OLLAMA_API_KEY")); apiKey != "" {
		return apiKey
	}
	return "ollama"
}

func defaultLocalOllamaChatModelName() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_CHAT_MODEL")); model != "" {
		return model
	}
	return defaultLocalOllamaChatModel
}
