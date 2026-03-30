package session

import (
	"strings"
	"sync"
)

// TaskType constants for the classifier. These alias the canonical TaskType
// values from task_spec.go under the names requested by the routing subsystem.
const (
	TaskTypeBugFix   = TaskBugfix
	TaskTypeFeature  = TaskFeature
	TaskTypeRefactor = TaskRefactor
	TaskTypeTest     = TaskTest
	TaskTypeDocs     = TaskDocs
	TaskTypeResearch = TaskResearch
)

// classifierKeywords maps keyword patterns to task types, checked in order.
// More specific categories come first to avoid ambiguous matches.
var classifierKeywords = []struct {
	taskType TaskType
	keywords []string
}{
	{TaskTypeBugFix, []string{"fix", "bug", "error", "broken", "crash", "issue", "patch", "hotfix"}},
	{TaskTypeTest, []string{"test", "spec", "coverage", "assert", "benchmark"}},
	{TaskTypeRefactor, []string{"refactor", "cleanup", "restructure", "reorganize", "extract", "simplify"}},
	{TaskTypeDocs, []string{"doc", "readme", "documentation", "comment", "changelog"}},
	{TaskTypeResearch, []string{"research", "investigate", "explore", "spike", "prototype", "evaluate"}},
	// Feature is the catch-all so it comes last among keyword-matched types.
	{TaskTypeFeature, []string{"add", "implement", "create", "new", "feature", "build", "enable"}},
}

// ClassifyTaskType uses keyword matching to determine the TaskType for a
// free-text task description. Returns TaskTypeFeature when no keywords match.
func ClassifyTaskType(description string) TaskType {
	lower := strings.ToLower(description)
	for _, entry := range classifierKeywords {
		for _, kw := range entry.keywords {
			if strings.Contains(lower, kw) {
				return entry.taskType
			}
		}
	}
	return TaskTypeFeature
}

// RouteConfig holds an ordered set of routing rules for the DynamicRouter.
type RouteConfig struct {
	Rules []TaskRoutingRule `json:"rules"`
}

// TaskRoutingRule maps a task type to provider preferences and cost limits.
type TaskRoutingRule struct {
	TaskType          TaskType `json:"task_type"`
	PreferredProvider string   `json:"preferred_provider"`
	MaxCost           float64  `json:"max_cost"`
	Priority          int      `json:"priority"` // lower = higher priority
}

// DynamicRouter selects routing rules based on task type. It is safe for
// concurrent use.
type DynamicRouter struct {
	mu     sync.RWMutex
	config RouteConfig
}

// NewDynamicRouter creates a DynamicRouter with the given configuration.
func NewDynamicRouter(config RouteConfig) *DynamicRouter {
	return &DynamicRouter{config: config}
}

// Route returns the best matching TaskRoutingRule for the given task spec.
// "Best" means the rule whose TaskType matches and has the lowest Priority
// value (highest priority). Returns nil if no rule matches.
func (dr *DynamicRouter) Route(task TypedTaskSpec) *TaskRoutingRule {
	dr.mu.RLock()
	defer dr.mu.RUnlock()

	var best *TaskRoutingRule
	for i := range dr.config.Rules {
		r := &dr.config.Rules[i]
		if r.TaskType == task.Type {
			if best == nil || r.Priority < best.Priority {
				best = r
			}
		}
	}
	return best
}

// UpdateConfig replaces the routing configuration atomically.
func (dr *DynamicRouter) UpdateConfig(config RouteConfig) {
	dr.mu.Lock()
	defer dr.mu.Unlock()
	dr.config = config
}
