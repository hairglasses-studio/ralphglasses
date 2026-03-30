package metrics

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Severity classifies how urgent an alert is.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// MetricType identifies what kind of metric a rule evaluates.
type MetricType string

const (
	MetricCost      MetricType = "cost"
	MetricErrorRate MetricType = "error_rate"
	MetricLatency   MetricType = "latency"
)

// Condition defines how a metric value is compared to a threshold.
type Condition string

const (
	CondGT  Condition = "gt"  // greater than
	CondGTE Condition = "gte" // greater than or equal
	CondLT  Condition = "lt"  // less than
	CondLTE Condition = "lte" // less than or equal
)

// AlertRule defines a threshold-based alerting rule.
type AlertRule struct {
	Name      string     `json:"name"`
	Metric    MetricType `json:"metric"`
	Condition Condition  `json:"condition"`
	Threshold float64    `json:"threshold"`
	Severity  Severity   `json:"severity"`
	Cooldown  time.Duration `json:"cooldown"`

	// Targets lists notification target names registered with the AlertManager.
	Targets []string `json:"targets,omitempty"`
}

// Alert represents a fired alert instance.
type Alert struct {
	Rule      AlertRule `json:"rule"`
	Value     float64   `json:"value"`
	FiredAt   time.Time `json:"fired_at"`
	SessionID string    `json:"session_id,omitempty"`
	RepoPath  string    `json:"repo_path,omitempty"`
	Message   string    `json:"message"`
}

// NotificationTarget receives alerts. Implementations must be safe for
// concurrent use.
type NotificationTarget interface {
	Name() string
	Notify(ctx context.Context, alert Alert) error
}

// ChannelTarget is a NotificationTarget that sends alerts to a Go channel.
type ChannelTarget struct {
	name string
	Ch   chan Alert
}

// NewChannelTarget creates a ChannelTarget with the given buffer size.
func NewChannelTarget(name string, bufSize int) *ChannelTarget {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &ChannelTarget{
		name: name,
		Ch:   make(chan Alert, bufSize),
	}
}

// Name returns the target identifier.
func (t *ChannelTarget) Name() string { return t.name }

// Notify sends the alert to the channel. Non-blocking: drops if full.
func (t *ChannelTarget) Notify(_ context.Context, a Alert) error {
	select {
	case t.Ch <- a:
		return nil
	default:
		return fmt.Errorf("channel target %q full, alert dropped", t.name)
	}
}

// AlertManager evaluates rules against metric values, enforces cooldowns,
// and dispatches alerts to registered notification targets.
type AlertManager struct {
	mu       sync.RWMutex
	rules    []AlertRule
	targets  map[string]NotificationTarget
	cooldowns map[string]time.Time // rule name -> earliest next fire time
	history   []Alert
	maxHistory int
}

// NewAlertManager creates an AlertManager with sensible defaults.
func NewAlertManager() *AlertManager {
	return &AlertManager{
		targets:    make(map[string]NotificationTarget),
		cooldowns:  make(map[string]time.Time),
		maxHistory: 500,
	}
}

// AddRule registers an alerting rule. Rules with duplicate names overwrite
// the previous definition.
func (am *AlertManager) AddRule(r AlertRule) {
	am.mu.Lock()
	defer am.mu.Unlock()
	for i, existing := range am.rules {
		if existing.Name == r.Name {
			am.rules[i] = r
			return
		}
	}
	am.rules = append(am.rules, r)
}

// RemoveRule removes a rule by name. Returns true if the rule existed.
func (am *AlertManager) RemoveRule(name string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	for i, r := range am.rules {
		if r.Name == name {
			am.rules = append(am.rules[:i], am.rules[i+1:]...)
			delete(am.cooldowns, name)
			return true
		}
	}
	return false
}

// Rules returns a copy of all registered rules.
func (am *AlertManager) Rules() []AlertRule {
	am.mu.RLock()
	defer am.mu.RUnlock()
	out := make([]AlertRule, len(am.rules))
	copy(out, am.rules)
	return out
}

// RegisterTarget adds a notification target.
func (am *AlertManager) RegisterTarget(t NotificationTarget) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.targets[t.Name()] = t
}

// UnregisterTarget removes a notification target by name.
func (am *AlertManager) UnregisterTarget(name string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.targets, name)
}

// MetricSample is a data point to evaluate against rules.
type MetricSample struct {
	Metric    MetricType
	Value     float64
	SessionID string
	RepoPath  string
	Timestamp time.Time
}

// Evaluate checks all rules against the sample and fires alerts for matches.
// Returns the list of alerts that were actually dispatched (i.e. not suppressed
// by cooldown).
func (am *AlertManager) Evaluate(ctx context.Context, sample MetricSample) []Alert {
	if sample.Timestamp.IsZero() {
		sample.Timestamp = time.Now()
	}

	am.mu.Lock()
	// Snapshot matching rules under lock.
	var matched []AlertRule
	for _, r := range am.rules {
		if r.Metric != sample.Metric {
			continue
		}
		if !conditionMet(r.Condition, sample.Value, r.Threshold) {
			continue
		}
		// Cooldown check
		if earliest, ok := am.cooldowns[r.Name]; ok && sample.Timestamp.Before(earliest) {
			continue
		}
		// Set cooldown
		if r.Cooldown > 0 {
			am.cooldowns[r.Name] = sample.Timestamp.Add(r.Cooldown)
		}
		matched = append(matched, r)
	}
	am.mu.Unlock()

	var fired []Alert
	for _, r := range matched {
		a := Alert{
			Rule:      r,
			Value:     sample.Value,
			FiredAt:   sample.Timestamp,
			SessionID: sample.SessionID,
			RepoPath:  sample.RepoPath,
			Message:   fmt.Sprintf("[%s] %s: %s %.4f threshold %.4f", r.Severity, r.Name, r.Metric, sample.Value, r.Threshold),
		}
		am.dispatch(ctx, a)
		fired = append(fired, a)
	}

	if len(fired) > 0 {
		am.mu.Lock()
		am.history = append(am.history, fired...)
		if len(am.history) > am.maxHistory {
			am.history = am.history[len(am.history)-am.maxHistory:]
		}
		am.mu.Unlock()
	}

	return fired
}

// History returns a copy of all fired alerts, newest last.
func (am *AlertManager) History() []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()
	out := make([]Alert, len(am.history))
	copy(out, am.history)
	return out
}

// ResetCooldown clears the cooldown for a specific rule, allowing it to
// fire immediately on the next matching sample.
func (am *AlertManager) ResetCooldown(ruleName string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.cooldowns, ruleName)
}

// ResetAllCooldowns clears cooldowns for every rule.
func (am *AlertManager) ResetAllCooldowns() {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.cooldowns = make(map[string]time.Time)
}

// dispatch sends an alert to the rule's configured targets. If no targets
// are specified on the rule, it broadcasts to all registered targets.
func (am *AlertManager) dispatch(ctx context.Context, a Alert) {
	am.mu.RLock()
	targets := make(map[string]NotificationTarget, len(am.targets))
	for k, v := range am.targets {
		targets[k] = v
	}
	am.mu.RUnlock()

	if len(a.Rule.Targets) == 0 {
		// Broadcast to all targets.
		for _, t := range targets {
			if err := t.Notify(ctx, a); err != nil {
				slog.Warn("alert dispatch failed", "target", t.Name(), "rule", a.Rule.Name, "err", err)
			}
		}
		return
	}

	for _, name := range a.Rule.Targets {
		t, ok := targets[name]
		if !ok {
			slog.Warn("alert target not found", "target", name, "rule", a.Rule.Name)
			continue
		}
		if err := t.Notify(ctx, a); err != nil {
			slog.Warn("alert dispatch failed", "target", t.Name(), "rule", a.Rule.Name, "err", err)
		}
	}
}

// conditionMet evaluates whether value satisfies the condition relative to threshold.
func conditionMet(cond Condition, value, threshold float64) bool {
	switch cond {
	case CondGT:
		return value > threshold
	case CondGTE:
		return value >= threshold
	case CondLT:
		return value < threshold
	case CondLTE:
		return value <= threshold
	default:
		return false
	}
}
