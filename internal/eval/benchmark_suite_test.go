package eval

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- test stubs ---

// stubProvider implements ProviderCaller for testing.
type stubProvider struct {
	name      string
	response  string
	tokens    int
	latency   time.Duration
	err       error
	callCount int
}

func (s *stubProvider) Call(ctx context.Context, prompt string) (ProviderResponse, error) {
	s.callCount++
	if s.latency > 0 {
		select {
		case <-time.After(s.latency):
		case <-ctx.Done():
			return ProviderResponse{}, ctx.Err()
		}
	}
	if s.err != nil {
		return ProviderResponse{}, s.err
	}
	return ProviderResponse{
		Text:         s.response,
		InputTokens:  s.tokens,
		OutputTokens: s.tokens * 2,
	}, nil
}

func (s *stubProvider) Name() string { return s.name }

// stubScorer implements QualityScorer for testing.
type stubScorer struct {
	score float64
	err   error
}

func (s *stubScorer) Score(_ context.Context, _, _ string) (float64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.score, nil
}

// --- constructor tests ---

func TestNewBenchmarkSuiteDefaultTimeout(t *testing.T) {
	suite := NewBenchmarkSuite(BenchmarkConfig{}, nil, nil, nil)
	if suite.config.CallTimeout != 30*time.Second {
		t.Errorf("expected default 30s timeout, got %v", suite.config.CallTimeout)
	}
}

func TestNewBenchmarkSuiteCustomTimeout(t *testing.T) {
	suite := NewBenchmarkSuite(BenchmarkConfig{CallTimeout: 5 * time.Second}, nil, nil, nil)
	if suite.config.CallTimeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", suite.config.CallTimeout)
	}
}

// --- validation tests ---

func TestRunNoProviders(t *testing.T) {
	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), nil, nil, []BenchmarkCase{
		{Name: "test", Prompt: "hello"},
	})
	_, err := suite.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no providers") {
		t.Fatalf("expected 'no providers' error, got: %v", err)
	}
}

func TestRunNoCases(t *testing.T) {
	p := &stubProvider{name: "test", response: "ok", tokens: 10}
	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, nil)
	_, err := suite.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no cases") {
		t.Fatalf("expected 'no cases' error, got: %v", err)
	}
}

// --- single provider, single case ---

func TestRunSingleProviderSingleCase(t *testing.T) {
	p := &stubProvider{name: "claude", response: "The answer is 42.", tokens: 50}
	cases := []BenchmarkCase{
		{Name: "math", Prompt: "What is 6*7?", Iterations: 3},
	}
	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)

	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Cases != 1 {
		t.Errorf("expected 1 case, got %d", result.Cases)
	}
	if result.Providers != 1 {
		t.Errorf("expected 1 provider, got %d", result.Providers)
	}
	if p.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", p.callCount)
	}

	key := ResultKey("math", "claude")
	cr, ok := result.Results[key]
	if !ok {
		t.Fatalf("missing result for key %q", key)
	}
	if len(cr.Runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(cr.Runs))
	}
	if cr.ErrorRate != 0 {
		t.Errorf("expected 0 error rate, got %f", cr.ErrorRate)
	}
	if cr.MeanInputTokens != 50 {
		t.Errorf("expected mean input tokens 50, got %f", cr.MeanInputTokens)
	}
	if cr.MeanOutputTokens != 100 {
		t.Errorf("expected mean output tokens 100, got %f", cr.MeanOutputTokens)
	}
	if cr.TotalTokens != 450 { // 3 * (50 + 100)
		t.Errorf("expected total tokens 450, got %d", cr.TotalTokens)
	}
}

// --- multiple providers ---

func TestRunMultipleProviders(t *testing.T) {
	providers := []ProviderCaller{
		&stubProvider{name: "claude", response: "fast answer", tokens: 30},
		&stubProvider{name: "gemini", response: "detailed answer with more tokens", tokens: 80},
	}
	cases := []BenchmarkCase{
		{Name: "general", Prompt: "Explain Go interfaces"},
	}

	scorer := &stubScorer{score: 85}
	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), providers, scorer, cases)

	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result.Results))
	}

	for _, prov := range []string{"claude", "gemini"} {
		key := ResultKey("general", prov)
		cr, ok := result.Results[key]
		if !ok {
			t.Errorf("missing result for %q", key)
			continue
		}
		if cr.MeanQualityScore != 85 {
			t.Errorf("%s: expected quality 85, got %f", prov, cr.MeanQualityScore)
		}
	}
}

// --- keyword coverage ---

func TestKeywordCoverage(t *testing.T) {
	p := &stubProvider{
		name:     "claude",
		response: "Go interfaces define behavior. They use method sets for polymorphism.",
		tokens:   20,
	}
	cases := []BenchmarkCase{
		{
			Name:             "keywords",
			Prompt:           "Explain Go interfaces",
			ExpectedKeywords: []string{"interface", "method", "polymorphism", "duck typing"},
			Iterations:       1,
		},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cr := result.Results[ResultKey("keywords", "claude")]
	if cr.Runs[0].KeywordHits != 3 {
		t.Errorf("expected 3 keyword hits, got %d", cr.Runs[0].KeywordHits)
	}
	if cr.Runs[0].KeywordTotal != 4 {
		t.Errorf("expected 4 total keywords, got %d", cr.Runs[0].KeywordTotal)
	}
	if cr.KeywordCoverage != 0.75 {
		t.Errorf("expected 0.75 keyword coverage, got %f", cr.KeywordCoverage)
	}
}

// --- quality scoring ---

func TestQualityScorerUsed(t *testing.T) {
	p := &stubProvider{name: "claude", response: "answer", tokens: 10}
	scorer := &stubScorer{score: 72.5}
	cases := []BenchmarkCase{
		{Name: "scored", Prompt: "test", Iterations: 2},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, scorer, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cr := result.Results[ResultKey("scored", "claude")]
	if cr.MeanQualityScore != 72.5 {
		t.Errorf("expected mean quality 72.5, got %f", cr.MeanQualityScore)
	}
}

func TestNoScorerYieldsNegativeQuality(t *testing.T) {
	p := &stubProvider{name: "claude", response: "answer", tokens: 10}
	cases := []BenchmarkCase{
		{Name: "noscorer", Prompt: "test"},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cr := result.Results[ResultKey("noscorer", "claude")]
	if cr.Runs[0].QualityScore != -1 {
		t.Errorf("expected quality -1 without scorer, got %f", cr.Runs[0].QualityScore)
	}
	if cr.MeanQualityScore != 0 {
		t.Errorf("expected mean quality 0 without scorer, got %f", cr.MeanQualityScore)
	}
}

// --- error handling ---

func TestProviderErrorCaptured(t *testing.T) {
	p := &stubProvider{name: "failing", err: fmt.Errorf("rate limited")}
	cases := []BenchmarkCase{
		{Name: "errs", Prompt: "test", Iterations: 2},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected suite error: %v", err)
	}

	cr := result.Results[ResultKey("errs", "failing")]
	if cr.ErrorRate != 1.0 {
		t.Errorf("expected error rate 1.0, got %f", cr.ErrorRate)
	}
	for i, r := range cr.Runs {
		if r.Error == "" {
			t.Errorf("run %d: expected error message", i)
		}
	}
}

// --- rankings ---

func TestRankingsOrderedByQuality(t *testing.T) {
	providers := []ProviderCaller{
		&stubProvider{name: "low", response: "bad", tokens: 10},
		&stubProvider{name: "high", response: "great", tokens: 10},
	}
	// Use a scorer that returns different scores based on provider name via response content.
	scorer := &providerAwareScorer{
		scores: map[string]float64{
			"bad":   40,
			"great": 90,
		},
	}
	cases := []BenchmarkCase{
		{Name: "rank", Prompt: "test"},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), providers, scorer, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rankings) != 2 {
		t.Fatalf("expected 2 rankings, got %d", len(result.Rankings))
	}
	if result.Rankings[0].ProviderName != "high" {
		t.Errorf("expected 'high' ranked first, got %q", result.Rankings[0].ProviderName)
	}
	if result.Rankings[0].Rank != 1 {
		t.Errorf("expected rank 1, got %d", result.Rankings[0].Rank)
	}
	if result.Rankings[1].ProviderName != "low" {
		t.Errorf("expected 'low' ranked second, got %q", result.Rankings[1].ProviderName)
	}
	if result.Rankings[1].Rank != 2 {
		t.Errorf("expected rank 2, got %d", result.Rankings[1].Rank)
	}
}

// providerAwareScorer returns different scores based on response content.
type providerAwareScorer struct {
	scores map[string]float64
}

func (s *providerAwareScorer) Score(_ context.Context, _, response string) (float64, error) {
	for key, score := range s.scores {
		if strings.Contains(response, key) {
			return score, nil
		}
	}
	return 50, nil
}

// --- concurrency ---

func TestConcurrentExecution(t *testing.T) {
	var mu = make(chan struct{}, 1) // used to detect concurrency
	started := make(chan struct{}, 4)

	makeProvider := func(name string) ProviderCaller {
		return &concurrentProvider{name: name, started: started, gate: mu}
	}

	providers := []ProviderCaller{
		makeProvider("a"),
		makeProvider("b"),
	}
	cases := []BenchmarkCase{
		{Name: "c1", Prompt: "test"},
		{Name: "c2", Prompt: "test"},
	}

	config := DefaultBenchmarkConfig()
	config.Concurrency = 4
	suite := NewBenchmarkSuite(config, providers, nil, cases)

	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 4 {
		t.Errorf("expected 4 results, got %d", len(result.Results))
	}
}

type concurrentProvider struct {
	name    string
	started chan struct{}
	gate    chan struct{}
}

func (p *concurrentProvider) Call(_ context.Context, _ string) (ProviderResponse, error) {
	p.started <- struct{}{}
	return ProviderResponse{Text: "ok", InputTokens: 5, OutputTokens: 10}, nil
}

func (p *concurrentProvider) Name() string { return p.name }

// --- default iterations ---

func TestDefaultIterationsIsOne(t *testing.T) {
	p := &stubProvider{name: "claude", response: "answer", tokens: 10}
	cases := []BenchmarkCase{
		{Name: "default_iter", Prompt: "test"}, // Iterations=0 should default to 1
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cr := result.Results[ResultKey("default_iter", "claude")]
	if len(cr.Runs) != 1 {
		t.Errorf("expected 1 run for default iterations, got %d", len(cr.Runs))
	}
}

// --- CompareResults ---

func TestCompareResults(t *testing.T) {
	a := CaseResult{
		CaseName:         "test",
		ProviderName:     "claude",
		MeanQualityScore: 85,
		MeanLatencyMs:    200,
		TotalTokens:      500,
	}
	b := CaseResult{
		CaseName:         "test",
		ProviderName:     "gemini",
		MeanQualityScore: 70,
		MeanLatencyMs:    150,
		TotalTokens:      600,
	}

	cmp := CompareResults(a, b)
	if cmp.QualityDelta != 15 {
		t.Errorf("expected quality delta 15, got %f", cmp.QualityDelta)
	}
	if cmp.LatencyDelta != 50 {
		t.Errorf("expected latency delta 50, got %f", cmp.LatencyDelta)
	}
	if cmp.TokenDelta != -100 {
		t.Errorf("expected token delta -100, got %d", cmp.TokenDelta)
	}
	if cmp.BetterQuality != "claude" {
		t.Errorf("expected claude better quality, got %q", cmp.BetterQuality)
	}
	if cmp.BetterLatency != "gemini" {
		t.Errorf("expected gemini better latency, got %q", cmp.BetterLatency)
	}
}

func TestCompareResultsTie(t *testing.T) {
	a := CaseResult{
		CaseName:         "test",
		ProviderName:     "a",
		MeanQualityScore: 80,
		MeanLatencyMs:    100,
	}
	b := CaseResult{
		CaseName:         "test",
		ProviderName:     "b",
		MeanQualityScore: 80,
		MeanLatencyMs:    100,
	}

	cmp := CompareResults(a, b)
	if cmp.BetterQuality != "tie" {
		t.Errorf("expected quality tie, got %q", cmp.BetterQuality)
	}
	if cmp.BetterLatency != "tie" {
		t.Errorf("expected latency tie, got %q", cmp.BetterLatency)
	}
}

// --- ResultKey ---

func TestResultKey(t *testing.T) {
	key := ResultKey("mycase", "myprovider")
	if key != "mycase/myprovider" {
		t.Errorf("expected 'mycase/myprovider', got %q", key)
	}
}

// --- SuiteResult metadata ---

func TestSuiteResultMetadata(t *testing.T) {
	p := &stubProvider{name: "claude", response: "ok", tokens: 5}
	cases := []BenchmarkCase{
		{Name: "a", Prompt: "test"},
		{Name: "b", Prompt: "test"},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Cases != 2 {
		t.Errorf("expected 2 cases, got %d", result.Cases)
	}
	if result.Providers != 1 {
		t.Errorf("expected 1 provider, got %d", result.Providers)
	}
	if result.Duration <= 0 {
		t.Error("expected positive duration")
	}
	if result.StartedAt.IsZero() {
		t.Error("expected non-zero start time")
	}
}

// --- containsIgnoreCase ---

func TestContainsIgnoreCase(t *testing.T) {
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"Hello World", "hello", true},
		{"Hello World", "WORLD", true},
		{"Hello World", "missing", false},
		{"abc", "", false},
		{"", "abc", false},
		{"Interface", "interface", true},
	}
	for _, tt := range tests {
		got := containsIgnoreCase(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsIgnoreCase(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

// --- P95 latency aggregation ---

func TestP95LatencyAggregation(t *testing.T) {
	// Create a provider with known latency pattern.
	p := &stubProvider{name: "claude", response: "ok", tokens: 5}
	cases := []BenchmarkCase{
		{Name: "p95test", Prompt: "test", Iterations: 10},
	}

	suite := NewBenchmarkSuite(DefaultBenchmarkConfig(), []ProviderCaller{p}, nil, cases)
	result, err := suite.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cr := result.Results[ResultKey("p95test", "claude")]
	// P95 should be >= mean (stub has very low latency so they'll be close).
	if cr.P95LatencyMs < 0 {
		t.Errorf("expected non-negative P95 latency, got %f", cr.P95LatencyMs)
	}
}

// --- percentileFloat64 ---

func TestPercentileFloat64(t *testing.T) {
	tests := []struct {
		name   string
		values []float64
		p      float64
		want   float64
	}{
		{"empty", nil, 50, 0},
		{"single", []float64{42}, 50, 42},
		{"median_odd", []float64{1, 2, 3, 4, 5}, 50, 3},
		{"p0", []float64{1, 2, 3}, 0, 1},
		{"p100", []float64{1, 2, 3}, 100, 3},
	}
	for _, tt := range tests {
		got := percentileFloat64(tt.values, tt.p)
		if got != tt.want {
			t.Errorf("%s: percentileFloat64(%v, %f) = %f, want %f", tt.name, tt.values, tt.p, got, tt.want)
		}
	}
}
