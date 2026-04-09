package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeFeedbackBoosts_IncludesSignalWorkflowAndSurfaceBoosts(t *testing.T) {
	ledger := &AutobuildExecutionLedger{
		Entries: []LedgerEntry{{
			PatchID: "telemetry_to_patch_feedback",
			Status:  "completed",
			TriggerSignal: LedgerTriggerSignal{
				Type:               "adoption",
				Source:             "ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption",
				Summary:            "discovery and adoption fronts aligned",
				SignalKey:          "adoption::ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption",
				RemoteMainVerified: true,
				MatchedWorkflows:   []string{"discover-and-load", "discover-and-load", "provider-parity"},
				MatchedSurfaces:    []string{"surface:test-one", "surface:test-one", "surface:test-two"},
			},
			Result: &LedgerResult{
				ClosureState:          "completed",
				Summary:               "success",
				PreventedFailureClass: []string{"test"},
			},
			NextRecommendedPatch: "telemetry_to_patch_feedback",
		}},
	}

	boosts := computeFeedbackBoosts(ledger)
	if got := boosts.PatchScoreBoost["telemetry_to_patch_feedback"]; got != 20 {
		t.Fatalf("expected patch boost 20, got %d", got)
	}
	if got := boosts.TypeScoreBoost["adoption"]; got != 5 {
		t.Fatalf("expected type boost 5, got %d", got)
	}
	if got := boosts.SignalKeyBoost["adoption::ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption"]; got != 8 {
		t.Fatalf("expected signal key boost 8, got %d", got)
	}
	if got := boosts.WorkflowScoreBoost["discover-and-load"]; got != 4 {
		t.Fatalf("expected discover-and-load workflow boost 4, got %d", got)
	}
	if got := boosts.WorkflowScoreBoost["provider-parity"]; got != 4 {
		t.Fatalf("expected provider-parity workflow boost 4, got %d", got)
	}
	if got := boosts.SurfaceScoreBoost["surface:test-one"]; got != 2 {
		t.Fatalf("expected surface:test-one boost 2, got %d", got)
	}
	if got := boosts.SurfaceScoreBoost["surface:test-two"]; got != 2 {
		t.Fatalf("expected surface:test-two boost 2, got %d", got)
	}
}

func TestComputeFeedbackBoosts_IgnoresNonCompletedEntries(t *testing.T) {
	ledger := &AutobuildExecutionLedger{
		Entries: []LedgerEntry{
			{
				TriggerSignal: LedgerTriggerSignal{
					Type:      "adoption",
					Source:    "docs/autobuild-patch-queue.json",
					SignalKey: "adoption::docs/autobuild-patch-queue.json",
				},
				NextRecommendedPatch: "telemetry_to_patch_feedback",
			},
			{
				TriggerSignal: LedgerTriggerSignal{
					Type:      "integrity",
					Source:    "telemetry.EventCrash",
					SignalKey: "integrity::telemetry.EventCrash",
				},
				Result: &LedgerResult{
					ClosureState: "blocked",
					Summary:      "not done",
				},
				NextRecommendedPatch: "remote_main_red_signal_filter",
			},
		},
	}

	boosts := computeFeedbackBoosts(ledger)
	if got := boosts.PatchScoreBoost["telemetry_to_patch_feedback"]; got != 0 {
		t.Fatalf("expected no boost for nil result entry, got %d", got)
	}
	if got := boosts.PatchScoreBoost["remote_main_red_signal_filter"]; got != 0 {
		t.Fatalf("expected no boost for blocked entry, got %d", got)
	}
	if got := boosts.TypeScoreBoost["adoption"]; got != 0 {
		t.Fatalf("expected no type boost for incomplete entry, got %d", got)
	}
	if got := boosts.SignalKeyBoost["integrity::telemetry.EventCrash"]; got != 0 {
		t.Fatalf("expected no signal key boost for blocked entry, got %d", got)
	}
}

func TestCappedFeedbackReasons_AppliesCapsAndDeduplicates(t *testing.T) {
	boosts := map[string]int{
		"discover-and-load": 4,
		"provider-parity":   4,
		"repo-triage":       4,
	}

	total, reasons := cappedFeedbackReasons(boosts, []string{
		"discover-and-load",
		"discover-and-load",
		"provider-parity",
		"repo-triage",
	}, 8, "workflow")

	if total != 8 {
		t.Fatalf("expected capped total of 8, got %d", total)
	}
	if len(reasons) != 2 {
		t.Fatalf("expected two applied reasons, got %d (%v)", len(reasons), reasons)
	}
	if !contains(reasons[0], "workflow discover-and-load +4") {
		t.Fatalf("expected discover-and-load reason, got %v", reasons)
	}
	if !contains(reasons[1], "workflow provider-parity +4") {
		t.Fatalf("expected provider-parity reason, got %v", reasons)
	}
}

func TestAutobuildTrancheSummary_AppliesPatchTypeAndSignalFeedback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_STATE_HOME", tmpHome)

	appSrv := NewServer(t.TempDir())

	docsDir := filepath.Join(appSrv.ScanPath, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ledgerContent := `{
    "entries": [
        {
            "patch_id": "seed_feedback",
            "status": "completed",
            "trigger_signal": {
                "type": "adoption",
                "source": "ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption",
                "summary": "something",
                "signal_key": "adoption::ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption",
                "remote_main_verified": true
            },
            "result": {
                "closure_state": "completed",
                "summary": "success",
                "prevented_failure_class": ["test"]
            },
            "next_recommended_patch": "telemetry_to_patch_feedback"
        }
    ]
}`

	if err := os.WriteFile(filepath.Join(docsDir, "autobuild-execution-ledger.json"), []byte(ledgerContent), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := appSrv.autobuildTrancheSummary()

	var foundTelemetry bool
	var foundDriftGate bool
	for _, cand := range summary.Candidates {
		switch cand.PatchID {
		case "telemetry_to_patch_feedback":
			foundTelemetry = true
			if cand.FeedbackScore != 33 {
				t.Fatalf("expected feedback score 33 for telemetry_to_patch_feedback, got %d", cand.FeedbackScore)
			}
			if !contains(cand.FeedbackSummary, "patch telemetry_to_patch_feedback +20") {
				t.Fatalf("expected patch feedback explanation, got %q", cand.FeedbackSummary)
			}
			if !contains(cand.FeedbackSummary, "signal type adoption +5") {
				t.Fatalf("expected type feedback explanation, got %q", cand.FeedbackSummary)
			}
			if !contains(cand.FeedbackSummary, "signal adoption::ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption +8") {
				t.Fatalf("expected signal-key feedback explanation, got %q", cand.FeedbackSummary)
			}
		case "generated_surface_drift_gate":
			foundDriftGate = true
			if cand.FeedbackScore != 13 {
				t.Fatalf("expected feedback score 13 for generated_surface_drift_gate, got %d", cand.FeedbackScore)
			}
		}
	}

	if !foundTelemetry || !foundDriftGate {
		t.Fatal("expected candidates not found in summary")
	}
}

func TestAppendExecutionLedgerEntry_PreservesMetadataAndEmitsTelemetry(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_STATE_HOME", tmpHome)

	scanDir := t.TempDir()
	docsDir := filepath.Join(scanDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	initialLedger := `{
    "$schema": "./autobuild-execution-ledger.schema.json",
    "track": "autobuild-execution-ledger",
    "repo": "hairglasses-studio/ralphglasses",
    "description": "test ledger",
    "instructions": ["preserve metadata"],
    "entry_template": {
        "patch_id": "<patch-id>"
    },
    "entries": []
}`
	if err := os.WriteFile(filepath.Join(docsDir, "autobuild-execution-ledger.json"), []byte(initialLedger), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := LedgerEntry{
		Date:    "2026-04-09",
		PatchID: "test_patch",
		Status:  "completed",
		TriggerSignal: LedgerTriggerSignal{
			Type:               "adoption",
			Source:             "docs/autobuild-patch-queue.json",
			Summary:            "test trigger",
			SignalKey:          "adoption::docs/autobuild-patch-queue.json",
			RemoteMainVerified: true,
		},
		RepoOwnedScope:          []string{"feedback scoring"},
		RecommendedEntrySurface: "docs/autobuild-execution-ledger.json",
		Changes:                 []string{"captured richer signal metadata"},
		AcceptanceCondition:     []string{"telemetry contains signal attribution"},
		StopCondition:           []string{"stop after append path succeeds"},
		Evidence: []LedgerEvidence{{
			Kind:    "file",
			Ref:     "docs/autobuild-execution-ledger.json",
			Summary: "ledger updated",
		}},
		EvidencePath: "docs/autobuild-execution-ledger.json",
		Result: &LedgerResult{
			ClosureState:          "completed",
			Summary:               "success",
			PreventedFailureClass: []string{"missing telemetry attribution"},
		},
		NextRecommendedPatch: "remote_main_red_signal_filter",
		Notes:                []string{"test note"},
	}

	if err := AppendExecutionLedgerEntry(scanDir, entry); err != nil {
		t.Fatalf("AppendExecutionLedgerEntry failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(docsDir, "autobuild-execution-ledger.json"))
	if err != nil {
		t.Fatal(err)
	}
	ledgerText := string(data)
	if !contains(ledgerText, `"$schema": "./autobuild-execution-ledger.schema.json"`) {
		t.Fatalf("ledger file lost schema metadata: %s", ledgerText)
	}
	if !contains(ledgerText, `"track": "autobuild-execution-ledger"`) {
		t.Fatalf("ledger file lost track metadata: %s", ledgerText)
	}
	if !contains(ledgerText, `"signal_key": "adoption::docs/autobuild-patch-queue.json"`) {
		t.Fatalf("ledger file missing signal_key: %s", ledgerText)
	}
	if !contains(ledgerText, `"evidence_path": "docs/autobuild-execution-ledger.json"`) {
		t.Fatalf("ledger file missing evidence_path: %s", ledgerText)
	}
	if !contains(ledgerText, "test_patch") {
		t.Fatalf("ledger file does not contain appended patch: %s", ledgerText)
	}

	telemetryPath := filepath.Join(tmpHome, "ralphglasses", "telemetry.jsonl")
	telData, err := os.ReadFile(telemetryPath)
	if err != nil {
		t.Fatal(err)
	}
	telemetryText := string(telData)
	if !contains(telemetryText, "tranche_close") || !contains(telemetryText, "test_patch") {
		t.Fatalf("telemetry missing tranche_close event: %s", telemetryText)
	}
	if !contains(telemetryText, "signal_key") || !contains(telemetryText, "adoption::docs/autobuild-patch-queue.json") {
		t.Fatalf("telemetry missing signal attribution: %s", telemetryText)
	}
	if !contains(telemetryText, "evidence_path") || !contains(telemetryText, "docs/autobuild-execution-ledger.json") {
		t.Fatalf("telemetry missing evidence path: %s", telemetryText)
	}
}

// contains is defined in tools_deferred_test.go
