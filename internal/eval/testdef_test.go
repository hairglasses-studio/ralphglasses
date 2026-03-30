package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `name: claude-vs-gemini
description: Compare Claude Sonnet and Gemini Flash on coding tasks
variant_a:
  name: claude-sonnet
  provider: claude
  model: claude-sonnet-4-20250514
  prompt: "Write a Go function that reverses a string"
  temperature: 0.3
variant_b:
  name: gemini-flash
  provider: gemini
  model: gemini-2.5-flash
  prompt: "Write a Go function that reverses a string"
  temperature: 0.5
metrics:
  - name: response_quality
    type: quality
  - name: response_latency
    type: latency
sample_size: 30
timeout: 5m
`

func TestParseTestDefinition(t *testing.T) {
	def, err := ParseTestDefinition([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.Name != "claude-vs-gemini" {
		t.Errorf("name = %q, want %q", def.Name, "claude-vs-gemini")
	}
	if def.VariantA.Provider != "claude" {
		t.Errorf("variant_a.provider = %q, want %q", def.VariantA.Provider, "claude")
	}
	if def.VariantB.Provider != "gemini" {
		t.Errorf("variant_b.provider = %q, want %q", def.VariantB.Provider, "gemini")
	}
	if len(def.Metrics) != 2 {
		t.Fatalf("len(metrics) = %d, want 2", len(def.Metrics))
	}
	if def.Metrics[0].Type != MetricTypeQuality {
		t.Errorf("metrics[0].type = %q, want %q", def.Metrics[0].Type, MetricTypeQuality)
	}
	if def.SampleSize != 30 {
		t.Errorf("sample_size = %d, want 30", def.SampleSize)
	}
	if def.Timeout != "5m" {
		t.Errorf("timeout = %q, want %q", def.Timeout, "5m")
	}
}

func TestParseTestDefinition_InvalidYAML(t *testing.T) {
	_, err := ParseTestDefinition([]byte("{{invalid"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidateTestDefinition_MissingName(t *testing.T) {
	def := &TestDefinition{
		VariantA:   VariantDef{Provider: "claude", Model: "claude-sonnet"},
		VariantB:   VariantDef{Provider: "gemini", Model: "gemini-flash"},
		Metrics:    []MetricSpec{{Name: "q", Type: MetricTypeQuality}},
		SampleSize: 10,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required', got: %v", err)
	}
}

func TestValidateTestDefinition_MissingVariantProvider(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Model: "claude-sonnet"},
		VariantB:   VariantDef{Provider: "gemini", Model: "gemini-flash"},
		Metrics:    []MetricSpec{{Name: "q", Type: MetricTypeQuality}},
		SampleSize: 10,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "variant_a.provider is required") {
		t.Errorf("expected variant_a.provider error, got: %v", err)
	}
}

func TestValidateTestDefinition_MissingVariantModel(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Provider: "claude", Model: "claude-sonnet"},
		VariantB:   VariantDef{Provider: "gemini"},
		Metrics:    []MetricSpec{{Name: "q", Type: MetricTypeQuality}},
		SampleSize: 10,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "variant_b.model is required") {
		t.Errorf("expected variant_b.model error, got: %v", err)
	}
}

func TestValidateTestDefinition_NoMetrics(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Provider: "claude", Model: "m"},
		VariantB:   VariantDef{Provider: "gemini", Model: "m"},
		Metrics:    nil,
		SampleSize: 10,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "at least one metric") {
		t.Errorf("expected metric error, got: %v", err)
	}
}

func TestValidateTestDefinition_InvalidMetricType(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Provider: "claude", Model: "m"},
		VariantB:   VariantDef{Provider: "gemini", Model: "m"},
		Metrics:    []MetricSpec{{Name: "x", Type: "bogus"}},
		SampleSize: 10,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected invalid metric type error, got: %v", err)
	}
}

func TestValidateTestDefinition_ZeroSampleSize(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Provider: "claude", Model: "m"},
		VariantB:   VariantDef{Provider: "gemini", Model: "m"},
		Metrics:    []MetricSpec{{Name: "q", Type: MetricTypeQuality}},
		SampleSize: 0,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "sample_size must be > 0") {
		t.Errorf("expected sample_size error, got: %v", err)
	}
}

func TestValidateTestDefinition_InvalidTimeout(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Provider: "claude", Model: "m"},
		VariantB:   VariantDef{Provider: "gemini", Model: "m"},
		Metrics:    []MetricSpec{{Name: "q", Type: MetricTypeQuality}},
		SampleSize: 10,
		Timeout:    "not-a-duration",
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "not a valid duration") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestValidateTestDefinition_MultipleErrors(t *testing.T) {
	def := &TestDefinition{}
	err := ValidateTestDefinition(def)
	if err == nil {
		t.Fatal("expected error")
	}
	// Should contain multiple errors separated by semicolons.
	if strings.Count(err.Error(), ";") < 3 {
		t.Errorf("expected multiple errors, got: %v", err)
	}
}

func TestLoadAndSaveTestDefinition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yml")

	def := &TestDefinition{
		Name:        "round-trip",
		Description: "Tests load/save round trip",
		VariantA:    VariantDef{Name: "a", Provider: "claude", Model: "claude-sonnet", Prompt: "hello", Temperature: 0.2},
		VariantB:    VariantDef{Name: "b", Provider: "gemini", Model: "gemini-flash", Prompt: "hello", Temperature: 0.8},
		Metrics:     []MetricSpec{{Name: "latency", Type: MetricTypeLatency}},
		SampleSize:  50,
		Timeout:     "10m",
	}

	if err := SaveTestDefinition(def, path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadTestDefinition(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Name != def.Name {
		t.Errorf("name = %q, want %q", loaded.Name, def.Name)
	}
	if loaded.VariantA.Provider != def.VariantA.Provider {
		t.Errorf("variant_a.provider = %q, want %q", loaded.VariantA.Provider, def.VariantA.Provider)
	}
	if loaded.VariantB.Temperature != def.VariantB.Temperature {
		t.Errorf("variant_b.temperature = %f, want %f", loaded.VariantB.Temperature, def.VariantB.Temperature)
	}
	if loaded.SampleSize != def.SampleSize {
		t.Errorf("sample_size = %d, want %d", loaded.SampleSize, def.SampleSize)
	}
}

func TestLoadTestDefinition_NotFound(t *testing.T) {
	_, err := LoadTestDefinition("/nonexistent/path.yml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSaveTestDefinition_InvalidDef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	def := &TestDefinition{} // invalid
	err := SaveTestDefinition(def, path)
	if err == nil {
		t.Fatal("expected validation error on save")
	}
	// File should not have been created.
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("file should not exist after failed save")
	}
}

func TestToABTest(t *testing.T) {
	def, err := ParseTestDefinition([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	ab, err := ToABTest(def)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if ab.Name != "claude-vs-gemini" {
		t.Errorf("name = %q", ab.Name)
	}
	if ab.VariantA.Provider != "claude" {
		t.Errorf("variant_a.provider = %q", ab.VariantA.Provider)
	}
	if ab.VariantB.Provider != "gemini" {
		t.Errorf("variant_b.provider = %q", ab.VariantB.Provider)
	}
	if ab.SampleSize != 30 {
		t.Errorf("sample_size = %d", ab.SampleSize)
	}
	if len(ab.Metrics) != 2 {
		t.Fatalf("len(metrics) = %d", len(ab.Metrics))
	}
	// Verify evaluators are non-nil and produce values.
	for _, m := range ab.Metrics {
		if m.Evaluator == nil {
			t.Errorf("metric %q has nil evaluator", m.Name)
			continue
		}
		_ = m.Evaluator("test output")
	}
}

func TestToABTest_InvalidDef(t *testing.T) {
	def := &TestDefinition{}
	_, err := ToABTest(def)
	if err == nil {
		t.Fatal("expected error for invalid def")
	}
}

func TestToABTest_AllMetricTypes(t *testing.T) {
	types := []MetricType{MetricTypeLatency, MetricTypeCost, MetricTypeQuality, MetricTypeSuccessRate}
	for _, mt := range types {
		def := &TestDefinition{
			Name:       "test-" + string(mt),
			VariantA:   VariantDef{Provider: "claude", Model: "m"},
			VariantB:   VariantDef{Provider: "gemini", Model: "m"},
			Metrics:    []MetricSpec{{Name: string(mt), Type: mt}},
			SampleSize: 5,
		}
		ab, err := ToABTest(def)
		if err != nil {
			t.Errorf("type %q: unexpected error: %v", mt, err)
			continue
		}
		if ab.Metrics[0].Name != string(mt) {
			t.Errorf("type %q: metric name = %q", mt, ab.Metrics[0].Name)
		}
	}
}

func TestMetricSpec_EmptyName(t *testing.T) {
	def := &TestDefinition{
		Name:       "test",
		VariantA:   VariantDef{Provider: "claude", Model: "m"},
		VariantB:   VariantDef{Provider: "gemini", Model: "m"},
		Metrics:    []MetricSpec{{Name: "", Type: MetricTypeQuality}},
		SampleSize: 10,
	}
	err := ValidateTestDefinition(def)
	if err == nil || !strings.Contains(err.Error(), "metrics[0].name is required") {
		t.Errorf("expected metric name error, got: %v", err)
	}
}
