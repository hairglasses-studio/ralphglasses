package mcpserver

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/telemetry"
)

type AutobuildTriggerSignal struct {
	Type               string   `json:"type"`
	Source             string   `json:"source"`
	Summary            string   `json:"summary"`
	RemoteMainVerified *bool    `json:"remote_main_verified,omitempty"`
	MatchedWorkflows   []string `json:"matched_workflows,omitempty"`
	MatchedSurfaces    []string `json:"matched_surfaces,omitempty"`
}

type AutobuildPatchCandidate struct {
	PatchID                 string                 `json:"patch_id"`
	Priority                string                 `json:"priority"`
	QueueRank               int                    `json:"queue_rank"`
	Score                   int                    `json:"score"`
	Confidence              float64                `json:"confidence"`
	ConfidenceLabel         string                 `json:"confidence_label"`
	RecommendedEntrySurface string                 `json:"recommended_entry_surface"`
	RepoOwnedScope          []string               `json:"repo_owned_scope,omitempty"`
	WhyNow                  []string               `json:"why_now,omitempty"`
	TriggerSignal           AutobuildTriggerSignal `json:"trigger_signal"`
}

type AutobuildTrancheSummary struct {
	Source                 string                    `json:"source"`
	GeneratedAt            string                    `json:"generated_at"`
	DiscoveryUsagePath     string                    `json:"discovery_usage_path"`
	ToolBenchPath          string                    `json:"tool_bench_path"`
	WorkflowCandidateCount int                       `json:"workflow_candidate_count"`
	SurfaceCandidateCount  int                       `json:"surface_candidate_count"`
	HighestPriorityPatch   string                    `json:"highest_priority_patch,omitempty"`
	Candidates             []AutobuildPatchCandidate `json:"candidates,omitempty"`
}

type autobuildCandidateDef struct {
	PatchID                 string
	Priority                string
	QueueRank               int
	BaseScore               int
	RecommendedEntrySurface string
	RepoOwnedScope          []string
	WhyNow                  []string
	RelevantWorkflows       []string
}

var autobuildCandidateDefs = []autobuildCandidateDef{
	{
		PatchID:                 "remote_main_red_signal_filter",
		Priority:                "P1",
		QueueRank:               1,
		BaseScore:               90,
		RecommendedEntrySurface: "ralph:///runtime/operator",
		RepoOwnedScope: []string{
			"red-signal intake",
			"remote-main verification metadata",
			"dirty-worktree filtering",
		},
		WhyNow: []string{
			"Stale local red state still burns operator time when runtime-heavy workflows are the active front door.",
			"The selector should bias toward a red-signal filter when session or operator control paths remain under-adopted.",
		},
		RelevantWorkflows: []string{"operator-control-plane", "session-execution", "runtime-recovery"},
	},
	{
		PatchID:                 "generated_surface_drift_gate",
		Priority:                "P2",
		QueueRank:               2,
		BaseScore:               72,
		RecommendedEntrySurface: "ralph:///catalog/provider-parity",
		RepoOwnedScope: []string{
			"generated runtime docs",
			"provider-parity docs",
			"drift verification",
		},
		WhyNow: []string{
			"Provider-parity and discovery fronts are already active enough that generated drift is a plausible next integrity failure class.",
			"The selector should promote a generated-surface gate when provider-parity workflows still dominate inactive adoption gaps.",
		},
		RelevantWorkflows: []string{"provider-parity", "discover-and-load"},
	},
	{
		PatchID:                 "telemetry_to_patch_feedback",
		Priority:                "P2",
		QueueRank:               3,
		BaseScore:               64,
		RecommendedEntrySurface: "docs/autobuild-execution-ledger.json",
		RepoOwnedScope: []string{
			"tranche metadata",
			"signal capture",
			"closure evidence",
			"ranking feedback",
		},
		WhyNow: []string{
			"The selector is more valuable once adoption telemetry can be tied back to which signals actually produced worthwhile patches.",
			"Existing discovery and priority fronts now have enough structure to feed a tranche-feedback loop instead of remaining one-way telemetry.",
		},
		RelevantWorkflows: []string{"discover-and-load", "provider-parity", "repo-triage"},
	},
}

func (s *Server) autobuildTrancheSummary() AutobuildTrancheSummary {
	discovery := s.discoveryAdoptionSummary()
	adoption := s.adoptionPrioritySummary()

	summary := AutobuildTrancheSummary{
		Source:                 "adoption_priority_summary + discovery_adoption_summary + autobuild candidate mapping",
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
		DiscoveryUsagePath:     discovery.DiscoveryUsagePath,
		ToolBenchPath:          discovery.ToolBenchPath,
		WorkflowCandidateCount: adoption.WorkflowCandidateCount,
		SurfaceCandidateCount:  adoption.SurfaceCandidateCount,
	}

	candidates := s.activeRedSignalCandidates()
	for _, def := range autobuildCandidateDefs {
		workflowMatches := matchingWorkflowCandidates(def.RelevantWorkflows, adoption.TopWorkflowCandidates)
		surfaceMatches := matchingSurfaceCandidates(def.RelevantWorkflows, adoption.TopSurfaceCandidates)
		score := def.BaseScore + workflowMatchBoost(workflowMatches) + surfaceMatchBoost(surfaceMatches)
		if discovery.DiscoveryTelemetryPresent {
			score += 6
		}
		if discovery.ToolBenchPresent {
			score += 6
		}

		confidence := 0.2
		if discovery.DiscoveryTelemetryPresent {
			confidence += 0.2
		}
		if discovery.ToolBenchPresent {
			confidence += 0.2
		}
		confidence += 0.08 * float64(len(workflowMatches))
		confidence += 0.04 * float64(len(surfaceMatches))
		if containsString(def.RelevantWorkflows, adoption.HighestPriorityWorkflow) {
			confidence += 0.15
		}
		if confidence > 0.95 {
			confidence = 0.95
		}

		candidates = append(candidates, AutobuildPatchCandidate{
			PatchID:                 def.PatchID,
			Priority:                def.Priority,
			QueueRank:               def.QueueRank,
			Score:                   score,
			Confidence:              confidence,
			ConfidenceLabel:         confidenceLabel(confidence),
			RecommendedEntrySurface: def.RecommendedEntrySurface,
			RepoOwnedScope:          append([]string(nil), def.RepoOwnedScope...),
			WhyNow:                  append([]string(nil), def.WhyNow...),
			TriggerSignal: AutobuildTriggerSignal{
				Type:             "adoption",
				Source:           "ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption",
				Summary:          buildAutobuildSignalSummary(def.RelevantWorkflows, workflowMatches, surfaceMatches, discovery),
				MatchedWorkflows: workflowCandidateNames(workflowMatches),
				MatchedSurfaces:  surfaceCandidateNames(surfaceMatches),
			},
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].QueueRank < candidates[j].QueueRank
		}
		return candidates[i].Score > candidates[j].Score
	})
	if len(candidates) > 0 {
		summary.HighestPriorityPatch = candidates[0].PatchID
	}
	summary.Candidates = candidates
	return summary
}

func matchingWorkflowCandidates(relevant []string, candidates []AdoptionWorkflowCandidate) []AdoptionWorkflowCandidate {
	matches := make([]AdoptionWorkflowCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if containsString(relevant, candidate.Name) {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func matchingSurfaceCandidates(relevantWorkflows []string, candidates []AdoptionPriorityItem) []AdoptionPriorityItem {
	matches := make([]AdoptionPriorityItem, 0, len(candidates))
	for _, candidate := range candidates {
		if stringListsIntersect(relevantWorkflows, candidate.RelatedWorkflows) {
			matches = append(matches, candidate)
		}
	}
	return matches
}

func workflowMatchBoost(matches []AdoptionWorkflowCandidate) int {
	boost := 0
	for i, match := range matches {
		if i >= 2 {
			break
		}
		boost += match.PriorityScore / 4
	}
	return boost
}

func surfaceMatchBoost(matches []AdoptionPriorityItem) int {
	boost := 0
	for i, match := range matches {
		if i >= 3 {
			break
		}
		boost += match.PriorityScore / 8
	}
	return boost
}

func workflowCandidateNames(matches []AdoptionWorkflowCandidate) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match.Name)
	}
	return out
}

func surfaceCandidateNames(matches []AdoptionPriorityItem) []string {
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, fmt.Sprintf("%s:%s", match.Kind, match.Name))
	}
	return out
}

func buildAutobuildSignalSummary(
	relevantWorkflows []string,
	workflowMatches []AdoptionWorkflowCandidate,
	surfaceMatches []AdoptionPriorityItem,
	discovery DiscoveryAdoptionSummary,
) string {
	var parts []string
	if len(workflowMatches) > 0 {
		parts = append(parts, "matched workflows "+summarizeNames(workflowCandidateNames(workflowMatches), 2))
	} else {
		parts = append(parts, "no direct workflow match across "+summarizeNames(relevantWorkflows, 2))
	}
	if len(surfaceMatches) > 0 {
		parts = append(parts, "related inactive surfaces "+summarizeNames(surfaceCandidateNames(surfaceMatches), 2))
	}
	if discovery.DiscoveryTelemetryPresent {
		parts = append(parts, "discovery telemetry present")
	}
	if discovery.ToolBenchPresent {
		parts = append(parts, "tool benchmark telemetry present")
	}
	return strings.Join(parts, "; ") + "."
}

func confidenceLabel(confidence float64) string {
	switch {
	case confidence >= 0.75:
		return "high"
	case confidence >= 0.5:
		return "medium"
	default:
		return "low"
	}
}

func stringListsIntersect(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	set := stringSliceSet(left)
	for _, value := range right {
		if _, ok := set[value]; ok {
			return true
		}
	}
	return false
}

func (s *Server) activeRedSignalCandidates() []AutobuildPatchCandidate {
	events, err := parity.LoadTelemetry(parity.TelemetryOptions{
		Type: string(telemetry.EventCrash),
		Repo: "ralphglasses",
	})
	if err != nil {
		return nil
	}

	var candidates []AutobuildPatchCandidate
	// Keep track of which sessions we've generated a candidate for
	seen := make(map[string]bool)

	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.SessionID == "" || seen[ev.SessionID] {
			continue
		}

		verified, _ := ev.Data["remote_main_verified"].(bool)
		dirty, _ := ev.Data["dirty_worktree"].(bool)

		if dirty || !verified {
			continue
		}

		seen[ev.SessionID] = true
		tTrue := true

		candidates = append(candidates, AutobuildPatchCandidate{
			PatchID:                 fmt.Sprintf("integrity_crash_%s", ev.SessionID),
			Priority:                "P0",
			QueueRank:               0,
			Score:                   1000,
			Confidence:              1.0,
			ConfidenceLabel:         "high",
			RecommendedEntrySurface: "ralph:///runtime/recovery",
			RepoOwnedScope:          []string{"crash repair"},
			WhyNow:                  []string{"Red signal on remote main requires immediate fix"},
			TriggerSignal: AutobuildTriggerSignal{
				Type:               "integrity",
				Source:             "telemetry.EventCrash",
				Summary:            fmt.Sprintf("Crash in session %s requires fix", ev.SessionID),
				RemoteMainVerified: &tTrue,
			},
		})
	}
	return candidates
}
