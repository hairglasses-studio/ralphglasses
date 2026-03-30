package eval

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestNewRunner_Defaults(t *testing.T) {
	test := ABTest{
		Name:     "defaults-test",
		VariantA: Config{Prompt: "hello"},
		VariantB: Config{Prompt: "world"},
		Metrics:  []MetricDef{MetricOutputLength()},
	}
	r := NewRunner(test, nil)
	if r.test.SampleSize != 30 {
		t.Errorf("expected default SampleSize 30, got %d", r.test.SampleSize)
	}
}

func TestRun_NoMetrics(t *testing.T) {
	test := ABTest{
		Name:       "no-metrics",
		VariantA:   Config{Prompt: "a"},
		VariantB:   Config{Prompt: "b"},
		SampleSize: 5,
	}
	r := NewRunner(test, nil)
	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for empty metrics")
	}
	if !strings.Contains(err.Error(), "at least one metric") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRun_BasicExecution(t *testing.T) {
	// Variant A returns short output, B returns long output.
	executor := func(_ context.Context, cfg Config) (string, error) {
		if cfg.Prompt == "short" {
			return "hi", nil
		}
		return "this is a much longer response with many words", nil
	}

	test := ABTest{
		Name:       "length-test",
		VariantA:   Config{Prompt: "short"},
		VariantB:   Config{Prompt: "long"},
		Metrics:    []MetricDef{MetricOutputLength()},
		SampleSize: 10,
	}

	r := NewRunner(test, executor)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TestName != "length-test" {
		t.Errorf("expected test name %q, got %q", "length-test", result.TestName)
	}
	if result.SampleSize != 10 {
		t.Errorf("expected sample size 10, got %d", result.SampleSize)
	}

	aLen := result.VariantAMetrics["output_length"]
	bLen := result.VariantBMetrics["output_length"]

	if aLen >= bLen {
		t.Errorf("expected variant A (short) < variant B (long): A=%f B=%f", aLen, bLen)
	}
	if result.Winner != "B" {
		t.Errorf("expected winner B, got %q", result.Winner)
	}
}

func TestRun_ExecutorError(t *testing.T) {
	executor := func(_ context.Context, cfg Config) (string, error) {
		if cfg.Prompt == "fail" {
			return "", fmt.Errorf("simulated failure")
		}
		return "ok", nil
	}

	test := ABTest{
		Name:       "error-test",
		VariantA:   Config{Prompt: "fail"},
		VariantB:   Config{Prompt: "ok"},
		Metrics:    []MetricDef{MetricOutputLength()},
		SampleSize: 5,
	}

	r := NewRunner(test, executor)
	_, err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from failing executor")
	}
	if !strings.Contains(err.Error(), "variant A") {
		t.Errorf("expected error to mention variant A: %v", err)
	}
}

func TestRun_MultipleMetrics(t *testing.T) {
	executor := func(_ context.Context, cfg Config) (string, error) {
		return cfg.Prompt, nil
	}

	test := ABTest{
		Name:     "multi-metric",
		VariantA: Config{Prompt: "one two three"},
		VariantB: Config{Prompt: "alpha beta gamma delta epsilon"},
		Metrics: []MetricDef{
			MetricOutputLength(),
			MetricTokenCount(),
		},
		SampleSize: 10,
	}

	r := NewRunner(test, executor)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both metrics should favor B (longer string, more tokens).
	if result.Winner != "B" {
		t.Errorf("expected winner B, got %q", result.Winner)
	}
	if _, ok := result.VariantAMetrics["output_length"]; !ok {
		t.Error("missing output_length metric for A")
	}
	if _, ok := result.VariantAMetrics["token_count"]; !ok {
		t.Error("missing token_count metric for A")
	}
}

func TestRun_Tie(t *testing.T) {
	executor := func(_ context.Context, cfg Config) (string, error) {
		return "same output", nil
	}

	test := ABTest{
		Name:       "tie-test",
		VariantA:   Config{Prompt: "a"},
		VariantB:   Config{Prompt: "b"},
		Metrics:    []MetricDef{MetricOutputLength()},
		SampleSize: 5,
	}

	r := NewRunner(test, executor)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner != "tie" {
		t.Errorf("expected tie, got %q", result.Winner)
	}
}

// --- t-test statistical tests ---

func TestTTestConfidence_IdenticalSamples(t *testing.T) {
	a := []float64{5, 5, 5, 5, 5}
	b := []float64{5, 5, 5, 5, 5}
	conf := TTestConfidence(a, b)
	if conf != 0 {
		t.Errorf("expected 0 confidence for identical constant samples, got %f", conf)
	}
}

func TestTTestConfidence_ClearDifference(t *testing.T) {
	// Two clearly different distributions.
	a := make([]float64, 50)
	b := make([]float64, 50)
	for i := range 50 {
		a[i] = 10 + float64(i%5)*0.1
		b[i] = 20 + float64(i%5)*0.1
	}
	conf := TTestConfidence(a, b)
	if conf < 0.95 {
		t.Errorf("expected high confidence (>0.95), got %f", conf)
	}
}

func TestTTestConfidence_OverlappingSamples(t *testing.T) {
	// Overlapping distributions should yield lower confidence.
	a := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	b := []float64{2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	conf := TTestConfidence(a, b)
	// Should be between 0 and 1, and relatively low.
	if conf < 0 || conf > 1 {
		t.Errorf("confidence out of range: %f", conf)
	}
	if conf > 0.95 {
		t.Errorf("expected low confidence for overlapping samples, got %f", conf)
	}
}

func TestTTestConfidence_InsufficientSamples(t *testing.T) {
	a := []float64{1}
	b := []float64{2, 3, 4}
	conf := TTestConfidence(a, b)
	if conf != 0 {
		t.Errorf("expected 0 for single-element sample, got %f", conf)
	}
}

func TestTTestConfidence_EmptySamples(t *testing.T) {
	conf := TTestConfidence(nil, nil)
	if conf != 0 {
		t.Errorf("expected 0 for nil samples, got %f", conf)
	}
}

// --- Helper function tests ---

func TestMean(t *testing.T) {
	tests := []struct {
		vals []float64
		want float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3, 4, 5}, 3},
		{[]float64{-1, 1}, 0},
	}
	for _, tc := range tests {
		got := mean(tc.vals)
		if math.Abs(got-tc.want) > 1e-10 {
			t.Errorf("mean(%v) = %f, want %f", tc.vals, got, tc.want)
		}
	}
}

func TestVariance(t *testing.T) {
	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	m := mean(vals)
	v := variance(vals, m)
	// Known sample variance for this data: 4.571...
	expected := 4.571428571428571
	if math.Abs(v-expected) > 1e-6 {
		t.Errorf("variance = %f, want ~%f", v, expected)
	}
}

func TestVariance_SingleElement(t *testing.T) {
	v := variance([]float64{42}, 42)
	if v != 0 {
		t.Errorf("expected 0 variance for single element, got %f", v)
	}
}

// --- Built-in metric tests ---

func TestMetricTokenCount(t *testing.T) {
	m := MetricTokenCount()
	if m.Name != "token_count" {
		t.Errorf("expected name %q, got %q", "token_count", m.Name)
	}
	score := m.Evaluator("hello world foo bar")
	if score != 4 {
		t.Errorf("expected 4 tokens, got %f", score)
	}
}

func TestMetricTokenCount_Empty(t *testing.T) {
	m := MetricTokenCount()
	score := m.Evaluator("")
	if score != 0 {
		t.Errorf("expected 0 tokens for empty string, got %f", score)
	}
}

func TestMetricOutputLength(t *testing.T) {
	m := MetricOutputLength()
	if m.Name != "output_length" {
		t.Errorf("expected name %q, got %q", "output_length", m.Name)
	}
	score := m.Evaluator("hello")
	if score != 5 {
		t.Errorf("expected length 5, got %f", score)
	}
}

func TestMetricQualityScore(t *testing.T) {
	m := MetricQualityScore()
	if m.Name != "quality_score" {
		t.Errorf("expected name %q, got %q", "quality_score", m.Name)
	}

	// Empty string should score 0.
	if s := m.Evaluator(""); s != 0 {
		t.Errorf("expected 0 for empty, got %f", s)
	}

	// A reasonably well-formed paragraph should score > 0.
	paragraph := "The quick brown fox jumps over the lazy dog. " +
		"This sentence provides additional context and detail. " +
		"A diverse vocabulary helps improve the quality score."
	score := m.Evaluator(paragraph)
	if score <= 0 || score > 1 {
		t.Errorf("expected score in (0,1], got %f", score)
	}
}

func TestMetricQualityScore_SingleWord(t *testing.T) {
	m := MetricQualityScore()
	score := m.Evaluator("word")
	if score < 0 || score > 1 {
		t.Errorf("expected score in [0,1], got %f", score)
	}
}

// --- Regularized incomplete beta / p-value tests ---

func TestTTestPValue_ZeroDF(t *testing.T) {
	p := tTestPValue(2.0, 0)
	if p != 1 {
		t.Errorf("expected p=1 for df=0, got %f", p)
	}
}

func TestRegIncBeta_Boundaries(t *testing.T) {
	if v := regIncBeta(1, 1, 0); v != 0 {
		t.Errorf("expected 0 at x=0, got %f", v)
	}
	if v := regIncBeta(1, 1, 1); v != 1 {
		t.Errorf("expected 1 at x=1, got %f", v)
	}
}

func TestRegIncBeta_Symmetry(t *testing.T) {
	// I_x(a,b) + I_{1-x}(b,a) = 1
	a, b, x := 2.0, 3.0, 0.3
	v1 := regIncBeta(a, b, x)
	v2 := regIncBeta(b, a, 1-x)
	sum := v1 + v2
	if math.Abs(sum-1) > 1e-8 {
		t.Errorf("symmetry violated: I_x(a,b) + I_{1-x}(b,a) = %f, want 1", sum)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	executor := func(ctx context.Context, cfg Config) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "ok", nil
	}

	test := ABTest{
		Name:       "cancel-test",
		VariantA:   Config{Prompt: "a"},
		VariantB:   Config{Prompt: "b"},
		Metrics:    []MetricDef{MetricOutputLength()},
		SampleSize: 5,
	}

	r := NewRunner(test, executor)
	_, err := r.Run(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestRun_CustomMetric(t *testing.T) {
	// Custom metric: count occurrences of "x".
	xCount := MetricDef{
		Name: "x_count",
		Evaluator: func(output string) float64 {
			return float64(strings.Count(output, "x"))
		},
	}

	executor := func(_ context.Context, cfg Config) (string, error) {
		return cfg.Prompt, nil
	}

	test := ABTest{
		Name:       "custom-metric",
		VariantA:   Config{Prompt: "xxx"},
		VariantB:   Config{Prompt: "x"},
		Metrics:    []MetricDef{xCount},
		SampleSize: 10,
	}

	r := NewRunner(test, executor)
	result, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.VariantAMetrics["x_count"] != 3 {
		t.Errorf("expected 3 x's for A, got %f", result.VariantAMetrics["x_count"])
	}
	if result.VariantBMetrics["x_count"] != 1 {
		t.Errorf("expected 1 x for B, got %f", result.VariantBMetrics["x_count"])
	}
	if result.Winner != "A" {
		t.Errorf("expected winner A, got %q", result.Winner)
	}
}
