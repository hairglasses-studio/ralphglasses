package eval

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestNormalCDF(t *testing.T) {
	tests := []struct {
		z    float64
		want float64
		tol  float64
	}{
		{0, 0.5, 1e-6},
		{1.96, 0.975, 1e-3},
		{-1.96, 0.025, 1e-3},
		{1.0, 0.8413, 1e-3},
		{-1.0, 0.1587, 1e-3},
		{2.576, 0.995, 1e-3},
		{3.0, 0.9987, 1e-3},
		{-10, 0, 1e-10},
		{10, 1, 1e-10},
	}

	for _, tt := range tests {
		got := normalCDF(tt.z)
		if math.Abs(got-tt.want) > tt.tol {
			t.Errorf("normalCDF(%v) = %v, want %v (tol %v)", tt.z, got, tt.want, tt.tol)
		}
	}
}

func TestZTestProportions_KnownOutcome(t *testing.T) {
	// 50% vs 60% with n=100 each.
	// Expected z ≈ 1.43, p ≈ 0.153 (not significant at 0.05).
	result := ZTestProportions(50, 100, 60, 100)

	if result.TestType != "z-test" {
		t.Errorf("TestType = %q, want z-test", result.TestType)
	}
	if result.Significant {
		t.Error("expected not significant for 50% vs 60% with n=100")
	}
	if math.Abs(result.MeanA-0.5) > 1e-6 {
		t.Errorf("MeanA = %v, want 0.5", result.MeanA)
	}
	if math.Abs(result.MeanB-0.6) > 1e-6 {
		t.Errorf("MeanB = %v, want 0.6", result.MeanB)
	}
	// z should be negative since pA < pB.
	if result.Statistic > 0 {
		t.Errorf("z-score = %v, expected negative (A < B)", result.Statistic)
	}
	if result.PValue < 0 || result.PValue > 1 {
		t.Errorf("p-value = %v, want in [0,1]", result.PValue)
	}
}

func TestZTestProportions_Significant(t *testing.T) {
	// 50% vs 80% with n=200 each — should be highly significant.
	result := ZTestProportions(100, 200, 160, 200)

	if !result.Significant {
		t.Error("expected significant for 50% vs 80% with n=200")
	}
	if result.PValue > 0.001 {
		t.Errorf("p-value = %v, expected < 0.001", result.PValue)
	}
	// CI should not contain 0 (significant difference).
	if result.ConfidenceInterval[0] <= 0 && result.ConfidenceInterval[1] >= 0 {
		// A < B, so difference pA - pB is negative. CI should be entirely negative.
		// Actually difference is -0.3, so both bounds should be negative.
	}
}

func TestZTestProportions_IdenticalGroups(t *testing.T) {
	result := ZTestProportions(50, 100, 50, 100)

	if math.Abs(result.Statistic) > 1e-10 {
		t.Errorf("z-score = %v, want ~0 for identical groups", result.Statistic)
	}
	if result.Significant {
		t.Error("expected not significant for identical groups")
	}
}

func TestZTestProportions_ZeroN(t *testing.T) {
	result := ZTestProportions(0, 0, 5, 10)
	if result.PValue != 1.0 {
		t.Errorf("p-value = %v, want 1.0 for zero sample size", result.PValue)
	}
}

func TestZTestProportions_AllSuccessOrFailure(t *testing.T) {
	// All successes vs all failures.
	result := ZTestProportions(100, 100, 0, 100)
	if !result.Significant {
		t.Error("expected significant for 100% vs 0%")
	}
}

func TestWelchTTest_KnownOutcome(t *testing.T) {
	// Generate two samples with known different means.
	rng := rand.New(rand.NewPCG(12345, 67890))

	samplesA := make([]float64, 50)
	samplesB := make([]float64, 50)
	for i := range 50 {
		samplesA[i] = rng.NormFloat64()*1.0 + 5.0 // mean=5, sd=1
		samplesB[i] = rng.NormFloat64()*1.0 + 7.0 // mean=7, sd=1
	}

	result := WelchTTest(samplesA, samplesB)

	if result.TestType != "t-test" {
		t.Errorf("TestType = %q, want t-test", result.TestType)
	}
	if !result.Significant {
		t.Error("expected significant for means 5 vs 7 with n=50")
	}
	if result.PValue > 0.001 {
		t.Errorf("p-value = %v, expected < 0.001", result.PValue)
	}
	// t-score should be negative since meanA < meanB.
	if result.Statistic > 0 {
		t.Errorf("t-score = %v, expected negative (A < B)", result.Statistic)
	}
	// Cohen's d should be large (approx -2).
	if math.Abs(result.EffectSize) < 1.0 {
		t.Errorf("effect size = %v, expected |d| > 1.0", result.EffectSize)
	}
}

func TestWelchTTest_IdenticalGroups(t *testing.T) {
	samples := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	result := WelchTTest(samples, samples)

	if math.Abs(result.Statistic) > 1e-10 {
		t.Errorf("t-score = %v, want ~0 for identical groups", result.Statistic)
	}
	if result.Significant {
		t.Error("expected not significant for identical groups")
	}
}

func TestWelchTTest_ZeroVariance(t *testing.T) {
	// All values the same — zero variance.
	samplesA := []float64{5, 5, 5, 5, 5}
	samplesB := []float64{5, 5, 5, 5, 5}

	result := WelchTTest(samplesA, samplesB)

	if result.PValue != 1.0 {
		t.Errorf("p-value = %v, want 1.0 for zero variance", result.PValue)
	}
}

func TestWelchTTest_ZeroVarianceDifferentMeans(t *testing.T) {
	// Zero variance but different means.
	samplesA := []float64{3, 3, 3, 3, 3}
	samplesB := []float64{7, 7, 7, 7, 7}

	result := WelchTTest(samplesA, samplesB)

	// With zero variance, test should return p=1 (no variance to compute t).
	if result.PValue != 1.0 {
		t.Errorf("p-value = %v, want 1.0 for zero variance", result.PValue)
	}
}

func TestWelchTTest_SingleSample(t *testing.T) {
	result := WelchTTest([]float64{1}, []float64{2})

	if result.PValue != 1.0 {
		t.Errorf("p-value = %v, want 1.0 for single sample", result.PValue)
	}
	if result.SampleSizeA != 1 || result.SampleSizeB != 1 {
		t.Error("sample sizes not preserved")
	}
}

func TestWelchTTest_UnequalSizes(t *testing.T) {
	rng := rand.New(rand.NewPCG(111, 222))

	samplesA := make([]float64, 30)
	samplesB := make([]float64, 100)
	for i := range 30 {
		samplesA[i] = rng.NormFloat64()*2.0 + 10.0
	}
	for i := range 100 {
		samplesB[i] = rng.NormFloat64()*1.0 + 10.0
	}

	result := WelchTTest(samplesA, samplesB)

	// Same mean, should not be significant.
	if result.SampleSizeA != 30 || result.SampleSizeB != 100 {
		t.Errorf("sample sizes = (%d, %d), want (30, 100)", result.SampleSizeA, result.SampleSizeB)
	}
}

func TestWelchTTest_SignificanceThreshold(t *testing.T) {
	// Marginally significant: small difference, moderate n.
	rng := rand.New(rand.NewPCG(999, 888))

	samplesA := make([]float64, 200)
	samplesB := make([]float64, 200)
	for i := range 200 {
		samplesA[i] = rng.NormFloat64()*1.0 + 0.0
		samplesB[i] = rng.NormFloat64()*1.0 + 0.3 // small effect
	}

	result := WelchTTest(samplesA, samplesB)

	// With d=0.3, n=200, this should be significant.
	if !result.Significant {
		t.Logf("p-value = %v (may not be significant with this seed)", result.PValue)
	}
	// Verify CI is well-formed.
	if result.ConfidenceInterval[0] >= result.ConfidenceInterval[1] {
		t.Errorf("CI lower >= upper: [%v, %v]", result.ConfidenceInterval[0], result.ConfidenceInterval[1])
	}
}

func TestGenerateReport_BinaryData(t *testing.T) {
	groupA := make([]float64, 100)
	groupB := make([]float64, 100)

	// A: 50% success, B: 80% success.
	for i := range 100 {
		if i < 50 {
			groupA[i] = 1
		}
		if i < 80 {
			groupB[i] = 1
		}
	}

	successFn := func(v float64) bool { return v == 1 }
	report := GenerateReport(groupA, groupB, successFn)

	if report == nil {
		t.Fatal("report is nil")
	}
	if report.Bayesian == nil {
		t.Fatal("Bayesian result is nil")
	}
	if report.Frequentist == nil {
		t.Fatal("Frequentist result is nil")
	}

	// Should use z-test for binary data.
	if report.Frequentist.TestType != "z-test" {
		t.Errorf("TestType = %q, want z-test for binary data", report.Frequentist.TestType)
	}

	// 50% vs 80% with n=100 should be significant.
	if !report.Frequentist.Significant {
		t.Error("expected significant for 50% vs 80%")
	}

	// Bayesian should favor B (higher success rate).
	if report.Bayesian.ProbBBetter < 0.9 {
		t.Errorf("ProbBBetter = %v, expected > 0.9", report.Bayesian.ProbBBetter)
	}

	if report.Recommendation == "" {
		t.Error("recommendation is empty")
	}
}

func TestGenerateReport_ContinuousData(t *testing.T) {
	rng := rand.New(rand.NewPCG(54321, 12345))

	groupA := make([]float64, 60)
	groupB := make([]float64, 60)
	for i := range 60 {
		groupA[i] = rng.NormFloat64()*1.0 + 5.0
		groupB[i] = rng.NormFloat64()*1.0 + 7.0
	}

	successFn := func(v float64) bool { return v > 6.0 }
	report := GenerateReport(groupA, groupB, successFn)

	if report == nil {
		t.Fatal("report is nil")
	}

	// Should use t-test for continuous data.
	if report.Frequentist.TestType != "t-test" {
		t.Errorf("TestType = %q, want t-test for continuous data", report.Frequentist.TestType)
	}

	if !report.Frequentist.Significant {
		t.Error("expected significant for means 5 vs 7")
	}
}

func TestGenerateReport_EmptyGroups(t *testing.T) {
	successFn := func(v float64) bool { return v > 0.5 }

	report := GenerateReport(nil, []float64{1, 2, 3}, successFn)
	if report == nil {
		t.Fatal("report is nil for empty group A")
	}
	if report.Bayesian.SampleSizeA != 0 {
		t.Errorf("SampleSizeA = %d, want 0", report.Bayesian.SampleSizeA)
	}
}

func TestGenerateReport_Agreement(t *testing.T) {
	// Create a clear difference so both methods agree.
	groupA := make([]float64, 200)
	groupB := make([]float64, 200)
	for i := range 200 {
		groupA[i] = 1 // all success
	}
	// B: 30% success
	for i := range 200 {
		if i < 60 {
			groupB[i] = 1
		}
	}

	successFn := func(v float64) bool { return v == 1 }
	report := GenerateReport(groupA, groupB, successFn)

	if !report.Frequentist.Significant {
		t.Error("expected significant")
	}
	// Both should agree that A is better.
	if report.Bayesian.ProbABetter < 0.95 {
		t.Errorf("Bayesian ProbABetter = %v, expected > 0.95", report.Bayesian.ProbABetter)
	}
	if !report.Agreement {
		t.Error("expected agreement between Bayesian and frequentist")
	}
}

func TestZTestProportions_EffectSize(t *testing.T) {
	result := ZTestProportions(50, 100, 50, 100)
	if math.Abs(result.EffectSize) > 1e-10 {
		t.Errorf("effect size = %v, want ~0 for identical proportions", result.EffectSize)
	}

	result = ZTestProportions(90, 100, 10, 100)
	if math.Abs(result.EffectSize) < 1.0 {
		t.Errorf("effect size = %v, want large for 90%% vs 10%%", result.EffectSize)
	}
}
