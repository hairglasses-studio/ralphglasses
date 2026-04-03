package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StrategicPrinciple is a distilled strategic insight extracted from offline
// trajectories (successful episodes and failure-pattern rules). Inspired by
// EvolveR-style self-distillation (ArXiv 2510.16079).
type StrategicPrinciple struct {
	ID          string    `json:"id"`
	Principle   string    `json:"principle"`
	Source      string    `json:"source"`       // "episode" or "reflexion"
	SourceCount int       `json:"source_count"` // trajectories that contributed
	Confidence  float64   `json:"confidence"`   // 0.0-1.0
	TaskTypes   []string  `json:"task_types"`   // applicable task types
	CreatedAt   time.Time `json:"created_at"`
	LastUsed    time.Time `json:"last_used"`
	UsageCount  int       `json:"usage_count"`
}

// TrajectoryDistiller extracts strategic principles from episodic memory
// (successful trajectories) and reflexion rules (failure patterns).
type TrajectoryDistiller struct {
	episodic   *EpisodicMemory
	reflexion  *ReflexionStore
	principles []StrategicPrinciple
	stateDir   string
	mu         sync.RWMutex
}

// principlesFile is the filename used to persist strategic principles.
const principlesFile = "strategic_principles.json"

// NewTrajectoryDistiller creates a distiller that reads from the given episodic
// memory and reflexion store. Principles are persisted under stateDir.
func NewTrajectoryDistiller(episodic *EpisodicMemory, reflexion *ReflexionStore, stateDir string) *TrajectoryDistiller {
	td := &TrajectoryDistiller{
		episodic:  episodic,
		reflexion: reflexion,
		stateDir:  stateDir,
	}
	// Best-effort load of previously distilled principles.
	_ = td.Load()
	return td
}

// Distill extracts strategic principles from the current set of episodes and
// reflexion rules. It groups trajectories by task type, identifies recurring
// patterns of success and failure, and generates human-readable principles
// with confidence scores proportional to the evidence count.
//
// New principles are merged with existing ones (deduplication by content
// similarity), and the result is persisted to disk.
func (td *TrajectoryDistiller) Distill() ([]StrategicPrinciple, error) {
	now := time.Now()
	var fresh []StrategicPrinciple

	// --- Phase 1: distill from successful episodes ---
	fresh = append(fresh, td.distillFromEpisodes(now)...)

	// --- Phase 2: distill from reflexion rules (failure patterns) ---
	fresh = append(fresh, td.distillFromReflexion(now)...)

	// --- Phase 3: merge with existing principles ---
	td.mu.Lock()
	td.principles = mergePrinciples(td.principles, fresh)
	out := make([]StrategicPrinciple, len(td.principles))
	copy(out, td.principles)
	td.mu.Unlock()

	if err := td.Save(); err != nil {
		return out, fmt.Errorf("distill: save failed: %w", err)
	}
	return out, nil
}

// distillFromEpisodes groups episodes by task type and extracts principles
// from clusters of successful trajectories.
func (td *TrajectoryDistiller) distillFromEpisodes(now time.Time) []StrategicPrinciple {
	if td.episodic == nil {
		return nil
	}

	td.episodic.mu.Lock()
	eps := make([]Episode, len(td.episodic.episodes))
	copy(eps, td.episodic.episodes)
	td.episodic.mu.Unlock()

	if len(eps) == 0 {
		return nil
	}

	// Group by task type.
	byType := make(map[string][]Episode)
	for _, ep := range eps {
		byType[ep.TaskType] = append(byType[ep.TaskType], ep)
	}

	var principles []StrategicPrinciple

	for taskType, episodes := range byType {
		if len(episodes) < 2 {
			continue // need at least 2 trajectories to distill a principle
		}

		// Collect all "worked" items across episodes for this task type.
		workedCounts := make(map[string]int)
		for _, ep := range episodes {
			for _, w := range ep.Worked {
				key := strings.ToLower(strings.TrimSpace(w))
				if key != "" {
					workedCounts[key]++
				}
			}
		}

		// Extract patterns that appear in 2+ episodes.
		for pattern, count := range workedCounts {
			if count < 2 {
				continue
			}
			confidence := clampConfidence(float64(count) / float64(len(episodes)))
			principles = append(principles, StrategicPrinciple{
				ID:          uuid.New().String(),
				Principle:   fmt.Sprintf("For %s tasks: %s", taskType, pattern),
				Source:      "episode",
				SourceCount: count,
				Confidence:  confidence,
				TaskTypes:   []string{taskType},
				CreatedAt:   now,
			})
		}

		// Extract insight-based principles from key insights.
		insightCounts := make(map[string]int)
		for _, ep := range episodes {
			for _, insight := range ep.KeyInsights {
				key := strings.ToLower(strings.TrimSpace(insight))
				if key != "" {
					insightCounts[key]++
				}
			}
		}
		for insight, count := range insightCounts {
			if count < 2 {
				continue
			}
			confidence := clampConfidence(float64(count) / float64(len(episodes)))
			principles = append(principles, StrategicPrinciple{
				ID:          uuid.New().String(),
				Principle:   fmt.Sprintf("Insight for %s tasks: %s", taskType, insight),
				Source:      "episode",
				SourceCount: count,
				Confidence:  confidence,
				TaskTypes:   []string{taskType},
				CreatedAt:   now,
			})
		}
	}

	// Cross-type principles: patterns that recur across 3+ task types.
	globalWorked := make(map[string]map[string]bool) // pattern -> set of task types
	for taskType, episodes := range byType {
		for _, ep := range episodes {
			for _, w := range ep.Worked {
				key := strings.ToLower(strings.TrimSpace(w))
				if key == "" {
					continue
				}
				if globalWorked[key] == nil {
					globalWorked[key] = make(map[string]bool)
				}
				globalWorked[key][taskType] = true
			}
		}
	}
	for pattern, types := range globalWorked {
		if len(types) < 3 {
			continue
		}
		typeList := mapKeys(types)
		sort.Strings(typeList)
		principles = append(principles, StrategicPrinciple{
			ID:          uuid.New().String(),
			Principle:   fmt.Sprintf("Universal principle: %s", pattern),
			Source:      "episode",
			SourceCount: len(types),
			Confidence:  clampConfidence(float64(len(types)) / float64(len(byType))),
			TaskTypes:   typeList,
			CreatedAt:   now,
		})
	}

	return principles
}

// distillFromReflexion converts reflexion rules (learned from repeated
// failures) into avoidance principles.
func (td *TrajectoryDistiller) distillFromReflexion(now time.Time) []StrategicPrinciple {
	if td.reflexion == nil {
		return nil
	}

	rules := td.reflexion.Rules()
	if len(rules) == 0 {
		return nil
	}

	// Also scan reflections to discover which task types each failure mode
	// affects, so we can tag the resulting principles.
	td.reflexion.mu.Lock()
	refs := make([]Reflection, len(td.reflexion.reflections))
	copy(refs, td.reflexion.reflections)
	td.reflexion.mu.Unlock()

	modeTaskTypes := make(map[string]map[string]bool)
	for _, r := range refs {
		if r.FailureMode == "" {
			continue
		}
		taskType := classifyTask(r.TaskTitle)
		if modeTaskTypes[r.FailureMode] == nil {
			modeTaskTypes[r.FailureMode] = make(map[string]bool)
		}
		modeTaskTypes[r.FailureMode][taskType] = true
	}

	var principles []StrategicPrinciple
	for _, rule := range rules {
		typeSet := modeTaskTypes[rule.FailureMode]
		var taskTypes []string
		if len(typeSet) > 0 {
			taskTypes = mapKeys(typeSet)
			sort.Strings(taskTypes)
		} else {
			taskTypes = []string{"general"}
		}

		confidence := clampConfidence(float64(rule.Count) / 10.0) // 10 occurrences = max confidence
		principles = append(principles, StrategicPrinciple{
			ID:          uuid.New().String(),
			Principle:   fmt.Sprintf("Avoid: %s", rule.Rule),
			Source:      "reflexion",
			SourceCount: rule.Count,
			Confidence:  confidence,
			TaskTypes:   taskTypes,
			CreatedAt:   now,
		})
	}
	return principles
}

// Applicable returns up to limit principles relevant to the given task type,
// sorted by confidence descending. An empty taskType matches all principles.
func (td *TrajectoryDistiller) Applicable(taskType string, limit int) []StrategicPrinciple {
	if limit <= 0 {
		limit = 5
	}

	td.mu.RLock()
	defer td.mu.RUnlock()

	var matched []StrategicPrinciple
	for _, p := range td.principles {
		if taskType == "" || containsString(p.TaskTypes, taskType) || containsString(p.TaskTypes, "general") {
			matched = append(matched, p)
		}
	}

	// Sort by confidence descending, then by source count descending.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Confidence != matched[j].Confidence {
			return matched[i].Confidence > matched[j].Confidence
		}
		return matched[i].SourceCount > matched[j].SourceCount
	})

	if len(matched) > limit {
		matched = matched[:limit]
	}
	return matched
}

// FormatForPrompt renders a list of principles as a markdown section suitable
// for injection into an LLM prompt.
func (td *TrajectoryDistiller) FormatForPrompt(principles []StrategicPrinciple) string {
	if len(principles) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Strategic Principles (distilled from past trajectories)\n\n")

	for i, p := range principles {
		confidenceLabel := confidenceGrade(p.Confidence)
		sb.WriteString(fmt.Sprintf("%d. **[%s]** %s", i+1, confidenceLabel, p.Principle))
		if p.SourceCount > 1 {
			sb.WriteString(fmt.Sprintf(" _(observed %dx)_", p.SourceCount))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\nApply these principles proactively. They are distilled from real session trajectories.\n")
	return sb.String()
}

// MarkUsed records that a principle was used in a session, updating its
// LastUsed timestamp and incrementing UsageCount.
func (td *TrajectoryDistiller) MarkUsed(principleID string) {
	td.mu.Lock()
	defer td.mu.Unlock()

	for i := range td.principles {
		if td.principles[i].ID == principleID {
			td.principles[i].LastUsed = time.Now()
			td.principles[i].UsageCount++
			return
		}
	}
}

// Save persists the current principles to stateDir/strategic_principles.json.
func (td *TrajectoryDistiller) Save() error {
	if td.stateDir == "" {
		return nil
	}
	if err := os.MkdirAll(td.stateDir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	td.mu.RLock()
	data, err := json.MarshalIndent(td.principles, "", "  ")
	td.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal principles: %w", err)
	}

	path := filepath.Join(td.stateDir, principlesFile)
	return os.WriteFile(path, data, 0644)
}

// Load reads previously persisted principles from stateDir/strategic_principles.json.
func (td *TrajectoryDistiller) Load() error {
	if td.stateDir == "" {
		return nil
	}
	path := filepath.Join(td.stateDir, principlesFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read principles: %w", err)
	}

	var principles []StrategicPrinciple
	if err := json.Unmarshal(data, &principles); err != nil {
		return fmt.Errorf("unmarshal principles: %w", err)
	}

	td.mu.Lock()
	td.principles = principles
	td.mu.Unlock()
	return nil
}

// Principles returns a snapshot of all currently held principles.
func (td *TrajectoryDistiller) Principles() []StrategicPrinciple {
	td.mu.RLock()
	defer td.mu.RUnlock()
	out := make([]StrategicPrinciple, len(td.principles))
	copy(out, td.principles)
	return out
}

// --- helpers ---

// mergePrinciples combines existing and fresh principles, deduplicating by
// content similarity (Jaccard on lowercased words, threshold 0.8). When a
// duplicate is found the existing principle's confidence and source count
// are updated rather than adding a new entry.
func mergePrinciples(existing, fresh []StrategicPrinciple) []StrategicPrinciple {
	merged := make([]StrategicPrinciple, len(existing))
	copy(merged, existing)

	for _, f := range fresh {
		found := false
		for i := range merged {
			if principlesSimilar(merged[i].Principle, f.Principle) {
				// Update existing: boost confidence and source count.
				merged[i].SourceCount += f.SourceCount
				merged[i].Confidence = clampConfidence(
					(merged[i].Confidence + f.Confidence) / 2.0,
				)
				// Merge task types.
				merged[i].TaskTypes = mergeStringSlices(merged[i].TaskTypes, f.TaskTypes)
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, f)
		}
	}
	return merged
}

// principlesSimilar returns true if two principle strings are similar enough
// to be considered duplicates (Jaccard similarity >= 0.8 on word sets).
func principlesSimilar(a, b string) bool {
	return jaccardSimilarity(a, b) >= 0.8
}

// mergeStringSlices combines two string slices, removing duplicates.
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, s := range a {
		seen[s] = true
	}
	result := make([]string, len(a))
	copy(result, a)
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// containsString checks if a string slice contains a specific value.
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// mapKeys returns the keys of a map[string]bool as a string slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// clampConfidence clamps a confidence value to the [0.0, 1.0] range.
func clampConfidence(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

// confidenceGrade converts a 0.0-1.0 confidence into a letter grade.
func confidenceGrade(c float64) string {
	switch {
	case c >= 0.9:
		return "HIGH"
	case c >= 0.6:
		return "MED"
	default:
		return "LOW"
	}
}
