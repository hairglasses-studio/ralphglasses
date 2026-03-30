package metrics

import (
	"context"
	"testing"
	"time"
)

func TestConditionMet(t *testing.T) {
	tests := []struct {
		cond   Condition
		value  float64
		thresh float64
		want   bool
	}{
		{CondGT, 10.0, 5.0, true},
		{CondGT, 5.0, 5.0, false},
		{CondGT, 3.0, 5.0, false},
		{CondGTE, 5.0, 5.0, true},
		{CondGTE, 4.9, 5.0, false},
		{CondLT, 3.0, 5.0, true},
		{CondLT, 5.0, 5.0, false},
		{CondLTE, 5.0, 5.0, true},
		{CondLTE, 5.1, 5.0, false},
		{Condition("invalid"), 5.0, 5.0, false},
	}
	for _, tt := range tests {
		got := conditionMet(tt.cond, tt.value, tt.thresh)
		if got != tt.want {
			t.Errorf("conditionMet(%s, %.1f, %.1f) = %v, want %v", tt.cond, tt.value, tt.thresh, got, tt.want)
		}
	}
}

func TestAlertManagerEvaluate(t *testing.T) {
	am := NewAlertManager()
	target := NewChannelTarget("test", 10)
	am.RegisterTarget(target)

	am.AddRule(AlertRule{
		Name:      "high-cost",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 1.0,
		Severity:  SeverityWarning,
		Targets:   []string{"test"},
	})

	fired := am.Evaluate(context.Background(), MetricSample{
		Metric: MetricCost,
		Value:  2.5,
	})

	if len(fired) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(fired))
	}
	if fired[0].Rule.Name != "high-cost" {
		t.Errorf("rule name = %q, want high-cost", fired[0].Rule.Name)
	}
	if fired[0].Value != 2.5 {
		t.Errorf("value = %f, want 2.5", fired[0].Value)
	}

	// Channel target should have received the alert.
	select {
	case a := <-target.Ch:
		if a.Rule.Name != "high-cost" {
			t.Errorf("channel alert rule = %q, want high-cost", a.Rule.Name)
		}
	default:
		t.Error("expected alert on channel target")
	}
}

func TestAlertManagerNoMatch(t *testing.T) {
	am := NewAlertManager()
	am.AddRule(AlertRule{
		Name:      "high-cost",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 10.0,
		Severity:  SeverityWarning,
	})

	fired := am.Evaluate(context.Background(), MetricSample{
		Metric: MetricCost,
		Value:  5.0,
	})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(fired))
	}
}

func TestAlertManagerWrongMetric(t *testing.T) {
	am := NewAlertManager()
	am.AddRule(AlertRule{
		Name:      "high-cost",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 1.0,
		Severity:  SeverityWarning,
	})

	fired := am.Evaluate(context.Background(), MetricSample{
		Metric: MetricErrorRate,
		Value:  99.0,
	})
	if len(fired) != 0 {
		t.Errorf("expected 0 alerts for wrong metric, got %d", len(fired))
	}
}

func TestCooldownEnforcement(t *testing.T) {
	am := NewAlertManager()
	target := NewChannelTarget("test", 10)
	am.RegisterTarget(target)

	am.AddRule(AlertRule{
		Name:      "latency-spike",
		Metric:    MetricLatency,
		Condition: CondGT,
		Threshold: 500.0,
		Severity:  SeverityCritical,
		Cooldown:  10 * time.Minute,
		Targets:   []string{"test"},
	})

	now := time.Now()

	// First evaluation should fire.
	fired := am.Evaluate(context.Background(), MetricSample{
		Metric:    MetricLatency,
		Value:     1000.0,
		Timestamp: now,
	})
	if len(fired) != 1 {
		t.Fatalf("first eval: expected 1, got %d", len(fired))
	}

	// Second evaluation within cooldown should be suppressed.
	fired = am.Evaluate(context.Background(), MetricSample{
		Metric:    MetricLatency,
		Value:     1000.0,
		Timestamp: now.Add(5 * time.Minute),
	})
	if len(fired) != 0 {
		t.Errorf("during cooldown: expected 0, got %d", len(fired))
	}

	// Third evaluation after cooldown should fire again.
	fired = am.Evaluate(context.Background(), MetricSample{
		Metric:    MetricLatency,
		Value:     1000.0,
		Timestamp: now.Add(11 * time.Minute),
	})
	if len(fired) != 1 {
		t.Errorf("after cooldown: expected 1, got %d", len(fired))
	}
}

func TestResetCooldown(t *testing.T) {
	am := NewAlertManager()

	am.AddRule(AlertRule{
		Name:      "cost-alert",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 1.0,
		Severity:  SeverityWarning,
		Cooldown:  1 * time.Hour,
	})

	now := time.Now()

	// Fire once.
	am.Evaluate(context.Background(), MetricSample{
		Metric:    MetricCost,
		Value:     5.0,
		Timestamp: now,
	})

	// Should be suppressed.
	fired := am.Evaluate(context.Background(), MetricSample{
		Metric:    MetricCost,
		Value:     5.0,
		Timestamp: now.Add(1 * time.Minute),
	})
	if len(fired) != 0 {
		t.Fatalf("expected cooldown suppression, got %d", len(fired))
	}

	// Reset cooldown.
	am.ResetCooldown("cost-alert")

	// Should fire again.
	fired = am.Evaluate(context.Background(), MetricSample{
		Metric:    MetricCost,
		Value:     5.0,
		Timestamp: now.Add(2 * time.Minute),
	})
	if len(fired) != 1 {
		t.Errorf("after reset: expected 1, got %d", len(fired))
	}
}

func TestBroadcastToAllTargets(t *testing.T) {
	am := NewAlertManager()
	t1 := NewChannelTarget("t1", 10)
	t2 := NewChannelTarget("t2", 10)
	am.RegisterTarget(t1)
	am.RegisterTarget(t2)

	// Rule with no specific targets -> broadcast.
	am.AddRule(AlertRule{
		Name:      "broadcast-rule",
		Metric:    MetricErrorRate,
		Condition: CondGTE,
		Threshold: 0.5,
		Severity:  SeverityCritical,
	})

	am.Evaluate(context.Background(), MetricSample{
		Metric: MetricErrorRate,
		Value:  0.75,
	})

	for _, target := range []*ChannelTarget{t1, t2} {
		select {
		case a := <-target.Ch:
			if a.Rule.Name != "broadcast-rule" {
				t.Errorf("target %s: rule = %q, want broadcast-rule", target.Name(), a.Rule.Name)
			}
		default:
			t.Errorf("target %s: expected alert", target.Name())
		}
	}
}

func TestRemoveRule(t *testing.T) {
	am := NewAlertManager()
	am.AddRule(AlertRule{
		Name:      "temp",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 0.0,
		Severity:  SeverityInfo,
	})

	if !am.RemoveRule("temp") {
		t.Error("RemoveRule should return true for existing rule")
	}
	if am.RemoveRule("temp") {
		t.Error("RemoveRule should return false for already-removed rule")
	}
	if len(am.Rules()) != 0 {
		t.Errorf("expected 0 rules after removal, got %d", len(am.Rules()))
	}
}

func TestOverwriteRule(t *testing.T) {
	am := NewAlertManager()
	am.AddRule(AlertRule{
		Name:      "r1",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 1.0,
		Severity:  SeverityInfo,
	})
	am.AddRule(AlertRule{
		Name:      "r1",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 99.0,
		Severity:  SeverityCritical,
	})

	rules := am.Rules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule after overwrite, got %d", len(rules))
	}
	if rules[0].Threshold != 99.0 {
		t.Errorf("threshold = %f, want 99.0", rules[0].Threshold)
	}
	if rules[0].Severity != SeverityCritical {
		t.Errorf("severity = %s, want critical", rules[0].Severity)
	}
}

func TestHistory(t *testing.T) {
	am := NewAlertManager()
	am.AddRule(AlertRule{
		Name:      "hist",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 0.0,
		Severity:  SeverityInfo,
	})

	am.Evaluate(context.Background(), MetricSample{Metric: MetricCost, Value: 1.0})
	am.Evaluate(context.Background(), MetricSample{Metric: MetricCost, Value: 2.0})

	hist := am.History()
	if len(hist) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(hist))
	}
	if hist[0].Value != 1.0 || hist[1].Value != 2.0 {
		t.Errorf("history values = [%f, %f], want [1.0, 2.0]", hist[0].Value, hist[1].Value)
	}
}

func TestUnregisterTarget(t *testing.T) {
	am := NewAlertManager()
	target := NewChannelTarget("removeme", 10)
	am.RegisterTarget(target)
	am.UnregisterTarget("removeme")

	am.AddRule(AlertRule{
		Name:      "test",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 0.0,
		Severity:  SeverityInfo,
		Targets:   []string{"removeme"},
	})

	// Should not panic; target simply not found.
	fired := am.Evaluate(context.Background(), MetricSample{Metric: MetricCost, Value: 1.0})
	if len(fired) != 1 {
		t.Errorf("expected 1 fired alert even with missing target, got %d", len(fired))
	}
}

func TestChannelTargetFull(t *testing.T) {
	target := NewChannelTarget("tiny", 1)

	// Fill the channel.
	_ = target.Notify(context.Background(), Alert{})

	// Next should return an error (non-blocking).
	err := target.Notify(context.Background(), Alert{})
	if err == nil {
		t.Error("expected error when channel is full")
	}
}

func TestMultipleRulesOnSameMetric(t *testing.T) {
	am := NewAlertManager()
	target := NewChannelTarget("multi", 10)
	am.RegisterTarget(target)

	am.AddRule(AlertRule{
		Name:      "warn",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 1.0,
		Severity:  SeverityWarning,
		Targets:   []string{"multi"},
	})
	am.AddRule(AlertRule{
		Name:      "crit",
		Metric:    MetricCost,
		Condition: CondGT,
		Threshold: 5.0,
		Severity:  SeverityCritical,
		Targets:   []string{"multi"},
	})

	fired := am.Evaluate(context.Background(), MetricSample{
		Metric: MetricCost,
		Value:  10.0,
	})
	if len(fired) != 2 {
		t.Errorf("expected 2 alerts for value exceeding both thresholds, got %d", len(fired))
	}

	// Only the warning should fire at value 3.0.
	am.ResetAllCooldowns()
	fired = am.Evaluate(context.Background(), MetricSample{
		Metric: MetricCost,
		Value:  3.0,
	})
	if len(fired) != 1 {
		t.Fatalf("expected 1 alert at 3.0, got %d", len(fired))
	}
	if fired[0].Rule.Name != "warn" {
		t.Errorf("expected warn rule, got %s", fired[0].Rule.Name)
	}
}
