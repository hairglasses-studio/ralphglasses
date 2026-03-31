package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
)

// SprintUnit represents a single parallelizable unit of work in a batch sprint.
type SprintUnit struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Phase       string  `json:"phase"`
	Section     string  `json:"section"`
	Prompt      string  `json:"prompt"`
	Provider    string  `json:"suggested_provider"`
	BudgetUSD   float64 `json:"estimated_budget_usd"`
	Size        string  `json:"size"` // S, M, L
	DependsOn   []string `json:"depends_on,omitempty"`
}

// SprintResult is the outcome of executing a SprintUnit.
type SprintResult struct {
	UnitID    string `json:"unit_id"`
	Status    string `json:"status"` // "success", "failed", "blocked"
	SessionID string `json:"session_id,omitempty"`
	Error     string `json:"error,omitempty"`
	FilesChanged int `json:"files_changed"`
}

// DecomposeToSprints converts uncompleted roadmap items into parallelizable sprint units.
func DecomposeToSprints(rm *roadmap.Roadmap, maxUnits int) []SprintUnit {
	var units []SprintUnit

	for _, phase := range rm.Phases {
		for _, section := range phase.Sections {
			for _, task := range section.Tasks {
				if task.Done {
					continue
				}

				size := "M"
				budgetUSD := 2.0
				provider := "claude"
				desc := task.Description

				// Extract size from tags.
				if strings.Contains(desc, "`S`") {
					size = "S"
					budgetUSD = 1.0
					provider = "gemini"
				} else if strings.Contains(desc, "`L`") {
					size = "L"
					budgetUSD = 5.0
				}

				// Extract priority — P1 items get Claude, P2 can use Gemini.
				if strings.Contains(desc, "P2") && size != "L" {
					provider = "gemini"
				}

				prompt := fmt.Sprintf("Implement ROADMAP item %s from phase %q, section %q:\n\n%s\n\nRun tests after changes. Commit with a descriptive message.",
					task.ID, phase.Name, section.Name, desc)

				units = append(units, SprintUnit{
					ID:        task.ID,
					Title:     desc,
					Phase:     phase.Name,
					Section:   section.Name,
					Prompt:    prompt,
					Provider:  provider,
					BudgetUSD: budgetUSD,
					Size:      size,
					DependsOn: task.DependsOn,
				})
			}
		}
	}

	// Sort by estimated budget (small first for quick wins).
	sort.Slice(units, func(i, j int) bool {
		return units[i].BudgetUSD < units[j].BudgetUSD
	})

	if len(units) > maxUnits {
		units = units[:maxUnits]
	}

	return units
}

// FilterParallelizable returns units that have no unmet dependencies.
func FilterParallelizable(units []SprintUnit) []SprintUnit {
	done := make(map[string]bool)
	var parallel []SprintUnit

	for _, u := range units {
		blocked := false
		for _, dep := range u.DependsOn {
			if !done[dep] {
				blocked = true
				break
			}
		}
		if !blocked {
			parallel = append(parallel, u)
			done[u.ID] = true
		}
	}
	return parallel
}
