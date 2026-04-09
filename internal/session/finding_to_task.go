package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type ScratchpadFinding struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Severity    string    `json:"severity"` // "critical", "high", "medium", "low"
	Category    string    `json:"category"` // "bug", "perf", "coverage", "quality"
	Description string    `json:"description"`
	Resolved    bool      `json:"resolved"`
}

func ReadFindingsJSONL(path string) ([]ScratchpadFinding, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var findings []ScratchpadFinding
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var finding ScratchpadFinding
		if err := json.Unmarshal(line, &finding); err != nil {
			continue
		}
		findings = append(findings, finding)
	}
	return findings, scanner.Err()
}

func FindingToTask(findings []ScratchpadFinding) []TaskSpec {
	var tasks []TaskSpec
	for _, f := range findings {
		if f.Resolved {
			continue
		}

		task := TaskSpec{
			ID:          f.ID,
			Name:        fmt.Sprintf("[%s] %s", f.Category, f.Description),
			Description: f.Description,
			Metadata: map[string]string{
				"finding_id": f.ID,
				"category":   f.Category,
				"severity":   f.Severity,
			},
		}

		// Classification logic as per task-02.md
		switch f.Category {
		case "bug":
			task.Type = TaskBugfix
			switch f.Severity {
			case "critical":
				task.Priority = PriorityP0
			case "high":
				task.Priority = PriorityP1
			case "medium":
				task.Priority = PriorityP2
			default:
				task.Priority = PriorityP3
			}
		case "perf":
			task.Type = TaskRefactor
			switch f.Severity {
			case "critical":
				task.Priority = PriorityP0
			case "high":
				task.Priority = PriorityP1
			default:
				task.Priority = PriorityP2
			}
		case "coverage":
			task.Type = TaskTest
			task.Priority = PriorityP2
		case "quality":
			task.Type = TaskRefactor
			if f.Severity == "high" {
				task.Priority = PriorityP1
			} else {
				task.Priority = PriorityP2
			}
		case "docs":
			task.Type = TaskDocs
			task.Priority = PriorityP3
		default:
			task.Type = TaskFeature
			task.Priority = PriorityP2
		}

		// Complexity heuristic
		if f.Severity == "critical" || f.Severity == "high" {
			task.EstimatedComplexity = ComplexityM
		} else {
			task.EstimatedComplexity = ComplexityS
		}

		tasks = append(tasks, task)
	}
	return tasks
}
