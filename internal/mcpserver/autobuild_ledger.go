package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type LedgerTriggerSignal struct {
	Type               string `json:"type"`
	Source             string `json:"source"`
	Summary            string `json:"summary"`
	RemoteMainVerified bool   `json:"remote_main_verified"`
}

type LedgerEvidence struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Summary string `json:"summary"`
}

type LedgerResult struct {
	ClosureState          string   `json:"closure_state"`
	Summary               string   `json:"summary"`
	PreventedFailureClass []string `json:"prevented_failure_class"`
}

type LedgerEntry struct {
	Date                    string              `json:"date"`
	PatchID                 string              `json:"patch_id"`
	Status                  string              `json:"status"`
	TriggerSignal           LedgerTriggerSignal `json:"trigger_signal"`
	RepoOwnedScope          []string            `json:"repo_owned_scope"`
	RecommendedEntrySurface string              `json:"recommended_entry_surface"`
	Changes                 []string            `json:"changes"`
	AcceptanceCondition     []string            `json:"acceptance_condition"`
	StopCondition           []string            `json:"stop_condition"`
	Evidence                []LedgerEvidence    `json:"evidence"`
	Result                  *LedgerResult       `json:"result,omitempty"`
	NextRecommendedPatch    string              `json:"next_recommended_patch,omitempty"`
}

type AutobuildExecutionLedger struct {
	Entries []LedgerEntry `json:"entries"`
}

func loadAutobuildLedger(scanPath string) (*AutobuildExecutionLedger, error) {
	path := filepath.Join(scanPath, "docs", "autobuild-execution-ledger.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ledger AutobuildExecutionLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return nil, err
	}
	return &ledger, nil
}

type FeedbackBoosts struct {
	PatchScoreBoost map[string]int
	TypeScoreBoost  map[string]int
}

func computeFeedbackBoosts(ledger *AutobuildExecutionLedger) FeedbackBoosts {
	boosts := FeedbackBoosts{
		PatchScoreBoost: make(map[string]int),
		TypeScoreBoost:  make(map[string]int),
	}
	if ledger == nil {
		return boosts
	}

	for _, entry := range ledger.Entries {
		if entry.Result != nil && entry.Result.ClosureState == "completed" {
			if entry.NextRecommendedPatch != "" {
				boosts.PatchScoreBoost[entry.NextRecommendedPatch] += 20
			}
			if entry.TriggerSignal.Type != "" {
				boosts.TypeScoreBoost[entry.TriggerSignal.Type] += 5
			}
		}
	}
	return boosts
}
