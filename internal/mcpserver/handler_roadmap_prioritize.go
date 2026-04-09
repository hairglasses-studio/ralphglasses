package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/docs/pkg/roadmap"
)

func (s *Server) handleRoadmapPrioritize(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	rmPath := roadmap.ResolvePath(repoPath, "")
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	// Parse weights.
	wImpact := 0.4
	wEffort := 0.3
	wDep := 0.3
	if weightsJSON := getStringArg(req, "weights"); weightsJSON != "" {
		var w struct {
			Impact     float64 `json:"impact"`
			Effort     float64 `json:"effort"`
			Dependency float64 `json:"dependency"`
		}
		if err := json.Unmarshal([]byte(weightsJSON), &w); err == nil {
			if w.Impact > 0 {
				wImpact = w.Impact
			}
			if w.Effort > 0 {
				wEffort = w.Effort
			}
			if w.Dependency > 0 {
				wDep = w.Dependency
			}
		}
	}

	topN := int(getNumberArg(req, "top_n", 20))
	phaseFilter := getStringArg(req, "phase_filter")

	// Build a set of completed phase names for dependency scoring.
	phaseComplete := make(map[string]bool)
	phaseCompletionPct := make(map[string]float64)
	for _, p := range rm.Phases {
		if p.Stats.Total > 0 {
			pct := float64(p.Stats.Completed) / float64(p.Stats.Total)
			phaseCompletionPct[p.Name] = pct
			if p.Stats.Completed == p.Stats.Total {
				phaseComplete[p.Name] = true
			}
		}
	}

	type scoredItem struct {
		Phase       string  `json:"phase"`
		Section     string  `json:"section"`
		TaskID      string  `json:"task_id"`
		Description string  `json:"description"`
		Impact      float64 `json:"impact_score"`
		Effort      float64 `json:"effort_score"`
		Dependency  float64 `json:"dependency_score"`
		Total       float64 `json:"total_score"`
		Blocked     bool    `json:"blocked,omitempty"`
	}

	var items []scoredItem
	for _, phase := range rm.Phases {
		// Apply phase filter.
		if phaseFilter != "" && !strings.Contains(strings.ToLower(phase.Name), strings.ToLower(phaseFilter)) {
			continue
		}

		// Phase momentum bonus: items in nearly-complete phases get a small boost.
		momentum := 0.0
		if pct, ok := phaseCompletionPct[phase.Name]; ok && pct > 0.8 {
			momentum = 0.1 // bonus for finishing a phase
		}

		for _, section := range phase.Sections {
			// Detect section-level blocking (e.g., "[BLOCKED BY 4.1]").
			sectionBlocked := strings.Contains(section.Name, "BLOCKED BY")

			for _, task := range section.Tasks {
				if task.Done {
					continue
				}

				// Impact: extract priority tag.
				impact := 0.6 // default P2
				desc := task.Description
				if strings.Contains(desc, "P1") {
					impact = 1.0
				} else if strings.Contains(desc, "P2") {
					impact = 0.6
				} else if strings.Contains(desc, "P3") {
					impact = 0.3
				}

				// Effort: extract size tag (inverse — small = high score).
				effort := 0.5 // default M
				if strings.Contains(desc, "`S`") {
					effort = 0.9
				} else if strings.Contains(desc, "`M`") {
					effort = 0.5
				} else if strings.Contains(desc, "`L`") {
					effort = 0.2
				}

				// Dependency: check task deps + section-level blocking.
				dep := 1.0
				blocked := sectionBlocked
				if sectionBlocked {
					dep = 0.1
				}
				for _, d := range task.DependsOn {
					if !phaseComplete[d] {
						dep = 0.3
						blocked = true
						break
					}
				}

				total := impact*wImpact + effort*wEffort + dep*wDep + momentum

				items = append(items, scoredItem{
					Phase:       phase.Name,
					Section:     section.Name,
					TaskID:      task.ID,
					Description: desc,
					Impact:      impact,
					Effort:      effort,
					Dependency:  dep,
					Total:       total,
					Blocked:     blocked,
				})
			}
		}
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Total > items[j].Total
	})

	totalRemaining := len(items)
	if len(items) > topN {
		items = items[:topN]
	}

	// Top 5 as recommended sprint.
	sprintSize := 5
	if len(items) < sprintSize {
		sprintSize = len(items)
	}

	result := map[string]any{
		"prioritized_items":       items,
		"total_remaining":         totalRemaining,
		"recommended_next_sprint": items[:sprintSize],
		"weights": map[string]float64{
			"impact":     wImpact,
			"effort":     wEffort,
			"dependency": wDep,
		},
	}
	return jsonResult(result), nil
}
