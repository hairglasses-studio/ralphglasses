package fleet

import (
	"math"
	"sync"
	"time"
)

// TaskCategory represents a classification of work for specialization tracking.
type TaskCategory string

const (
	CategoryCodeGen    TaskCategory = "code_generation"
	CategoryCodeReview TaskCategory = "code_review"
	CategoryRefactor   TaskCategory = "refactoring"
	CategoryTesting    TaskCategory = "testing"
	CategoryDebug      TaskCategory = "debugging"
	CategoryDocs       TaskCategory = "documentation"
	CategoryResearch   TaskCategory = "research"
	CategoryGeneral    TaskCategory = "general"
)

// AllCategories returns the list of known task categories.
func AllCategories() []TaskCategory {
	return []TaskCategory{
		CategoryCodeGen, CategoryCodeReview, CategoryRefactor,
		CategoryTesting, CategoryDebug, CategoryDocs,
		CategoryResearch, CategoryGeneral,
	}
}

// CategoryRecord tracks a single worker's performance in a single category.
type CategoryRecord struct {
	Successes  int       `json:"successes"`
	Failures   int       `json:"failures"`
	TotalTimeS float64   `json:"total_time_seconds"`
	LastUsed   time.Time `json:"last_used"`
}

// Total returns total attempts for this record.
func (r *CategoryRecord) Total() int { return r.Successes + r.Failures }

// SuccessRate returns the success rate, or 0 if no attempts.
func (r *CategoryRecord) SuccessRate() float64 {
	if r.Total() == 0 {
		return 0
	}
	return float64(r.Successes) / float64(r.Total())
}

// AvgDuration returns average task duration in seconds, or 0 if no successes.
func (r *CategoryRecord) AvgDuration() float64 {
	if r.Successes == 0 {
		return 0
	}
	return r.TotalTimeS / float64(r.Successes)
}

// workerKey builds a lookup key for worker+category.
type workerKey struct {
	WorkerID string
	Category TaskCategory
}

// SpecializerConfig holds tuning parameters for the WorkerSpecializer.
type SpecializerConfig struct {
	// ColdStartScore is the default score assigned when a worker has no history
	// for a category. Higher values encourage exploration. Range: 0.0-1.0.
	ColdStartScore float64

	// MinSamplesForConfidence is the number of completed tasks before the
	// specializer trusts the historical success rate over the cold-start score.
	MinSamplesForConfidence int

	// RecencyDecayDays controls how quickly old performance data loses weight.
	// Records older than this many days are halved in influence.
	RecencyDecayDays int

	// SpeedWeight controls how much faster completion time improves a score.
	// Range: 0.0-1.0. 0 means speed is ignored; 1.0 means speed is as
	// important as success rate.
	SpeedWeight float64
}

// DefaultSpecializerConfig returns reasonable defaults.
func DefaultSpecializerConfig() SpecializerConfig {
	return SpecializerConfig{
		ColdStartScore:          0.5,
		MinSamplesForConfidence: 3,
		RecencyDecayDays:        7,
		SpeedWeight:             0.3,
	}
}

// WorkerSpecializer assigns workers to task categories based on historical
// performance. It tracks success rate, speed, and recency per worker/category
// pair and uses weighted scoring for optimal assignment.
type WorkerSpecializer struct {
	mu      sync.Mutex
	records map[workerKey]*CategoryRecord
	config  SpecializerConfig
}

// NewWorkerSpecializer creates a specializer with the given config.
func NewWorkerSpecializer(cfg SpecializerConfig) *WorkerSpecializer {
	return &WorkerSpecializer{
		records: make(map[workerKey]*CategoryRecord),
		config:  cfg,
	}
}

// RecordOutcome records a task completion (success or failure) for a worker
// in a given category. durationS is the wall-clock seconds for the task.
func (ws *WorkerSpecializer) RecordOutcome(workerID string, category TaskCategory, success bool, durationS float64) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	key := workerKey{WorkerID: workerID, Category: category}
	rec, ok := ws.records[key]
	if !ok {
		rec = &CategoryRecord{}
		ws.records[key] = rec
	}

	if success {
		rec.Successes++
		rec.TotalTimeS += durationS
	} else {
		rec.Failures++
	}
	rec.LastUsed = time.Now()
}

// Score returns the specialization score for a worker in a category.
// Higher is better. Range roughly 0.0-1.0.
func (ws *WorkerSpecializer) Score(workerID string, category TaskCategory) float64 {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.scoreLocked(workerID, category)
}

func (ws *WorkerSpecializer) scoreLocked(workerID string, category TaskCategory) float64 {
	key := workerKey{WorkerID: workerID, Category: category}
	rec, ok := ws.records[key]
	if !ok || rec.Total() == 0 {
		return ws.config.ColdStartScore
	}

	// Blend cold-start and empirical success rate based on sample count.
	confidence := math.Min(float64(rec.Total())/float64(ws.config.MinSamplesForConfidence), 1.0)
	blended := (1-confidence)*ws.config.ColdStartScore + confidence*rec.SuccessRate()

	// Apply speed bonus: normalize avg duration against a 300s baseline.
	// Faster workers get a small boost.
	speedBonus := 0.0
	if rec.Successes > 0 && ws.config.SpeedWeight > 0 {
		avgDur := rec.AvgDuration()
		const baseline = 300.0 // 5 minutes as the reference
		if avgDur > 0 {
			// Ratio < 1 means faster than baseline → positive bonus.
			speedBonus = ws.config.SpeedWeight * math.Max(0, 1.0-avgDur/baseline)
		}
	}

	// Apply recency decay: reduce score if last use was long ago.
	recencyFactor := 1.0
	if ws.config.RecencyDecayDays > 0 {
		daysSince := time.Since(rec.LastUsed).Hours() / 24
		halfLives := daysSince / float64(ws.config.RecencyDecayDays)
		recencyFactor = math.Pow(0.5, halfLives)
	}

	score := (blended + speedBonus) * recencyFactor
	return math.Min(score, 1.0)
}

// BestWorker selects the best worker ID for a given category from the
// provided candidate list. Returns empty string if candidates is empty.
func (ws *WorkerSpecializer) BestWorker(candidates []string, category TaskCategory) string {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if len(candidates) == 0 {
		return ""
	}

	bestID := candidates[0]
	bestScore := ws.scoreLocked(candidates[0], category)

	for _, id := range candidates[1:] {
		s := ws.scoreLocked(id, category)
		if s > bestScore {
			bestScore = s
			bestID = id
		}
	}
	return bestID
}

// RankWorkers returns all candidates ranked by score for a category,
// highest first. Each entry is a WorkerScore.
func (ws *WorkerSpecializer) RankWorkers(candidates []string, category TaskCategory) []WorkerScore {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ranked := make([]WorkerScore, len(candidates))
	for i, id := range candidates {
		ranked[i] = WorkerScore{
			WorkerID: id,
			Score:    ws.scoreLocked(id, category),
		}
	}

	// Sort descending by score (insertion sort — candidate lists are small).
	for i := 1; i < len(ranked); i++ {
		j := i
		for j > 0 && ranked[j].Score > ranked[j-1].Score {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
			j--
		}
	}
	return ranked
}

// WorkerScore pairs a worker ID with its specialization score.
type WorkerScore struct {
	WorkerID string  `json:"worker_id"`
	Score    float64 `json:"score"`
}

// WorkerStrengths returns the top categories for a worker, sorted by score.
func (ws *WorkerSpecializer) WorkerStrengths(workerID string) []CategoryScore {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	cats := AllCategories()
	scores := make([]CategoryScore, len(cats))
	for i, c := range cats {
		scores[i] = CategoryScore{
			Category: c,
			Score:    ws.scoreLocked(workerID, c),
		}
	}

	// Sort descending.
	for i := 1; i < len(scores); i++ {
		j := i
		for j > 0 && scores[j].Score > scores[j-1].Score {
			scores[j], scores[j-1] = scores[j-1], scores[j]
			j--
		}
	}
	return scores
}

// CategoryScore pairs a category with its score.
type CategoryScore struct {
	Category TaskCategory `json:"category"`
	Score    float64      `json:"score"`
}

// GetRecord returns the performance record for a worker/category pair,
// or nil if no history exists.
func (ws *WorkerSpecializer) GetRecord(workerID string, category TaskCategory) *CategoryRecord {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	key := workerKey{WorkerID: workerID, Category: category}
	rec, ok := ws.records[key]
	if !ok {
		return nil
	}
	// Return a copy to avoid data races.
	cp := *rec
	return &cp
}

// Reset clears all specialization history.
func (ws *WorkerSpecializer) Reset() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.records = make(map[workerKey]*CategoryRecord)
}
