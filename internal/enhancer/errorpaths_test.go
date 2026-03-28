package enhancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- CircuitBreaker.Reset ---

func TestCircuitBreaker_Reset(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker()

	// Trip the circuit breaker
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != "open" {
		t.Fatalf("expected open, got %s", cb.State())
	}

	// Reset should close it
	cb.Reset()
	if cb.State() != "closed" {
		t.Errorf("expected closed after Reset, got %s", cb.State())
	}
	if !cb.Allow() {
		t.Error("expected Allow() true after Reset")
	}

	// Should need 3 new failures to open again
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Errorf("expected closed after 2 failures post-Reset, got %s", cb.State())
	}
	cb.RecordFailure()
	if cb.State() != "open" {
		t.Errorf("expected open after 3 new failures, got %s", cb.State())
	}
}

// --- ClassifyDetailed ---

func TestClassifyDetailed_CodePrompt(t *testing.T) {
	t.Parallel()
	best, alts := ClassifyDetailed("implement a function with error handling and unit test coverage")

	if best.TaskType != TaskTypeCode {
		t.Errorf("expected code, got %s", best.TaskType)
	}
	if best.RawScore <= 0 {
		t.Error("expected positive raw score")
	}
	if best.Confidence <= 0 {
		t.Error("expected positive confidence")
	}
	// Should have some alternatives
	_ = alts
}

func TestClassifyDetailed_NoPatternMatches(t *testing.T) {
	t.Parallel()
	best, alts := ClassifyDetailed("hello world")

	if best.TaskType != TaskTypeGeneral {
		t.Errorf("expected general for no-match prompt, got %s", best.TaskType)
	}
	if best.Confidence != 0.3 {
		t.Errorf("expected 0.3 default confidence, got %f", best.Confidence)
	}
	if alts != nil {
		t.Errorf("expected nil alternatives for no-match, got %v", alts)
	}
}

func TestClassifyDetailed_MixedSignals(t *testing.T) {
	t.Parallel()
	// Prompt with signals from multiple categories
	best, alts := ClassifyDetailed("debug the build pipeline and review the code for error handling issues")

	if best.RawScore <= 0 {
		t.Error("expected positive raw score")
	}
	if best.Confidence < 0.3 {
		t.Errorf("confidence should be at least 0.3, got %f", best.Confidence)
	}
	if len(alts) == 0 {
		t.Error("expected alternatives for mixed-signal prompt")
	}
}

// --- Config functions: ValidateConfig, ResolveConfig, LoadConfigWithFallback, configFound ---

func TestValidateConfig_AllWarnings(t *testing.T) {
	t.Parallel()
	cfg := Config{
		LLM: LLMConfig{
			Enabled:     true,
			Timeout:     2 * time.Second,  // too short
			Provider:    "unknown_vendor",  // unknown provider
			EffortLevel: "extreme",         // unknown effort
		},
		Hook: HookConfig{
			SkipScoreThreshold: 200, // out of range
			MinWordCount:       -5,  // negative
		},
		TargetProvider: "bogus",
	}

	warnings := ValidateConfig(cfg)
	if len(warnings) == 0 {
		t.Fatal("expected warnings for misconfigured config")
	}

	// Check for specific warnings
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "very short") {
		t.Error("expected short timeout warning")
	}
	if !strings.Contains(joined, "unknown LLM provider") {
		t.Error("expected unknown provider warning")
	}
	if !strings.Contains(joined, "unknown effort level") {
		t.Error("expected unknown effort warning")
	}
	if !strings.Contains(joined, "out of range") {
		t.Error("expected threshold out of range warning")
	}
	if !strings.Contains(joined, "negative") {
		t.Error("expected negative min_word_count warning")
	}
	if !strings.Contains(joined, "unknown target provider") {
		t.Error("expected unknown target provider warning")
	}
}

func TestValidateConfig_LongTimeout(t *testing.T) {
	t.Parallel()
	cfg := Config{
		LLM: LLMConfig{
			Timeout: 300 * time.Second,
		},
	}
	warnings := ValidateConfig(cfg)
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "very long") {
		t.Error("expected long timeout warning")
	}
}

func TestValidateConfig_EnabledNoModel(t *testing.T) {
	t.Parallel()
	cfg := Config{
		LLM: LLMConfig{
			Enabled: true,
			Model:   "",
		},
	}
	warnings := ValidateConfig(cfg)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "model name is empty") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected warning about empty model when LLM is enabled")
	}
}

func TestValidateConfig_Clean(t *testing.T) {
	t.Parallel()
	cfg := Config{
		LLM: LLMConfig{
			Enabled:     true,
			Provider:    "claude",
			Model:       "sonnet",
			Timeout:     30 * time.Second,
			EffortLevel: "medium",
		},
		Hook: HookConfig{
			SkipScoreThreshold: 75,
			MinWordCount:       5,
		},
		TargetProvider: "gemini",
	}
	warnings := ValidateConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid config, got: %v", warnings)
	}
}

func TestResolveConfig_EnvOverrides(t *testing.T) {
	dir := t.TempDir()
	// Write a base config
	content := []byte("preamble: base\nllm:\n  enabled: false\n  model: base-model\n")
	if err := os.WriteFile(filepath.Join(dir, ".prompt-improver.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	// Set env overrides
	t.Setenv("PROMPT_IMPROVER_LLM", "1")
	t.Setenv("PROMPT_IMPROVER_MODEL", "sonnet-override")
	t.Setenv("PROMPT_IMPROVER_TIMEOUT", "45s")
	t.Setenv("PROMPT_IMPROVER_PROVIDER", "gemini")
	t.Setenv("PROMPT_IMPROVER_TARGET", "openai")
	t.Setenv("PROMPT_IMPROVER_EFFORT", "high")
	t.Setenv("PROMPT_IMPROVER_CACHE", "true")

	cfg := ResolveConfig(dir)

	if !cfg.LLM.Enabled {
		t.Error("expected LLM.Enabled=true from env")
	}
	if cfg.LLM.Model != "sonnet-override" {
		t.Errorf("expected model 'sonnet-override', got %q", cfg.LLM.Model)
	}
	if cfg.LLM.Timeout != 45*time.Second {
		t.Errorf("expected timeout 45s, got %v", cfg.LLM.Timeout)
	}
	if cfg.LLM.Provider != "gemini" {
		t.Errorf("expected provider 'gemini', got %q", cfg.LLM.Provider)
	}
	if cfg.TargetProvider != "openai" {
		t.Errorf("expected target 'openai', got %q", cfg.TargetProvider)
	}
	if cfg.LLM.EffortLevel != "high" {
		t.Errorf("expected effort 'high', got %q", cfg.LLM.EffortLevel)
	}
	if !cfg.LLM.CacheControl || !cfg.LLM.cacheControlSet {
		t.Error("expected CacheControl=true from env")
	}
}

func TestResolveConfig_DisableLLM(t *testing.T) {
	dir := t.TempDir()
	content := []byte("llm:\n  enabled: true\n")
	if err := os.WriteFile(filepath.Join(dir, ".prompt-improver.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PROMPT_IMPROVER_LLM", "false")
	cfg := ResolveConfig(dir)
	if cfg.LLM.Enabled {
		t.Error("expected LLM.Enabled=false from env override")
	}
}

func TestResolveConfig_DisableCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROMPT_IMPROVER_CACHE", "0")
	cfg := ResolveConfig(dir)
	if cfg.LLM.CacheControl {
		t.Error("expected CacheControl=false from env")
	}
	if !cfg.LLM.cacheControlSet {
		t.Error("expected cacheControlSet=true when env is set")
	}
}

func TestResolveConfig_InvalidTimeout(t *testing.T) {
	dir := t.TempDir()

	// Write a config with a known timeout value
	content := []byte("llm:\n  timeout: 10s\n")
	if err := os.WriteFile(filepath.Join(dir, ".prompt-improver.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PROMPT_IMPROVER_TIMEOUT", "not-a-duration")
	cfg := ResolveConfig(dir)
	// Invalid duration should leave the file-based timeout unchanged
	if cfg.LLM.Timeout != 10*time.Second {
		t.Errorf("expected 10s timeout (from file), got %v", cfg.LLM.Timeout)
	}
}

func TestLoadConfigWithFallback_ProjectFirst(t *testing.T) {
	dir := t.TempDir()
	content := []byte("preamble: project-level\n")
	if err := os.WriteFile(filepath.Join(dir, ".prompt-improver.yaml"), content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadConfigWithFallback(dir)
	if cfg.Preamble != "project-level" {
		t.Errorf("expected preamble 'project-level', got %q", cfg.Preamble)
	}
}

func TestLoadConfigWithFallback_NoConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := LoadConfigWithFallback(dir)
	// Should return zero config (no fallback without home dir config)
	if cfg.Preamble != "" {
		t.Errorf("expected empty preamble, got %q", cfg.Preamble)
	}
}

func TestConfigFound_Empty(t *testing.T) {
	t.Parallel()
	cfg := Config{}
	if configFound(cfg) {
		t.Error("expected configFound=false for zero Config")
	}
}

func TestConfigFound_WithPreamble(t *testing.T) {
	t.Parallel()
	cfg := Config{Preamble: "test"}
	if !configFound(cfg) {
		t.Error("expected configFound=true when Preamble is set")
	}
}

func TestConfigFound_WithLLMEnabled(t *testing.T) {
	t.Parallel()
	cfg := Config{LLM: LLMConfig{Enabled: true}}
	if !configFound(cfg) {
		t.Error("expected configFound=true when LLM.Enabled is set")
	}
}

func TestConfigFound_WithTargetProvider(t *testing.T) {
	t.Parallel()
	cfg := Config{TargetProvider: "gemini"}
	if !configFound(cfg) {
		t.Error("expected configFound=true when TargetProvider is set")
	}
}

// --- NewHybridEngine ---

func TestNewHybridEngine_Disabled(t *testing.T) {
	t.Parallel()
	engine := NewHybridEngine(LLMConfig{Enabled: false})
	if engine != nil {
		t.Error("expected nil engine when LLM is disabled")
	}
}

func TestNewHybridEngine_NoAPIKey(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv
	// Remove API keys to ensure NewPromptImprover returns nil
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	engine := NewHybridEngine(LLMConfig{
		Enabled:  true,
		Provider: "claude",
	})
	if engine != nil {
		t.Error("expected nil engine when no API key is available")
	}
}

// --- LLM API error paths ---

func TestEnhanceHybrid_Timeout(t *testing.T) {
	t.Parallel()
	// Server that hangs for 5 seconds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		resp := newMockResponse("too late")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	engine := newTestEngine(server.URL)
	engine.Cfg.Timeout = 100 * time.Millisecond

	result := EnhanceHybrid(context.Background(), "fix the bug in the auth module", "", Config{}, engine, ModeAuto, "")
	if result.Source != "local_fallback" {
		t.Errorf("expected local_fallback on timeout, got %q", result.Source)
	}
}

func TestEnhanceHybrid_RateLimitError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the authentication bug", "", Config{}, engine, ModeAuto, "")

	if result.Source != "local_fallback" {
		t.Errorf("expected local_fallback on rate limit, got %q", result.Source)
	}
}

func TestEnhanceHybrid_MalformedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"not_valid": true}`))
	}))
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the authentication module bug", "", Config{}, engine, ModeAuto, "")

	// Should either succeed (if the empty text triggers error) or fallback
	if result.Source != "local_fallback" && result.Source != "llm" {
		t.Errorf("expected local_fallback or llm on malformed response, got %q", result.Source)
	}
}

func TestEnhanceHybrid_TargetProvider(t *testing.T) {
	t.Parallel()
	server := newTestLLMServer("improved for gemini")
	defer server.Close()

	engine := newTestEngine(server.URL)
	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeAuto, "gemini")

	if result.Source != "llm" {
		t.Errorf("expected llm, got %q", result.Source)
	}
}

// --- Scoring edge cases ---

func TestScoreTaskFocus_NoVerbs(t *testing.T) {
	t.Parallel()
	// A prompt with no imperative verbs
	ar := Analyze("the system architecture and design patterns")
	lints := Lint("the system architecture and design patterns")
	report := Score("the system architecture and design patterns", TaskTypeAnalysis, lints, &ar, "")
	// Should not panic; taskFocus score should be lower
	found := false
	for _, d := range report.Dimensions {
		if d.Name == "Task Focus" {
			found = true
			// No verbs should result in score penalty
			if d.Score > 80 {
				t.Errorf("expected lower task focus score for no-verb prompt, got %d", d.Score)
			}
		}
	}
	if !found {
		t.Error("Task Focus dimension not found in report")
	}
}

func TestScoreDocumentPlacement_LongNoXML(t *testing.T) {
	t.Parallel()
	// A long prompt without XML structure
	long := strings.Repeat("Analyze the system. ", 500) // >5000 tokens
	ar := Analyze(long)
	lints := Lint(long)
	report := Score(long, TaskTypeAnalysis, lints, &ar, "")

	for _, d := range report.Dimensions {
		if d.Name == "Document Placement" {
			if d.Score > 60 {
				t.Errorf("expected lower placement score for long non-XML prompt, got %d", d.Score)
			}
		}
	}
}

// --- ShouldEnhance edge cases ---

func TestShouldEnhance_InvalidRegexInSkipPatterns(t *testing.T) {
	t.Parallel()
	cfg := Config{
		Hook: HookConfig{
			SkipPatterns: []string{"[invalid regex"},
		},
	}
	// Should not panic on invalid regex, just skip the pattern
	got := ShouldEnhance("fix the sorting bug in the user module", cfg)
	if !got {
		t.Error("expected true — invalid regex should be skipped gracefully")
	}
}

// --- LoadConfig malformed YAML ---

func TestLoadConfig_MalformedYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".prompt-improver.yaml"), []byte(":\n  bad:\n  yaml: [unclosed"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := LoadConfig(dir)
	// Should return zero config for malformed YAML
	if cfg.Preamble != "" {
		t.Errorf("expected empty config for malformed YAML, got preamble %q", cfg.Preamble)
	}
}

// --- ValidMode for sampling ---

func TestValidMode_Sampling(t *testing.T) {
	t.Parallel()
	got := ValidMode("sampling")
	if got != ModeSampling {
		t.Errorf("ValidMode('sampling') = %q, want %q", got, ModeSampling)
	}
}
