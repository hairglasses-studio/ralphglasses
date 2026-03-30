package session

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

// PromptEvolution tracks prompt variants, runs A/B tests via tournament
// selection, and generates mutations of high-performing prompts.
// It maintains a population of prompt variants per task type, selects
// the best performers for production use, and periodically mutates
// to explore the prompt space.
type PromptEvolution struct {
	mu         sync.Mutex
	population map[string]*PromptPopulation // keyed by task type
	rng        *rand.Rand

	// tournamentSize is the number of candidates sampled for tournament selection.
	tournamentSize int

	// mutationRate controls how often mutations are introduced (0.0-1.0).
	mutationRate float64

	// maxVariants caps the population size per task type.
	maxVariants int
}

// PromptPopulation holds all variants for a given task type.
type PromptPopulation struct {
	TaskType string           `json:"task_type"`
	Variants []PromptVariant  `json:"variants"`
	BestID   string           `json:"best_id,omitempty"`
	Generation int            `json:"generation"`
}

// PromptVariant is a single prompt variant with tracked performance.
type PromptVariant struct {
	ID         string    `json:"id"`
	Template   string    `json:"template"`
	ParentID   string    `json:"parent_id,omitempty"` // empty for seed prompts
	Generation int       `json:"generation"`
	CreatedAt  time.Time `json:"created_at"`

	// A/B test results.
	Trials    int     `json:"trials"`
	Successes int     `json:"successes"`
	TotalCost float64 `json:"total_cost_usd"`
	TotalDur  float64 `json:"total_duration_sec"`
	AvgScore  float64 `json:"avg_score"` // composite fitness score

	// Mutation metadata.
	MutationType string `json:"mutation_type,omitempty"` // "rephrase", "expand", "simplify", "combine"
}

// PromptTrialResult records the outcome of using a prompt variant.
type PromptTrialResult struct {
	VariantID string  `json:"variant_id"`
	TaskType  string  `json:"task_type"`
	Success   bool    `json:"success"`
	CostUSD   float64 `json:"cost_usd"`
	DurSec    float64 `json:"duration_sec"`
	Quality   float64 `json:"quality"` // 0.0-1.0 quality score from output
}

// PromptEvolutionConfig holds tuning parameters.
type PromptEvolutionConfig struct {
	TournamentSize int     `json:"tournament_size"` // candidates per selection (default 3)
	MutationRate   float64 `json:"mutation_rate"`   // probability of mutation (default 0.2)
	MaxVariants    int     `json:"max_variants"`    // max population size per task (default 10)
}

// DefaultPromptEvolutionConfig returns sensible defaults.
func DefaultPromptEvolutionConfig() PromptEvolutionConfig {
	return PromptEvolutionConfig{
		TournamentSize: 3,
		MutationRate:   0.2,
		MaxVariants:    10,
	}
}

// NewPromptEvolution creates a prompt evolution tracker.
func NewPromptEvolution(cfg PromptEvolutionConfig) *PromptEvolution {
	if cfg.TournamentSize <= 0 {
		cfg.TournamentSize = 3
	}
	if cfg.MutationRate <= 0 {
		cfg.MutationRate = 0.2
	}
	if cfg.MaxVariants <= 0 {
		cfg.MaxVariants = 10
	}
	return &PromptEvolution{
		population:     make(map[string]*PromptPopulation),
		rng:            rand.New(rand.NewSource(time.Now().UnixNano())),
		tournamentSize: cfg.TournamentSize,
		mutationRate:   cfg.MutationRate,
		maxVariants:    cfg.MaxVariants,
	}
}

// AddVariant registers a new prompt variant for the given task type.
// If the variant is the first for this task type, it becomes the seed.
func (pe *PromptEvolution) AddVariant(taskType, template string) string {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pop, ok := pe.population[taskType]
	if !ok {
		pop = &PromptPopulation{TaskType: taskType}
		pe.population[taskType] = pop
	}

	id := fmt.Sprintf("%s-v%d-%d", taskType, pop.Generation, len(pop.Variants))
	variant := PromptVariant{
		ID:         id,
		Template:   template,
		Generation: pop.Generation,
		CreatedAt:  time.Now(),
	}
	pop.Variants = append(pop.Variants, variant)

	// Enforce population cap by removing the weakest.
	if len(pop.Variants) > pe.maxVariants {
		pe.pruneWeakest(pop)
	}

	return id
}

// RecordTrial records the outcome of using a specific prompt variant.
func (pe *PromptEvolution) RecordTrial(result PromptTrialResult) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pop, ok := pe.population[result.TaskType]
	if !ok {
		return
	}

	for i := range pop.Variants {
		if pop.Variants[i].ID == result.VariantID {
			v := &pop.Variants[i]
			v.Trials++
			if result.Success {
				v.Successes++
			}
			v.TotalCost += result.CostUSD
			v.TotalDur += result.DurSec

			// Update composite fitness score.
			v.AvgScore = pe.computeFitness(v)
			return
		}
	}
}

// SelectBest uses tournament selection to pick the best-performing variant
// for the given task type. Returns the variant template and ID.
// If no variants exist, returns empty strings.
func (pe *PromptEvolution) SelectBest(taskType string) (template string, variantID string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pop, ok := pe.population[taskType]
	if !ok || len(pop.Variants) == 0 {
		return "", ""
	}

	// If only one variant, return it.
	if len(pop.Variants) == 1 {
		return pop.Variants[0].Template, pop.Variants[0].ID
	}

	winner := pe.tournamentSelect(pop)
	if winner == nil {
		return pop.Variants[0].Template, pop.Variants[0].ID
	}

	pop.BestID = winner.ID
	return winner.Template, winner.ID
}

// tournamentSelect samples tournamentSize variants and returns the fittest.
// Must be called with mu held.
func (pe *PromptEvolution) tournamentSelect(pop *PromptPopulation) *PromptVariant {
	n := len(pop.Variants)
	if n == 0 {
		return nil
	}

	k := pe.tournamentSize
	if k > n {
		k = n
	}

	// Sample k distinct indices.
	indices := pe.rng.Perm(n)[:k]

	var best *PromptVariant
	bestFitness := -1.0

	for _, idx := range indices {
		v := &pop.Variants[idx]
		fitness := pe.computeFitness(v)
		if fitness > bestFitness {
			bestFitness = fitness
			best = v
		}
	}

	return best
}

// computeFitness calculates a composite fitness score for a variant.
// Balances success rate (50%), cost efficiency (30%), and trial count bonus (20%).
func (pe *PromptEvolution) computeFitness(v *PromptVariant) float64 {
	if v.Trials == 0 {
		return 0.5 // neutral for untested variants
	}

	successRate := float64(v.Successes) / float64(v.Trials)
	avgCost := v.TotalCost / float64(v.Trials)

	// Cost efficiency: cheaper is better.
	costScore := 1.0 / (1.0 + avgCost)

	// Trial count bonus: more tested variants get a small credibility boost.
	// Asymptotic bonus up to 1.0 at ~20 trials.
	trialBonus := 1.0 - 1.0/(1.0+float64(v.Trials)/10.0)

	return 0.50*successRate + 0.30*costScore + 0.20*trialBonus
}

// Mutate generates a mutated variant from the best performer for the given
// task type. Returns the new variant ID, or empty string if mutation was
// skipped (no variants, or random chance didn't trigger).
func (pe *PromptEvolution) Mutate(taskType string) string {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pop, ok := pe.population[taskType]
	if !ok || len(pop.Variants) == 0 {
		return ""
	}

	// Check mutation probability.
	if pe.rng.Float64() >= pe.mutationRate {
		return ""
	}

	// Pick parent via tournament selection.
	parent := pe.tournamentSelect(pop)
	if parent == nil {
		return ""
	}

	// Apply a random mutation strategy.
	mutationType, mutated := pe.applyMutation(parent.Template)

	pop.Generation++
	id := fmt.Sprintf("%s-v%d-m%d", taskType, pop.Generation, len(pop.Variants))
	variant := PromptVariant{
		ID:           id,
		Template:     mutated,
		ParentID:     parent.ID,
		Generation:   pop.Generation,
		CreatedAt:    time.Now(),
		MutationType: mutationType,
	}
	pop.Variants = append(pop.Variants, variant)

	if len(pop.Variants) > pe.maxVariants {
		pe.pruneWeakest(pop)
	}

	return id
}

// applyMutation applies a deterministic text transformation to create
// a prompt variant. Returns the mutation type name and the mutated template.
func (pe *PromptEvolution) applyMutation(template string) (string, string) {
	strategies := []struct {
		name string
		fn   func(string) string
	}{
		{"rephrase", pe.mutateRephrase},
		{"expand", pe.mutateExpand},
		{"simplify", pe.mutateSimplify},
		{"reorder", pe.mutateReorder},
	}

	idx := pe.rng.Intn(len(strategies))
	s := strategies[idx]
	return s.name, s.fn(template)
}

// mutateRephrase swaps common instruction words with synonyms.
func (pe *PromptEvolution) mutateRephrase(template string) string {
	replacements := [][2]string{
		{"implement", "create"},
		{"ensure", "make sure"},
		{"should", "must"},
		{"analyze", "examine"},
		{"fix", "resolve"},
		{"add", "include"},
		{"remove", "eliminate"},
		{"update", "modify"},
	}

	result := template
	// Apply one random replacement.
	pe.rng.Shuffle(len(replacements), func(i, j int) {
		replacements[i], replacements[j] = replacements[j], replacements[i]
	})
	for _, r := range replacements {
		if strings.Contains(strings.ToLower(result), r[0]) {
			result = strings.Replace(result, r[0], r[1], 1)
			break
		}
	}
	return result
}

// mutateExpand adds specificity markers to the prompt.
func (pe *PromptEvolution) mutateExpand(template string) string {
	suffixes := []string{
		" Be thorough and precise.",
		" Include edge cases in your analysis.",
		" Consider performance implications.",
		" Ensure backward compatibility.",
	}
	idx := pe.rng.Intn(len(suffixes))
	return template + suffixes[idx]
}

// mutateSimplify removes trailing sentences to create a more concise variant.
func (pe *PromptEvolution) mutateSimplify(template string) string {
	sentences := strings.Split(template, ". ")
	if len(sentences) <= 1 {
		return template
	}
	// Remove the last sentence.
	return strings.Join(sentences[:len(sentences)-1], ". ") + "."
}

// mutateReorder shuffles sentence order in the prompt.
func (pe *PromptEvolution) mutateReorder(template string) string {
	sentences := strings.Split(template, ". ")
	if len(sentences) <= 2 {
		return template
	}
	// Keep first sentence (usually the main instruction), shuffle the rest.
	rest := sentences[1:]
	pe.rng.Shuffle(len(rest), func(i, j int) {
		rest[i], rest[j] = rest[j], rest[i]
	})
	return sentences[0] + ". " + strings.Join(rest, ". ")
}

// pruneWeakest removes the lowest-fitness variant from the population.
// Must be called with mu held.
func (pe *PromptEvolution) pruneWeakest(pop *PromptPopulation) {
	if len(pop.Variants) == 0 {
		return
	}

	worstIdx := 0
	worstFitness := pe.computeFitness(&pop.Variants[0])

	for i := 1; i < len(pop.Variants); i++ {
		f := pe.computeFitness(&pop.Variants[i])
		if f < worstFitness {
			worstFitness = f
			worstIdx = i
		}
	}

	pop.Variants = append(pop.Variants[:worstIdx], pop.Variants[worstIdx+1:]...)
}

// PopulationSnapshot returns a JSON-serializable view of all populations.
func (pe *PromptEvolution) PopulationSnapshot() map[string]*PromptPopulation {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	result := make(map[string]*PromptPopulation, len(pe.population))
	for k, v := range pe.population {
		// Deep copy to avoid race.
		cp := *v
		cp.Variants = make([]PromptVariant, len(v.Variants))
		copy(cp.Variants, v.Variants)
		result[k] = &cp
	}
	return result
}

// VariantCount returns the number of variants tracked across all task types.
func (pe *PromptEvolution) VariantCount() int {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	total := 0
	for _, pop := range pe.population {
		total += len(pop.Variants)
	}
	return total
}

// Leaderboard returns the top N variants for a given task type, sorted
// by fitness descending.
func (pe *PromptEvolution) Leaderboard(taskType string, n int) []PromptVariant {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pop, ok := pe.population[taskType]
	if !ok || len(pop.Variants) == 0 {
		return nil
	}

	// Copy and sort by fitness.
	sorted := make([]PromptVariant, len(pop.Variants))
	copy(sorted, pop.Variants)

	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0; j-- {
			fi := pe.computeFitness(&sorted[j])
			fj := pe.computeFitness(&sorted[j-1])
			if fi > fj {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			}
		}
	}

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// MarshalJSON implements json.Marshaler for state export.
func (pe *PromptEvolution) MarshalJSON() ([]byte, error) {
	snapshot := pe.PopulationSnapshot()
	return json.Marshal(snapshot)
}
