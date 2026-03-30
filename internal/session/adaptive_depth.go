package session

import (
	"math"
	"sync"
	"time"
)

// ComplexityLevel classifies task difficulty for adaptive depth adjustment.
type ComplexityLevel int

const (
	ComplexityLow    ComplexityLevel = 1
	ComplexityMedium ComplexityLevel = 2
	ComplexityHigh   ComplexityLevel = 3
)

// AdaptiveDepthConfig holds tuning parameters for AdaptiveDepth.
type AdaptiveDepthConfig struct {
	// BaseDepth is the default iteration count when no signals are available.
	BaseDepth int

	// MinDepth is the absolute floor for iteration depth.
	MinAdaptiveDepth int

	// MaxDepth is the absolute ceiling for iteration depth.
	MaxAdaptiveDepth int

	// QuickWinThreshold: if a session completes within this fraction of its
	// depth with positive progress, the depth may be reduced next time.
	// Range: 0.0-1.0.
	QuickWinThreshold float64

	// StallThreshold: if progress rate drops below this fraction per iteration,
	// the task is considered stalled and depth increases. Range: 0.0-1.0.
	StallThreshold float64

	// DepthIncrement controls how many iterations to add when increasing depth.
	DepthIncrement int

	// DepthDecrement controls how many iterations to remove when reducing depth.
	DepthDecrement int
}

// DefaultAdaptiveDepthConfig returns reasonable defaults.
func DefaultAdaptiveDepthConfig() AdaptiveDepthConfig {
	return AdaptiveDepthConfig{
		BaseDepth:         DefaultDepth, // reuse existing constant (10)
		MinAdaptiveDepth:  MinDepth,     // reuse existing constant (3)
		MaxAdaptiveDepth:  MaxDepth,     // reuse existing constant (50)
		QuickWinThreshold: 0.4,
		StallThreshold:    0.1,
		DepthIncrement:    3,
		DepthDecrement:    2,
	}
}

// ProgressSignal represents a single observation of session progress.
type ProgressSignal struct {
	// Iteration is the current iteration number (1-based).
	Iteration int

	// TotalDepth is the configured iteration depth for this session.
	TotalDepth int

	// ProgressRate is a 0.0-1.0 measure of how much useful work was done
	// in this iteration. 1.0 means maximum progress; 0.0 means no progress.
	ProgressRate float64

	// Completed indicates the session finished its task.
	Completed bool

	// Errored indicates the session hit an error.
	Errored bool

	// Timestamp is when this signal was observed.
	Timestamp time.Time
}

// adaptiveHistory tracks depth adjustment state for a single task pattern.
type adaptiveHistory struct {
	// recentSignals holds the last N signals for trend analysis.
	recentSignals []ProgressSignal

	// adjustedDepth is the current recommended depth (0 means use base).
	adjustedDepth int

	// completions tracks how many sessions with this pattern completed successfully.
	completions int

	// stalls tracks sessions that appeared stalled.
	stalls int

	// quickWins tracks sessions that completed well under budget.
	quickWins int
}

// AdaptiveDepth dynamically adjusts session iteration depth based on task
// complexity and progress rate. It increases depth for hard tasks that stall
// and reduces it for tasks that consistently complete early (quick wins).
//
// Unlike DepthEstimator (which provides static estimates from task metadata),
// AdaptiveDepth learns at runtime from progress signals across sessions.
type AdaptiveDepth struct {
	mu      sync.Mutex
	config  AdaptiveDepthConfig
	history map[string]*adaptiveHistory // keyed by task pattern
}

// NewAdaptiveDepth creates an AdaptiveDepth controller with the given config.
func NewAdaptiveDepth(cfg AdaptiveDepthConfig) *AdaptiveDepth {
	return &AdaptiveDepth{
		config:  cfg,
		history: make(map[string]*adaptiveHistory),
	}
}

// RecommendDepth returns the recommended iteration depth for a task pattern
// at the given complexity level. The pattern is an opaque string that groups
// similar tasks (e.g., "refactor:internal/session" or "test:fleet").
func (ad *AdaptiveDepth) RecommendDepth(pattern string, complexity ComplexityLevel) int {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	base := ad.baseForComplexity(complexity)

	hist, ok := ad.history[pattern]
	if !ok || hist.adjustedDepth == 0 {
		return ad.clampAdaptive(base)
	}

	return ad.clampAdaptive(hist.adjustedDepth)
}

// RecordSignal records a progress observation and updates depth recommendations.
func (ad *AdaptiveDepth) RecordSignal(pattern string, signal ProgressSignal) {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	hist, ok := ad.history[pattern]
	if !ok {
		hist = &adaptiveHistory{}
		ad.history[pattern] = hist
	}

	// Keep last 20 signals per pattern.
	hist.recentSignals = append(hist.recentSignals, signal)
	if len(hist.recentSignals) > 20 {
		hist.recentSignals = hist.recentSignals[len(hist.recentSignals)-20:]
	}

	// Detect quick win: completed early with good progress.
	if signal.Completed && signal.TotalDepth > 0 {
		usedFraction := float64(signal.Iteration) / float64(signal.TotalDepth)
		if usedFraction <= ad.config.QuickWinThreshold {
			hist.quickWins++
			ad.adjustDown(hist)
			return
		}
		hist.completions++
	}

	// Detect stall: low progress rate past the midpoint.
	if signal.ProgressRate <= ad.config.StallThreshold && !signal.Completed && !signal.Errored {
		if signal.TotalDepth > 0 && signal.Iteration > signal.TotalDepth/2 {
			hist.stalls++
			ad.adjustUp(hist)
			return
		}
	}

	// Detect error near depth exhaustion: give more room next time.
	if signal.Errored && signal.TotalDepth > 0 {
		fractionUsed := float64(signal.Iteration) / float64(signal.TotalDepth)
		if fractionUsed > 0.8 {
			ad.adjustUp(hist)
		}
	}
}

// adjustDown reduces the recommended depth for a pattern.
func (ad *AdaptiveDepth) adjustDown(hist *adaptiveHistory) {
	current := hist.adjustedDepth
	if current == 0 {
		current = ad.config.BaseDepth
	}
	hist.adjustedDepth = ad.clampAdaptive(current - ad.config.DepthDecrement)
}

// adjustUp increases the recommended depth for a pattern.
func (ad *AdaptiveDepth) adjustUp(hist *adaptiveHistory) {
	current := hist.adjustedDepth
	if current == 0 {
		current = ad.config.BaseDepth
	}
	hist.adjustedDepth = ad.clampAdaptive(current + ad.config.DepthIncrement)
}

func (ad *AdaptiveDepth) baseForComplexity(c ComplexityLevel) int {
	switch c {
	case ComplexityLow:
		return int(math.Max(float64(ad.config.MinAdaptiveDepth), float64(ad.config.BaseDepth)*0.6))
	case ComplexityHigh:
		return int(math.Min(float64(ad.config.MaxAdaptiveDepth), float64(ad.config.BaseDepth)*1.8))
	default:
		return ad.config.BaseDepth
	}
}

func (ad *AdaptiveDepth) clampAdaptive(depth int) int {
	if depth < ad.config.MinAdaptiveDepth {
		return ad.config.MinAdaptiveDepth
	}
	if depth > ad.config.MaxAdaptiveDepth {
		return ad.config.MaxAdaptiveDepth
	}
	return depth
}

// CurrentDepth returns the currently adjusted depth for a pattern,
// or 0 if no adjustment has been made.
func (ad *AdaptiveDepth) CurrentDepth(pattern string) int {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	hist, ok := ad.history[pattern]
	if !ok {
		return 0
	}
	return hist.adjustedDepth
}

// AdaptiveDepthStats holds summary information about depth adjustments for a pattern.
type AdaptiveDepthStats struct {
	AdjustedDepth   int     `json:"adjusted_depth"`
	Completions     int     `json:"completions"`
	QuickWins       int     `json:"quick_wins"`
	Stalls          int     `json:"stalls"`
	SignalCount     int     `json:"signal_count"`
	AvgProgressRate float64 `json:"avg_progress_rate"`
}

// Stats returns summary statistics for a task pattern.
func (ad *AdaptiveDepth) Stats(pattern string) AdaptiveDepthStats {
	ad.mu.Lock()
	defer ad.mu.Unlock()

	hist, ok := ad.history[pattern]
	if !ok {
		return AdaptiveDepthStats{}
	}

	avgProgress := 0.0
	if len(hist.recentSignals) > 0 {
		sum := 0.0
		for _, s := range hist.recentSignals {
			sum += s.ProgressRate
		}
		avgProgress = sum / float64(len(hist.recentSignals))
	}

	return AdaptiveDepthStats{
		AdjustedDepth:   hist.adjustedDepth,
		Completions:     hist.completions,
		QuickWins:       hist.quickWins,
		Stalls:          hist.stalls,
		SignalCount:     len(hist.recentSignals),
		AvgProgressRate: avgProgress,
	}
}

// Reset clears all adaptive depth history.
func (ad *AdaptiveDepth) Reset() {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	ad.history = make(map[string]*adaptiveHistory)
}
