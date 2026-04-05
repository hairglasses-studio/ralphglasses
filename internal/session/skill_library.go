package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Skill is a reusable task-solving pattern extracted from successful sessions.
// Informed by SAGE (ArXiv 2512.17102): RL framework building reusable skill
// library from successful trajectories.
type Skill struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	TaskTypes      []string  `json:"task_types"`
	Prerequisites  []string  `json:"prerequisites"`
	Steps          []string  `json:"steps"`
	SourceEpisodes []string  `json:"source_episodes"`
	SuccessRate    float64   `json:"success_rate"`
	AvgCostUSD     float64   `json:"avg_cost_usd"`
	UsageCount     int       `json:"usage_count"`
	TotalTrials    int       `json:"total_trials"`
	TotalSuccesses int       `json:"total_successes"`
	TotalCostUSD   float64   `json:"total_cost_usd"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SkillLibrary manages a collection of reusable task-solving patterns.
type SkillLibrary struct {
	mu       sync.RWMutex
	skills   map[string]*Skill
	stateDir string
}

// NewSkillLibrary creates a skill library, loading persisted state if available.
func NewSkillLibrary(stateDir string) *SkillLibrary {
	sl := &SkillLibrary{
		skills:   make(map[string]*Skill),
		stateDir: stateDir,
	}
	if err := sl.Load(); err != nil {
		slog.Debug("skill_library: no persisted state", "error", err)
	}
	return sl
}

// ExtractFromEpisode analyzes a successful episode and extracts a reusable skill.
// Returns nil if the episode doesn't contain enough information for a skill.
func (sl *SkillLibrary) ExtractFromEpisode(ep Episode) *Skill {
	if len(ep.Worked) == 0 && len(ep.KeyInsights) == 0 {
		return nil
	}

	name := skillNameFromTask(ep.TaskType, ep.TaskTitle)

	// Check for existing skill with similar name
	sl.mu.RLock()
	for _, existing := range sl.skills {
		if existing.Name == name {
			// Merge into existing skill
			sl.mu.RUnlock()
			sl.mergeEpisodeInto(existing, ep)
			return existing
		}
	}
	sl.mu.RUnlock()

	// Create new skill
	skill := &Skill{
		ID:          fmt.Sprintf("skill-%d", time.Now().UnixMilli()),
		Name:        name,
		Description: fmt.Sprintf("Pattern for: %s", ep.TaskTitle),
		TaskTypes:   []string{ep.TaskType},
		Steps:       append(ep.Worked, ep.KeyInsights...),
		SourceEpisodes: []string{fmt.Sprintf("%s-%d",
			ep.TaskType, ep.Timestamp.UnixMilli())},
		SuccessRate: 1.0,
		AvgCostUSD:  ep.CostUSD,
		TotalTrials: 1, TotalSuccesses: 1,
		TotalCostUSD: ep.CostUSD,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	sl.mu.Lock()
	sl.skills[skill.ID] = skill
	sl.mu.Unlock()

	return skill
}

// FindApplicable returns skills relevant to a task, sorted by success rate.
func (sl *SkillLibrary) FindApplicable(taskDesc, taskType string, limit int) []*Skill {
	if limit <= 0 {
		limit = 5
	}

	sl.mu.RLock()
	defer sl.mu.RUnlock()

	var matches []*Skill
	descLower := strings.ToLower(taskDesc)
	typeLower := strings.ToLower(taskType)

	for _, skill := range sl.skills {
		score := 0.0
		// Task type match
		for _, st := range skill.TaskTypes {
			if strings.EqualFold(st, taskType) {
				score += 2.0
				break
			}
		}
		// Keyword match in name/description
		nameLower := strings.ToLower(skill.Name + " " + skill.Description)
		if typeLower != "" && strings.Contains(nameLower, typeLower) {
			score += 1.0
		}
		for word := range strings.FieldsSeq(descLower) {
			if len(word) > 3 && strings.Contains(nameLower, word) {
				score += 0.5
			}
		}
		if score > 0 {
			matches = append(matches, skill)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].SuccessRate > matches[j].SuccessRate
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// RecordUsage updates skill statistics after a usage attempt.
func (sl *SkillLibrary) RecordUsage(skillID string, success bool, costUSD float64) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	skill, ok := sl.skills[skillID]
	if !ok {
		return
	}

	skill.UsageCount++
	skill.TotalTrials++
	skill.TotalCostUSD += costUSD
	if success {
		skill.TotalSuccesses++
	}
	if skill.TotalTrials > 0 {
		skill.SuccessRate = float64(skill.TotalSuccesses) / float64(skill.TotalTrials)
		skill.AvgCostUSD = skill.TotalCostUSD / float64(skill.TotalTrials)
	}
	skill.UpdatedAt = time.Now()
}

// FormatForPrompt formats skills as markdown for injection into planner prompts.
func (sl *SkillLibrary) FormatForPrompt(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Applicable Skills from Library\n\n")
	for _, s := range skills {
		sb.WriteString(fmt.Sprintf("### %s (%.0f%% success, $%.3f avg)\n",
			s.Name, s.SuccessRate*100, s.AvgCostUSD))
		if s.Description != "" {
			sb.WriteString(s.Description + "\n")
		}
		if len(s.Steps) > 0 {
			sb.WriteString("**Steps:**\n")
			for i, step := range s.Steps {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// All returns all skills.
func (sl *SkillLibrary) All() []*Skill {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	result := make([]*Skill, 0, len(sl.skills))
	for _, s := range sl.skills {
		copy := *s
		result = append(result, &copy)
	}
	return result
}

// Get returns a skill by ID, or nil if not found.
func (sl *SkillLibrary) Get(id string) *Skill {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	s, ok := sl.skills[id]
	if !ok {
		return nil
	}
	copy := *s
	return &copy
}

// Count returns the number of skills.
func (sl *SkillLibrary) Count() int {
	sl.mu.RLock()
	defer sl.mu.RUnlock()
	return len(sl.skills)
}

// Save persists the skill library to disk.
func (sl *SkillLibrary) Save() error {
	if sl.stateDir == "" {
		return nil
	}
	sl.mu.RLock()
	skills := make([]*Skill, 0, len(sl.skills))
	for _, s := range sl.skills {
		skills = append(skills, s)
	}
	sl.mu.RUnlock()

	data, err := json.MarshalIndent(skills, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(sl.stateDir, "skill_library.json")
	return os.WriteFile(path, data, 0644)
}

// Load restores the skill library from disk.
func (sl *SkillLibrary) Load() error {
	if sl.stateDir == "" {
		return nil
	}
	path := filepath.Join(sl.stateDir, "skill_library.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var skills []*Skill
	if err := json.Unmarshal(data, &skills); err != nil {
		return err
	}
	sl.mu.Lock()
	for _, s := range skills {
		sl.skills[s.ID] = s
	}
	sl.mu.Unlock()
	return nil
}

// mergeEpisodeInto adds episode data to an existing skill.
func (sl *SkillLibrary) mergeEpisodeInto(skill *Skill, ep Episode) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Add new insights as steps
	for _, insight := range ep.KeyInsights {
		if !containsStr(skill.Steps, insight) {
			skill.Steps = append(skill.Steps, insight)
		}
	}
	// Add task type if new
	if !containsStr(skill.TaskTypes, ep.TaskType) {
		skill.TaskTypes = append(skill.TaskTypes, ep.TaskType)
	}
	skill.TotalTrials++
	skill.TotalSuccesses++
	skill.TotalCostUSD += ep.CostUSD
	skill.SuccessRate = float64(skill.TotalSuccesses) / float64(skill.TotalTrials)
	skill.AvgCostUSD = skill.TotalCostUSD / float64(skill.TotalTrials)
	skill.UpdatedAt = time.Now()
}

// skillNameFromTask generates a kebab-case skill name from task metadata.
func skillNameFromTask(taskType, taskTitle string) string {
	name := taskType
	if taskTitle != "" {
		// Use first few words of title
		words := strings.Fields(strings.ToLower(taskTitle))
		if len(words) > 4 {
			words = words[:4]
		}
		name = strings.Join(words, "-")
	}
	// Clean to kebab-case
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		if r == ' ' || r == '_' {
			return '-'
		}
		return -1
	}, strings.ToLower(name))
	if name == "" {
		name = "unnamed-skill"
	}
	return name
}

func containsStr(slice []string, s string) bool {
	return slices.Contains(slice, s)
}
