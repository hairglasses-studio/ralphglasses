package session

import (
	"fmt"
	"sync"
)

// RoutingDecision is the output of the SmartRouter: which provider and model
// to use, the cost ceiling, and an explanation of why.
type RoutingDecision struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model,omitempty"`
	MaxCost  float64 `json:"max_cost"`
	Reason   string  `json:"reason"`
}

// ProviderInfo describes a provider's capabilities and cost characteristics.
type ProviderInfo struct {
	Name          string  `json:"name"`
	Model         string  `json:"model,omitempty"`
	CostPerToken  float64 `json:"cost_per_token"`  // USD per 1K tokens (blended)
	MaxConcurrent int     `json:"max_concurrent"`   // 0 = unlimited
	Available     bool    `json:"available"`
}

// ProviderPool tracks the set of providers and their current availability.
type ProviderPool struct {
	mu        sync.RWMutex
	providers []ProviderInfo
}

// NewProviderPool creates a pool from a slice of provider info.
func NewProviderPool(providers []ProviderInfo) *ProviderPool {
	cp := make([]ProviderInfo, len(providers))
	copy(cp, providers)
	return &ProviderPool{providers: cp}
}

// Available returns all providers currently marked as available.
func (pp *ProviderPool) Available() []ProviderInfo {
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	var out []ProviderInfo
	for _, p := range pp.providers {
		if p.Available {
			out = append(out, p)
		}
	}
	return out
}

// SetAvailable toggles the availability flag for a named provider.
func (pp *ProviderPool) SetAvailable(name string, available bool) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	for i := range pp.providers {
		if pp.providers[i].Name == name {
			pp.providers[i].Available = available
		}
	}
}

// SmartRouter combines task classification with cost-aware provider selection.
// It classifies the task, then picks the cheapest available provider whose
// cost fits the budget, preferring the provider recommended by the routing
// rules when one is configured.
type SmartRouter struct {
	mu       sync.RWMutex
	pool     *ProviderPool
	router   *DynamicRouter
}

// NewSmartRouter creates a SmartRouter with the given providers and an
// optional RouteConfig. Pass an empty RouteConfig to rely solely on
// cost-based selection.
func NewSmartRouter(providers []ProviderInfo) *SmartRouter {
	return &SmartRouter{
		pool:   NewProviderPool(providers),
		router: NewDynamicRouter(RouteConfig{}),
	}
}

// SetRouteConfig updates the routing rules used by the SmartRouter.
func (sr *SmartRouter) SetRouteConfig(config RouteConfig) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.router.UpdateConfig(config)
}

// Decide classifies the task description, applies routing rules, then
// selects the best available provider within budget.
func (sr *SmartRouter) Decide(description string, budget float64) RoutingDecision {
	taskType := ClassifyTaskType(description)

	// Build a minimal TypedTaskSpec so the DynamicRouter can match.
	spec := TypedTaskSpec{TaskSpec: TaskSpec{Type: taskType}}

	sr.mu.RLock()
	rule := sr.router.Route(spec)
	sr.mu.RUnlock()

	available := sr.pool.Available()
	if len(available) == 0 {
		return RoutingDecision{
			Reason: "no providers available",
		}
	}

	// If a routing rule matched, try its preferred provider first.
	if rule != nil {
		maxCost := rule.MaxCost
		if budget > 0 && budget < maxCost {
			maxCost = budget
		}
		for _, p := range available {
			if p.Name == rule.PreferredProvider {
				return RoutingDecision{
					Provider: p.Name,
					Model:    p.Model,
					MaxCost:  maxCost,
					Reason:   fmt.Sprintf("rule match: %s tasks prefer %s", taskType, p.Name),
				}
			}
		}
	}

	// Fall back to cheapest available provider within budget.
	var best *ProviderInfo
	for i := range available {
		p := &available[i]
		if budget > 0 && p.CostPerToken > budget {
			continue
		}
		if best == nil || p.CostPerToken < best.CostPerToken {
			best = p
		}
	}

	if best == nil {
		// All providers exceed budget; pick cheapest anyway.
		for i := range available {
			p := &available[i]
			if best == nil || p.CostPerToken < best.CostPerToken {
				best = p
			}
		}
		return RoutingDecision{
			Provider: best.Name,
			Model:    best.Model,
			MaxCost:  budget,
			Reason:   fmt.Sprintf("cheapest provider for %s task (over budget)", taskType),
		}
	}

	maxCost := budget
	if maxCost <= 0 {
		maxCost = 10.0 // no budget cap
	}

	return RoutingDecision{
		Provider: best.Name,
		Model:    best.Model,
		MaxCost:  maxCost,
		Reason:   fmt.Sprintf("cheapest available provider for %s task", taskType),
	}
}
