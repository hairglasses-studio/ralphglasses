package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddResultAndRetrieval(t *testing.T) {
	d := NewDashboard()
	d.AddResult("BenchmarkFoo", 1200, 64, 2)
	d.AddResult("BenchmarkBar", 800, 32, 1)

	if len(d.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(d.Results))
	}
	if d.Results[0].Name != "BenchmarkFoo" {
		t.Errorf("expected BenchmarkFoo, got %s", d.Results[0].Name)
	}
	if d.Results[0].NsPerOp != 1200 {
		t.Errorf("expected 1200 ns/op, got %d", d.Results[0].NsPerOp)
	}
	if d.Results[1].AllocBytes != 32 {
		t.Errorf("expected 32 alloc bytes, got %d", d.Results[1].AllocBytes)
	}
}

func TestAddResultWithMeta(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 1000, 64, 2, "run-1", "abc123")

	if d.Results[0].RunLabel != "run-1" {
		t.Errorf("expected run label run-1, got %s", d.Results[0].RunLabel)
	}
	if d.Results[0].GitCommit != "abc123" {
		t.Errorf("expected git commit abc123, got %s", d.Results[0].GitCommit)
	}
	if d.Results[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestCompareFaster(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 1000, 64, 2, "baseline", "aaa")
	d.AddResultWithMeta("BenchmarkFoo", 500, 32, 1, "current", "bbb")

	comps := d.Compare("baseline", "current")
	if len(comps) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comps))
	}
	c := comps[0]
	if c.Verdict != "faster" {
		t.Errorf("expected faster, got %s", c.Verdict)
	}
	if c.DeltaNs != -500 {
		t.Errorf("expected delta -500, got %d", c.DeltaNs)
	}
	if c.DeltaPercent != -50 {
		t.Errorf("expected -50%%, got %f", c.DeltaPercent)
	}
}

func TestCompareSlower(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 1000, 64, 2, "baseline", "aaa")
	d.AddResultWithMeta("BenchmarkFoo", 1200, 64, 2, "current", "bbb")

	comps := d.Compare("baseline", "current")
	if len(comps) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comps))
	}
	if comps[0].Verdict != "slower" {
		t.Errorf("expected slower, got %s", comps[0].Verdict)
	}
}

func TestCompareSame(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 1000, 64, 2, "baseline", "aaa")
	d.AddResultWithMeta("BenchmarkFoo", 1020, 64, 2, "current", "bbb") // 2% change

	comps := d.Compare("baseline", "current")
	if len(comps) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(comps))
	}
	if comps[0].Verdict != "same" {
		t.Errorf("expected same (2%% change), got %s", comps[0].Verdict)
	}
}

func TestCompareNoOverlap(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 1000, 64, 2, "baseline", "aaa")
	d.AddResultWithMeta("BenchmarkBar", 500, 32, 1, "current", "bbb")

	comps := d.Compare("baseline", "current")
	if len(comps) != 0 {
		t.Errorf("expected 0 comparisons for non-overlapping benchmarks, got %d", len(comps))
	}
}

func TestTrend(t *testing.T) {
	d := NewDashboard()
	// Add 5 results for the same benchmark.
	for i := 0; i < 5; i++ {
		d.AddResultWithMeta("BenchmarkFoo", int64(1000-i*100), 64, 2, "run-"+string(rune('0'+i)), "commit")
	}

	pts := d.Trend("BenchmarkFoo", 3)
	if len(pts) != 3 {
		t.Fatalf("expected 3 trend points, got %d", len(pts))
	}
	// Should be the last 3 by timestamp.
	if pts[0].NsPerOp != 800 {
		t.Errorf("expected first trend point 800 ns/op, got %d", pts[0].NsPerOp)
	}
}

func TestTrendAll(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 100, 0, 0, "a", "")
	d.AddResultWithMeta("BenchmarkFoo", 200, 0, 0, "b", "")

	pts := d.Trend("BenchmarkFoo", 0)
	if len(pts) != 2 {
		t.Fatalf("expected 2 trend points, got %d", len(pts))
	}
}

func TestTrendNonExistent(t *testing.T) {
	d := NewDashboard()
	pts := d.Trend("BenchmarkNope", 5)
	if len(pts) != 0 {
		t.Errorf("expected 0 trend points, got %d", len(pts))
	}
}

func TestSummary(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFast", 100, 8, 1, "run-1", "aaa")
	d.AddResultWithMeta("BenchmarkSlow", 5000, 512, 10, "run-1", "aaa")
	d.AddResultWithMeta("BenchmarkFast", 50, 4, 1, "run-2", "bbb")   // improved
	d.AddResultWithMeta("BenchmarkSlow", 6000, 600, 12, "run-2", "bbb") // regressed

	s := d.Summary()
	if s.TotalBenchmarks != 2 {
		t.Errorf("expected 2 benchmarks, got %d", s.TotalBenchmarks)
	}
	if s.TotalRuns != 2 {
		t.Errorf("expected 2 runs, got %d", s.TotalRuns)
	}
	if s.Fastest == nil || s.Fastest.NsPerOp != 50 {
		t.Errorf("expected fastest 50 ns/op, got %v", s.Fastest)
	}
	if s.Slowest == nil || s.Slowest.NsPerOp != 6000 {
		t.Errorf("expected slowest 6000 ns/op, got %v", s.Slowest)
	}
	if s.MostImproved == nil || s.MostImproved.Name != "BenchmarkFast" {
		t.Errorf("expected most improved BenchmarkFast, got %v", s.MostImproved)
	}
	if len(s.Regressions) != 1 || s.Regressions[0].Name != "BenchmarkSlow" {
		t.Errorf("expected 1 regression (BenchmarkSlow), got %v", s.Regressions)
	}
}

func TestSummaryEmpty(t *testing.T) {
	d := NewDashboard()
	s := d.Summary()
	if s.TotalBenchmarks != 0 {
		t.Errorf("expected 0 benchmarks, got %d", s.TotalBenchmarks)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	d := NewDashboard()
	d.AddResultWithMeta("BenchmarkFoo", 1234, 64, 2, "run-1", "abc123")
	d.AddResultWithMeta("BenchmarkBar", 5678, 128, 4, "run-1", "abc123")

	dir := t.TempDir()
	path := filepath.Join(dir, "bench.json")

	if err := d.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	// Verify file was created.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	d2 := NewDashboard()
	if err := d2.LoadJSON(path); err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}

	if len(d2.Results) != 2 {
		t.Fatalf("expected 2 results after load, got %d", len(d2.Results))
	}
	if d2.Results[0].Name != "BenchmarkFoo" {
		t.Errorf("expected BenchmarkFoo, got %s", d2.Results[0].Name)
	}
	if d2.Results[0].NsPerOp != 1234 {
		t.Errorf("expected 1234 ns/op, got %d", d2.Results[0].NsPerOp)
	}
	if d2.Results[1].AllocCount != 4 {
		t.Errorf("expected 4 allocs, got %d", d2.Results[1].AllocCount)
	}
	if d2.Results[0].GitCommit != "abc123" {
		t.Errorf("expected git commit abc123, got %s", d2.Results[0].GitCommit)
	}
}

func TestLoadJSONFileNotFound(t *testing.T) {
	d := NewDashboard()
	err := d.LoadJSON("/nonexistent/path/bench.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFormatTable(t *testing.T) {
	d := NewDashboard()
	d.AddResult("BenchmarkFoo", 1200, 64, 2)
	d.AddResult("BenchmarkBar", 800, 32, 1)

	table := d.FormatTable()
	if !strings.Contains(table, "Benchmark") {
		t.Error("table missing header")
	}
	if !strings.Contains(table, "ns/op") {
		t.Error("table missing ns/op column")
	}
	if !strings.Contains(table, "BenchmarkFoo") {
		t.Error("table missing BenchmarkFoo")
	}
	if !strings.Contains(table, "BenchmarkBar") {
		t.Error("table missing BenchmarkBar")
	}
	if !strings.Contains(table, "1200") {
		t.Error("table missing 1200 value")
	}
}

func TestFormatTableEmpty(t *testing.T) {
	d := NewDashboard()
	table := d.FormatTable()
	if table != "(no benchmark results)" {
		t.Errorf("expected empty message, got %q", table)
	}
}

func TestFormatTableLatestWins(t *testing.T) {
	d := NewDashboard()
	d.AddResult("BenchmarkFoo", 1000, 64, 2)
	d.AddResult("BenchmarkFoo", 500, 32, 1) // later, should win

	table := d.FormatTable()
	if !strings.Contains(table, "500") {
		t.Error("table should show latest result (500)")
	}
}
