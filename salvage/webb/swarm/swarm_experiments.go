// Package clients provides A/B testing framework for swarm experiments.
package clients

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ExperimentManager manages A/B tests and experiments
type ExperimentManager struct {
	mu              sync.RWMutex
	experiments     map[string]*Experiment
	activeVariants  map[SwarmWorkerType]string
	results         []*ExperimentResult
}

// Experiment represents an A/B test configuration
type Experiment struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Status          ExperimentStatus    `json:"status"`
	CreatedAt       time.Time           `json:"created_at"`
	StartedAt       time.Time           `json:"started_at,omitempty"`
	EndedAt         time.Time           `json:"ended_at,omitempty"`
	TrafficSplit    float64             `json:"traffic_split"` // 0.5 = 50/50
	TargetWorkers   []SwarmWorkerType   `json:"target_workers"`
	ControlConfig   *ExperimentConfig   `json:"control_config"`
	TreatmentConfig *ExperimentConfig   `json:"treatment_config"`
	MetricTargets   []string            `json:"metric_targets"`
	MinSampleSize   int                 `json:"min_sample_size"`
	ControlData     *ExperimentData     `json:"control_data"`
	TreatmentData   *ExperimentData     `json:"treatment_data"`
	Result          *StatisticalResult  `json:"result,omitempty"`
}

// ExperimentStatus represents experiment lifecycle
type ExperimentStatus string

const (
	ExperimentDraft     ExperimentStatus = "draft"
	ExperimentRunning   ExperimentStatus = "running"
	ExperimentPaused    ExperimentStatus = "paused"
	ExperimentComplete  ExperimentStatus = "complete"
	ExperimentCancelled ExperimentStatus = "cancelled"
)

// ExperimentConfig captures experiment variant configuration
type ExperimentConfig struct {
	Name             string             `json:"name"`
	BudgetMultiplier float64            `json:"budget_multiplier"`
	ConfidenceThreshold float64         `json:"confidence_threshold"`
	MaxTokensPerRun  int64              `json:"max_tokens_per_run"`
	EnabledCategories []string          `json:"enabled_categories"`
	DisabledCategories []string         `json:"disabled_categories"`
	CustomParams     map[string]interface{} `json:"custom_params"`
}

// ExperimentData captures metrics for an experiment variant
type ExperimentData struct {
	SampleSize       int       `json:"sample_size"`
	Findings         int       `json:"findings"`
	AcceptedFindings int       `json:"accepted_findings"`
	TokensUsed       int64     `json:"tokens_used"`
	TotalConfidence  float64   `json:"total_confidence"`
	Values           []float64 `json:"values"` // Raw metric values for statistical analysis
}

// StatisticalResult captures A/B test statistical analysis
type StatisticalResult struct {
	Metric           string    `json:"metric"`
	ControlMean      float64   `json:"control_mean"`
	ControlStdDev    float64   `json:"control_std_dev"`
	TreatmentMean    float64   `json:"treatment_mean"`
	TreatmentStdDev  float64   `json:"treatment_std_dev"`
	AbsoluteDiff     float64   `json:"absolute_diff"`
	RelativeDiff     float64   `json:"relative_diff_pct"`
	EffectSize       float64   `json:"effect_size"`       // Cohen's d
	TStatistic       float64   `json:"t_statistic"`
	PValue           float64   `json:"p_value"`
	ConfidenceInterval [2]float64 `json:"confidence_interval"`
	StatisticalPower float64   `json:"statistical_power"`
	IsSignificant    bool      `json:"is_significant"`
	Winner           string    `json:"winner"` // "control", "treatment", "none"
	Recommendation   string    `json:"recommendation"`
}

// ExperimentResult captures final experiment outcomes
type ExperimentResult struct {
	ExperimentID   string             `json:"experiment_id"`
	CompletedAt    time.Time          `json:"completed_at"`
	Duration       time.Duration      `json:"duration"`
	SampleSize     int                `json:"sample_size"`
	Statistics     *StatisticalResult `json:"statistics"`
	ActionTaken    string             `json:"action_taken"`
}

// CanaryDeployment represents a canary release
type CanaryDeployment struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Status          string    `json:"status"` // "deploying", "running", "promoting", "rolled_back"
	StartedAt       time.Time `json:"started_at"`
	CanaryPercent   float64   `json:"canary_percent"`
	BaselineMetrics *CanaryMetrics `json:"baseline_metrics"`
	CanaryMetrics   *CanaryMetrics `json:"canary_metrics"`
	HealthScore     float64   `json:"health_score"`
	AutoPromote     bool      `json:"auto_promote"`
	PromoteThreshold float64  `json:"promote_threshold"`
}

// CanaryMetrics captures canary deployment metrics
type CanaryMetrics struct {
	ErrorRate       float64 `json:"error_rate"`
	LatencyP50      float64 `json:"latency_p50"`
	LatencyP99      float64 `json:"latency_p99"`
	AcceptanceRate  float64 `json:"acceptance_rate"`
	FindingsPerHour float64 `json:"findings_per_hour"`
}

// NewExperimentManager creates a new experiment manager
func NewExperimentManager() *ExperimentManager {
	return &ExperimentManager{
		experiments:    make(map[string]*Experiment),
		activeVariants: make(map[SwarmWorkerType]string),
		results:        make([]*ExperimentResult, 0),
	}
}

// CreateExperiment creates a new experiment
func (m *ExperimentManager) CreateExperiment(name, description string, control, treatment *ExperimentConfig) (*Experiment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := generateExperimentID()
	exp := &Experiment{
		ID:              id,
		Name:            name,
		Description:     description,
		Status:          ExperimentDraft,
		CreatedAt:       time.Now(),
		TrafficSplit:    0.5,
		ControlConfig:   control,
		TreatmentConfig: treatment,
		MinSampleSize:   100,
		ControlData:     &ExperimentData{Values: make([]float64, 0)},
		TreatmentData:   &ExperimentData{Values: make([]float64, 0)},
		MetricTargets:   []string{"acceptance_rate", "findings_per_hour"},
	}

	m.experiments[id] = exp
	return exp, nil
}

// StartExperiment begins an experiment
func (m *ExperimentManager) StartExperiment(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, ok := m.experiments[id]
	if !ok {
		return fmt.Errorf("experiment not found: %s", id)
	}

	if exp.Status != ExperimentDraft && exp.Status != ExperimentPaused {
		return fmt.Errorf("experiment cannot be started from status: %s", exp.Status)
	}

	exp.Status = ExperimentRunning
	exp.StartedAt = time.Now()
	return nil
}

// RecordObservation records a data point for an experiment
func (m *ExperimentManager) RecordObservation(id string, isControl bool, value float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, ok := m.experiments[id]
	if !ok {
		return fmt.Errorf("experiment not found: %s", id)
	}

	if exp.Status != ExperimentRunning {
		return fmt.Errorf("experiment not running")
	}

	data := exp.TreatmentData
	if isControl {
		data = exp.ControlData
	}

	data.SampleSize++
	data.Values = append(data.Values, value)

	// Check if we have enough data to analyze
	if exp.ControlData.SampleSize >= exp.MinSampleSize &&
		exp.TreatmentData.SampleSize >= exp.MinSampleSize {
		m.analyzeExperiment(exp)
	}

	return nil
}

// analyzeExperiment performs statistical analysis
func (m *ExperimentManager) analyzeExperiment(exp *Experiment) {
	controlMean, controlStdDev := calculateStats(exp.ControlData.Values)
	treatmentMean, treatmentStdDev := calculateStats(exp.TreatmentData.Values)

	// Calculate pooled standard deviation
	n1 := float64(len(exp.ControlData.Values))
	n2 := float64(len(exp.TreatmentData.Values))
	pooledVar := ((n1-1)*controlStdDev*controlStdDev + (n2-1)*treatmentStdDev*treatmentStdDev) / (n1 + n2 - 2)
	pooledStdDev := math.Sqrt(pooledVar)

	// Effect size (Cohen's d)
	effectSize := 0.0
	if pooledStdDev > 0 {
		effectSize = (treatmentMean - controlMean) / pooledStdDev
	}

	// T-statistic
	se := pooledStdDev * math.Sqrt(1/n1+1/n2)
	tStat := 0.0
	if se > 0 {
		tStat = (treatmentMean - controlMean) / se
	}

	// Approximate p-value (two-tailed)
	df := n1 + n2 - 2
	pValue := approximatePValue(math.Abs(tStat), df)

	// Confidence interval (95%)
	tCritical := 1.96 // Approximation for large samples
	margin := tCritical * se
	ci := [2]float64{treatmentMean - controlMean - margin, treatmentMean - controlMean + margin}

	// Statistical power (simplified)
	power := 1 - pValue // Simplified approximation

	// Determine significance and winner
	isSignificant := pValue < 0.05
	winner := "none"
	recommendation := "Continue collecting data"

	if isSignificant {
		if treatmentMean > controlMean {
			winner = "treatment"
			recommendation = "Adopt treatment configuration"
		} else {
			winner = "control"
			recommendation = "Keep control configuration"
		}
	}

	exp.Result = &StatisticalResult{
		Metric:             exp.MetricTargets[0],
		ControlMean:        controlMean,
		ControlStdDev:      controlStdDev,
		TreatmentMean:      treatmentMean,
		TreatmentStdDev:    treatmentStdDev,
		AbsoluteDiff:       treatmentMean - controlMean,
		RelativeDiff:       (treatmentMean - controlMean) / controlMean * 100,
		EffectSize:         effectSize,
		TStatistic:         tStat,
		PValue:             pValue,
		ConfidenceInterval: ci,
		StatisticalPower:   power,
		IsSignificant:      isSignificant,
		Winner:             winner,
		Recommendation:     recommendation,
	}
}

func calculateStats(values []float64) (mean, stdDev float64) {
	if len(values) == 0 {
		return 0, 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(len(values))

	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	stdDev = math.Sqrt(sumSquares / float64(len(values)))

	return mean, stdDev
}

func approximatePValue(tStat, df float64) float64 {
	// Simplified approximation using normal distribution for large df
	if df > 30 {
		// Use standard normal approximation
		return 2 * (1 - normalCDF(tStat))
	}
	// For smaller df, use a rougher approximation
	return 2 * (1 - normalCDF(tStat*math.Sqrt(df/(df-2))))
}

func normalCDF(x float64) float64 {
	return 0.5 * (1 + math.Erf(x/math.Sqrt(2)))
}

// CompleteExperiment ends an experiment
func (m *ExperimentManager) CompleteExperiment(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, ok := m.experiments[id]
	if !ok {
		return fmt.Errorf("experiment not found: %s", id)
	}

	exp.Status = ExperimentComplete
	exp.EndedAt = time.Now()

	// Record result
	result := &ExperimentResult{
		ExperimentID: id,
		CompletedAt:  time.Now(),
		Duration:     time.Since(exp.StartedAt),
		SampleSize:   exp.ControlData.SampleSize + exp.TreatmentData.SampleSize,
		Statistics:   exp.Result,
	}
	m.results = append(m.results, result)

	return nil
}

// GetExperiment returns an experiment by ID
func (m *ExperimentManager) GetExperiment(id string) (*Experiment, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exp, ok := m.experiments[id]
	return exp, ok
}

// ListExperiments returns all experiments
func (m *ExperimentManager) ListExperiments() []*Experiment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	experiments := make([]*Experiment, 0, len(m.experiments))
	for _, exp := range m.experiments {
		experiments = append(experiments, exp)
	}

	sort.Slice(experiments, func(i, j int) bool {
		return experiments[i].CreatedAt.After(experiments[j].CreatedAt)
	})

	return experiments
}

// GetActiveExperiments returns running experiments
func (m *ExperimentManager) GetActiveExperiments() []*Experiment {
	m.mu.RLock()
	defer m.mu.RUnlock()

	active := make([]*Experiment, 0)
	for _, exp := range m.experiments {
		if exp.Status == ExperimentRunning {
			active = append(active, exp)
		}
	}
	return active
}

// GetExperimentResults returns completed experiment results
func (m *ExperimentManager) GetExperimentResults(limit int) []*ExperimentResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.results) {
		return m.results
	}
	return m.results[len(m.results)-limit:]
}

// AssignVariant determines which variant a worker should use
func (m *ExperimentManager) AssignVariant(expID string, workerType SwarmWorkerType) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	exp, ok := m.experiments[expID]
	if !ok || exp.Status != ExperimentRunning {
		return "control"
	}

	// Check if already assigned
	if variant, ok := m.activeVariants[workerType]; ok {
		return variant
	}

	// Random assignment based on traffic split
	b := make([]byte, 1)
	rand.Read(b)
	if float64(b[0])/255.0 < exp.TrafficSplit {
		m.activeVariants[workerType] = "treatment"
		return "treatment"
	}

	m.activeVariants[workerType] = "control"
	return "control"
}

func generateExperimentID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Global singleton
var (
	globalExperimentManager   *ExperimentManager
	globalExperimentManagerMu sync.RWMutex
)

// GetExperimentManager returns or creates the global experiment manager
func GetExperimentManager() *ExperimentManager {
	globalExperimentManagerMu.Lock()
	defer globalExperimentManagerMu.Unlock()

	if globalExperimentManager == nil {
		globalExperimentManager = NewExperimentManager()
	}
	return globalExperimentManager
}
