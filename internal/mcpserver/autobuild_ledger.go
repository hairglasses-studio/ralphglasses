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
	Result                  *LedgerResult       `json:"result,omitempty"`
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
