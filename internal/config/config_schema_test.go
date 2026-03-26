package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateConfig_ValidConfig(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		ScanPaths:           []string{dir},
		DefaultProvider:     "claude",
		WorkerProvider:      "gemini",
		MaxWorkers:          4,
		DefaultBudgetUSD:    5.0,
		CostRateMultiplier:  1.0,
		SessionTimeout:      10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}

	errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid config, got %v", errs)
	}
}

func TestValidateConfig_Nil(t *testing.T) {
	errs := ValidateConfig(nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors for nil config, got %v", errs)
	}
}

func TestValidateConfig_NonExistentScanPath(t *testing.T) {
	cfg := &Config{
		ScanPaths: []string{"/nonexistent/path/that/does/not/exist"},
	}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if errs[0] == nil {
		t.Fatal("expected non-nil error")
	}
}

func TestValidateConfig_UnknownProvider(t *testing.T) {
	cfg := &Config{
		DefaultProvider: "unknown_llm",
	}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_UnknownWorkerProvider(t *testing.T) {
	cfg := &Config{
		WorkerProvider: "mystery",
	}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_WorkerCountZero(t *testing.T) {
	// Zero is allowed (means "use default" or "no workers").
	cfg := &Config{MaxWorkers: 0}
	errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors for MaxWorkers=0, got %v", errs)
	}
}

func TestValidateConfig_WorkerCountNegative(t *testing.T) {
	cfg := &Config{MaxWorkers: -1}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_WorkerCountTooHigh(t *testing.T) {
	cfg := &Config{MaxWorkers: 100}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_NegativeBudget(t *testing.T) {
	cfg := &Config{DefaultBudgetUSD: -10.0}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_NegativeCostRate(t *testing.T) {
	cfg := &Config{CostRateMultiplier: -0.5}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_NegativeTimeout(t *testing.T) {
	cfg := &Config{SessionTimeout: -1 * time.Second}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_NegativeHealthCheckInterval(t *testing.T) {
	cfg := &Config{HealthCheckInterval: -5 * time.Second}
	errs := ValidateConfig(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_MultipleIssues(t *testing.T) {
	cfg := &Config{
		ScanPaths:          []string{"/nonexistent/aaa", "/nonexistent/bbb"},
		DefaultProvider:    "foo",
		WorkerProvider:     "bar",
		MaxWorkers:         100,
		DefaultBudgetUSD:   -1,
		CostRateMultiplier: -2,
		SessionTimeout:     -1 * time.Second,
	}
	errs := ValidateConfig(cfg)
	// 2 scan paths + 2 unknown providers + 1 workers + 1 budget + 1 cost rate + 1 timeout = 8
	if len(errs) != 8 {
		t.Errorf("expected 8 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateConfig_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty config, got %v", errs)
	}
}

func TestConfigValidateMethod(t *testing.T) {
	cfg := &Config{DefaultProvider: "bad"}
	errs := cfg.Validate()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error from Validate(), got %d: %v", len(errs), errs)
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/config.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoad_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	data := `{"scan_paths":["/tmp"],"default_provider":"claude","max_workers":4}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.DefaultProvider != "claude" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.DefaultProvider, "claude")
	}
	if cfg.MaxWorkers != 4 {
		t.Errorf("MaxWorkers = %d, want 4", cfg.MaxWorkers)
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestAllKnownProviders(t *testing.T) {
	for _, p := range []string{"claude", "gemini", "codex", "openai", "ollama"} {
		cfg := &Config{DefaultProvider: p}
		errs := ValidateConfig(cfg)
		if len(errs) != 0 {
			t.Errorf("provider %q should be valid, got errors: %v", p, errs)
		}
	}
}
