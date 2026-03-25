package eval

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"gonum.org/v1/gonum/stat/distuv"
)

func makeObs(n int, successRate float64, provider string, baseTime time.Time) []session.LoopObservation {
	obs := make([]session.LoopObservation, n)
	successes := int(float64(n) * successRate)
	for i := range obs {
		obs[i] = session.LoopObservation{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Second),
			LoopID:         "test",
			WorkerProvider: provider,
			VerifyPassed:   i < successes,
		}
	}
	return obs
}

func TestCompareABEmpty(t *testing.T) {
	successFn := func(o session.LoopObservation) bool { return o.VerifyPassed }

	// Both empty.
	r := CompareABSeeded(nil, nil, successFn, 42)
	if r.ProbABetter != 0.5 || r.ProbBBetter != 0.5 {
		t.Errorf("empty groups: want 0.5/0.5, got %f/%f", r.ProbABetter, r.ProbBBetter)
	}

	// One empty.
	obs := makeObs(10, 0.8, "claude", time.Now())
	r = CompareABSeeded(obs, nil, successFn, 42)
	if r.ProbABetter != 0.5 || r.ProbBBetter != 0.5 {
		t.Errorf("one empty group: want 0.5/0.5, got %f/%f", r.ProbABetter, r.ProbBBetter)
	}
}

func TestCompareABClearWinner(t *testing.T) {
	successFn := func(o session.LoopObservation) bool { return o.VerifyPassed }
	groupA := makeObs(100, 0.90, "a", time.Now())
	groupB := makeObs(100, 0.40, "b", time.Now())

	r := CompareABSeeded(groupA, groupB, successFn, 42)
	if r.ProbABetter < 0.99 {
		t.Errorf("clear winner: ProbABetter=%f, want > 0.99", r.ProbABetter)
	}
	if r.SampleSizeA != 100 || r.SampleSizeB != 100 {
		t.Errorf("sample sizes: got %d/%d, want 100/100", r.SampleSizeA, r.SampleSizeB)
	}
}

func TestCompareABEqualGroups(t *testing.T) {
	successFn := func(o session.LoopObservation) bool { return o.VerifyPassed }
	groupA := makeObs(100, 0.50, "a", time.Now())
	groupB := makeObs(100, 0.50, "b", time.Now())

	r := CompareABSeeded(groupA, groupB, successFn, 42)
	if r.ProbABetter < 0.4 || r.ProbABetter > 0.6 {
		t.Errorf("equal groups: ProbABetter=%f, want between 0.4 and 0.6", r.ProbABetter)
	}
}

func TestCompareProviders(t *testing.T) {
	base := time.Now()
	claude := makeObs(100, 0.80, "claude", base)
	gemini := makeObs(100, 0.60, "gemini", base)

	all := append(claude, gemini...)
	r := CompareProviders(all, "claude", "gemini")

	if r.ProbABetter < 0.9 {
		t.Errorf("provider comparison: ProbABetter=%f (claude), want > 0.9", r.ProbABetter)
	}
}

func TestComparePeriods(t *testing.T) {
	base := time.Now()
	successFn := func(o session.LoopObservation) bool { return o.VerifyPassed }

	early := makeObs(100, 0.50, "x", base)
	late := makeObs(100, 0.80, "x", base.Add(time.Hour))

	all := append(early, late...)
	splitTime := base.Add(30 * time.Minute)

	r := ComparePeriods(all, splitTime, successFn)
	// B is the "after" group (late, 80% success), should be better.
	if r.ProbBBetter < 0.9 {
		t.Errorf("period comparison: ProbBBetter=%f, want > 0.9", r.ProbBBetter)
	}
}

func TestBetaPosteriorCI(t *testing.T) {
	// Known distribution: 80 successes out of 100 => Beta(81, 21).
	// True rate = 0.8.
	p := computePosterior(81, 21)

	if p.CI95[0] >= 0.8 || p.CI95[1] <= 0.8 {
		t.Errorf("95%% CI [%f, %f] does not contain true rate 0.8", p.CI95[0], p.CI95[1])
	}

	// Verify mean is close to expected.
	expectedMean := 81.0 / (81.0 + 21.0)
	if diff := p.Mean - expectedMean; diff > 0.001 || diff < -0.001 {
		t.Errorf("mean=%f, want ~%f", p.Mean, expectedMean)
	}

	// Verify CI matches gonum directly.
	dist := distuv.Beta{Alpha: 81, Beta: 21}
	lo := dist.Quantile(0.025)
	hi := dist.Quantile(0.975)
	if p.CI95[0] != lo || p.CI95[1] != hi {
		t.Errorf("CI mismatch: got [%f,%f], want [%f,%f]", p.CI95[0], p.CI95[1], lo, hi)
	}
}
