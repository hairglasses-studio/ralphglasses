package session

import (
	"fmt"
	"testing"
	"time"
)

func TestRiskLevel_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		level RiskLevel
		want  string
	}{
		{RiskNone, "none"},
		{RiskLow, "low"},
		{RiskMedium, "medium"},
		{RiskHigh, "high"},
		{RiskLevel(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDefaultAutoModeConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	if cfg.Enabled {
		t.Fatal("default should be disabled")
	}
	if cfg.AutonomyLevel != LevelObserve {
		t.Fatalf("expected LevelObserve, got %v", cfg.AutonomyLevel)
	}
	if len(cfg.Policies) != 5 {
		t.Fatalf("expected 5 policies, got %d", len(cfg.Policies))
	}
}

func TestNewAutoMode(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(AutoModeConfig{})
	if am == nil {
		t.Fatal("expected non-nil AutoMode")
	}
	// Default history limit should be set.
	if am.config.HistoryLimit != 1000 {
		t.Fatalf("expected HistoryLimit=1000, got %d", am.config.HistoryLimit)
	}
	// Nil policies should get defaults.
	if am.config.Policies == nil {
		t.Fatal("expected non-nil Policies")
	}
}

func TestAutoMode_ScoreRisk(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())

	tests := []struct {
		name   string
		action AutoAction
		want   RiskLevel
	}{
		{
			"read_only is none",
			AutoAction{Category: CategoryReadOnly},
			RiskNone,
		},
		{
			"reversible is low",
			AutoAction{Category: CategoryReversible},
			RiskLow,
		},
		{
			"cost_bearing is medium",
			AutoAction{Category: CategoryCostBearing},
			RiskMedium,
		},
		{
			"irreversible is high",
			AutoAction{Category: CategoryIrreversible},
			RiskHigh,
		},
		{
			"security is high",
			AutoAction{Category: CategorySecurity},
			RiskHigh,
		},
		{
			"high cost escalates",
			AutoAction{Category: CategoryReadOnly, CostUSD: 6.0},
			RiskHigh,
		},
		{
			"medium cost escalates",
			AutoAction{Category: CategoryReadOnly, CostUSD: 2.0},
			RiskMedium,
		},
		{
			"low cost escalates",
			AutoAction{Category: CategoryReadOnly, CostUSD: 0.15},
			RiskLow,
		},
		{
			"security metadata escalates",
			AutoAction{
				Category: CategoryReadOnly,
				Metadata: map[string]string{"security_scope": "admin"},
			},
			RiskMedium,
		},
		{
			"destructive metadata escalates",
			AutoAction{
				Category: CategoryReadOnly,
				Metadata: map[string]string{"destructive": "true"},
			},
			RiskHigh,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := am.ScoreRisk(tt.action)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestAutoMode_RequestPermission_Disabled(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig()) // disabled by default
	ok, reason := am.RequestPermission(AutoAction{Category: CategoryReadOnly})
	if ok {
		t.Fatal("expected denial when disabled")
	}
	if reason != "auto-mode disabled" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestAutoMode_RequestPermission_AutonomyGate(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelObserve
	am := NewAutoMode(cfg)

	// Read-only should be allowed at observe level.
	ok, _ := am.RequestPermission(AutoAction{Category: CategoryReadOnly})
	if !ok {
		t.Fatal("expected read_only permitted at observe level")
	}

	// Reversible should be denied at observe level.
	ok, reason := am.RequestPermission(AutoAction{Category: CategoryReversible})
	if ok {
		t.Fatal("expected reversible denied at observe level")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestAutoMode_RequestPermission_RiskExceeded(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelFullAutonomy
	am := NewAutoMode(cfg)

	// Irreversible with default policy has MaxRisk=RiskNone, so anything > none is denied.
	ok, _ := am.RequestPermission(AutoAction{Category: CategoryIrreversible})
	if ok {
		t.Fatal("expected denial for irreversible (risk too high)")
	}
}

func TestAutoMode_RequestPermission_CostLimit(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelAutoOptimize
	am := NewAutoMode(cfg)

	// Cost-bearing action over the per-action limit of $0.50.
	ok, _ := am.RequestPermission(AutoAction{Category: CategoryCostBearing, CostUSD: 0.60})
	if ok {
		t.Fatal("expected denial for cost exceeding limit")
	}
}

func TestAutoMode_RequestPermission_GlobalCostLimit(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelAutoOptimize
	cfg.GlobalCostLimitUSD = 0.50
	am := NewAutoMode(cfg)

	// Record a prior action that spent $0.40.
	am.RecordAction(AutoAction{Category: CategoryCostBearing, CostUSD: 0.40}, true, "success", "ok")

	// Next action with $0.15 should exceed the $0.50 global limit.
	ok, _ := am.RequestPermission(AutoAction{Category: CategoryCostBearing, CostUSD: 0.15})
	if ok {
		t.Fatal("expected denial for global cost limit exceeded")
	}
}

func TestAutoMode_RequestPermission_RateLimit(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelAutoRecover
	cfg.Policies = map[ActionCategory]PermissionPolicy{
		CategoryReversible: {MaxRisk: RiskLow, RateLimitPerHour: 2},
	}
	am := NewAutoMode(cfg)

	// Record 2 actions to hit the rate limit.
	am.RecordAction(AutoAction{Category: CategoryReversible}, true, "success", "ok")
	am.RecordAction(AutoAction{Category: CategoryReversible}, true, "success", "ok")

	// Third should be denied.
	ok, _ := am.RequestPermission(AutoAction{Category: CategoryReversible})
	if ok {
		t.Fatal("expected denial for rate limit")
	}
}

func TestAutoMode_RequestPermission_NoPolicyDenies(t *testing.T) {
	t.Parallel()
	cfg := AutoModeConfig{
		Enabled:       true,
		AutonomyLevel: LevelFullAutonomy,
		Policies:      map[ActionCategory]PermissionPolicy{}, // empty
		HistoryLimit:  100,
	}
	am := NewAutoMode(cfg)

	ok, reason := am.RequestPermission(AutoAction{Category: CategoryReadOnly})
	if ok {
		t.Fatal("expected denial when no policy exists")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestAutoMode_RecordAction(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())

	am.RecordAction(AutoAction{Name: "test", Category: CategoryReadOnly}, true, "success", "ok")
	hist := am.History()
	if len(hist) != 1 {
		t.Fatalf("expected 1 record, got %d", len(hist))
	}
	if hist[0].Action.Name != "test" {
		t.Fatalf("expected action name 'test', got %q", hist[0].Action.Name)
	}
	if !hist[0].Permitted {
		t.Fatal("expected permitted=true")
	}
}

func TestAutoMode_RecordAction_TrimsHistory(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(AutoModeConfig{HistoryLimit: 5})

	for i := 0; i < 10; i++ {
		am.RecordAction(AutoAction{Name: "test"}, true, "success", "ok")
	}
	if len(am.History()) != 5 {
		t.Fatalf("expected history trimmed to 5, got %d", len(am.History()))
	}
}

func TestAutoMode_RecentActions(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())

	am.RecordAction(AutoAction{Name: "a1"}, true, "success", "ok")
	am.RecordAction(AutoAction{Name: "a2"}, true, "success", "ok")

	recent := am.RecentActions(time.Hour)
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent actions, got %d", len(recent))
	}
	// Should be in chronological order.
	if recent[0].Action.Name != "a1" {
		t.Fatalf("expected first action 'a1', got %q", recent[0].Action.Name)
	}
}

func TestAutoMode_RecentActions_Empty(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())
	recent := am.RecentActions(time.Hour)
	if len(recent) != 0 {
		t.Fatalf("expected 0 recent actions, got %d", len(recent))
	}
}

func TestAutoMode_Stats(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())

	am.RecordAction(AutoAction{CostUSD: 0.10}, true, "success", "ok")
	am.RecordAction(AutoAction{CostUSD: 0.20}, false, "blocked", "denied")
	am.RecordAction(AutoAction{CostUSD: 0.05}, true, "failed", "error")

	stats := am.Stats()
	if stats.TotalActions != 3 {
		t.Fatalf("expected 3 total, got %d", stats.TotalActions)
	}
	if stats.Permitted != 2 {
		t.Fatalf("expected 2 permitted, got %d", stats.Permitted)
	}
	if stats.Denied != 1 {
		t.Fatalf("expected 1 denied, got %d", stats.Denied)
	}
	if stats.Succeeded != 1 {
		t.Fatalf("expected 1 succeeded, got %d", stats.Succeeded)
	}
	if stats.Failed != 1 {
		t.Fatalf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.TotalCostUSD < 0.34 || stats.TotalCostUSD > 0.36 {
		t.Fatalf("expected total cost ~0.35, got %f", stats.TotalCostUSD)
	}
	if stats.ApprovalRate < 0.66 || stats.ApprovalRate > 0.67 {
		t.Fatalf("expected approval rate ~0.667, got %f", stats.ApprovalRate)
	}
}

func TestAutoMode_Stats_Empty(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())
	stats := am.Stats()
	if stats.TotalActions != 0 || stats.ApprovalRate != 0 {
		t.Fatalf("expected zero stats, got %+v", stats)
	}
}

func TestAutoMode_Config(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoModeConfig()
	am := NewAutoMode(cfg)
	got := am.Config()
	if got.GlobalCostLimitUSD != cfg.GlobalCostLimitUSD {
		t.Fatalf("expected GlobalCostLimitUSD=%f, got %f", cfg.GlobalCostLimitUSD, got.GlobalCostLimitUSD)
	}
}

func TestAutoMode_SetAutonomyLevel(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())
	am.SetAutonomyLevel(LevelFullAutonomy)
	if am.Config().AutonomyLevel != LevelFullAutonomy {
		t.Fatalf("expected LevelFullAutonomy, got %v", am.Config().AutonomyLevel)
	}
}

func TestAutoMode_SetEnabled(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig())
	am.SetEnabled(true)
	if !am.Config().Enabled {
		t.Fatal("expected enabled=true")
	}
}

func TestAutoMode_NeedsHumanApproval(t *testing.T) {
	t.Parallel()
	am := NewAutoMode(DefaultAutoModeConfig()) // disabled
	if !am.NeedsHumanApproval(AutoAction{Category: CategoryReadOnly}) {
		t.Fatal("expected needs human approval when disabled")
	}
}

func TestParseActionCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  ActionCategory
	}{
		{"read_only", CategoryReadOnly},
		{"READ_ONLY", CategoryReadOnly},
		{"reversible", CategoryReversible},
		{"irreversible", CategoryIrreversible},
		{"cost_bearing", CategoryCostBearing},
		{"security", CategorySecurity},
		{"unknown", CategoryReadOnly},
		{"", CategoryReadOnly},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ParseActionCategory(tt.input); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestAutoMode_AutonomyLevelGating(t *testing.T) {
	t.Parallel()
	tests := []struct {
		level    AutonomyLevel
		category ActionCategory
		allowed  bool
	}{
		{LevelObserve, CategoryReadOnly, true},
		{LevelObserve, CategoryReversible, false},
		{LevelAutoRecover, CategoryReadOnly, true},
		{LevelAutoRecover, CategoryReversible, true},
		{LevelAutoRecover, CategoryCostBearing, false},
		{LevelAutoOptimize, CategoryReadOnly, true},
		{LevelAutoOptimize, CategoryReversible, true},
		{LevelAutoOptimize, CategoryCostBearing, true},
		{LevelAutoOptimize, CategoryIrreversible, false},
		{LevelAutoOptimize, CategorySecurity, false},
		{LevelFullAutonomy, CategoryReadOnly, true},
		{LevelFullAutonomy, CategoryIrreversible, true},
		{LevelFullAutonomy, CategorySecurity, true},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%d/%s", tt.level, tt.category)
		t.Run(name, func(t *testing.T) {
			cfg := DefaultAutoModeConfig()
			cfg.Enabled = true
			cfg.AutonomyLevel = tt.level
			am := NewAutoMode(cfg)
			// We test autonomyAllows indirectly via RequestPermission.
			// For categories with policy MaxRisk=RiskNone, even if allowed by autonomy,
			// they'll be denied by risk check. We only test the autonomy gating here.
			am.mu.Lock()
			got := am.autonomyAllows(tt.category)
			am.mu.Unlock()
			if got != tt.allowed {
				t.Fatalf("expected autonomyAllows=%v, got %v", tt.allowed, got)
			}
		})
	}
}
