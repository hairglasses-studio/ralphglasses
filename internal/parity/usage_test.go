package parity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIParityUsage_Summary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	benchPath := filepath.Join(dir, "tool_benchmarks.jsonl")
	now := time.Date(2026, time.April, 8, 12, 0, 0, 0, time.UTC)
	data := strings.Join([]string{
		`{"tool":"ralphglasses_doctor","ts":"2026-04-08T11:00:00Z","latency_ms":10,"ok":true}`,
		`{"tool":"ralphglasses_doctor","ts":"2026-04-08T11:30:00Z","latency_ms":12,"ok":true}`,
		`{"tool":"ralphglasses_repo_scaffold","ts":"2026-04-08T10:00:00Z","latency_ms":15,"ok":true}`,
		`{"tool":"ralphglasses_session_status","ts":"2026-04-08T09:00:00Z","latency_ms":5,"ok":true}`,
		`{"tool":"ralphglasses_scan","ts":"2026-04-08T08:00:00Z","latency_ms":20,"ok":true}`,
		`{"tool":"ralphglasses_marathon","ts":"2026-01-01T00:00:00Z","latency_ms":50,"ok":true}`,
	}, "\n") + "\n"
	if err := os.WriteFile(benchPath, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	summary := CLIParityUsage(CLIParityUsageOptions{
		BenchPath: benchPath,
		Since:     now.Add(-7 * 24 * time.Hour),
		Until:     now,
	})

	if !summary.TelemetryAvailable {
		t.Fatal("expected telemetry to be available")
	}
	if summary.TotalToolCalls != 5 {
		t.Fatalf("TotalToolCalls = %d, want 5", summary.TotalToolCalls)
	}
	if summary.MatchedToolCalls != 4 {
		t.Fatalf("MatchedToolCalls = %d, want 4", summary.MatchedToolCalls)
	}
	if summary.ObservableSurfaces != 19 {
		t.Fatalf("ObservableSurfaces = %d, want 19", summary.ObservableSurfaces)
	}
	if summary.ActiveObservableSurfaces != 4 {
		t.Fatalf("ActiveObservableSurfaces = %d, want 4", summary.ActiveObservableSurfaces)
	}
	if summary.ObservableCoveragePct != 21.1 {
		t.Fatalf("ObservableCoveragePct = %.1f, want 21.1", summary.ObservableCoveragePct)
	}
	if len(summary.UninstrumentedCovered) != 2 {
		t.Fatalf("UninstrumentedCovered = %d, want 2", len(summary.UninstrumentedCovered))
	}
	if got := summary.UninstrumentedCovered[0]; got != "ralphglasses root TUI" && got != "ralphglasses tmux list/attach/detach" {
		t.Fatalf("unexpected first uninstrumented surface: %q", got)
	}
	if len(summary.TopActiveSurfaces) == 0 {
		t.Fatal("expected active surfaces in usage summary")
	}
	if summary.TopActiveSurfaces[0].Surface != "ralphglasses doctor" {
		t.Fatalf("top active surface = %q, want ralphglasses doctor", summary.TopActiveSurfaces[0].Surface)
	}
	if summary.TopActiveSurfaces[0].CallCount != 2 {
		t.Fatalf("doctor call count = %d, want 2", summary.TopActiveSurfaces[0].CallCount)
	}
}

func TestCLIParityUsage_MissingFile(t *testing.T) {
	t.Parallel()

	summary := CLIParityUsage(CLIParityUsageOptions{
		BenchPath: filepath.Join(t.TempDir(), "missing.jsonl"),
	})
	if summary.TelemetryAvailable {
		t.Fatal("expected telemetry to be unavailable for missing file")
	}
	if summary.TotalToolCalls != 0 {
		t.Fatalf("TotalToolCalls = %d, want 0", summary.TotalToolCalls)
	}
	if summary.LoadError != "" {
		t.Fatalf("LoadError = %q, want empty", summary.LoadError)
	}
}
