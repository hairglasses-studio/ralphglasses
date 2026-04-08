package mcpserver

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/parity"
)

const (
	adoptionPriorityKindCLISurface = "cli_surface"
	adoptionPriorityKindResource   = "resource"
	adoptionPriorityKindPrompt     = "prompt"
	adoptionPriorityKindSkill      = "skill"

	maxWorkflowPriorityCandidates = 5
	maxSurfacePriorityCandidates  = 12
)

type AdoptionPriorityItem struct {
	Kind             string   `json:"kind"`
	Name             string   `json:"name"`
	PriorityScore    int      `json:"priority_score"`
	Status           string   `json:"status,omitempty"`
	RelatedWorkflows []string `json:"related_workflows,omitempty"`
	RelatedSkills    []string `json:"related_skills,omitempty"`
	RelatedSignals   []string `json:"related_signals,omitempty"`
	Recommendation   string   `json:"recommendation,omitempty"`
	Notes            []string `json:"notes,omitempty"`
}

type AdoptionWorkflowCandidate struct {
	Name                string   `json:"name"`
	PriorityScore       int      `json:"priority_score"`
	ToolGroups          []string `json:"tool_groups,omitempty"`
	Skills              []string `json:"skills,omitempty"`
	InactiveCLISurfaces []string `json:"inactive_cli_surfaces,omitempty"`
	InactiveResources   []string `json:"inactive_resources,omitempty"`
	InactivePrompts     []string `json:"inactive_prompts,omitempty"`
	InactiveSkills      []string `json:"inactive_skills,omitempty"`
	Recommendation      string   `json:"recommendation,omitempty"`
}

type AdoptionPrioritySummary struct {
	Source                  string                      `json:"source"`
	DiscoveryUsagePath      string                      `json:"discovery_usage_path"`
	ToolBenchPath           string                      `json:"tool_bench_path"`
	WindowDays              int                         `json:"window_days"`
	InactiveObservableCLI   int                         `json:"inactive_observable_cli_surfaces"`
	InactiveResources       int                         `json:"inactive_resources"`
	InactivePrompts         int                         `json:"inactive_prompts"`
	InactiveSkills          int                         `json:"inactive_skills"`
	WorkflowCandidateCount  int                         `json:"workflow_candidate_count"`
	SurfaceCandidateCount   int                         `json:"surface_candidate_count"`
	HighestPriorityWorkflow string                      `json:"highest_priority_workflow,omitempty"`
	TopWorkflowCandidates   []AdoptionWorkflowCandidate `json:"top_workflow_candidates,omitempty"`
	TopSurfaceCandidates    []AdoptionPriorityItem      `json:"top_surface_candidates,omitempty"`
	NextTrancheFocus        []string                    `json:"next_tranche_focus,omitempty"`
}

func (s *Server) adoptionPrioritySummary() AdoptionPrioritySummary {
	usage := parity.CLIParityUsage(parity.DefaultCLIParityUsageOptions(s.ScanPath))
	discovery := s.discoveryAdoptionSummary()

	summary := AdoptionPrioritySummary{
		Source:                "cli_parity_usage + discovery_adoption_summary",
		DiscoveryUsagePath:    discovery.DiscoveryUsagePath,
		ToolBenchPath:         usage.BenchPath,
		WindowDays:            maxPriorityInt(usage.WindowDays, discovery.WindowDays),
		InactiveObservableCLI: len(usage.InactiveObservableSurfaces),
		InactiveResources:     len(discovery.InactiveResources),
		InactivePrompts:       len(discovery.InactivePrompts),
		InactiveSkills:        len(discovery.InactiveSkills),
	}

	inactiveCLISurfaces := make(map[string]parity.CLIParityEntry, len(usage.InactiveObservableSurfaces))
	for _, entry := range parity.CLIParityEntries() {
		if containsString(usage.InactiveObservableSurfaces, entry.Surface) {
			inactiveCLISurfaces[entry.Surface] = entry
		}
	}
	inactiveResources := stringSliceSet(discovery.InactiveResources)
	inactivePrompts := stringSliceSet(discovery.InactivePrompts)
	inactiveSkills := make(map[string]SkillCatalogDef, len(discovery.InactiveSkills))
	for _, skill := range skillCatalog() {
		if containsString(discovery.InactiveSkills, skill.Name) {
			inactiveSkills[skill.Name] = skill
		}
	}

	workflowCandidates := make([]AdoptionWorkflowCandidate, 0, len(workflowCatalog()))
	for _, workflow := range workflowCatalog() {
		candidate := buildWorkflowCandidate(workflow, inactiveCLISurfaces, inactiveResources, inactivePrompts, inactiveSkills)
		if candidate.PriorityScore == 0 {
			continue
		}
		workflowCandidates = append(workflowCandidates, candidate)
	}
	sort.Slice(workflowCandidates, func(i, j int) bool {
		if workflowCandidates[i].PriorityScore == workflowCandidates[j].PriorityScore {
			return workflowCandidates[i].Name < workflowCandidates[j].Name
		}
		return workflowCandidates[i].PriorityScore > workflowCandidates[j].PriorityScore
	})
	summary.WorkflowCandidateCount = len(workflowCandidates)
	summary.TopWorkflowCandidates = trimWorkflowCandidates(workflowCandidates, maxWorkflowPriorityCandidates)
	if len(workflowCandidates) > 0 {
		summary.HighestPriorityWorkflow = workflowCandidates[0].Name
	}

	surfaceCandidates := make([]AdoptionPriorityItem, 0, len(inactiveCLISurfaces)+len(inactiveResources)+len(inactivePrompts)+len(inactiveSkills))
	for _, entry := range parity.CLIParityEntries() {
		if _, ok := inactiveCLISurfaces[entry.Surface]; !ok {
			continue
		}
		relatedWorkflows, relatedSkills := relatedWorkflowAndSkillNamesForTools(entry.UsageSignals, entry.MCPSurfaces)
		surfaceCandidates = append(surfaceCandidates, AdoptionPriorityItem{
			Kind:             adoptionPriorityKindCLISurface,
			Name:             entry.Surface,
			PriorityScore:    cliSurfacePriorityScore(entry, relatedWorkflows, relatedSkills),
			Status:           string(entry.Status),
			RelatedWorkflows: relatedWorkflows,
			RelatedSkills:    relatedSkills,
			RelatedSignals:   uniqueSortedStrings(append(append([]string{}, entry.UsageSignals...), entry.MCPSurfaces...)),
			Recommendation:   buildSurfaceRecommendation(adoptionPriorityKindCLISurface, entry.Surface, relatedWorkflows, relatedSkills),
			Notes:            nonEmptyStrings(entry.Notes),
		})
	}
	for _, resource := range discovery.InactiveResources {
		relatedWorkflows := relatedWorkflowNamesForResource(resource)
		relatedSkills := relatedSkillNamesForResource(resource)
		surfaceCandidates = append(surfaceCandidates, AdoptionPriorityItem{
			Kind:             adoptionPriorityKindResource,
			Name:             resource,
			PriorityScore:    discoverySurfacePriorityScore(adoptionPriorityKindResource, relatedWorkflows, relatedSkills, nil),
			RelatedWorkflows: relatedWorkflows,
			RelatedSkills:    relatedSkills,
			RelatedSignals:   []string{"resource:" + resource},
			Recommendation:   buildSurfaceRecommendation(adoptionPriorityKindResource, resource, relatedWorkflows, relatedSkills),
		})
	}
	for _, prompt := range discovery.InactivePrompts {
		relatedWorkflows := relatedWorkflowNamesForPrompt(prompt)
		relatedSkills := relatedSkillNamesForPrompt(prompt)
		surfaceCandidates = append(surfaceCandidates, AdoptionPriorityItem{
			Kind:             adoptionPriorityKindPrompt,
			Name:             prompt,
			PriorityScore:    discoverySurfacePriorityScore(adoptionPriorityKindPrompt, relatedWorkflows, relatedSkills, nil),
			RelatedWorkflows: relatedWorkflows,
			RelatedSkills:    relatedSkills,
			RelatedSignals:   []string{"prompt:" + prompt},
			Recommendation:   buildSurfaceRecommendation(adoptionPriorityKindPrompt, prompt, relatedWorkflows, relatedSkills),
		})
	}
	for _, skill := range skillCatalog() {
		if _, ok := inactiveSkills[skill.Name]; !ok {
			continue
		}
		relatedWorkflows := relatedWorkflowNamesForSkill(skill.Name)
		surfaceCandidates = append(surfaceCandidates, AdoptionPriorityItem{
			Kind:             adoptionPriorityKindSkill,
			Name:             skill.Name,
			PriorityScore:    discoverySurfacePriorityScore(adoptionPriorityKindSkill, relatedWorkflows, nil, skill.KeyTools) + len(skill.Resources)*3 + len(skill.Prompts)*4,
			RelatedWorkflows: relatedWorkflows,
			RelatedSignals:   skillSignalList(skill),
			Recommendation:   buildSurfaceRecommendation(adoptionPriorityKindSkill, skill.Name, relatedWorkflows, nil),
			Notes:            nonEmptyStrings(skill.Description),
		})
	}

	sort.Slice(surfaceCandidates, func(i, j int) bool {
		if surfaceCandidates[i].PriorityScore == surfaceCandidates[j].PriorityScore {
			if surfaceCandidates[i].Kind == surfaceCandidates[j].Kind {
				return surfaceCandidates[i].Name < surfaceCandidates[j].Name
			}
			return surfaceCandidates[i].Kind < surfaceCandidates[j].Kind
		}
		return surfaceCandidates[i].PriorityScore > surfaceCandidates[j].PriorityScore
	})
	summary.SurfaceCandidateCount = len(surfaceCandidates)
	summary.TopSurfaceCandidates = trimSurfaceCandidates(surfaceCandidates, maxSurfacePriorityCandidates)
	summary.NextTrancheFocus = buildNextTrancheFocus(summary.TopWorkflowCandidates)
	return summary
}

func buildWorkflowCandidate(
	workflow WorkflowDef,
	inactiveCLISurfaces map[string]parity.CLIParityEntry,
	inactiveResources map[string]struct{},
	inactivePrompts map[string]struct{},
	inactiveSkills map[string]SkillCatalogDef,
) AdoptionWorkflowCandidate {
	candidate := AdoptionWorkflowCandidate{
		Name:       workflow.Name,
		ToolGroups: append([]string(nil), workflow.ToolGroups...),
		Skills:     append([]string(nil), workflow.Skills...),
	}

	for _, resource := range workflow.Resources {
		if _, ok := inactiveResources[resource]; ok {
			candidate.InactiveResources = append(candidate.InactiveResources, resource)
			candidate.PriorityScore += 8
		}
	}
	for _, prompt := range workflow.Prompts {
		if _, ok := inactivePrompts[prompt]; ok {
			candidate.InactivePrompts = append(candidate.InactivePrompts, prompt)
			candidate.PriorityScore += 10
		}
	}
	for _, skill := range workflow.Skills {
		if _, ok := inactiveSkills[skill]; ok {
			candidate.InactiveSkills = append(candidate.InactiveSkills, skill)
			candidate.PriorityScore += 14
		}
	}
	for _, entry := range inactiveCLISurfaces {
		if !toolListsIntersect(workflow.KeyTools, entry.UsageSignals, entry.MCPSurfaces) {
			continue
		}
		candidate.InactiveCLISurfaces = append(candidate.InactiveCLISurfaces, entry.Surface)
		candidate.PriorityScore += cliWorkflowWeight(entry.Status)
	}

	sort.Strings(candidate.InactiveCLISurfaces)
	sort.Strings(candidate.InactiveResources)
	sort.Strings(candidate.InactivePrompts)
	sort.Strings(candidate.InactiveSkills)

	if candidate.PriorityScore > 0 {
		candidate.Recommendation = buildWorkflowRecommendation(candidate)
	}
	return candidate
}

func cliSurfacePriorityScore(entry parity.CLIParityEntry, relatedWorkflows, relatedSkills []string) int {
	base := 0
	switch entry.Status {
	case parity.CLIParityMCPNative:
		base = 70
	case parity.CLIParityHybrid:
		base = 55
	case parity.CLIParitySkillBacked:
		base = 35
	default:
		base = 10
	}
	return base + len(relatedWorkflows)*8 + len(relatedSkills)*4 + len(entry.UsageSignals)
}

func discoverySurfacePriorityScore(kind string, relatedWorkflows, relatedSkills, signals []string) int {
	base := 12
	switch kind {
	case adoptionPriorityKindResource:
		base = 20
	case adoptionPriorityKindPrompt:
		base = 26
	case adoptionPriorityKindSkill:
		base = 32
	}
	return base + len(relatedWorkflows)*8 + len(relatedSkills)*4 + len(signals)*2
}

func cliWorkflowWeight(status parity.CLIParityStatus) int {
	switch status {
	case parity.CLIParityMCPNative:
		return 18
	case parity.CLIParityHybrid:
		return 14
	case parity.CLIParitySkillBacked:
		return 9
	default:
		return 4
	}
}

func buildWorkflowRecommendation(candidate AdoptionWorkflowCandidate) string {
	var parts []string
	if len(candidate.InactiveCLISurfaces) > 0 {
		parts = append(parts, "reactivate the dormant CLI parity path")
	}
	if len(candidate.InactiveResources) > 0 || len(candidate.InactivePrompts) > 0 {
		parts = append(parts, "route discovery through the missing read-first front doors")
	}
	if len(candidate.InactiveSkills) > 0 {
		parts = append(parts, "push operators toward the focused skill entrypoint")
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("Prioritize %s by %s.", candidate.Name, strings.Join(parts, ", "))
}

func buildSurfaceRecommendation(kind, name string, relatedWorkflows, relatedSkills []string) string {
	targets := relatedWorkflows
	if len(targets) == 0 {
		targets = relatedSkills
	}
	targetSummary := summarizeNames(targets, 2)
	switch kind {
	case adoptionPriorityKindCLISurface:
		if targetSummary != "" {
			return fmt.Sprintf("Drive the existing MCP path for %s through %s before adding more handler surface.", name, targetSummary)
		}
		return fmt.Sprintf("Drive the existing MCP path for %s through the focused discovery surfaces before adding more handler surface.", name)
	case adoptionPriorityKindResource, adoptionPriorityKindPrompt:
		if targetSummary != "" {
			return fmt.Sprintf("Promote %s as the read-first front door for %s.", name, targetSummary)
		}
		return fmt.Sprintf("Promote %s as a read-first discovery front door instead of relying on ad hoc shell loops.", name)
	case adoptionPriorityKindSkill:
		if targetSummary != "" {
			return fmt.Sprintf("Route %s through %s and check whether the compatibility mega-skill is still stealing adoption.", targetSummary, name)
		}
		return fmt.Sprintf("Route more self-serve workflow traffic through %s before adding new bespoke surface area.", name)
	default:
		return ""
	}
}

func buildNextTrancheFocus(candidates []AdoptionWorkflowCandidate) []string {
	limit := maxWorkflowPriorityCandidates
	if limit > len(candidates) {
		limit = len(candidates)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		candidate := candidates[i]
		var parts []string
		if len(candidate.InactiveCLISurfaces) > 0 {
			parts = append(parts, "inactive CLI parity: "+summarizeNames(candidate.InactiveCLISurfaces, 2))
		}
		if len(candidate.InactiveResources) > 0 {
			parts = append(parts, "inactive resources: "+summarizeNames(candidate.InactiveResources, 2))
		}
		if len(candidate.InactivePrompts) > 0 {
			parts = append(parts, "inactive prompts: "+summarizeNames(candidate.InactivePrompts, 2))
		}
		if len(candidate.InactiveSkills) > 0 {
			parts = append(parts, "inactive skills: "+summarizeNames(candidate.InactiveSkills, 2))
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s (score %d): %s.", candidate.Name, candidate.PriorityScore, strings.Join(parts, "; ")))
	}
	return out
}

func relatedWorkflowNamesForResource(resource string) []string {
	names := make([]string, 0, len(workflowCatalog()))
	for _, workflow := range workflowCatalog() {
		if containsString(workflow.Resources, resource) {
			names = append(names, workflow.Name)
		}
	}
	return names
}

func relatedSkillNamesForResource(resource string) []string {
	names := make([]string, 0, len(skillCatalog()))
	for _, skill := range skillCatalog() {
		if containsString(skill.Resources, resource) {
			names = append(names, skill.Name)
		}
	}
	return names
}

func relatedWorkflowNamesForPrompt(prompt string) []string {
	names := make([]string, 0, len(workflowCatalog()))
	for _, workflow := range workflowCatalog() {
		if containsString(workflow.Prompts, prompt) {
			names = append(names, workflow.Name)
		}
	}
	return names
}

func relatedSkillNamesForPrompt(prompt string) []string {
	names := make([]string, 0, len(skillCatalog()))
	for _, skill := range skillCatalog() {
		if containsString(skill.Prompts, prompt) {
			names = append(names, skill.Name)
		}
	}
	return names
}

func relatedWorkflowNamesForSkill(name string) []string {
	names := make([]string, 0, len(workflowCatalog()))
	for _, workflow := range workflowCatalog() {
		if containsString(workflow.Skills, name) {
			names = append(names, workflow.Name)
		}
	}
	return names
}

func relatedWorkflowAndSkillNamesForTools(toolSignals, mcpSurfaces []string) ([]string, []string) {
	workflows := make([]string, 0, len(workflowCatalog()))
	for _, workflow := range workflowCatalog() {
		if toolListsIntersect(workflow.KeyTools, toolSignals, mcpSurfaces) {
			workflows = append(workflows, workflow.Name)
		}
	}
	skills := make([]string, 0, len(skillCatalog()))
	for _, skill := range skillCatalog() {
		if toolListsIntersect(skill.KeyTools, toolSignals, mcpSurfaces) {
			skills = append(skills, skill.Name)
		}
	}
	return workflows, skills
}

func toolListsIntersect(primary, secondary, tertiary []string) bool {
	values := stringSliceSet(primary)
	for _, value := range secondary {
		if _, ok := values[value]; ok {
			return true
		}
	}
	for _, value := range tertiary {
		if _, ok := values[value]; ok {
			return true
		}
	}
	return false
}

func skillSignalList(skill SkillCatalogDef) []string {
	signals := make([]string, 0, len(skill.Resources)+len(skill.Prompts)+len(skill.KeyTools))
	for _, resource := range skill.Resources {
		signals = append(signals, "resource:"+resource)
	}
	for _, prompt := range skill.Prompts {
		signals = append(signals, "prompt:"+prompt)
	}
	for _, tool := range skill.KeyTools {
		signals = append(signals, "tool:"+tool)
	}
	return uniqueSortedStrings(signals)
}

func stringSliceSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func trimWorkflowCandidates(values []AdoptionWorkflowCandidate, limit int) []AdoptionWorkflowCandidate {
	if len(values) <= limit {
		return values
	}
	out := make([]AdoptionWorkflowCandidate, limit)
	copy(out, values[:limit])
	return out
}

func trimSurfaceCandidates(values []AdoptionPriorityItem, limit int) []AdoptionPriorityItem {
	if len(values) <= limit {
		return values
	}
	out := make([]AdoptionPriorityItem, limit)
	copy(out, values[:limit])
	return out
}

func summarizeNames(values []string, limit int) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) <= limit {
		return strings.Join(values, ", ")
	}
	return fmt.Sprintf("%s +%d more", strings.Join(values[:limit], ", "), len(values)-limit)
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func maxPriorityInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
