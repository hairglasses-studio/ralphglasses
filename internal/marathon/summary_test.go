package marathon

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRunSummary_Empty(t *testing.T) {
	rs := NewRunSummary()
	out := rs.Render()
	if !strings.Contains(out, "Total cycles: 0") {
		t.Fatalf("expected 0 total cycles in render, got:\n%s", out)
	}
	if !strings.Contains(out, "Success rate: 0.0%") {
		t.Fatalf("expected 0%% success rate, got:\n%s", out)
	}
}

func TestRunSummary_RecordCycle(t *testing.T) {
	rs := NewRunSummary()

	rs.RecordCycle(CycleResult{
		SessionID: "sess-1",
		Success:   true,
		CostUSD:   0.05,
		Duration:  10 * time.Second,
	})
	rs.RecordCycle(CycleResult{
		SessionID: "sess-1",
		Success:   false,
		CostUSD:   0.03,
		Duration:  5 * time.Second,
		ExitCode:  1,
	})
	rs.RecordCycle(CycleResult{
		SessionID: "sess-2",
		Success:   true,
		CostUSD:   0.10,
		Duration:  20 * time.Second,
	})

	out := rs.Render()
	if !strings.Contains(out, "Total cycles: 3") {
		t.Fatalf("expected 3 cycles, got:\n%s", out)
	}
	if !strings.Contains(out, "Successes:    2") {
		t.Fatalf("expected 2 successes, got:\n%s", out)
	}
	if !strings.Contains(out, "Failures:     1") {
		t.Fatalf("expected 1 failure, got:\n%s", out)
	}
	if !strings.Contains(out, "66.7%") {
		t.Fatalf("expected ~66.7%% success rate, got:\n%s", out)
	}
	if !strings.Contains(out, "$0.1800") {
		t.Fatalf("expected total cost $0.18, got:\n%s", out)
	}
	if !strings.Contains(out, "sess-1") || !strings.Contains(out, "sess-2") {
		t.Fatalf("expected per-session breakdown, got:\n%s", out)
	}
}

func TestRunSummary_JSON(t *testing.T) {
	rs := NewRunSummary()

	rs.RecordCycle(CycleResult{
		SessionID: "s1",
		Success:   true,
		CostUSD:   0.25,
		Duration:  30 * time.Second,
	})
	rs.RecordCycle(CycleResult{
		SessionID: "s1",
		Success:   false,
		CostUSD:   0.10,
		Duration:  15 * time.Second,
		ExitCode:  2,
	})

	data, err := rs.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal JSON: %v", err)
	}

	if int(parsed["total_cycles"].(float64)) != 2 {
		t.Fatalf("expected total_cycles=2 in JSON, got %v", parsed["total_cycles"])
	}
	if int(parsed["successes"].(float64)) != 1 {
		t.Fatalf("expected successes=1 in JSON, got %v", parsed["successes"])
	}
	if int(parsed["failures"].(float64)) != 1 {
		t.Fatalf("expected failures=1 in JSON, got %v", parsed["failures"])
	}
	if parsed["total_cost_usd"].(float64) != 0.35 {
		t.Fatalf("expected total_cost_usd=0.35 in JSON, got %v", parsed["total_cost_usd"])
	}

	sessions := parsed["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session in JSON, got %d", len(sessions))
	}
}

func TestRunSummary_PerSessionBreakdown(t *testing.T) {
	rs := NewRunSummary()

	for range 5 {
		rs.RecordCycle(CycleResult{
			SessionID: "alpha",
			Success:   true,
			CostUSD:   0.01,
			Duration:  time.Second,
		})
	}
	for i := range 3 {
		rs.RecordCycle(CycleResult{
			SessionID: "beta",
			Success:   i < 2,
			CostUSD:   0.02,
			Duration:  2 * time.Second,
		})
	}

	data, err := rs.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var snap summarySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if snap.TotalCycles != 8 {
		t.Fatalf("expected 8 total cycles, got %d", snap.TotalCycles)
	}

	found := map[string]bool{}
	for _, sb := range snap.Sessions {
		found[sb.SessionID] = true
		switch sb.SessionID {
		case "alpha":
			if sb.Cycles != 5 || sb.Successes != 5 || sb.Failures != 0 {
				t.Fatalf("alpha breakdown wrong: %+v", sb)
			}
		case "beta":
			if sb.Cycles != 3 || sb.Successes != 2 || sb.Failures != 1 {
				t.Fatalf("beta breakdown wrong: %+v", sb)
			}
		}
	}
	if !found["alpha"] || !found["beta"] {
		t.Fatal("missing session in breakdown")
	}
}

func TestRunSummary_RenderHeader(t *testing.T) {
	rs := NewRunSummary()
	out := rs.Render()
	if !strings.HasPrefix(out, "=== Marathon Run Summary ===") {
		t.Fatalf("expected header, got:\n%s", out)
	}
}

func TestRunSummary_ConcurrentAccess(t *testing.T) {
	rs := NewRunSummary()

	done := make(chan struct{})
	for i := range 10 {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := range 50 {
				rs.RecordCycle(CycleResult{
					SessionID: "concurrent",
					Success:   j%2 == 0,
					CostUSD:   0.001,
					Duration:  time.Millisecond,
				})
				rs.Render()
				_, _ = rs.JSON()
			}
		}(i)
	}
	for range 10 {
		<-done
	}

	data, err := rs.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}
	var snap summarySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.TotalCycles != 500 {
		t.Fatalf("expected 500 total cycles, got %d", snap.TotalCycles)
	}
}
