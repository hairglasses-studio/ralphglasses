package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultOllamaBaseURL          = "http://127.0.0.1:11434"
	defaultOllamaChatModel        = "code-primary"
	defaultOllamaFastModel        = "code-fast"
	defaultOllamaCodeModel        = "code-primary"
	defaultOllamaHeavyCodeModel   = "code-heavy"
	defaultOllamaHighContextModel = "code-long"
	defaultOllamaCloudCodeModel   = "glm-5.1:cloud"
	defaultOllamaCloudVerifyModel = "glm-5:cloud"
	defaultOllamaMultiCodeModel   = "minimax-m2.1:cloud"
	defaultOllamaThinkingModel    = "kimi-k2-thinking:cloud"
	defaultOllamaEmbedModel       = "nomic-embed-text:v1.5"
	defaultOllamaKeepAlive        = "15m"
)

type ollamaTagsResponse struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

func normalizeSessionProvider(provider Provider) Provider {
	raw := strings.ToLower(strings.TrimSpace(string(provider)))
	switch raw {
	case "":
		return ""
	case "openai":
		return ProviderCodex
	default:
		return Provider(raw)
	}
}

func resolveOllamaBaseURL() string {
	baseURL := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}
	return baseURL
}

func resolveOllamaCompatBaseURL() string {
	return resolveOllamaBaseURL() + "/v1"
}

func resolveOllamaAPIKey() string {
	if apiKey := strings.TrimSpace(os.Getenv("OLLAMA_API_KEY")); apiKey != "" {
		return apiKey
	}
	return "ollama"
}

func resolveOllamaKeepAlive() string {
	if keepAlive := strings.TrimSpace(os.Getenv("OLLAMA_KEEP_ALIVE")); keepAlive != "" {
		return keepAlive
	}
	return defaultOllamaKeepAlive
}

func resolveOllamaChatModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_CHAT_MODEL")); model != "" {
		return model
	}
	return defaultOllamaChatModel
}

func resolveOllamaFastModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_FAST_MODEL")); model != "" {
		return model
	}
	return defaultOllamaFastModel
}

func resolveOllamaCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaCodeModel
}

func resolveOllamaHeavyCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_HEAVY_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaHeavyCodeModel
}

func resolveOllamaHighContextCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_HIGH_CONTEXT_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaHighContextModel
}

func resolveOllamaCloudCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_CLOUD_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaCloudCodeModel
}

func resolveOllamaCloudVerifiedCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_CLOUD_VERIFIED_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaCloudVerifyModel
}

func resolveOllamaMultilingualCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_MULTILINGUAL_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaMultiCodeModel
}

func resolveOllamaThinkingCodeModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_THINKING_CODE_MODEL")); model != "" {
		return model
	}
	return defaultOllamaThinkingModel
}

func resolveOllamaEmbedModel() string {
	if model := strings.TrimSpace(os.Getenv("OLLAMA_EMBED_MODEL")); model != "" {
		return model
	}
	return defaultOllamaEmbedModel
}

func prepareOllamaLaunch(opts *LaunchOptions) error {
	if opts == nil {
		return nil
	}
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = resolveOllamaCodeModel()
	}
	if strings.TrimSpace(opts.Model) == "" {
		return fmt.Errorf("ollama launch requires a model; set OLLAMA_CODE_MODEL or pass --model explicitly")
	}
	if isOllamaCloudModel(opts.Model) {
		return nil
	}

	models, err := fetchOllamaModels(context.Background(), 5*time.Second)
	if err != nil {
		return err
	}
	if ollamaModelInstalledExact(opts.Model, models) {
		return nil
	}
	if sourceModel := ollamaAliasSourceModel(opts.Model); sourceModel != "" && ollamaModelInstalledExact(sourceModel, models) {
		opts.Model = sourceModel
		return nil
	}
	return fmt.Errorf("ollama model %q is not available at %s; sync aliases with `~/hairglasses-studio/dotfiles/scripts/hg-ollama-sync-aliases.sh` or pull the backing model with `ollama pull %s`",
		opts.Model, resolveOllamaBaseURL(), ollamaPullHintModel(opts.Model))
}

func buildOllamaClaudeCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	cmd := buildClaudeCmd(ctx, opts)
	env := cmd.Env
	if env == nil {
		env = quietAgentSessionEnv(os.Environ())
	}
	cmd.Env = upsertEnv(
		env,
		"OLLAMA_BASE_URL", resolveOllamaBaseURL(),
		"OLLAMA_API_KEY", resolveOllamaAPIKey(),
		"OLLAMA_KEEP_ALIVE", resolveOllamaKeepAlive(),
		"ANTHROPIC_BASE_URL", resolveOllamaCompatBaseURL(),
		"ANTHROPIC_API_KEY", resolveOllamaAPIKey(),
		"ANTHROPIC_AUTH_TOKEN", resolveOllamaAPIKey(),
	)
	return cmd
}

func upsertEnv(env []string, kv ...string) []string {
	if len(kv)%2 != 0 {
		return env
	}
	out := append([]string(nil), env...)
	for i := 0; i < len(kv); i += 2 {
		key := kv[i]
		value := kv[i+1]
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = filterEnv(out, key)
		if value == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func fetchOllamaModels(ctx context.Context, timeout time.Duration) ([]string, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, resolveOllamaBaseURL()+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("build ollama tags request: %w", err)
	}
	if apiKey := resolveOllamaAPIKey(); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama endpoint %s is not reachable: %w", resolveOllamaBaseURL(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama endpoint %s returned HTTP %d", resolveOllamaBaseURL(), resp.StatusCode)
	}

	var payload ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode ollama tags response: %w", err)
	}

	models := make([]string, 0, len(payload.Models)*2)
	for _, model := range payload.Models {
		if name := strings.TrimSpace(model.Name); name != "" {
			models = append(models, name)
		}
		if id := strings.TrimSpace(model.Model); id != "" {
			models = append(models, id)
		}
	}
	return models, nil
}

func ollamaModelInstalledExact(model string, available []string) bool {
	candidates := ollamaInstalledModelCandidates(model)
	for _, current := range available {
		normalized := strings.TrimSpace(current)
		for _, candidate := range candidates {
			if normalized == candidate {
				return true
			}
		}
	}
	return false
}

func ollamaInstalledModelCandidates(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	candidates := []string{model}
	if strings.HasSuffix(model, ":latest") {
		candidates = append(candidates, strings.TrimSuffix(model, ":latest"))
	} else if !strings.Contains(model, ":") {
		candidates = append(candidates, model+":latest")
	}
	return candidates
}

func ollamaAliasSourceModel(model string) string {
	switch strings.TrimSpace(model) {
	case "code-fast", "code-compact":
		return "qwen2.5-coder:7b"
	case "code-primary", "code-reasoner":
		return "devstral-small-2"
	case "code-long":
		return "qwen3-coder-next"
	case "code-heavy":
		return "devstral-2"
	default:
		return ""
	}
}

func isOllamaCloudModel(model string) bool {
	return strings.Contains(strings.TrimSpace(model), ":cloud")
}

func ollamaPullHintModel(model string) string {
	if sourceModel := ollamaAliasSourceModel(model); sourceModel != "" {
		return sourceModel
	}
	return model
}
