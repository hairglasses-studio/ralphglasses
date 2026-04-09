package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/telemetry"
	"github.com/mark3labs/mcp-go/mcp"
)

type LedgerTriggerSignal struct {
	Type               string   `json:"type"`
	Source             string   `json:"source"`
	Summary            string   `json:"summary"`
	SignalKey          string   `json:"signal_key,omitempty"`
	RemoteMainVerified bool     `json:"remote_main_verified"`
	MatchedWorkflows   []string `json:"matched_workflows,omitempty"`
	MatchedSurfaces    []string `json:"matched_surfaces,omitempty"`
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
	EvidencePath            string              `json:"evidence_path,omitempty"`
	Result                  *LedgerResult       `json:"result,omitempty"`
	NextRecommendedPatch    string              `json:"next_recommended_patch,omitempty"`
	Notes                   []string            `json:"notes,omitempty"`
}

type AutobuildExecutionLedger struct {
	Schema        string        `json:"$schema,omitempty"`
	Track         string        `json:"track,omitempty"`
	Repo          string        `json:"repo,omitempty"`
	Description   string        `json:"description,omitempty"`
	Instructions  []string      `json:"instructions,omitempty"`
	EntryTemplate *LedgerEntry  `json:"entry_template,omitempty"`
	Entries       []LedgerEntry `json:"entries"`
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
	PatchScoreBoost    map[string]int
	TypeScoreBoost     map[string]int
	SignalKeyBoost     map[string]int
	WorkflowScoreBoost map[string]int
	SurfaceScoreBoost  map[string]int
}

func computeFeedbackBoosts(ledger *AutobuildExecutionLedger) FeedbackBoosts {
	boosts := FeedbackBoosts{
		PatchScoreBoost:    make(map[string]int),
		TypeScoreBoost:     make(map[string]int),
		SignalKeyBoost:     make(map[string]int),
		WorkflowScoreBoost: make(map[string]int),
		SurfaceScoreBoost:  make(map[string]int),
	}
	if ledger == nil {
		return boosts
	}

	for _, entry := range ledger.Entries {
		if entry.Result == nil || entry.Result.ClosureState != "completed" {
			continue
		}
		if entry.NextRecommendedPatch != "" {
			boosts.PatchScoreBoost[entry.NextRecommendedPatch] += 20
		}
		if entry.TriggerSignal.Type != "" {
			boosts.TypeScoreBoost[entry.TriggerSignal.Type] += 5
		}
		if signalKey := ledgerSignalKey(entry.TriggerSignal); signalKey != "" {
			boosts.SignalKeyBoost[signalKey] += 8
		}
		workflowSeen := make(map[string]struct{}, len(entry.TriggerSignal.MatchedWorkflows))
		for _, workflow := range entry.TriggerSignal.MatchedWorkflows {
			if workflow == "" {
				continue
			}
			if _, ok := workflowSeen[workflow]; ok {
				continue
			}
			workflowSeen[workflow] = struct{}{}
			boosts.WorkflowScoreBoost[workflow] += 4
		}

		surfaceSeen := make(map[string]struct{}, len(entry.TriggerSignal.MatchedSurfaces))
		for _, surface := range entry.TriggerSignal.MatchedSurfaces {
			if surface == "" {
				continue
			}
			if _, ok := surfaceSeen[surface]; ok {
				continue
			}
			surfaceSeen[surface] = struct{}{}
			boosts.SurfaceScoreBoost[surface] += 2
		}
	}
	return boosts
}

func ledgerSignalKey(signal LedgerTriggerSignal) string {
	if signal.SignalKey != "" {
		return signal.SignalKey
	}
	if signal.Type == "" || signal.Source == "" {
		return ""
	}
	return signal.Type + "::" + signal.Source
}

// handleAutobuildLedgerAppend is the MCP tool handler for ralphglasses_autobuild_ledger_append.
func (s *Server) handleAutobuildLedgerAppend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	patchID, errResult := p.RequireString("patch_id")
	if errResult != nil {
		return errResult, nil
	}
	status, errResult := p.RequireString("status")
	if errResult != nil {
		return errResult, nil
	}

	entry := LedgerEntry{
		PatchID:                 patchID,
		Status:                  status,
		RecommendedEntrySurface: p.OptionalString("recommended_entry_surface", ""),
		NextRecommendedPatch:    p.OptionalString("next_recommended_patch", ""),
		Changes:                 p.OptionalStringList("changes"),
		AcceptanceCondition:     p.OptionalStringList("acceptance_condition"),
		StopCondition:           p.OptionalStringList("stop_condition"),
		RepoOwnedScope:          p.OptionalStringList("repo_owned_scope"),
		TriggerSignal: LedgerTriggerSignal{
			Type:               p.OptionalString("trigger_type", ""),
			Source:             p.OptionalString("trigger_source", ""),
			Summary:            p.OptionalString("trigger_summary", ""),
			RemoteMainVerified: p.OptionalBool("remote_main_verified", false),
		},
	}

	closureState := p.OptionalString("closure_state", "")
	if closureState != "" {
		entry.Result = &LedgerResult{
			ClosureState:          closureState,
			Summary:               p.OptionalString("closure_summary", ""),
			PreventedFailureClass: p.OptionalStringList("prevented_failure_class"),
		}
	}

	if err := AppendExecutionLedgerEntry(s.ScanPath, entry); err != nil {
		return codedError(ErrInternal, "ledger append failed: "+err.Error()), nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.NewTextContent(fmt.Sprintf("Ledger entry %q appended (status: %s)", patchID, status))},
	}, nil
}

func AppendExecutionLedgerEntry(scanPath string, entry LedgerEntry) error {
	if signalKey := ledgerSignalKey(entry.TriggerSignal); signalKey != "" {
		entry.TriggerSignal.SignalKey = signalKey
	}

	ledger, err := loadAutobuildLedger(scanPath)
	if err != nil {
		return err
	}
	ledger.Entries = append(ledger.Entries, entry)

	path := filepath.Join(scanPath, "docs", "autobuild-execution-ledger.json")
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	writer := telemetry.NewWriter()
	telemetryEvent := telemetry.Event{
		RepoName: "ralphglasses",
		Data: map[string]any{
			"patch_id": entry.PatchID,
			"status":   entry.Status,
		},
	}
	if signalKey := ledgerSignalKey(entry.TriggerSignal); signalKey != "" {
		telemetryEvent.Data["signal_key"] = signalKey
	}
	if entry.EvidencePath != "" {
		telemetryEvent.Data["evidence_path"] = entry.EvidencePath
	}
	if entry.Result != nil {
		telemetryEvent.Type = telemetry.EventTrancheClose
		telemetryEvent.Data["closure_state"] = entry.Result.ClosureState
	} else {
		telemetryEvent.Type = telemetry.EventTrancheOpen
	}

	return writer.Write(telemetryEvent)
}
