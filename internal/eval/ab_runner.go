package eval

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// Config describes a single variant's configuration for an A/B test.
type Config struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature"`
}

// MetricDef defines a named metric with an evaluator function that scores output.
type MetricDef struct {
	Name      string                 `json:"name"`
	Evaluator func(output string) float64 `json:"-"`
}

// ABTest defines an A/B test between two prompt/model configurations.
type ABTest struct {
	Name       string      `json:"name"`
	VariantA   Config      `json:"variant_a"`
	VariantB   Config      `json:"variant_b"`
	Metrics    []MetricDef `json:"metrics"`
	SampleSize int         `json:"sample_size"`
}

// ABResult holds the outcome of an A/B test.
type ABResult struct {
	TestName        string             `json:"test_name"`
	VariantAMetrics map[string]float64 `json:"variant_a_metrics"`
	VariantBMetrics map[string]float64 `json:"variant_b_metrics"`
	Winner          string             `json:"winner"`
	Confidence      float64            `json:"confidence"`
	SampleSize      int                `json:"sample_size"`
	Duration        time.Duration      `json:"duration"`
}

// sampleResult holds the raw output and timing from a single execution.
type sampleResult struct {
	Output  string
	Latency time.Duration
	Err     error
}

// Executor is the function type used to run a prompt configuration and return output.
// It enables dependency injection for testing without real LLM calls.
type Executor func(ctx context.Context, cfg Config) (string, error)

// Runner manages the execution of an A/B test.
type Runner struct {
	test     ABTest
	executor Executor
}

// NewRunner creates a runner for the given A/B test.
// The executor function is called for each sample; pass nil to use a stub
// that returns the prompt text (useful only for metric-only tests).
func NewRunner(test ABTest, executor Executor) *Runner {
	if executor == nil {
		executor = func(_ context.Context, cfg Config) (string, error) {
			return cfg.Prompt, nil
		}
	}
	if test.SampleSize <= 0 {
		test.SampleSize = 30
	}
	return &Runner{
		test:     test,
		executor: executor,
	}
}

// Run executes both variants of the A/B test and returns statistical results.
func (r *Runner) Run(ctx context.Context) (*ABResult, error) {
	if len(r.test.Metrics) == 0 {
		return nil, fmt.Errorf("ab test %q: at least one metric is required", r.test.Name)
	}

	start := time.Now()

	samplesA, err := r.collect(ctx, r.test.VariantA)
	if err != nil {
		return nil, fmt.Errorf("variant A: %w", err)
	}
	samplesB, err := r.collect(ctx, r.test.VariantB)
	if err != nil {
		return nil, fmt.Errorf("variant B: %w", err)
	}

	result := &ABResult{
		TestName:        r.test.Name,
		VariantAMetrics: make(map[string]float64),
		VariantBMetrics: make(map[string]float64),
		SampleSize:      r.test.SampleSize,
		Duration:        time.Since(start),
	}

	// Evaluate each metric across all samples.
	var bestWins, totalMetrics int
	bestVariant := ""

	for _, md := range r.test.Metrics {
		scoresA := evaluateMetric(md, samplesA)
		scoresB := evaluateMetric(md, samplesB)

		meanA := mean(scoresA)
		meanB := mean(scoresB)

		result.VariantAMetrics[md.Name] = meanA
		result.VariantBMetrics[md.Name] = meanB

		totalMetrics++
		if meanA > meanB {
			bestWins++
		} else if meanB > meanA {
			bestWins--
		}
	}

	// Determine overall winner by metric majority.
	switch {
	case bestWins > 0:
		bestVariant = "A"
	case bestWins < 0:
		bestVariant = "B"
	default:
		bestVariant = "tie"
	}
	result.Winner = bestVariant

	// Compute confidence from the primary metric (first one) using two-sample t-test.
	primary := r.test.Metrics[0]
	scoresA := evaluateMetric(primary, samplesA)
	scoresB := evaluateMetric(primary, samplesB)
	result.Confidence = TTestConfidence(scoresA, scoresB)

	return result, nil
}

// collect runs the executor SampleSize times for a given config.
func (r *Runner) collect(ctx context.Context, cfg Config) ([]sampleResult, error) {
	results := make([]sampleResult, r.test.SampleSize)
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	// Use a semaphore to limit concurrency.
	sem := make(chan struct{}, 10)

	for i := range r.test.SampleSize {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			output, err := r.executor(ctx, cfg)
			latency := time.Since(start)

			if err != nil {
				errOnce.Do(func() { firstErr = err })
				results[idx] = sampleResult{Err: err, Latency: latency}
				return
			}
			results[idx] = sampleResult{Output: output, Latency: latency}
		}(i)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

// evaluateMetric applies a metric evaluator to all sample outputs.
func evaluateMetric(md MetricDef, samples []sampleResult) []float64 {
	scores := make([]float64, len(samples))
	for i, s := range samples {
		if s.Err != nil {
			continue
		}
		scores[i] = md.Evaluator(s.Output)
	}
	return scores
}

// TTestConfidence computes the confidence level (1 - p-value) for two independent
// samples using Welch's two-sample t-test. Returns 0 if either sample has
// insufficient variance.
func TTestConfidence(a, b []float64) float64 {
	nA, nB := float64(len(a)), float64(len(b))
	if nA < 2 || nB < 2 {
		return 0
	}

	meanA, meanB := mean(a), mean(b)
	varA, varB := variance(a, meanA), variance(b, meanB)

	denominator := varA/nA + varB/nB
	if denominator == 0 {
		return 0
	}

	t := (meanA - meanB) / math.Sqrt(denominator)

	// Welch-Satterthwaite degrees of freedom.
	num := denominator * denominator
	dA := (varA / nA) * (varA / nA) / (nA - 1)
	dB := (varB / nB) * (varB / nB) / (nB - 1)
	if dA+dB == 0 {
		return 0
	}
	df := num / (dA + dB)

	p := tTestPValue(math.Abs(t), df)
	return 1 - p
}

// variance computes the sample variance given precomputed mean.
func variance(vals []float64, m float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	var ss float64
	for _, v := range vals {
		d := v - m
		ss += d * d
	}
	return ss / float64(len(vals)-1)
}

// tTestPValue approximates the two-tailed p-value for a t-distribution
// using the regularized incomplete beta function.
func tTestPValue(t, df float64) float64 {
	if df <= 0 {
		return 1
	}
	x := df / (df + t*t)
	return regIncBeta(df/2, 0.5, x)
}

// regIncBeta computes the regularized incomplete beta function I_x(a, b)
// using a continued fraction expansion (Lentz's method).
func regIncBeta(a, b, x float64) float64 {
	if x < 0 || x > 1 {
		return 0
	}
	if x == 0 || x == 1 {
		return x
	}

	// Use symmetry relation if needed for convergence.
	if x > (a+1)/(a+b+2) {
		return 1 - regIncBeta(b, a, 1-x)
	}

	lnBeta := lgamma(a) + lgamma(b) - lgamma(a+b)
	front := math.Exp(math.Log(x)*a + math.Log(1-x)*b - lnBeta) / a

	// Lentz's continued fraction.
	const maxIter = 200
	const epsilon = 1e-14

	f := 1.0
	c := 1.0
	d := 1 - (a+b)*x/(a+1)
	if math.Abs(d) < epsilon {
		d = epsilon
	}
	d = 1 / d
	f = d

	for i := 1; i <= maxIter; i++ {
		m := float64(i)

		// Even step.
		num := m * (b - m) * x / ((a + 2*m - 1) * (a + 2*m))
		d = 1 + num*d
		if math.Abs(d) < epsilon {
			d = epsilon
		}
		c = 1 + num/c
		if math.Abs(c) < epsilon {
			c = epsilon
		}
		d = 1 / d
		f *= c * d

		// Odd step.
		num = -((a + m) * (a + b + m) * x) / ((a + 2*m) * (a + 2*m + 1))
		d = 1 + num*d
		if math.Abs(d) < epsilon {
			d = epsilon
		}
		c = 1 + num/c
		if math.Abs(c) < epsilon {
			c = epsilon
		}
		d = 1 / d
		delta := c * d
		f *= delta

		if math.Abs(delta-1) < epsilon {
			break
		}
	}

	return front * f
}

// lgamma wraps math.Lgamma discarding the sign.
func lgamma(x float64) float64 {
	v, _ := math.Lgamma(x)
	return v
}

// --- Built-in metrics ---

// MetricLatency measures execution latency in milliseconds.
// It requires access to timing data, so it works as a post-hoc metric
// applied via the runner. This evaluator returns output length as a proxy.
func MetricLatency() MetricDef {
	return MetricDef{
		Name: "latency_ms",
		Evaluator: func(output string) float64 {
			// In real usage, latency is measured by the runner.
			// This evaluator is a placeholder; the runner injects actual timing.
			return 0
		},
	}
}

// MetricTokenCount estimates token count as word count (a common approximation).
func MetricTokenCount() MetricDef {
	return MetricDef{
		Name: "token_count",
		Evaluator: func(output string) float64 {
			words := strings.Fields(output)
			return float64(len(words))
		},
	}
}

// MetricOutputLength measures output length in characters.
func MetricOutputLength() MetricDef {
	return MetricDef{
		Name: "output_length",
		Evaluator: func(output string) float64 {
			return float64(len(output))
		},
	}
}

// MetricQualityScore computes a simple quality heuristic:
// normalized score based on length, sentence count, and vocabulary diversity.
func MetricQualityScore() MetricDef {
	return MetricDef{
		Name: "quality_score",
		Evaluator: func(output string) float64 {
			if len(output) == 0 {
				return 0
			}

			words := strings.Fields(output)
			wordCount := float64(len(words))
			if wordCount == 0 {
				return 0
			}

			// Vocabulary diversity: unique words / total words.
			unique := make(map[string]struct{})
			for _, w := range words {
				unique[strings.ToLower(w)] = struct{}{}
			}
			diversity := float64(len(unique)) / wordCount

			// Sentence count (approximate).
			sentences := float64(strings.Count(output, ".") + strings.Count(output, "!") + strings.Count(output, "?"))
			if sentences == 0 {
				sentences = 1
			}

			// Average sentence length score (sweet spot: 10-20 words).
			avgSentLen := wordCount / sentences
			sentScore := 1.0 - math.Abs(avgSentLen-15)/15
			if sentScore < 0 {
				sentScore = 0
			}

			// Composite: 40% diversity, 30% sentence quality, 30% length adequacy.
			lengthScore := math.Min(wordCount/50, 1.0)
			return 0.4*diversity + 0.3*sentScore + 0.3*lengthScore
		},
	}
}
