package mcpserver

import (
	"slices"
	"sort"
	"strings"
)

type toolGroupInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	ToolCount   int      `json:"tool_count"`
	Loaded      bool     `json:"loaded"`
	Tools       []string `json:"tools"`
}

type toolGroupDiscoveryResponse struct {
	Query           string            `json:"query,omitempty"`
	ToolGroup       string            `json:"tool_group,omitempty"`
	Limit           int               `json:"limit,omitempty"`
	GroupCount      int               `json:"group_count"`
	WorkflowCount   int               `json:"workflow_count"`
	SkillCount      int               `json:"skill_count"`
	Groups          []toolGroupInfo   `json:"groups"`
	WorkflowMatches []WorkflowDef     `json:"workflow_matches,omitempty"`
	SkillMatches    []SkillCatalogDef `json:"skill_matches,omitempty"`
}

func (s *Server) buildToolGroupInfos() []toolGroupInfo {
	groups := s.buildToolGroups()
	out := make([]toolGroupInfo, 0, len(groups))
	for _, group := range groups {
		tools := make([]string, 0, len(group.Tools))
		for _, entry := range group.Tools {
			tools = append(tools, entry.Tool.Name)
		}
		sort.Strings(tools)
		s.mu.RLock()
		loaded := s.loadedGroups[group.Name]
		s.mu.RUnlock()
		out = append(out, toolGroupInfo{
			Name:        group.Name,
			Description: group.Description,
			ToolCount:   len(group.Tools),
			Loaded:      loaded,
			Tools:       tools,
		})
	}
	return out
}

func validToolGroupFilter(group string) bool {
	if group == "" || group == "management" {
		return true
	}
	return slices.Contains(ToolGroupNames, group)
}

func filterToolGroupInfos(groups []toolGroupInfo, query, toolGroup string, limit int) []toolGroupInfo {
	out := make([]toolGroupInfo, 0, len(groups))
	for _, group := range groups {
		if toolGroup != "" && group.Name != toolGroup {
			continue
		}
		if !matchesQuery(query, group.Name, group.Description, strings.Join(group.Tools, " ")) {
			continue
		}
		out = append(out, group)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func filterWorkflowDefs(defs []WorkflowDef, query, toolGroup string, limit int) []WorkflowDef {
	out := make([]WorkflowDef, 0, len(defs))
	for _, def := range defs {
		if toolGroup != "" && !slices.Contains(def.ToolGroups, toolGroup) {
			continue
		}
		if !matchesWorkflowQuery(def, query) {
			continue
		}
		out = append(out, def)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func filterSkillDefs(defs []SkillCatalogDef, query, toolGroup string, limit int) []SkillCatalogDef {
	out := make([]SkillCatalogDef, 0, len(defs))
	for _, def := range defs {
		if toolGroup != "" && !slices.Contains(def.ToolGroups, toolGroup) {
			continue
		}
		if !matchesSkillQuery(def, query) {
			continue
		}
		out = append(out, def)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func matchesWorkflowQuery(def WorkflowDef, query string) bool {
	return matchesQuery(query,
		def.Name,
		def.Description,
		strings.Join(def.Resources, " "),
		strings.Join(def.Prompts, " "),
		strings.Join(def.Skills, " "),
		strings.Join(def.ToolGroups, " "),
		strings.Join(def.KeyTools, " "),
	)
}

func matchesSkillQuery(def SkillCatalogDef, query string) bool {
	return matchesQuery(query,
		def.Name,
		def.Description,
		strings.Join(def.Tags, " "),
		strings.Join(def.Workflows, " "),
		strings.Join(def.ToolGroups, " "),
		strings.Join(def.Resources, " "),
		strings.Join(def.Prompts, " "),
		strings.Join(def.KeyTools, " "),
		def.CanonicalPath,
	)
}

func matchesQuery(query string, haystacks ...string) bool {
	query = strings.TrimSpace(strings.ToLower(query))
	if query == "" {
		return true
	}
	for _, haystack := range haystacks {
		if strings.Contains(strings.ToLower(haystack), query) {
			return true
		}
	}
	return false
}
