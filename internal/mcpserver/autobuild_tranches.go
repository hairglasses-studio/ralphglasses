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
	SignalKey          string   `json:"signal_key,omitempty"`
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
	FeedbackScore           int
	FeedbackSummary         string
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
		RelevantWorkflows: []string{"operator-control-plane", "session-execution", "runtime-recovery", "repo-hygiene"},
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
	ledger, _ := loadAutobuildLedger(s.ScanPath)
	feedback := computeFeedbackBoosts(ledger)

	summary := AutobuildTrancheSummary{
		Source:                 "adoption_priority_summary + discovery_adoption_summary + autobuild candidate mapping",
		GeneratedAt:            time.Now().UTC().Format(time.RFC3339),
		DiscoveryUsagePath:     discovery.DiscoveryUsagePath,
		ToolBenchPath:          discovery.ToolBenchPath,
		WorkflowCandidateCount: adoption.WorkflowCandidateCount,
		SurfaceCandidateCount:  adoption.SurfaceCandidateCount,
	}

	candidates := s.activeRedSignalCandidates(feedback)
	for _, def := range autobuildCandidateDefs {
		workflowMatches := matchingWorkflowCandidates(def.RelevantWorkflows, adoption.TopWorkflowCandidates)
		surfaceMatches := matchingSurfaceCandidates(def.RelevantWorkflows, adoption.TopSurfaceCandidates)
		workflowNames := workflowCandidateNames(workflowMatches)
		surfaceNames := surfaceCandidateNames(surfaceMatches)
		triggerSignal := AutobuildTriggerSignal{
			Type:             "adoption",
			Source:           "ralph:///catalog/adoption-priorities + ralph:///catalog/discovery-adoption",
			Summary:          buildAutobuildSignalSummary(def.RelevantWorkflows, workflowMatches, surfaceMatches, discovery),
			MatchedWorkflows: workflowNames,
			MatchedSurfaces:  surfaceNames,
		}
		triggerSignal.SignalKey = autobuildSignalKey(triggerSignal)

		feedbackBoost := 0
		feedbackReasons := make([]string, 0, 5)
		if boost := feedback.PatchScoreBoost[def.PatchID]; boost > 0 {
			feedbackBoost += boost
			feedbackReasons = append(feedbackReasons, fmt.Sprintf("patch %s +%d", def.PatchID, boost))
		}
		if boost := feedback.TypeScoreBoost[triggerSignal.Type]; boost > 0 {
			feedbackBoost += boost
			feedbackReasons = append(feedbackReasons, fmt.Sprintf("signal type %s +%d", triggerSignal.Type, boost))
		}
		if boost := feedback.SignalKeyBoost[triggerSignal.SignalKey]; boost > 0 {
			feedbackBoost += boost
			feedbackReasons = append(feedbackReasons, fmt.Sprintf("signal %s +%d", triggerSignal.SignalKey, boost))
		}
		workflowFeedback, workflowReasons := cappedFeedbackReasons(feedback.WorkflowScoreBoost, workflowNames, 8, "workflow")
		surfaceFeedback, surfaceReasons := cappedFeedbackReasons(feedback.SurfaceScoreBoost, surfaceNames, 6, "surface")
		feedbackBoost += workflowFeedback + surfaceFeedback
		feedbackReasons = append(feedbackReasons, workflowReasons...)
		feedbackReasons = append(feedbackReasons, surfaceReasons...)
		feedbackSummary := buildFeedbackSummary(feedbackReasons)

		score := def.BaseScore + workflowMatchBoost(workflowMatches) + surfaceMatchBoost(surfaceMatches) + feedbackBoost
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
			FeedbackScore:           feedbackBoost,
			FeedbackSummary:         feedbackSummary,
			Confidence:              confidence,
			ConfidenceLabel:         confidenceLabel(confidence),
			RecommendedEntrySurface: def.RecommendedEntrySurface,
			RepoOwnedScope:          append([]string(nil), def.RepoOwnedScope...),
			WhyNow:                  append([]string(nil), def.WhyNow...),
			TriggerSignal:           triggerSignal,
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
func autobuildSignalKey(signal AutobuildTriggerSignal) string {
	if signal.SignalKey != "" {
		return signal.SignalKey
	}
	if signal.Type == "" || signal.Source == "" {
		return ""
	}
	return signal.Type + "::" + signal.Source
}

func cappedFeedbackReasons(boosts map[string]int, names []string, capTotal int, kind string) (int, []string) {
	total := 0
	reasons := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		boost := boosts[name]
		if boost <= 0 {
			continue
		}
		if capTotal > 0 && total >= capTotal {
			break
		}
		applied := boost
		if capTotal > 0 && total+applied > capTotal {
			applied = capTotal - total
		}
		total += applied
		reasons = append(reasons, fmt.Sprintf("%s %s +%d", kind, name, applied))
	}
	return total, reasons
}

func buildFeedbackSummary(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	return "Historical completed tranches boosted this candidate via " + strings.Join(reasons, "; ") + "."
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

func (s *Server) activeRedSignalCandidates(feedback FeedbackBoosts) []AutobuildPatchCandidate {
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

		patchID := fmt.Sprintf("integrity_crash_%s", ev.SessionID)
		triggerSignal := AutobuildTriggerSignal{
			Type:               "integrity",
			Source:             "telemetry.EventCrash",
			Summary:            fmt.Sprintf("Crash in session %s requires fix", ev.SessionID),
			RemoteMainVerified: &tTrue,
		}
		triggerSignal.SignalKey = autobuildSignalKey(triggerSignal)

		feedbackBoost := 0
		feedbackReasons := make([]string, 0, 3)
		if boost := feedback.PatchScoreBoost[patchID]; boost > 0 {
			feedbackBoost += boost
			feedbackReasons = append(feedbackReasons, fmt.Sprintf("patch %s +%d", patchID, boost))
		}
		if boost := feedback.TypeScoreBoost[triggerSignal.Type]; boost > 0 {
			feedbackBoost += boost
			feedbackReasons = append(feedbackReasons, fmt.Sprintf("signal type %s +%d", triggerSignal.Type, boost))
		}
		if boost := feedback.SignalKeyBoost[triggerSignal.SignalKey]; boost > 0 {
			feedbackBoost += boost
			feedbackReasons = append(feedbackReasons, fmt.Sprintf("signal %s +%d", triggerSignal.SignalKey, boost))
		}
		feedbackSummary := buildFeedbackSummary(feedbackReasons)

		candidates = append(candidates, AutobuildPatchCandidate{
			PatchID:                 patchID,
			Priority:                "P0",
			QueueRank:               0,
			Score:                   1000 + feedbackBoost,
			FeedbackScore:           feedbackBoost,
			FeedbackSummary:         feedbackSummary,
			Confidence:              1.0,
			ConfidenceLabel:         "high",
			RecommendedEntrySurface: "ralph:///runtime/recovery",
			RepoOwnedScope:          []string{"crash repair"},
			WhyNow:                  []string{"Red signal on remote main requires immediate fix"},
			TriggerSignal:           triggerSignal,
		})
	}
	return candidates
}
