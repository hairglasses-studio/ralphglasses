package views

import (
	"math"
	"testing"
)

func TestPercentile_Empty(t *testing.T) {
	t.Parallel()
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("percentile(nil, 0.5) = %f, want 0", got)
	}
	if got := percentile([]float64{}, 0.95); got != 0 {
		t.Errorf("percentile([], 0.95) = %f, want 0", got)
	}
}

func TestPercentile_SingleValue(t *testing.T) {
	t.Parallel()
	vals := []float64{42.0}
	if got := percentile(vals, 0.5); got != 42.0 {
		t.Errorf("percentile([42], 0.5) = %f, want 42", got)
	}
	if got := percentile(vals, 0.95); got != 42.0 {
		t.Errorf("percentile([42], 0.95) = %f, want 42", got)
	}
}

func TestPercentile_Sorted(t *testing.T) {
	t.Parallel()
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	p50 := percentile(vals, 0.5)
	if p50 < 5 || p50 > 6 {
		t.Errorf("P50 = %f, expected ~5-6", p50)
	}

	p95 := percentile(vals, 0.95)
	if p95 < 9 {
		t.Errorf("P95 = %f, expected >= 9", p95)
	}

	p0 := percentile(vals, 0.0)
	if p0 != 1 {
		t.Errorf("P0 = %f, want 1", p0)
	}
}

func TestPercentile_Unsorted(t *testing.T) {
	t.Parallel()
	// percentile sorts internally, so unsorted input should work.
	vals := []float64{5, 1, 9, 3, 7}

	p0 := percentile(vals, 0.0)
	if p0 != 1 {
		t.Errorf("P0 of unsorted = %f, want 1 (min)", p0)
	}

	// P100 should give the max value.
	p100 := percentile(vals, 1.0)
	if p100 != 9 {
		t.Errorf("P100 of unsorted = %f, want 9 (max)", p100)
	}
}

func TestPercentile_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	vals := []float64{5, 1, 9, 3, 7}
	original := make([]float64, len(vals))
	copy(original, vals)

	_ = percentile(vals, 0.5)

	for i := range vals {
		if math.Abs(vals[i]-original[i]) > 1e-10 {
			t.Fatalf("percentile mutated input: vals[%d] = %f, want %f", i, vals[i], original[i])
		}
	}
}
