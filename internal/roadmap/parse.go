package roadmap

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Roadmap represents a parsed ROADMAP.md file.
type Roadmap struct {
	Title  string  `json:"title"`
	Phases []Phase `json:"phases"`
	Stats  Stats   `json:"stats"`
}

// Phase is a top-level section (## Phase ...).
type Phase struct {
	Name     string    `json:"name"`
	Sections []Section `json:"sections"`
	Stats    Stats     `json:"stats"`
}

// Section is a subsection (### ...).
type Section struct {
	Name       string `json:"name"`
	Tasks      []Task `json:"tasks"`
	Acceptance string `json:"acceptance,omitempty"`
}

// Task is a checkbox item.
type Task struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Done        bool     `json:"done"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// Stats tracks completion counts.
type Stats struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Remaining int `json:"remaining"`
}

var (
	phaseRe      = regexp.MustCompile(`^##\s+(.+)`)
	sectionRe    = regexp.MustCompile(`^###\s+(.+)`)
	taskRe       = regexp.MustCompile(`^-\s+\[([ xX])\]\s+(.+)`)
	taskIDRe     = regexp.MustCompile(`^(\S+)\s+[—–-]\s+(.+)`)
	blockedByRe  = regexp.MustCompile(`\[BLOCKED BY (.+?)\]`)
	acceptanceRe = regexp.MustCompile(`^-?\s*\*\*Acceptance:\*\*\s*(.+)`)
)

// Parse reads a ROADMAP.md file and returns structured data.
func Parse(filePath string) (*Roadmap, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open roadmap: %w", err)
	}
	defer f.Close()

	rm := &Roadmap{}
	var curPhase *Phase
	var curSection *Section

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Title (first # heading)
		if rm.Title == "" && strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			rm.Title = strings.TrimPrefix(trimmed, "# ")
			continue
		}

		// Phase heading
		if m := phaseRe.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(trimmed, "###") {
			// Save previous phase
			if curPhase != nil {
				if curSection != nil {
					curPhase.Sections = append(curPhase.Sections, *curSection)
					curSection = nil
				}
				curPhase.Stats = calcPhaseStats(curPhase)
				rm.Phases = append(rm.Phases, *curPhase)
			}
			curPhase = &Phase{Name: m[1]}
			continue
		}

		// Section heading
		if m := sectionRe.FindStringSubmatch(trimmed); m != nil {
			if curSection != nil && curPhase != nil {
				curPhase.Sections = append(curPhase.Sections, *curSection)
			}
			curSection = &Section{Name: m[1]}
			continue
		}

		// Acceptance criteria
		if m := acceptanceRe.FindStringSubmatch(trimmed); m != nil {
			if curSection != nil {
				curSection.Acceptance = m[1]
			}
			continue
		}

		// Task checkbox
		if m := taskRe.FindStringSubmatch(trimmed); m != nil {
			done := m[1] == "x" || m[1] == "X"
			desc := m[2]

			task := Task{
				Done:        done,
				Description: desc,
			}

			// Extract task ID (e.g., "0.5.1.1 — description")
			if idm := taskIDRe.FindStringSubmatch(desc); idm != nil {
				task.ID = idm[1]
				task.Description = idm[2]
			}

			// Extract dependencies
			if bm := blockedByRe.FindStringSubmatch(desc); bm != nil {
				task.DependsOn = strings.Split(bm[1], ",")
				for i := range task.DependsOn {
					task.DependsOn[i] = strings.TrimSpace(task.DependsOn[i])
				}
			}

			if curSection != nil {
				curSection.Tasks = append(curSection.Tasks, task)
			} else if curPhase != nil {
				// Tasks directly under a phase (no section)
				if len(curPhase.Sections) == 0 || curPhase.Sections[len(curPhase.Sections)-1].Name != "" {
					curPhase.Sections = append(curPhase.Sections, Section{Name: curPhase.Name})
				}
				curPhase.Sections[len(curPhase.Sections)-1].Tasks = append(
					curPhase.Sections[len(curPhase.Sections)-1].Tasks, task,
				)
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan roadmap: %w", err)
	}

	// Flush remaining
	if curPhase != nil {
		if curSection != nil {
			curPhase.Sections = append(curPhase.Sections, *curSection)
		}
		curPhase.Stats = calcPhaseStats(curPhase)
		rm.Phases = append(rm.Phases, *curPhase)
	}

	rm.Stats = calcTotalStats(rm)
	return rm, nil
}

// ResolvePath finds the roadmap file given a repo path and optional filename override.
func ResolvePath(repoPath, file string) string {
	if file == "" {
		file = "ROADMAP.md"
	}
	// If repoPath is already a .md file, use it directly
	if strings.HasSuffix(repoPath, ".md") {
		return repoPath
	}
	return filepath.Join(repoPath, file)
}

func calcPhaseStats(p *Phase) Stats {
	var s Stats
	for _, sec := range p.Sections {
		for _, t := range sec.Tasks {
			s.Total++
			if t.Done {
				s.Completed++
			}
		}
	}
	s.Remaining = s.Total - s.Completed
	return s
}

func calcTotalStats(rm *Roadmap) Stats {
	var s Stats
	for _, p := range rm.Phases {
		s.Total += p.Stats.Total
		s.Completed += p.Stats.Completed
	}
	s.Remaining = s.Total - s.Completed
	return s
}
