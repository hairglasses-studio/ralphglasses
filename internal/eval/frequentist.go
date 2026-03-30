package eval

import (
	"math"
	"math/rand/v2"
)

// FrequentistResult holds the outcome of a frequentist hypothesis test.
type FrequentistResult struct {
	TestType           string     `json:"test_type"`            // "z-test" or "t-test"
	Statistic          float64    `json:"statistic"`            // z-score or t-score
	PValue             float64    `json:"p_value"`              // two-tailed p-value
	ConfidenceInterval [2]float64 `json:"confidence_interval"`  // 95% CI for difference
	Significant        bool       `json:"significant"`          // p < 0.05
	EffectSize         float64    `json:"effect_size"`          // Cohen's h (z-test) or Cohen's d (t-test)
	MeanA              float64    `json:"mean_a"`               // sample mean/proportion A
	MeanB              float64    `json:"mean_b"`               // sample mean/proportion B
	SampleSizeA        int        `json:"sample_size_a"`
	SampleSizeB        int        `json:"sample_size_b"`
}

// ComparisonReport combines Bayesian and frequentist analysis with a recommendation.
type ComparisonReport struct {
	Bayesian      *ABTestResult      `json:"bayesian"`
	Frequentist   *FrequentistResult `json:"frequentist"`
	Recommendation string            `json:"recommendation"`
	Agreement     bool               `json:"agreement"` // whether Bayesian and frequentist agree
}

// ZTestProportions performs a two-proportion z-test.
// H0: pA = pB, H1: pA != pB (two-tailed).
func ZTestProportions(succA, nA, succB, nB int) *FrequentistResult {
	if nA <= 0 || nB <= 0 {
		return &FrequentistResult{
			TestType:    "z-test",
			PValue:      1.0,
			SampleSizeA: nA,
			SampleSizeB: nB,
		}
	}

	pA := float64(succA) / float64(nA)
	pB := float64(succB) / float64(nB)

	// Pooled proportion under H0.
	pPool := float64(succA+succB) / float64(nA+nB)

	// Standard error of the difference.
	se := math.Sqrt(pPool * (1 - pPool) * (1.0/float64(nA) + 1.0/float64(nB)))

	var z float64
	if se == 0 {
		// No variance — proportions are identical or all successes/failures.
		z = 0
	} else {
		z = (pA - pB) / se
	}

	pValue := 2 * (1 - normalCDF(math.Abs(z)))

	// 95% CI for the difference pA - pB (using unpooled SE for CI).
	seDiff := math.Sqrt(pA*(1-pA)/float64(nA) + pB*(1-pB)/float64(nB))
	diff := pA - pB
	ci := [2]float64{diff - 1.96*seDiff, diff + 1.96*seDiff}

	// Cohen's h effect size for proportions.
	h := 2*math.Asin(math.Sqrt(pA)) - 2*math.Asin(math.Sqrt(pB))

	return &FrequentistResult{
		TestType:           "z-test",
		Statistic:          z,
		PValue:             pValue,
		ConfidenceInterval: ci,
		Significant:        pValue < 0.05,
		EffectSize:         h,
		MeanA:              pA,
		MeanB:              pB,
		SampleSizeA:        nA,
		SampleSizeB:        nB,
	}
}

// WelchTTest performs Welch's t-test for two independent samples with
// potentially unequal variances and sample sizes.
// H0: muA = muB, H1: muA != muB (two-tailed).
func WelchTTest(samplesA, samplesB []float64) *FrequentistResult {
	nA, nB := len(samplesA), len(samplesB)

	if nA < 2 || nB < 2 {
		return &FrequentistResult{
			TestType:    "t-test",
			PValue:      1.0,
			SampleSizeA: nA,
			SampleSizeB: nB,
		}
	}

	meanA := mean(samplesA)
	meanB := mean(samplesB)
	varA := variance(samplesA, meanA)
	varB := variance(samplesB, meanB)

	fnA := float64(nA)
	fnB := float64(nB)

	denom := varA/fnA + varB/fnB
	if denom == 0 {
		// Zero variance in both groups — means are exact, no test possible.
		return &FrequentistResult{
			TestType:    "t-test",
			PValue:      1.0,
			MeanA:       meanA,
			MeanB:       meanB,
			SampleSizeA: nA,
			SampleSizeB: nB,
		}
	}

	t := (meanA - meanB) / math.Sqrt(denom)

	// Welch-Satterthwaite degrees of freedom.
	num := denom * denom
	dA := (varA / fnA) * (varA / fnA) / (fnA - 1)
	dB := (varB / fnB) * (varB / fnB) / (fnB - 1)
	if dA+dB == 0 {
		return &FrequentistResult{
			TestType:    "t-test",
			PValue:      1.0,
			MeanA:       meanA,
			MeanB:       meanB,
			SampleSizeA: nA,
			SampleSizeB: nB,
		}
	}
	df := num / (dA + dB)

	pValue := tTestPValue(math.Abs(t), df)

	// 95% CI for the difference.
	seDiff := math.Sqrt(denom)
	// Use normal approximation for critical value (good for df > 30).
	tCrit := 1.96
	if df < 30 {
		// Better approximation for small df using inverse t via normal approximation
		// adjusted with Cornish-Fisher expansion.
		tCrit = approxTCritical(df)
	}
	diff := meanA - meanB
	ci := [2]float64{diff - tCrit*seDiff, diff + tCrit*seDiff}

	// Cohen's d effect size using pooled standard deviation.
	pooledSD := math.Sqrt(((fnA-1)*varA + (fnB-1)*varB) / (fnA + fnB - 2))
	var d float64
	if pooledSD > 0 {
		d = (meanA - meanB) / pooledSD
	}

	return &FrequentistResult{
		TestType:           "t-test",
		Statistic:          t,
		PValue:             pValue,
		ConfidenceInterval: ci,
		Significant:        pValue < 0.05,
		EffectSize:         d,
		MeanA:              meanA,
		MeanB:              meanB,
		SampleSizeA:        nA,
		SampleSizeB:        nB,
	}
}

// GenerateReport produces a combined Bayesian + frequentist comparison report.
// groupA and groupB are raw metric values; successFn converts each value to
// a binary outcome for the Bayesian Beta-Bernoulli model and z-test.
func GenerateReport(groupA, groupB []float64, successFn func(float64) bool) *ComparisonReport {
	// Count successes for Bayesian and z-test.
	succA, succB := 0, 0
	for _, v := range groupA {
		if successFn(v) {
			succA++
		}
	}
	for _, v := range groupB {
		if successFn(v) {
			succB++
		}
	}

	// Bayesian comparison using session observations wrapper.
	// We build a lightweight ABTestResult directly using the counts.
	bayesian := bayesianFromCounts(succA, len(groupA), succB, len(groupB))

	// Frequentist: z-test on proportions + Welch's t-test on raw values.
	zResult := ZTestProportions(succA, len(groupA), succB, len(groupB))
	tResult := WelchTTest(groupA, groupB)

	// Pick the more appropriate test: z-test for binary, t-test for continuous.
	// If all values are 0 or 1 (binary), prefer z-test; otherwise prefer t-test.
	isBinary := true
	for _, v := range groupA {
		if v != 0 && v != 1 {
			isBinary = false
			break
		}
	}
	if isBinary {
		for _, v := range groupB {
			if v != 0 && v != 1 {
				isBinary = false
				break
			}
		}
	}

	var freq *FrequentistResult
	if isBinary {
		freq = zResult
	} else {
		freq = tResult
	}

	// Determine agreement between Bayesian and frequentist.
	bayesianFavorsA := bayesian.ProbABetter > 0.95
	bayesianFavorsB := bayesian.ProbBBetter > 0.95
	freqFavorsA := freq.Significant && freq.Statistic > 0
	freqFavorsB := freq.Significant && freq.Statistic < 0

	agreement := (bayesianFavorsA == freqFavorsA) && (bayesianFavorsB == freqFavorsB)

	// Build recommendation.
	rec := buildRecommendation(bayesian, freq, bayesianFavorsA, bayesianFavorsB, freqFavorsA, freqFavorsB, agreement)

	return &ComparisonReport{
		Bayesian:       bayesian,
		Frequentist:    freq,
		Recommendation: rec,
		Agreement:      agreement,
	}
}

// normalCDF computes the cumulative distribution function of the standard
// normal distribution using the Abramowitz and Stegun rational approximation
// (formula 26.2.17). Maximum error: 7.5e-8.
func normalCDF(z float64) float64 {
	if z < -8 {
		return 0
	}
	if z > 8 {
		return 1
	}

	// Use symmetry for negative z.
	if z < 0 {
		return 1 - normalCDF(-z)
	}

	// Constants for the rational approximation.
	const (
		p  = 0.2316419
		b1 = 0.319381530
		b2 = -0.356563782
		b3 = 1.781477937
		b4 = -1.821255978
		b5 = 1.330274429
	)

	t := 1.0 / (1.0 + p*z)
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t

	pdf := math.Exp(-0.5*z*z) / math.Sqrt(2*math.Pi)
	return 1 - pdf*(b1*t+b2*t2+b3*t3+b4*t4+b5*t5)
}

// approxTCritical returns an approximate 0.975 quantile of the t-distribution
// (for a 95% two-tailed CI) using the approximation:
// t ≈ z + (z^3 + z) / (4*df) where z = 1.96.
func approxTCritical(df float64) float64 {
	const z = 1.96
	return z + (z*z*z+z)/(4*df)
}

// bayesianFromCounts constructs an ABTestResult from success/total counts
// without requiring session.LoopObservation.
func bayesianFromCounts(succA, nA, succB, nB int) *ABTestResult {
	if nA == 0 || nB == 0 {
		return &ABTestResult{
			ProbABetter: 0.5,
			ProbBBetter: 0.5,
			SampleSizeA: nA,
			SampleSizeB: nB,
		}
	}

	alphaA := float64(succA + 1)
	betaA := float64(nA - succA + 1)
	alphaB := float64(succB + 1)
	betaB := float64(nB - succB + 1)

	posteriorA := computePosterior(alphaA, betaA)
	posteriorB := computePosterior(alphaB, betaB)

	// Use a fixed seed for reproducibility in report generation.
	src := rand.NewPCG(42, 42)
	probA, probB := monteCarloCompare(alphaA, betaA, alphaB, betaB, src)

	return &ABTestResult{
		ProbABetter: probA,
		ProbBBetter: probB,
		PosteriorA:  posteriorA,
		PosteriorB:  posteriorB,
		SampleSizeA: nA,
		SampleSizeB: nB,
	}
}

func buildRecommendation(bayesian *ABTestResult, freq *FrequentistResult,
	bayesianFavorsA, bayesianFavorsB, freqFavorsA, freqFavorsB, agreement bool) string {

	if !freq.Significant && bayesian.ProbABetter < 0.95 && bayesian.ProbBBetter < 0.95 {
		return "No significant difference detected. Collect more data or the variants are equivalent."
	}

	if agreement {
		if freqFavorsA {
			return "Both Bayesian and frequentist analysis favor variant A. Strong evidence for A."
		}
		if freqFavorsB {
			return "Both Bayesian and frequentist analysis favor variant B. Strong evidence for B."
		}
	}

	// Disagreement or partial significance.
	if freq.Significant && !bayesianFavorsA && !bayesianFavorsB {
		return "Frequentist test is significant but Bayesian posterior is uncertain. Consider collecting more data."
	}
	if !freq.Significant && (bayesianFavorsA || bayesianFavorsB) {
		winner := "A"
		if bayesianFavorsB {
			winner = "B"
		}
		return "Bayesian analysis favors variant " + winner + " but frequentist test is not significant. Effect may be real but small."
	}

	return "Mixed signals between methods. Review effect sizes and collect more data."
}
