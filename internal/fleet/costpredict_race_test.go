package fleet

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestCostPredictorConcurrentRecordForecast(t *testing.T) {
	cp := NewCostPredictor(2.5)

	var wg sync.WaitGroup

	baseTime := time.Now()

	// 10 goroutines recording CostSample.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cp.Record(CostSample{
					Timestamp: baseTime.Add(time.Duration(id*50+j) * time.Second),
					CostUSD:   0.01 * float64(j+1),
					Provider:  "claude",
					TaskType:  "coding",
				})
			}
		}(i)
	}

	// 5 goroutines calling Forecast.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				f := cp.Forecast(100.0)
				if f.BurnRatePerHour < 0 {
					t.Error("negative burn rate from Forecast")
				}
				if f.TrendDirection != "stable" && f.TrendDirection != "increasing" && f.TrendDirection != "decreasing" {
					t.Errorf("unexpected trend direction: %s", f.TrendDirection)
				}
			}
		}()
	}

	// 5 goroutines calling BurnRate.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				rate := cp.BurnRate()
				if math.IsNaN(rate) {
					t.Error("BurnRate returned NaN")
				}
			}
		}()
	}

	wg.Wait()

	// Verify samples were recorded.
	n := cp.Len()
	if n == 0 {
		t.Error("expected recorded samples, got 0")
	}
}
