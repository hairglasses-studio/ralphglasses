package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ActionCategory classifies autonomous actions by their potential impact.
type ActionCategory string

const (
	// CategoryReadOnly covers actions that observe but don't mutate state.
	CategoryReadOnly ActionCategory = "read_only"
	// CategoryReversible covers actions that can be undone (restart, pause).
	CategoryReversible ActionCategory = "reversible"
	// CategoryIrreversible covers actions that cannot be undone (delete, force-push).
	CategoryIrreversible ActionCategory = "irreversible"
	// CategoryCostBearing covers actions that spend money (launch sessions, increase budgets).
	CategoryCostBearing ActionCategory = "cost_bearing"
	// CategorySecurity covers actions with security implications (tool allowlists, sandboxing).
	CategorySecurity ActionCategory = "security"
)

// RiskLevel represents the assessed risk of an autonomous action.
type RiskLevel int

const (
	RiskNone   RiskLevel = 0
	RiskLow    RiskLevel = 1
	RiskMedium RiskLevel = 2
	RiskHigh   RiskLevel = 3
)

// String returns the human-readable name for a risk level.
func (r RiskLevel) String() string {
	switch r {
	case RiskNone:
		return "none"
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	default:
		return fmt.Sprintf("unknown(%d)", r)
	}
}

// AutoAction represents an action the system wants to take autonomously.
type AutoAction struct {
	Name        string         `json:"name"`
	Category    ActionCategory `json:"category"`
	Description string         `json:"description"`
	SessionID   string         `json:"session_id,omitempty"`
	CostUSD     float64        `json:"cost_usd,omitempty"` // estimated cost if applicable
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ActionRecord logs a completed autonomous action with its outcome.
type ActionRecord struct {
	Action    AutoAction `json:"action"`
	Risk      RiskLevel  `json:"risk"`
	Permitted bool       `json:"permitted"`
	Timestamp time.Time  `json:"timestamp"`
	Outcome   string     `json:"outcome,omitempty"` // "success", "failed", "blocked"
	Reason    string     `json:"reason,omitempty"`  // why it was permitted or denied
}

// PermissionPolicy defines per-category rules for autonomous action.
type PermissionPolicy struct {
	// MaxRisk is the maximum risk level allowed without human approval.
	MaxRisk RiskLevel `json:"max_risk"`
	// CostLimitUSD is the maximum single-action cost allowed autonomously.
	// Zero means no cost-bearing actions allowed.
	CostLimitUSD float64 `json:"cost_limit_usd"`
	// RateLimitPerHour caps how many actions of this category per hour.
	// Zero means unlimited.
	RateLimitPerHour int `json:"rate_limit_per_hour"`
}

// AutoModeConfig configures the autonomous operation controller.
type AutoModeConfig struct {
	// Enabled activates auto-mode. When false, all actions require human approval.
	Enabled bool `json:"enabled"`
	// AutonomyLevel gates which action categories are permitted.
	AutonomyLevel AutonomyLevel `json:"autonomy_level"`
	// Policies override default permission rules per category.
	Policies map[ActionCategory]PermissionPolicy `json:"policies,omitempty"`
	// GlobalCostLimitUSD caps total autonomous spending per hour.
	GlobalCostLimitUSD float64 `json:"global_cost_limit_usd"`
	// HistoryLimit caps how many action records to retain.
	HistoryLimit int `json:"history_limit"`
}

// DefaultAutoModeConfig returns a conservative default configuration.
func DefaultAutoModeConfig() AutoModeConfig {
	return AutoModeConfig{
		Enabled:            false,
		AutonomyLevel:      LevelObserve,
		GlobalCostLimitUSD: 1.0,
		HistoryLimit:       1000,
		Policies: map[ActionCategory]PermissionPolicy{
			CategoryReadOnly: {
				MaxRisk:          RiskMedium,
				RateLimitPerHour: 0, // unlimited
			},
			CategoryReversible: {
				MaxRisk:          RiskLow,
				RateLimitPerHour: 30,
			},
			CategoryIrreversible: {
				MaxRisk:          RiskNone,
				RateLimitPerHour: 0, // blocked by default
			},
			CategoryCostBearing: {
				MaxRisk:          RiskLow,
				CostLimitUSD:     0.50,
				RateLimitPerHour: 10,
			},
			CategorySecurity: {
				MaxRisk:          RiskNone,
				RateLimitPerHour: 0, // blocked by default
			},
		},
	}
}

// AutoMode controls autonomous operation, deciding when actions can proceed
// without human approval and tracking action history for auditing.
type AutoMode struct {
	mu      sync.Mutex
	config  AutoModeConfig
	history []ActionRecord
}

// NewAutoMode creates an AutoMode controller with the given config.
func NewAutoMode(cfg AutoModeConfig) *AutoMode {
	if cfg.HistoryLimit <= 0 {
		cfg.HistoryLimit = 1000
	}
	if cfg.Policies == nil {
		cfg.Policies = DefaultAutoModeConfig().Policies
	}
	return &AutoMode{
		config:  cfg,
		history: make([]ActionRecord, 0, 64),
	}
}

// ScoreRisk evaluates the risk level of an action based on its category,
// estimated cost, and contextual signals.
func (am *AutoMode) ScoreRisk(action AutoAction) RiskLevel {
	// Base risk from category.
	base := categoryBaseRisk(action.Category)

	// Escalate for high-cost actions.
	if action.CostUSD > 0 {
		if action.CostUSD > 5.0 {
			base = maxRisk(base, RiskHigh)
		} else if action.CostUSD > 1.0 {
			base = maxRisk(base, RiskMedium)
		} else if action.CostUSD > 0.10 {
			base = maxRisk(base, RiskLow)
		}
	}

	// Escalate for actions with security metadata.
	if action.Metadata != nil {
		if _, ok := action.Metadata["security_scope"]; ok {
			base = maxRisk(base, RiskMedium)
		}
		if v, ok := action.Metadata["destructive"]; ok && v == "true" {
			base = maxRisk(base, RiskHigh)
		}
	}

	return base
}

// RequestPermission checks whether an action is permitted under the current
// auto-mode policy. Returns (permitted, reason). If permitted is false, the
// action requires human-in-the-loop approval.
func (am *AutoMode) RequestPermission(action AutoAction) (bool, string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if !am.config.Enabled {
		return false, "auto-mode disabled"
	}

	// Check autonomy level gates.
	if !am.autonomyAllows(action.Category) {
		return false, fmt.Sprintf("autonomy level %s does not permit %s actions",
			am.config.AutonomyLevel, action.Category)
	}

	risk := am.ScoreRisk(action)

	// Look up the policy for this category.
	policy, ok := am.config.Policies[action.Category]
	if !ok {
		// No policy means deny.
		return false, fmt.Sprintf("no policy for category %s", action.Category)
	}

	// Check risk threshold.
	if risk > policy.MaxRisk {
		return false, fmt.Sprintf("risk %s exceeds max %s for %s",
			risk, policy.MaxRisk, action.Category)
	}

	// Check per-action cost limit.
	if action.CostUSD > 0 && action.CostUSD > policy.CostLimitUSD && policy.CostLimitUSD > 0 {
		return false, fmt.Sprintf("cost $%.2f exceeds limit $%.2f for %s",
			action.CostUSD, policy.CostLimitUSD, action.Category)
	}

	// Check global cost limit (sum of cost-bearing actions in last hour).
	if action.CostUSD > 0 && am.config.GlobalCostLimitUSD > 0 {
		hourlySpend := am.hourlySpend()
		if hourlySpend+action.CostUSD > am.config.GlobalCostLimitUSD {
			return false, fmt.Sprintf("hourly spend $%.2f + $%.2f exceeds global limit $%.2f",
				hourlySpend, action.CostUSD, am.config.GlobalCostLimitUSD)
		}
	}

	// Check rate limit.
	if policy.RateLimitPerHour > 0 {
		count := am.hourlyActionCount(action.Category)
		if count >= policy.RateLimitPerHour {
			return false, fmt.Sprintf("rate limit %d/hr reached for %s",
				policy.RateLimitPerHour, action.Category)
		}
	}

	return true, fmt.Sprintf("permitted: risk=%s, category=%s", risk, action.Category)
}

// RecordAction logs an action and its outcome to the history.
func (am *AutoMode) RecordAction(action AutoAction, permitted bool, outcome, reason string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	record := ActionRecord{
		Action:    action,
		Risk:      am.ScoreRisk(action),
		Permitted: permitted,
		Timestamp: time.Now(),
		Outcome:   outcome,
		Reason:    reason,
	}

	am.history = append(am.history, record)

	// Trim history if over limit.
	if len(am.history) > am.config.HistoryLimit {
		excess := len(am.history) - am.config.HistoryLimit
		am.history = am.history[excess:]
	}
}

// History returns a copy of the action history.
func (am *AutoMode) History() []ActionRecord {
	am.mu.Lock()
	defer am.mu.Unlock()

	out := make([]ActionRecord, len(am.history))
	copy(out, am.history)
	return out
}

// RecentActions returns actions from the last duration.
func (am *AutoMode) RecentActions(d time.Duration) []ActionRecord {
	am.mu.Lock()
	defer am.mu.Unlock()

	cutoff := time.Now().Add(-d)
	var out []ActionRecord
	// Walk backward since history is chronological.
	for i := len(am.history) - 1; i >= 0; i-- {
		if am.history[i].Timestamp.Before(cutoff) {
			break
		}
		out = append(out, am.history[i])
	}
	// Reverse to restore chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Stats returns summary statistics for the action history.
func (am *AutoMode) Stats() AutoModeStats {
	am.mu.Lock()
	defer am.mu.Unlock()

	stats := AutoModeStats{}
	for _, r := range am.history {
		stats.TotalActions++
		if r.Permitted {
			stats.Permitted++
		} else {
			stats.Denied++
		}
		if r.Outcome == "success" {
			stats.Succeeded++
		} else if r.Outcome == "failed" {
			stats.Failed++
		}
		stats.TotalCostUSD += r.Action.CostUSD
	}
	if stats.TotalActions > 0 {
		stats.ApprovalRate = float64(stats.Permitted) / float64(stats.TotalActions)
	}
	return stats
}

// AutoModeStats summarizes auto-mode action history.
type AutoModeStats struct {
	TotalActions int     `json:"total_actions"`
	Permitted    int     `json:"permitted"`
	Denied       int     `json:"denied"`
	Succeeded    int     `json:"succeeded"`
	Failed       int     `json:"failed"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	ApprovalRate float64 `json:"approval_rate"` // 0.0-1.0
}

// Config returns the current configuration.
func (am *AutoMode) Config() AutoModeConfig {
	am.mu.Lock()
	defer am.mu.Unlock()
	return am.config
}

// SetAutonomyLevel updates the autonomy level.
func (am *AutoMode) SetAutonomyLevel(level AutonomyLevel) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.config.AutonomyLevel = level
}

// SetEnabled enables or disables auto-mode.
func (am *AutoMode) SetEnabled(enabled bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.config.Enabled = enabled
}

// NeedsHumanApproval is a convenience that returns true when the action
// cannot proceed autonomously.
func (am *AutoMode) NeedsHumanApproval(action AutoAction) bool {
	permitted, _ := am.RequestPermission(action)
	return !permitted
}

// hourlySpend sums cost of permitted actions in the last hour.
// Must be called with mu held.
func (am *AutoMode) hourlySpend() float64 {
	cutoff := time.Now().Add(-time.Hour)
	var total float64
	for i := len(am.history) - 1; i >= 0; i-- {
		r := am.history[i]
		if r.Timestamp.Before(cutoff) {
			break
		}
		if r.Permitted && r.Action.CostUSD > 0 {
			total += r.Action.CostUSD
		}
	}
	return total
}

// hourlyActionCount counts permitted actions of a category in the last hour.
// Must be called with mu held.
func (am *AutoMode) hourlyActionCount(cat ActionCategory) int {
	cutoff := time.Now().Add(-time.Hour)
	count := 0
	for i := len(am.history) - 1; i >= 0; i-- {
		r := am.history[i]
		if r.Timestamp.Before(cutoff) {
			break
		}
		if r.Permitted && r.Action.Category == cat {
			count++
		}
	}
	return count
}

// autonomyAllows checks whether the current autonomy level permits the category.
// Must be called with mu held.
func (am *AutoMode) autonomyAllows(cat ActionCategory) bool {
	switch am.config.AutonomyLevel {
	case LevelObserve:
		// Only read-only actions at observe level.
		return cat == CategoryReadOnly
	case LevelAutoRecover:
		// Read-only + reversible actions (restart, pause, failover).
		return cat == CategoryReadOnly || cat == CategoryReversible
	case LevelAutoOptimize:
		// All except security and irreversible.
		return cat != CategoryIrreversible && cat != CategorySecurity
	case LevelFullAutonomy:
		// Everything is allowed (policy still applies).
		return true
	default:
		return false
	}
}

// categoryBaseRisk returns the inherent risk level for an action category.
func categoryBaseRisk(cat ActionCategory) RiskLevel {
	switch cat {
	case CategoryReadOnly:
		return RiskNone
	case CategoryReversible:
		return RiskLow
	case CategoryCostBearing:
		return RiskMedium
	case CategoryIrreversible:
		return RiskHigh
	case CategorySecurity:
		return RiskHigh
	default:
		return RiskMedium
	}
}

// maxRisk returns the higher of two risk levels.
func maxRisk(a, b RiskLevel) RiskLevel {
	if a > b {
		return a
	}
	return b
}

// ParseActionCategory converts a string to an ActionCategory.
// Returns CategoryReadOnly if unrecognized.
func ParseActionCategory(s string) ActionCategory {
	switch strings.ToLower(s) {
	case "read_only":
		return CategoryReadOnly
	case "reversible":
		return CategoryReversible
	case "irreversible":
		return CategoryIrreversible
	case "cost_bearing":
		return CategoryCostBearing
	case "security":
		return CategorySecurity
	default:
		return CategoryReadOnly
	}
}
