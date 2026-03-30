package session

import (
	"sync"
	"testing"
)

func TestClassifyTaskType_BugFix(t *testing.T) {
	cases := []string{
		"fix the login bug",
		"Bug in user registration",
		"crash when opening settings",
		"error handling is broken",
		"patch the auth issue",
		"hotfix for payment flow",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeBugFix {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", desc, got, TaskTypeBugFix)
		}
	}
}

func TestClassifyTaskType_Test(t *testing.T) {
	cases := []string{
		"add unit tests for router",
		"improve test coverage",
		"write spec for parser",
		"add assertions for edge cases",
		"benchmark the hash function",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeTest {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", desc, got, TaskTypeTest)
		}
	}
}

func TestClassifyTaskType_Refactor(t *testing.T) {
	cases := []string{
		"refactor the session manager",
		"cleanup dead code",
		"restructure the config package",
		"extract helper functions",
		"simplify the router logic",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeRefactor {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", desc, got, TaskTypeRefactor)
		}
	}
}

func TestClassifyTaskType_Docs(t *testing.T) {
	cases := []string{
		"update the README",
		"add documentation for API",
		"write doc comments",
		"update changelog for v2",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeDocs {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", desc, got, TaskTypeDocs)
		}
	}
}

func TestClassifyTaskType_Research(t *testing.T) {
	cases := []string{
		"research new auth libraries",
		"investigate memory leak",
		"explore alternative approaches",
		"spike on gRPC integration",
		"prototype the new UI",
		"evaluate caching strategies",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeResearch {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", desc, got, TaskTypeResearch)
		}
	}
}

func TestClassifyTaskType_Feature(t *testing.T) {
	cases := []string{
		"add support for webhooks",
		"implement OAuth2 flow",
		"create user dashboard",
		"build the notification system",
		"enable dark mode",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeFeature {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", desc, got, TaskTypeFeature)
		}
	}
}

func TestClassifyTaskType_Default(t *testing.T) {
	// Descriptions with no matching keywords should default to Feature.
	cases := []string{
		"something completely unrelated",
		"adjust the margins",
		"update version number",
	}
	for _, desc := range cases {
		got := ClassifyTaskType(desc)
		if got != TaskTypeFeature {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q (default)", desc, got, TaskTypeFeature)
		}
	}
}

func TestDynamicRouter_Route(t *testing.T) {
	config := RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeBugFix, PreferredProvider: "claude", MaxCost: 0.50, Priority: 1},
			{TaskType: TaskTypeBugFix, PreferredProvider: "gemini", MaxCost: 0.10, Priority: 2},
			{TaskType: TaskTypeFeature, PreferredProvider: "claude", MaxCost: 2.00, Priority: 1},
			{TaskType: TaskTypeDocs, PreferredProvider: "gemini", MaxCost: 0.05, Priority: 1},
		},
	}
	router := NewDynamicRouter(config)

	t.Run("matches highest priority rule", func(t *testing.T) {
		task := TypedTaskSpec{TaskSpec: TaskSpec{Type: TaskTypeBugFix}}
		rule := router.Route(task)
		if rule == nil {
			t.Fatal("expected a matching rule, got nil")
		}
		if rule.PreferredProvider != "claude" {
			t.Errorf("provider = %q, want %q", rule.PreferredProvider, "claude")
		}
		if rule.Priority != 1 {
			t.Errorf("priority = %d, want 1", rule.Priority)
		}
	})

	t.Run("returns nil for unmatched type", func(t *testing.T) {
		task := TypedTaskSpec{TaskSpec: TaskSpec{Type: TaskTypeResearch}}
		rule := router.Route(task)
		if rule != nil {
			t.Errorf("expected nil for unmatched type, got %+v", rule)
		}
	})

	t.Run("feature routing", func(t *testing.T) {
		task := TypedTaskSpec{TaskSpec: TaskSpec{Type: TaskTypeFeature}}
		rule := router.Route(task)
		if rule == nil {
			t.Fatal("expected a matching rule")
		}
		if rule.MaxCost != 2.00 {
			t.Errorf("max_cost = %f, want 2.00", rule.MaxCost)
		}
	})
}

func TestDynamicRouter_UpdateConfig(t *testing.T) {
	router := NewDynamicRouter(RouteConfig{})
	task := TypedTaskSpec{TaskSpec: TaskSpec{Type: TaskTypeTest}}

	// Initially no rules.
	if rule := router.Route(task); rule != nil {
		t.Fatal("expected nil with empty config")
	}

	// Update config and verify new rule matches.
	router.UpdateConfig(RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeTest, PreferredProvider: "codex", MaxCost: 0.20, Priority: 1},
		},
	})
	rule := router.Route(task)
	if rule == nil {
		t.Fatal("expected a matching rule after update")
	}
	if rule.PreferredProvider != "codex" {
		t.Errorf("provider = %q, want %q", rule.PreferredProvider, "codex")
	}
}

func TestDynamicRouter_ConcurrentAccess(t *testing.T) {
	config := RouteConfig{
		Rules: []TaskRoutingRule{
			{TaskType: TaskTypeBugFix, PreferredProvider: "claude", MaxCost: 1.0, Priority: 1},
		},
	}
	router := NewDynamicRouter(config)
	task := TypedTaskSpec{TaskSpec: TaskSpec{Type: TaskTypeBugFix}}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = router.Route(task)
		}()
		go func() {
			defer wg.Done()
			router.UpdateConfig(config)
		}()
	}
	wg.Wait()
}
