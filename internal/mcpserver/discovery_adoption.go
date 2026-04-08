package mcpserver

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const DefaultDiscoveryAdoptionWindow = 30 * 24 * time.Hour

type DiscoveryUsageKind string

const (
	DiscoveryUsageResource DiscoveryUsageKind = "resource"
	DiscoveryUsagePrompt   DiscoveryUsageKind = "prompt"
)

type DiscoveryUsageEntry struct {
	Kind      DiscoveryUsageKind `json:"kind"`
	Name      string             `json:"name"`
	ActualURI string             `json:"actual_uri,omitempty"`
	Timestamp time.Time          `json:"ts"`
}

type DiscoveryUsageRecorder struct {
	mu   sync.Mutex
	path string
}

type DiscoverySurfaceUsage struct {
	Name      string `json:"name"`
	CallCount int    `json:"call_count"`
	LastSeen  string `json:"last_seen,omitempty"`
}

type DiscoverySkillUsage struct {
	Name            string   `json:"name"`
	FrontDoorEvents int      `json:"front_door_events"`
	LastSeen        string   `json:"last_seen,omitempty"`
	MatchedSignals  []string `json:"matched_signals,omitempty"`
}

type DiscoveryAdoptionSummary struct {
	Source                    string                  `json:"source"`
	DiscoveryUsagePath        string                  `json:"discovery_usage_path"`
	ToolBenchPath             string                  `json:"tool_bench_path"`
	DiscoveryTelemetryPresent bool                    `json:"discovery_telemetry_present"`
	ToolBenchPresent          bool                    `json:"tool_bench_present"`
	LoadError                 string                  `json:"load_error,omitempty"`
	WindowStart               string                  `json:"window_start"`
	WindowEnd                 string                  `json:"window_end"`
	WindowDays                int                     `json:"window_days"`
	ResourceSurfaces          int                     `json:"resource_surfaces"`
	ActiveResourceSurfaces    int                     `json:"active_resource_surfaces"`
	ResourceCoveragePct       float64                 `json:"resource_coverage_pct"`
	PromptSurfaces            int                     `json:"prompt_surfaces"`
	ActivePromptSurfaces      int                     `json:"active_prompt_surfaces"`
	PromptCoveragePct         float64                 `json:"prompt_coverage_pct"`
	SkillSurfaces             int                     `json:"skill_surfaces"`
	ActiveSkillSurfaces       int                     `json:"active_skill_surfaces"`
	SkillCoveragePct          float64                 `json:"skill_coverage_pct"`
	InactiveResources         []string                `json:"inactive_resources,omitempty"`
	InactivePrompts           []string                `json:"inactive_prompts,omitempty"`
	InactiveSkills            []string                `json:"inactive_skills,omitempty"`
	TopResources              []DiscoverySurfaceUsage `json:"top_resources,omitempty"`
	TopPrompts                []DiscoverySurfaceUsage `json:"top_prompts,omitempty"`
	TopSkills                 []DiscoverySkillUsage   `json:"top_skills,omitempty"`
}

type discoveryUsageAggregate struct {
	callCount int
	lastSeen  time.Time
}

func NewDiscoveryUsageRecorder(path string) *DiscoveryUsageRecorder {
	return &DiscoveryUsageRecorder{path: path}
}

func (r *DiscoveryUsageRecorder) RecordResource(name, actualURI string) {
	r.record(DiscoveryUsageEntry{
		Kind:      DiscoveryUsageResource,
		Name:      name,
		ActualURI: actualURI,
		Timestamp: time.Now().UTC(),
	})
}

func (r *DiscoveryUsageRecorder) RecordPrompt(name string) {
	r.record(DiscoveryUsageEntry{
		Kind:      DiscoveryUsagePrompt,
		Name:      name,
		Timestamp: time.Now().UTC(),
	})
}

func (r *DiscoveryUsageRecorder) record(entry DiscoveryUsageEntry) {
	if r == nil || r.path == "" {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}

func (r *DiscoveryUsageRecorder) LoadEntries(since time.Time) ([]DiscoveryUsageEntry, error) {
	if r == nil || r.path == "" {
		return nil, nil
	}
	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []DiscoveryUsageEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry DiscoveryUsageEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if !entry.Timestamp.Before(since) {
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}

func (s *Server) discoveryAdoptionSummary() DiscoveryAdoptionSummary {
	until := time.Now().UTC()
	since := until.Add(-DefaultDiscoveryAdoptionWindow)

	discoveryUsagePath := filepath.Join(s.ScanPath, ".ralph", "discovery_usage.jsonl")
	toolBenchPath := filepath.Join(s.ScanPath, ".ralph", "tool_benchmarks.jsonl")
	if s.DiscoveryRecorder != nil && s.DiscoveryRecorder.path != "" {
		discoveryUsagePath = s.DiscoveryRecorder.path
	}
	if s.ToolRecorder != nil && s.ToolRecorder.filePath != "" {
		toolBenchPath = s.ToolRecorder.filePath
	}

	summary := DiscoveryAdoptionSummary{
		Source:             "discovery_usage.jsonl + tool_benchmarks.jsonl",
		DiscoveryUsagePath: discoveryUsagePath,
		ToolBenchPath:      toolBenchPath,
		WindowStart:        since.Format(time.RFC3339),
		WindowEnd:          until.Format(time.RFC3339),
		WindowDays:         int(until.Sub(since).Hours() / 24),
		ResourceSurfaces:   len(staticResourceCatalog()) + len(resourceTemplateCatalog()),
		PromptSurfaces:     len(promptCatalog()),
		SkillSurfaces:      len(skillCatalog()),
	}

	discoveryRecorder := s.DiscoveryRecorder
	if discoveryRecorder == nil {
		discoveryRecorder = NewDiscoveryUsageRecorder(discoveryUsagePath)
	}
	discoveryEntries, err := discoveryRecorder.LoadEntries(since)
	if err != nil {
		summary.LoadError = err.Error()
		return summary
	}
	summary.DiscoveryTelemetryPresent = len(discoveryEntries) > 0 || discoveryFileExists(discoveryUsagePath)

	resourceUsage := make(map[string]*discoveryUsageAggregate)
	promptUsage := make(map[string]*discoveryUsageAggregate)
	for _, entry := range discoveryEntries {
		if entry.Timestamp.After(until) {
			continue
		}
		switch entry.Kind {
		case DiscoveryUsageResource:
			accumulateDiscoveryUsage(resourceUsage, entry.Name, entry.Timestamp)
		case DiscoveryUsagePrompt:
			accumulateDiscoveryUsage(promptUsage, entry.Name, entry.Timestamp)
		}
	}

	toolRecorder := s.ToolRecorder
	if toolRecorder == nil {
		toolRecorder = NewToolCallRecorder(toolBenchPath, nil, 50)
	}
	toolEntries, err := toolRecorder.LoadEntries(since)
	if err != nil {
		if summary.LoadError == "" {
			summary.LoadError = err.Error()
		}
		return summary
	}
	summary.ToolBenchPresent = len(toolEntries) > 0 || discoveryFileExists(toolBenchPath)
	toolUsage := make(map[string]*discoveryUsageAggregate)
	for _, entry := range toolEntries {
		if entry.Timestamp.After(until) {
			continue
		}
		accumulateDiscoveryUsage(toolUsage, entry.ToolName, entry.Timestamp)
	}

	for _, resource := range staticResourceCatalog() {
		agg := resourceUsage[resource.URI]
		if agg == nil || agg.callCount == 0 {
			summary.InactiveResources = append(summary.InactiveResources, resource.URI)
			continue
		}
		summary.ActiveResourceSurfaces++
		summary.TopResources = append(summary.TopResources, DiscoverySurfaceUsage{
			Name:      resource.URI,
			CallCount: agg.callCount,
			LastSeen:  agg.lastSeen.Format(time.RFC3339),
		})
	}
	for _, resource := range resourceTemplateCatalog() {
		agg := resourceUsage[resource.URI]
		if agg == nil || agg.callCount == 0 {
			summary.InactiveResources = append(summary.InactiveResources, resource.URI)
			continue
		}
		summary.ActiveResourceSurfaces++
		summary.TopResources = append(summary.TopResources, DiscoverySurfaceUsage{
			Name:      resource.URI,
			CallCount: agg.callCount,
			LastSeen:  agg.lastSeen.Format(time.RFC3339),
		})
	}

	for _, prompt := range promptCatalog() {
		agg := promptUsage[prompt.Name]
		if agg == nil || agg.callCount == 0 {
			summary.InactivePrompts = append(summary.InactivePrompts, prompt.Name)
			continue
		}
		summary.ActivePromptSurfaces++
		summary.TopPrompts = append(summary.TopPrompts, DiscoverySurfaceUsage{
			Name:      prompt.Name,
			CallCount: agg.callCount,
			LastSeen:  agg.lastSeen.Format(time.RFC3339),
		})
	}

	for _, skill := range skillCatalog() {
		totalCalls := 0
		var lastSeen time.Time
		matchedSignals := make(map[string]struct{})
		for _, resource := range skill.Resources {
			if agg := resourceUsage[resource]; agg != nil {
				totalCalls += agg.callCount
				if agg.lastSeen.After(lastSeen) {
					lastSeen = agg.lastSeen
				}
				matchedSignals["resource:"+resource] = struct{}{}
			}
		}
		for _, prompt := range skill.Prompts {
			if agg := promptUsage[prompt]; agg != nil {
				totalCalls += agg.callCount
				if agg.lastSeen.After(lastSeen) {
					lastSeen = agg.lastSeen
				}
				matchedSignals["prompt:"+prompt] = struct{}{}
			}
		}
		for _, tool := range skill.KeyTools {
			if agg := toolUsage[tool]; agg != nil {
				totalCalls += agg.callCount
				if agg.lastSeen.After(lastSeen) {
					lastSeen = agg.lastSeen
				}
				matchedSignals["tool:"+tool] = struct{}{}
			}
		}
		if totalCalls == 0 {
			summary.InactiveSkills = append(summary.InactiveSkills, skill.Name)
			continue
		}
		summary.ActiveSkillSurfaces++
		summary.TopSkills = append(summary.TopSkills, DiscoverySkillUsage{
			Name:            skill.Name,
			FrontDoorEvents: totalCalls,
			LastSeen:        lastSeen.Format(time.RFC3339),
			MatchedSignals:  sortedStringSet(matchedSignals),
		})
	}

	sort.Strings(summary.InactiveResources)
	sort.Strings(summary.InactivePrompts)
	sort.Strings(summary.InactiveSkills)
	sort.Slice(summary.TopResources, func(i, j int) bool {
		if summary.TopResources[i].CallCount == summary.TopResources[j].CallCount {
			return summary.TopResources[i].Name < summary.TopResources[j].Name
		}
		return summary.TopResources[i].CallCount > summary.TopResources[j].CallCount
	})
	sort.Slice(summary.TopPrompts, func(i, j int) bool {
		if summary.TopPrompts[i].CallCount == summary.TopPrompts[j].CallCount {
			return summary.TopPrompts[i].Name < summary.TopPrompts[j].Name
		}
		return summary.TopPrompts[i].CallCount > summary.TopPrompts[j].CallCount
	})
	sort.Slice(summary.TopSkills, func(i, j int) bool {
		if summary.TopSkills[i].FrontDoorEvents == summary.TopSkills[j].FrontDoorEvents {
			return summary.TopSkills[i].Name < summary.TopSkills[j].Name
		}
		return summary.TopSkills[i].FrontDoorEvents > summary.TopSkills[j].FrontDoorEvents
	})

	summary.ResourceCoveragePct = roundPercentage(summary.ActiveResourceSurfaces, summary.ResourceSurfaces)
	summary.PromptCoveragePct = roundPercentage(summary.ActivePromptSurfaces, summary.PromptSurfaces)
	summary.SkillCoveragePct = roundPercentage(summary.ActiveSkillSurfaces, summary.SkillSurfaces)
	return summary
}

func accumulateDiscoveryUsage(target map[string]*discoveryUsageAggregate, name string, ts time.Time) {
	agg := target[name]
	if agg == nil {
		agg = &discoveryUsageAggregate{}
		target[name] = agg
	}
	agg.callCount++
	if ts.After(agg.lastSeen) {
		agg.lastSeen = ts
	}
}

func sortedStringSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func discoveryFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func roundPercentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*1000) / 10
}
