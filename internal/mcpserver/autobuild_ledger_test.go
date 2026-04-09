package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAutobuildTrancheSummary_AppliesFeedbackBoosts(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_STATE_HOME", tmpHome)

	appSrv := NewServer(t.TempDir())

	docsDir := filepath.Join(appSrv.ScanPath, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ledgerContent := `{
	"entries": [
		{
			"patch_id": "telemetry_to_patch_feedback",
			"status": "completed",
			"trigger_signal": {
				"type": "adoption",
				"source": "docs/autobuild-patch-queue.json",
				"summary": "something",
				"remote_main_verified": true
			},
			"result": {
				"closure_state": "completed",
				"summary": "success",
				"prevented_failure_class": ["test"]
			},
			"next_recommended_patch": "remote_main_red_signal_filter"
		}
	]
}`

	if err := os.WriteFile(filepath.Join(docsDir, "autobuild-execution-ledger.json"), []byte(ledgerContent), 0644); err != nil {
		t.Fatal(err)
	}

	summary := appSrv.autobuildTrancheSummary()

	// "remote_main_red_signal_filter" should get a +20 boost
	// "adoption" type gets +5 boost
	var foundFilter bool
	var foundDriftGate bool
	for _, cand := range summary.Candidates {
		if cand.PatchID == "remote_main_red_signal_filter" {
			foundFilter = true
			if cand.FeedbackScore != 25 { // 20 from PatchScoreBoost, 5 from TypeScoreBoost
				t.Errorf("expected FeedbackScore of 25 for remote_main_red_signal_filter, got %d", cand.FeedbackScore)
			}
		}
		if cand.PatchID == "generated_surface_drift_gate" {
			foundDriftGate = true
			if cand.FeedbackScore != 5 { // 0 from PatchScoreBoost, 5 from TypeScoreBoost
				t.Errorf("expected FeedbackScore of 5 for generated_surface_drift_gate, got %d", cand.FeedbackScore)
			}
		}
	}

	if !foundFilter || !foundDriftGate {
		t.Fatal("expected candidates not found in summary")
	}
}
