package roadmap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Analysis is the result of comparing a roadmap against a codebase.
type Analysis struct {
	Gaps     []GapItem     `json:"gaps"`
	Stale    []StaleItem   `json:"stale"`
	Orphaned []OrphanedItem `json:"orphaned"`
	Ready    []ReadyItem   `json:"ready"`
	Summary  AnalysisSummary `json:"summary"`
}

// GapItem is a task with no codebase evidence.
type GapItem struct {
	TaskID      string `json:"task_id"`
	Description string `json:"description"`
	Phase       string `json:"phase"`
	Section     string `json:"section"`
	Status      string `json:"status"` // not_started, partially_implemented
}

// StaleItem is a task marked incomplete but with implementation evidence.
type StaleItem struct {
	TaskID      string   `json:"task_id"`
	Description string   `json:"description"`
	Phase       string   `json:"phase"`
	Evidence    []string `json:"evidence"`
}

// OrphanedItem is code/config with no roadmap entry.
type OrphanedItem struct {
	Path        string `json:"path"`
	Description string `json:"description"`
}

// ReadyItem is an incomplete task whose dependencies are all met.
type ReadyItem struct {
	TaskID      string `json:"task_id"`
	Description string `json:"description"`
	Phase       string `json:"phase"`
	Section     string `json:"section"`
}

// AnalysisSummary provides counts.
type AnalysisSummary struct {
	TotalTasks   int `json:"total_tasks"`
	GapCount     int `json:"gap_count"`
	StaleCount   int `json:"stale_count"`
	OrphanCount  int `json:"orphan_count"`
	ReadyCount   int `json:"ready_count"`
}

// Analyze compares a roadmap against the repo filesystem.
func Analyze(rm *Roadmap, repoPath string) (*Analysis, error) {
	a := &Analysis{}
	completedIDs := buildCompletedSet(rm)

	for _, phase := range rm.Phases {
		for _, sec := range phase.Sections {
			for _, task := range sec.Tasks {
				if task.Done {
					continue
				}

				// Check if dependencies are met
				depsReady := true
				for _, dep := range task.DependsOn {
					if _, ok := completedIDs[dep]; !ok {
						depsReady = false
						break
					}
				}

				// Search for evidence in the repo
				evidence := findEvidence(task, repoPath)

				if len(evidence) > 0 {
					// Has code but not checked off
					a.Stale = append(a.Stale, StaleItem{
						TaskID:      task.ID,
						Description: task.Description,
						Phase:       phase.Name,
						Evidence:    evidence,
					})
				} else {
					// No evidence found
					a.Gaps = append(a.Gaps, GapItem{
						TaskID:      task.ID,
						Description: task.Description,
						Phase:       phase.Name,
						Section:     sec.Name,
						Status:      "not_started",
					})
				}

				if depsReady && len(task.DependsOn) > 0 {
					a.Ready = append(a.Ready, ReadyItem{
						TaskID:      task.ID,
						Description: task.Description,
						Phase:       phase.Name,
						Section:     sec.Name,
					})
				} else if depsReady && len(task.DependsOn) == 0 {
					a.Ready = append(a.Ready, ReadyItem{
						TaskID:      task.ID,
						Description: task.Description,
						Phase:       phase.Name,
						Section:     sec.Name,
					})
				}
			}
		}
	}

	// Look for orphaned dirs (common patterns not in roadmap)
	a.Orphaned = findOrphaned(rm, repoPath)

	a.Summary = AnalysisSummary{
		TotalTasks:  rm.Stats.Total,
		GapCount:    len(a.Gaps),
		StaleCount:  len(a.Stale),
		OrphanCount: len(a.Orphaned),
		ReadyCount:  len(a.Ready),
	}

	return a, nil
}

func buildCompletedSet(rm *Roadmap) map[string]struct{} {
	set := make(map[string]struct{})
	for _, p := range rm.Phases {
		for _, s := range p.Sections {
			for _, t := range s.Tasks {
				if t.Done && t.ID != "" {
					set[t.ID] = struct{}{}
				}
			}
		}
	}
	return set
}

// findEvidence looks for filesystem signals that a task might be implemented.
func findEvidence(task Task, repoPath string) []string {
	var evidence []string
	desc := strings.ToLower(task.Description)

	// Extract keywords from task description (paths, package names, filenames)
	keywords := extractKeywords(desc)

	for _, kw := range keywords {
		// Check if file/dir exists
		candidate := filepath.Join(repoPath, kw)
		if _, err := os.Stat(candidate); err == nil {
			evidence = append(evidence, fmt.Sprintf("exists: %s", kw))
		}
	}

	return evidence
}

// extractKeywords pulls potential file paths and package names from a description.
func extractKeywords(desc string) []string {
	var keywords []string

	// Look for backtick-quoted paths/identifiers
	inBacktick := false
	var current strings.Builder
	for _, r := range desc {
		if r == '`' {
			if inBacktick {
				word := current.String()
				if word != "" && (strings.Contains(word, "/") || strings.Contains(word, ".") || strings.Contains(word, "_")) {
					keywords = append(keywords, word)
				}
			}
			current.Reset()
			inBacktick = !inBacktick
			continue
		}
		if inBacktick {
			current.WriteRune(r)
		}
	}

	// Common path patterns
	for _, prefix := range []string{"internal/", "cmd/", "distro/", "scripts/"} {
		if strings.Contains(desc, prefix) {
			// Try to extract the full path
			idx := strings.Index(desc, prefix)
			end := idx
			for end < len(desc) && desc[end] != ' ' && desc[end] != ')' && desc[end] != ',' {
				end++
			}
			keywords = append(keywords, desc[idx:end])
		}
	}

	return keywords
}

// findOrphaned looks for code directories that aren't referenced in the roadmap.
func findOrphaned(rm *Roadmap, repoPath string) []OrphanedItem {
	var orphaned []OrphanedItem

	// Collect all text from roadmap
	var allText strings.Builder
	for _, p := range rm.Phases {
		allText.WriteString(p.Name)
		for _, s := range p.Sections {
			allText.WriteString(s.Name)
			for _, t := range s.Tasks {
				allText.WriteString(t.Description)
			}
		}
	}
	roadmapText := strings.ToLower(allText.String())

	// Check internal/ subdirs
	internalPath := filepath.Join(repoPath, "internal")
	entries, err := os.ReadDir(internalPath)
	if err != nil {
		return orphaned
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.Contains(roadmapText, e.Name()) {
			orphaned = append(orphaned, OrphanedItem{
				Path:        filepath.Join("internal", e.Name()),
				Description: fmt.Sprintf("package %q not referenced in roadmap", e.Name()),
			})
		}
	}

	return orphaned
}
