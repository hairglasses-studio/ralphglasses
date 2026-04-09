package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutobuildTrancheSummary_ReturnsRankedPatchCandidates(t *testing.T) {
	t.Parallel()

	scanDir := t.TempDir()
	appSrv := NewServer(scanDir)
	ralphDir := filepath.Join(scanDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "discovery_usage.jsonl"), []byte(
		`{"kind":"resource","name":"ralph:///catalog/provider-parity","ts":"2026-04-08T11:00:00Z"}`+"\n"+
			`{"kind":"resource","name":"ralph:///runtime/operator","ts":"2026-04-08T11:01:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "tool_benchmarks.jsonl"), []byte(
		`{"tool":"ralphglasses_marathon","ts":"2026-04-08T11:02:00Z","latency_ms":10,"ok":true}`+"\n"+
			`{"tool":"ralphglasses_session_status","ts":"2026-04-08T11:03:00Z","latency_ms":9,"ok":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := appSrv.autobuildTrancheSummary()
	if summary.HighestPriorityPatch != "remote_main_red_signal_filter" {
		t.Fatalf("HighestPriorityPatch = %q, want remote_main_red_signal_filter", summary.HighestPriorityPatch)
	}
	if len(summary.Candidates) != len(autobuildCandidateDefs) {
		t.Fatalf("candidate count = %d, want %d", len(summary.Candidates), len(autobuildCandidateDefs))
	}
	first := summary.Candidates[0]
	if first.RecommendedEntrySurface == "" {
		t.Fatal("expected recommended entry surface")
	}
	if first.Confidence <= 0 {
		t.Fatalf("confidence = %f, want > 0", first.Confidence)
	}
	if len(first.TriggerSignal.MatchedWorkflows) == 0 {
		t.Fatalf("expected matched workflows in first candidate: %+v", first)
	}
	if first.TriggerSignal.Source == "" || first.TriggerSignal.Summary == "" {
		t.Fatalf("expected populated trigger signal: %+v", first.TriggerSignal)
	}
}

func TestAutobuildTrancheSummary_FiltersRedSignals(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_STATE_HOME", tmpHome)

	telemetryDir := filepath.Join(tmpHome, "ralphglasses")
	if err := os.MkdirAll(telemetryDir, 0755); err != nil {
		t.Fatal(err)
	}

	appSrv := NewServer(t.TempDir())

	// Write telemetry events
	events := `{"type":"crash","timestamp":"2026-04-08T12:00:00Z","session_id":"s1","repo_name":"ralphglasses","data":{"remote_main_verified":true,"dirty_worktree":false}}
{"type":"crash","timestamp":"2026-04-08T12:01:00Z","session_id":"s2","repo_name":"ralphglasses","data":{"remote_main_verified":false,"dirty_worktree":false}}
{"type":"crash","timestamp":"2026-04-08T12:02:00Z","session_id":"s3","repo_name":"ralphglasses","data":{"remote_main_verified":true,"dirty_worktree":true}}
`
	if err := os.WriteFile(filepath.Join(telemetryDir, "telemetry.jsonl"), []byte(events), 0644); err != nil {
		t.Fatal(err)
	}

	summary := appSrv.autobuildTrancheSummary()

	// Should only have s1 as a candidate, prepended to the static list
	// Wait, the static list has 3 items. Total = 4.
	if len(summary.Candidates) != len(autobuildCandidateDefs)+1 {
		t.Fatalf("expected %d candidates, got %d", len(autobuildCandidateDefs)+1, len(summary.Candidates))
	}

	if summary.Candidates[0].PatchID != "integrity_crash_s1" {
		t.Fatalf("Highest priority patch should be integrity_crash_s1, got %q", summary.Candidates[0].PatchID)
	}
}
