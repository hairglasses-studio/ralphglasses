package eval

import (
	"math/rand/v2"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"gonum.org/v1/gonum/stat/distuv"
)

// ABTestResult holds the Bayesian comparison of two observation groups.
type ABTestResult struct {
	ProbABetter float64       `json:"prob_a_better"`
	ProbBBetter float64       `json:"prob_b_better"`
	PosteriorA  BetaPosterior `json:"posterior_a"`
	PosteriorB  BetaPosterior `json:"posterior_b"`
	SampleSizeA int           `json:"sample_size_a"`
	SampleSizeB int           `json:"sample_size_b"`
}

// BetaPosterior holds Beta distribution parameters and summary statistics.
type BetaPosterior struct {
	Alpha float64    `json:"alpha"` // successes + prior alpha
	Beta  float64    `json:"beta"`  // failures + prior beta
	Mean  float64    `json:"mean"`
	CI95  [2]float64 `json:"ci_95"` // 95% credible interval
}

const monteCarloDraws = 10000

// CompareAB performs a Bayesian A/B test comparing two observation groups.
// successFn determines whether each observation counts as a success.
// It uses a Beta-Bernoulli model with uninformative prior Beta(1,1)
// and Monte Carlo sampling to estimate P(A > B).
func CompareAB(groupA, groupB []session.LoopObservation, successFn func(session.LoopObservation) bool) ABTestResult {
	if len(groupA) == 0 || len(groupB) == 0 {
		return ABTestResult{
			ProbABetter: 0.5,
			ProbBBetter: 0.5,
			SampleSizeA: len(groupA),
			SampleSizeB: len(groupB),
		}
	}

	alphaA, betaA, alphaB, betaB := computeAlphaBeta(groupA, groupB, successFn)
	posteriorA := computePosterior(alphaA, betaA)
	posteriorB := computePosterior(alphaB, betaB)

	src := rand.NewPCG(uint64(time.Now().UnixNano()), uint64(time.Now().UnixNano()>>32))
	probA, probB := monteCarloCompare(alphaA, betaA, alphaB, betaB, src)

	return ABTestResult{
		ProbABetter: probA,
		ProbBBetter: probB,
		PosteriorA:  posteriorA,
		PosteriorB:  posteriorB,
		SampleSizeA: len(groupA),
		SampleSizeB: len(groupB),
	}
}

// CompareABSeeded is like CompareAB but uses a fixed random seed for reproducible results.
func CompareABSeeded(groupA, groupB []session.LoopObservation, successFn func(session.LoopObservation) bool, seed int64) ABTestResult {
	if len(groupA) == 0 || len(groupB) == 0 {
		return ABTestResult{
			ProbABetter: 0.5,
			ProbBBetter: 0.5,
			SampleSizeA: len(groupA),
			SampleSizeB: len(groupB),
		}
	}

	alphaA, betaA, alphaB, betaB := computeAlphaBeta(groupA, groupB, successFn)
	posteriorA := computePosterior(alphaA, betaA)
	posteriorB := computePosterior(alphaB, betaB)

	src := rand.NewPCG(uint64(seed), uint64(seed))
	probA, probB := monteCarloCompare(alphaA, betaA, alphaB, betaB, src)

	return ABTestResult{
		ProbABetter: probA,
		ProbBBetter: probB,
		PosteriorA:  posteriorA,
		PosteriorB:  posteriorB,
		SampleSizeA: len(groupA),
		SampleSizeB: len(groupB),
	}
}

// CompareProviders splits observations by WorkerProvider and performs a
// Bayesian A/B test using VerifyPassed as the success criterion.
func CompareProviders(observations []session.LoopObservation, provA, provB string) ABTestResult {
	var groupA, groupB []session.LoopObservation
	for _, o := range observations {
		switch o.WorkerProvider {
		case provA:
			groupA = append(groupA, o)
		case provB:
			groupB = append(groupB, o)
		}
	}
	return CompareAB(groupA, groupB, func(o session.LoopObservation) bool {
		return o.VerifyPassed
	})
}

// ComparePeriods splits observations at splitTime into before/after groups
// and performs a Bayesian A/B test using the provided success function.
func ComparePeriods(observations []session.LoopObservation, splitTime time.Time, successFn func(session.LoopObservation) bool) ABTestResult {
	var before, after []session.LoopObservation
	for _, o := range observations {
		if o.Timestamp.Before(splitTime) {
			before = append(before, o)
		} else {
			after = append(after, o)
		}
	}
	return CompareAB(before, after, successFn)
}

// computeAlphaBeta counts successes/failures in each group and returns
// Beta distribution parameters with uninformative prior Beta(1,1).
func computeAlphaBeta(groupA, groupB []session.LoopObservation, successFn func(session.LoopObservation) bool) (alphaA, betaA, alphaB, betaB float64) {
	successA, successB := 0, 0
	for _, o := range groupA {
		if successFn(o) {
			successA++
		}
	}
	for _, o := range groupB {
		if successFn(o) {
			successB++
		}
	}
	alphaA = float64(successA + 1)
	betaA = float64(len(groupA) - successA + 1)
	alphaB = float64(successB + 1)
	betaB = float64(len(groupB) - successB + 1)
	return
}

// computePosterior builds a BetaPosterior with mean and 95% credible interval.
func computePosterior(alpha, beta float64) BetaPosterior {
	dist := distuv.Beta{Alpha: alpha, Beta: beta}
	return BetaPosterior{
		Alpha: alpha,
		Beta:  beta,
		Mean:  dist.Mean(),
		CI95:  [2]float64{dist.Quantile(0.025), dist.Quantile(0.975)},
	}
}

// monteCarloCompare draws from two Beta distributions and returns the fraction
// of times A > B and B > A.
func monteCarloCompare(alphaA, betaA, alphaB, betaB float64, src rand.Source) (probA, probB float64) {
	rng := rand.New(src)
	srcB := rand.NewPCG(rng.Uint64(), rng.Uint64())

	distA := distuv.Beta{Alpha: alphaA, Beta: betaA, Src: src}
	distB := distuv.Beta{Alpha: alphaB, Beta: betaB, Src: srcB}

	aWins, bWins := 0, 0
	for i := 0; i < monteCarloDraws; i++ {
		sA := distA.Rand()
		sB := distB.Rand()
		if sA > sB {
			aWins++
		} else if sB > sA {
			bWins++
		}
	}

	probA = float64(aWins) / float64(monteCarloDraws)
	probB = float64(bWins) / float64(monteCarloDraws)
	return probA, probB
}
