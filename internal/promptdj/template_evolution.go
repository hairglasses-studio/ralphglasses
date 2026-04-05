package promptdj

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// TemplateEvolution manages the lifecycle of prompt templates, promoting
// high-performing ones and demoting low-performing ones based on outcome feedback.
// It bridges the PromptEvolution genetic algorithm with the Prompt DJ routing system.
type TemplateEvolution struct {
	mu        sync.Mutex
	templates map[string]*EvolvedTemplate // name -> template
	history   []EvolutionEvent
	maxHistory int
}

// EvolvedTemplate is a template with tracked performance across routing outcomes.
type EvolvedTemplate struct {
	Name        string    `json:"name"`
	Template    string    `json:"template"`
	TaskType    string    `json:"task_type"`
	Generation  int       `json:"generation"`
	CreatedAt   time.Time `json:"created_at"`
	Trials      int       `json:"trials"`
	Successes   int       `json:"successes"`
	TotalCost   float64   `json:"total_cost"`
	AvgScore    float64   `json:"avg_score"`    // prompt quality score average
	Fitness     float64   `json:"fitness"`       // composite: 50% success + 30% cost efficiency + 20% trial bonus
	Status      string    `json:"status"`        // "active", "promoted", "demoted", "retired"
	ParentName  string    `json:"parent_name,omitempty"`
	MutationType string   `json:"mutation_type,omitempty"` // "rephrase", "expand", "simplify", "reorder"
}

// EvolutionEvent records a lifecycle event for audit trail.
type EvolutionEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	EventType  string    `json:"event_type"` // "created", "trial", "promoted", "demoted", "retired"
	TemplateName string  `json:"template_name"`
	Details    string    `json:"details"`
}

// NewTemplateEvolution creates an evolution manager.
func NewTemplateEvolution() *TemplateEvolution {
	return &TemplateEvolution{
		templates:  make(map[string]*EvolvedTemplate),
		maxHistory: 500,
	}
}

// AddTemplate registers a new template (from mining or manual creation).
func (te *TemplateEvolution) AddTemplate(name, template, taskType string) {
	te.mu.Lock()
	defer te.mu.Unlock()

	te.templates[name] = &EvolvedTemplate{
		Name:      name,
		Template:  template,
		TaskType:  taskType,
		CreatedAt: time.Now(),
		Status:    "active",
	}
	te.record("created", name, "New template registered")
}

// RecordTrial records a routing outcome for a template.
func (te *TemplateEvolution) RecordTrial(name string, success bool, costUSD, qualityScore float64) {
	te.mu.Lock()
	defer te.mu.Unlock()

	tmpl, ok := te.templates[name]
	if !ok {
		return
	}

	tmpl.Trials++
	if success {
		tmpl.Successes++
	}
	tmpl.TotalCost += costUSD

	// Rolling average for quality score
	tmpl.AvgScore += (qualityScore - tmpl.AvgScore) / float64(tmpl.Trials)

	// Recompute fitness (same formula as PromptEvolution)
	successRate := float64(tmpl.Successes) / float64(tmpl.Trials)
	costEfficiency := 1.0 / (1.0 + tmpl.TotalCost/float64(tmpl.Trials))
	trialBonus := 1.0 - 1.0/(1.0+float64(tmpl.Trials)/10.0)
	tmpl.Fitness = 0.50*successRate + 0.30*costEfficiency + 0.20*trialBonus

	te.record("trial", name, fmt.Sprintf("success=%v cost=%.3f score=%.0f fitness=%.3f",
		success, costUSD, qualityScore, tmpl.Fitness))

	// Auto-promote/demote based on fitness after minimum trials
	if tmpl.Trials >= 5 {
		if tmpl.Fitness >= 0.7 && tmpl.Status == "active" {
			tmpl.Status = "promoted"
			te.record("promoted", name, fmt.Sprintf("Fitness %.3f >= 0.7 after %d trials", tmpl.Fitness, tmpl.Trials))
		} else if tmpl.Fitness < 0.3 && tmpl.Status == "active" {
			tmpl.Status = "demoted"
			te.record("demoted", name, fmt.Sprintf("Fitness %.3f < 0.3 after %d trials", tmpl.Fitness, tmpl.Trials))
		}
	}
}

// SelectBest returns the highest-fitness active or promoted template for a task type.
func (te *TemplateEvolution) SelectBest(taskType string) (*EvolvedTemplate, bool) {
	te.mu.Lock()
	defer te.mu.Unlock()

	var best *EvolvedTemplate
	for _, tmpl := range te.templates {
		if tmpl.TaskType != taskType {
			continue
		}
		if tmpl.Status == "demoted" || tmpl.Status == "retired" {
			continue
		}
		if best == nil || tmpl.Fitness > best.Fitness {
			best = tmpl
		}
	}
	if best == nil {
		return nil, false
	}
	return best, true
}

// Leaderboard returns templates sorted by fitness for a task type.
func (te *TemplateEvolution) Leaderboard(taskType string, n int) []EvolvedTemplate {
	te.mu.Lock()
	defer te.mu.Unlock()

	var candidates []EvolvedTemplate
	for _, tmpl := range te.templates {
		if taskType != "" && tmpl.TaskType != taskType {
			continue
		}
		candidates = append(candidates, *tmpl)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Fitness > candidates[j].Fitness
	})
	if n > 0 && len(candidates) > n {
		candidates = candidates[:n]
	}
	return candidates
}

// History returns recent evolution events.
func (te *TemplateEvolution) History(n int) []EvolutionEvent {
	te.mu.Lock()
	defer te.mu.Unlock()

	if n <= 0 || n > len(te.history) {
		n = len(te.history)
	}
	// Return most recent n events
	start := len(te.history) - n
	if start < 0 {
		start = 0
	}
	result := make([]EvolutionEvent, n)
	copy(result, te.history[start:])
	return result
}

// Stats returns aggregate evolution statistics.
func (te *TemplateEvolution) Stats() map[string]any {
	te.mu.Lock()
	defer te.mu.Unlock()

	byStatus := map[string]int{}
	byTaskType := map[string]int{}
	var totalFitness float64
	var totalTrials int

	for _, tmpl := range te.templates {
		byStatus[tmpl.Status]++
		byTaskType[tmpl.TaskType]++
		totalFitness += tmpl.Fitness
		totalTrials += tmpl.Trials
	}

	var avgFitness float64
	if len(te.templates) > 0 {
		avgFitness = totalFitness / float64(len(te.templates))
	}

	return map[string]any{
		"total_templates": len(te.templates),
		"by_status":       byStatus,
		"by_task_type":    byTaskType,
		"avg_fitness":     avgFitness,
		"total_trials":    totalTrials,
		"event_count":     len(te.history),
	}
}

func (te *TemplateEvolution) record(eventType, name, details string) {
	te.history = append(te.history, EvolutionEvent{
		Timestamp:    time.Now(),
		EventType:    eventType,
		TemplateName: name,
		Details:      details,
	})
	if len(te.history) > te.maxHistory {
		te.history = te.history[len(te.history)-te.maxHistory:]
	}
}
