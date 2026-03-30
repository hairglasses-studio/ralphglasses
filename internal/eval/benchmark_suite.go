package eval

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ProviderCaller abstracts calling an LLM provider with a prompt.
// Implementations wrap real API clients; tests use stubs.
type ProviderCaller interface {
	// Call sends the prompt to the provider and returns the response text,
	// token counts, and any error.
	Call(ctx context.Context, prompt string) (ProviderResponse, error)

	// Name returns the provider identifier (e.g. "claude", "gemini", "openai").
	Name() string
}

// ProviderResponse holds the raw output from a single provider call.
type ProviderResponse struct {
	Text        string `json:"text"`
	InputTokens int    `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
}

// QualityScorer evaluates response quality, returning a 0-100 score.
// Implementations can use deterministic heuristics or LLM-as-judge.
type QualityScorer interface {
	Score(ctx context.Context, prompt, response string) (float64, error)
}

// BenchmarkCase defines a single prompt to benchmark across providers.
type BenchmarkCase struct {
	// Name identifies this case in results.
	Name string `json:"name"`

	// Prompt is the input text sent to each provider.
	Prompt string `json:"prompt"`

	// ExpectedKeywords are optional substrings the response should contain.
	// Each matched keyword contributes to a keyword coverage metric.
	ExpectedKeywords []string `json:"expected_keywords,omitempty"`

	// Iterations is how many times to run this case per provider for
	// statistical stability. Defaults to 1 if zero.
	Iterations int `json:"iterations,omitempty"`
}

// BenchmarkConfig controls suite execution.
type BenchmarkConfig struct {
	// Timeout per individual provider call.
	CallTimeout time.Duration `json:"call_timeout"`

	// Concurrency is the max number of parallel provider calls.
	// Zero means sequential execution.
	Concurrency int `json:"concurrency"`
}

// DefaultBenchmarkConfig returns sensible defaults.
func DefaultBenchmarkConfig() BenchmarkConfig {
	return BenchmarkConfig{
		CallTimeout: 30 * time.Second,
		Concurrency: 1,
	}
}

// RunMetrics captures the measurements from a single benchmark run
// (one case, one provider, one iteration).
type RunMetrics struct {
	LatencyMs    int64   `json:"latency_ms"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	QualityScore float64 `json:"quality_score"` // 0-100, -1 if scorer is nil
	KeywordHits  int     `json:"keyword_hits"`
	KeywordTotal int     `json:"keyword_total"`
	Error        string  `json:"error,omitempty"`
}

// CaseResult aggregates all iterations for one case + one provider.
type CaseResult struct {
	CaseName     string       `json:"case_name"`
	ProviderName string       `json:"provider_name"`
	Runs         []RunMetrics `json:"runs"`

	// Aggregates computed from Runs.
	MeanLatencyMs    float64 `json:"mean_latency_ms"`
	P95LatencyMs     float64 `json:"p95_latency_ms"`
	MeanQualityScore float64 `json:"mean_quality_score"`
	MeanInputTokens  float64 `json:"mean_input_tokens"`
	MeanOutputTokens float64 `json:"mean_output_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	ErrorRate        float64 `json:"error_rate"` // fraction of runs with errors
	KeywordCoverage  float64 `json:"keyword_coverage"` // fraction of keywords hit (best run)
}

// SuiteResult is the top-level output from running a full benchmark suite.
type SuiteResult struct {
	StartedAt time.Time              `json:"started_at"`
	Duration  time.Duration          `json:"duration"`
	Cases     int                    `json:"cases"`
	Providers int                    `json:"providers"`
	Results   map[string]CaseResult  `json:"results"` // key: "caseName/providerName"
	Rankings  []ProviderRanking      `json:"rankings"`
}

// ProviderRanking summarizes a provider's performance across all cases.
type ProviderRanking struct {
	ProviderName     string  `json:"provider_name"`
	MeanQualityScore float64 `json:"mean_quality_score"`
	MeanLatencyMs    float64 `json:"mean_latency_ms"`
	TotalTokens      int     `json:"total_tokens"`
	ErrorRate        float64 `json:"error_rate"`
	Rank             int     `json:"rank"` // 1-based, by quality score descending
}

// BenchmarkSuite orchestrates running benchmark cases against multiple providers.
type BenchmarkSuite struct {
	config    BenchmarkConfig
	providers []ProviderCaller
	scorer    QualityScorer // may be nil
	cases     []BenchmarkCase
}

// NewBenchmarkSuite creates a suite. Scorer may be nil (quality scoring skipped).
func NewBenchmarkSuite(config BenchmarkConfig, providers []ProviderCaller, scorer QualityScorer, cases []BenchmarkCase) *BenchmarkSuite {
	if config.CallTimeout == 0 {
		config.CallTimeout = DefaultBenchmarkConfig().CallTimeout
	}
	return &BenchmarkSuite{
		config:    config,
		providers: providers,
		scorer:    scorer,
		cases:     cases,
	}
}

// Run executes all benchmark cases against all providers and returns aggregated results.
func (s *BenchmarkSuite) Run(ctx context.Context) (*SuiteResult, error) {
	if len(s.providers) == 0 {
		return nil, fmt.Errorf("benchmark suite: no providers configured")
	}
	if len(s.cases) == 0 {
		return nil, fmt.Errorf("benchmark suite: no cases configured")
	}

	startedAt := time.Now()
	results := make(map[string]CaseResult)
	var mu sync.Mutex

	// Build work items.
	type workItem struct {
		bc       BenchmarkCase
		provider ProviderCaller
	}
	var items []workItem
	for _, bc := range s.cases {
		for _, p := range s.providers {
			items = append(items, workItem{bc: bc, provider: p})
		}
	}

	// Execute with concurrency control.
	concurrency := s.config.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var firstErr error

	for _, item := range items {
		wg.Add(1)
		go func(wi workItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cr := s.runCase(ctx, wi.bc, wi.provider)
			key := resultKey(wi.bc.Name, wi.provider.Name())

			mu.Lock()
			results[key] = cr
			mu.Unlock()
		}(item)
	}

	wg.Wait()

	result := &SuiteResult{
		StartedAt: startedAt,
		Duration:  time.Since(startedAt),
		Cases:     len(s.cases),
		Providers: len(s.providers),
		Results:   results,
		Rankings:  s.computeRankings(results),
	}

	return result, firstErr
}

// runCase executes all iterations of a single case against a single provider.
func (s *BenchmarkSuite) runCase(ctx context.Context, bc BenchmarkCase, provider ProviderCaller) CaseResult {
	iterations := bc.Iterations
	if iterations <= 0 {
		iterations = 1
	}

	runs := make([]RunMetrics, 0, iterations)
	for i := 0; i < iterations; i++ {
		run := s.runSingle(ctx, bc, provider)
		runs = append(runs, run)
	}

	cr := CaseResult{
		CaseName:     bc.Name,
		ProviderName: provider.Name(),
		Runs:         runs,
	}
	computeAggregates(&cr)
	return cr
}

// runSingle executes one iteration of a case against a provider.
func (s *BenchmarkSuite) runSingle(ctx context.Context, bc BenchmarkCase, provider ProviderCaller) RunMetrics {
	callCtx, cancel := context.WithTimeout(ctx, s.config.CallTimeout)
	defer cancel()

	start := time.Now()
	resp, err := provider.Call(callCtx, bc.Prompt)
	latency := time.Since(start).Milliseconds()

	rm := RunMetrics{
		LatencyMs:    latency,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		QualityScore: -1,
		KeywordTotal: len(bc.ExpectedKeywords),
	}

	if err != nil {
		rm.Error = err.Error()
		return rm
	}

	// Keyword coverage.
	for _, kw := range bc.ExpectedKeywords {
		if containsIgnoreCase(resp.Text, kw) {
			rm.KeywordHits++
		}
	}

	// Quality scoring (optional).
	if s.scorer != nil {
		score, scoreErr := s.scorer.Score(ctx, bc.Prompt, resp.Text)
		if scoreErr == nil {
			rm.QualityScore = score
		}
	}

	return rm
}

// computeAggregates fills in the summary fields on a CaseResult from its Runs.
func computeAggregates(cr *CaseResult) {
	n := len(cr.Runs)
	if n == 0 {
		return
	}

	var (
		sumLat     float64
		sumQuality float64
		qualityN   int
		sumIn      float64
		sumOut     float64
		totalTok   int
		errors     int
		bestKWHit  int
	)

	latencies := make([]float64, 0, n)

	for _, r := range cr.Runs {
		sumLat += float64(r.LatencyMs)
		latencies = append(latencies, float64(r.LatencyMs))
		sumIn += float64(r.InputTokens)
		sumOut += float64(r.OutputTokens)
		totalTok += r.InputTokens + r.OutputTokens
		if r.Error != "" {
			errors++
		}
		if r.QualityScore >= 0 {
			sumQuality += r.QualityScore
			qualityN++
		}
		if r.KeywordHits > bestKWHit {
			bestKWHit = r.KeywordHits
		}
	}

	cr.MeanLatencyMs = sumLat / float64(n)
	cr.MeanInputTokens = sumIn / float64(n)
	cr.MeanOutputTokens = sumOut / float64(n)
	cr.TotalTokens = totalTok
	cr.ErrorRate = float64(errors) / float64(n)

	if qualityN > 0 {
		cr.MeanQualityScore = sumQuality / float64(qualityN)
	}

	if cr.Runs[0].KeywordTotal > 0 {
		cr.KeywordCoverage = float64(bestKWHit) / float64(cr.Runs[0].KeywordTotal)
	}

	// P95 latency.
	sort.Float64s(latencies)
	cr.P95LatencyMs = percentileFloat64(latencies, 95)
}

// computeRankings aggregates per-provider stats across all cases and ranks by quality.
func (s *BenchmarkSuite) computeRankings(results map[string]CaseResult) []ProviderRanking {
	type accum struct {
		sumQuality float64
		qualityN   int
		sumLatency float64
		latencyN   int
		totalTok   int
		errors     int
		totalRuns  int
	}

	byProvider := make(map[string]*accum)
	for _, cr := range results {
		a, ok := byProvider[cr.ProviderName]
		if !ok {
			a = &accum{}
			byProvider[cr.ProviderName] = a
		}
		for _, r := range cr.Runs {
			a.sumLatency += float64(r.LatencyMs)
			a.latencyN++
			a.totalTok += r.InputTokens + r.OutputTokens
			a.totalRuns++
			if r.Error != "" {
				a.errors++
			}
			if r.QualityScore >= 0 {
				a.sumQuality += r.QualityScore
				a.qualityN++
			}
		}
	}

	rankings := make([]ProviderRanking, 0, len(byProvider))
	for name, a := range byProvider {
		pr := ProviderRanking{
			ProviderName: name,
			TotalTokens:  a.totalTok,
		}
		if a.qualityN > 0 {
			pr.MeanQualityScore = a.sumQuality / float64(a.qualityN)
		}
		if a.latencyN > 0 {
			pr.MeanLatencyMs = a.sumLatency / float64(a.latencyN)
		}
		if a.totalRuns > 0 {
			pr.ErrorRate = float64(a.errors) / float64(a.totalRuns)
		}
		rankings = append(rankings, pr)
	}

	// Sort by quality score descending, then latency ascending as tiebreaker.
	sort.Slice(rankings, func(i, j int) bool {
		if rankings[i].MeanQualityScore != rankings[j].MeanQualityScore {
			return rankings[i].MeanQualityScore > rankings[j].MeanQualityScore
		}
		return rankings[i].MeanLatencyMs < rankings[j].MeanLatencyMs
	})

	for i := range rankings {
		rankings[i].Rank = i + 1
	}

	return rankings
}

// resultKey builds the map key for a case+provider combination.
func resultKey(caseName, providerName string) string {
	return caseName + "/" + providerName
}

// ResultKey is the exported version for test access.
func ResultKey(caseName, providerName string) string {
	return resultKey(caseName, providerName)
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	sLower := toLower(s)
	subLower := toLower(substr)
	return len(subLower) > 0 && indexOf(sLower, subLower) >= 0
}

// toLower is a simple ASCII-aware lowercase.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// indexOf returns the index of substr in s, or -1 if not found.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// percentileFloat64 computes the p-th percentile from a pre-sorted slice
// using linear interpolation.
func percentileFloat64(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p / 100.0 * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	weight := idx - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// CompareResults compares two CaseResults (e.g. same case, different providers)
// and returns which is better, the quality delta, and latency delta.
func CompareResults(a, b CaseResult) ResultComparison {
	return ResultComparison{
		CaseName:      a.CaseName,
		ProviderA:     a.ProviderName,
		ProviderB:     b.ProviderName,
		QualityDelta:  a.MeanQualityScore - b.MeanQualityScore,
		LatencyDelta:  a.MeanLatencyMs - b.MeanLatencyMs,
		TokenDelta:    a.TotalTokens - b.TotalTokens,
		BetterQuality: betterOf(a.ProviderName, b.ProviderName, a.MeanQualityScore, b.MeanQualityScore, true),
		BetterLatency: betterOf(a.ProviderName, b.ProviderName, a.MeanLatencyMs, b.MeanLatencyMs, false),
	}
}

// ResultComparison summarizes the difference between two case results.
type ResultComparison struct {
	CaseName      string  `json:"case_name"`
	ProviderA     string  `json:"provider_a"`
	ProviderB     string  `json:"provider_b"`
	QualityDelta  float64 `json:"quality_delta"`  // positive = A is better
	LatencyDelta  float64 `json:"latency_delta"`  // negative = A is faster
	TokenDelta    int     `json:"token_delta"`     // negative = A uses fewer tokens
	BetterQuality string  `json:"better_quality"`  // provider name
	BetterLatency string  `json:"better_latency"`  // provider name
}

// betterOf returns which provider is better for a given metric.
// higherIsBetter controls the comparison direction.
func betterOf(nameA, nameB string, valA, valB float64, higherIsBetter bool) string {
	if valA == valB {
		return "tie"
	}
	if higherIsBetter {
		if valA > valB {
			return nameA
		}
		return nameB
	}
	if valA < valB {
		return nameA
	}
	return nameB
}
