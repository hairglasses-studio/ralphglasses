package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// ConfigurationTools returns tools for managing webb scoring and alias configuration
func ConfigurationTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_scoring_config",
				mcp.WithDescription("View or update work item scoring configuration. Controls base scores, priority boosts, customer tiers, time decay, and SLA scoring."),
				mcp.WithString("action", mcp.Description("Action: view (default), set_base_score, set_priority_boost, set_customer_tier")),
				mcp.WithString("source", mcp.Description("For set_base_score: source type (incident, pylon, shortcut, slack, grafana, github)")),
				mcp.WithString("priority", mcp.Description("For set_priority_boost: priority level (P0, P1, P2, P3, P4, high, medium, low)")),
				mcp.WithString("customer", mcp.Description("For set_customer_tier: customer name")),
				mcp.WithNumber("score", mcp.Description("Score value to set")),
			),
			Handler:     handleScoringConfig,
			Category:    "discovery",
			Subcategory: "configuration",
			Tags:        []string{"scoring", "config", "priority", "customer-tier", "sla"},
			UseCases:    []string{"View scoring weights", "Adjust customer tiers", "Tune priority boosts"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_alias_manage",
				mcp.WithDescription("Manage entity aliases for customers, components, and symptoms. Aliases enable flexible entity matching across webb tools."),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action: list, resolve, add, stats")),
				mcp.WithString("type", mcp.Description("Entity type: customer (default), component, symptom")),
				mcp.WithString("canonical", mcp.Description("For add: canonical name to add alias to")),
				mcp.WithString("alias", mcp.Description("For add/resolve: alias string")),
				mcp.WithBoolean("save", mcp.Description("For add: persist to vault (default: false)")),
			),
			Handler:     handleAliasManage,
			Category:    "discovery",
			Subcategory: "configuration",
			Tags:        []string{"alias", "entity", "customer", "component", "resolution"},
			UseCases:    []string{"List entities", "Resolve aliases", "Add new aliases"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
	}
}

func handleScoringConfig(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := request.GetString("action", "view")
	engine := clients.GetScoringEngine()

	switch action {
	case "view":
		return viewScoringConfig(engine)
	case "set_base_score":
		source := request.GetString("source", "")
		score := request.GetFloat("score", -1)
		if source == "" || score < 0 {
			return tools.ErrorResult(fmt.Errorf("source and score required")), nil
		}
		engine.SetBaseScore(source, int(score))
		return tools.TextResult(fmt.Sprintf("Updated base score for '%s' to %d", source, int(score))), nil
	case "set_customer_tier":
		customer := request.GetString("customer", "")
		score := request.GetFloat("score", -1)
		if customer == "" || score < 0 {
			return tools.ErrorResult(fmt.Errorf("customer and score required")), nil
		}
		engine.SetCustomerTier(customer, int(score))
		tier := engine.GetCustomerTier(customer)
		return tools.TextResult(fmt.Sprintf("Updated '%s' tier score to %d (tier: %s)", customer, int(score), tier)), nil
	case "set_priority_boost":
		priority := request.GetString("priority", "")
		score := request.GetFloat("score", -1)
		if priority == "" || score < 0 {
			return tools.ErrorResult(fmt.Errorf("priority and score required")), nil
		}
		engine.SetPriorityBoost(priority, int(score))
		return tools.TextResult(fmt.Sprintf("Updated priority boost for '%s' to +%d", priority, int(score))), nil
	default:
		return tools.ErrorResult(fmt.Errorf("unknown action: %s", action)), nil
	}
}

func viewScoringConfig(engine *clients.ScoringEngine) (*mcp.CallToolResult, error) {
	config := engine.GetConfig()
	var sb strings.Builder
	sb.WriteString("# Work Item Scoring Configuration\n\n")

	// Base Scores
	sb.WriteString("## Base Scores\n| Source | Score |\n|--------|-------|\n")
	sources := make([]string, 0, len(config.BaseScores))
	for s := range config.BaseScores {
		sources = append(sources, s)
	}
	sort.Strings(sources)
	for _, s := range sources {
		sb.WriteString(fmt.Sprintf("| %s | %d |\n", s, config.BaseScores[s]))
	}

	// Priority Boosts
	sb.WriteString("\n## Priority Boosts\n| Priority | Boost |\n|----------|-------|\n")
	priorityDisplay := map[string]string{"p0": "P0", "p1": "P1", "p2": "P2", "p3": "P3", "p4": "P4", "high": "high", "medium": "medium", "low": "low"}
	for _, p := range []string{"p0", "p1", "p2", "p3", "p4", "high", "medium", "low"} {
		if b, ok := config.PriorityBoosts[p]; ok {
			sb.WriteString(fmt.Sprintf("| %s | +%d |\n", priorityDisplay[p], b))
		}
	}

	// Customer Tiers
	sb.WriteString("\n## Customer Tiers\n| Tier | Customers |\n|------|----------|\n")
	t1, t2, t3 := []string{}, []string{}, []string{}
	for c, s := range config.CustomerTiers {
		switch {
		case s >= 20:
			t1 = append(t1, fmt.Sprintf("%s(%d)", c, s))
		case s >= 15:
			t2 = append(t2, fmt.Sprintf("%s(%d)", c, s))
		case s >= 10:
			t3 = append(t3, fmt.Sprintf("%s(%d)", c, s))
		}
	}
	sort.Strings(t1)
	sort.Strings(t2)
	sort.Strings(t3)
	sb.WriteString(fmt.Sprintf("| Tier 1 (20+) | %s |\n", strings.Join(t1, ", ")))
	sb.WriteString(fmt.Sprintf("| Tier 2 (15-19) | %s |\n", strings.Join(t2, ", ")))
	sb.WriteString(fmt.Sprintf("| Tier 3 (10-14) | %s |\n", strings.Join(t3, ", ")))

	// Time Decay & SLA
	sb.WriteString("\n## Time Decay\n")
	sb.WriteString(fmt.Sprintf("- Fresh (<%dh): +%d\n", config.TimeDecay.FreshThresholdHours, config.TimeDecay.FreshBoost))
	sb.WriteString(fmt.Sprintf("- Stale (>%dh): %d\n", config.TimeDecay.StaleThresholdHours, config.TimeDecay.StalePenalty))
	sb.WriteString(fmt.Sprintf("- Very stale (>%dh): %d\n", config.TimeDecay.VeryStaleHours, config.TimeDecay.VeryStalepenalty))

	sb.WriteString("\n## SLA Scoring\n")
	sb.WriteString(fmt.Sprintf("- Breached: +%d | At Risk (>%d%%): +%d\n",
		config.SLAScoring.BreachedBoost, config.SLAScoring.AtRiskThreshold, config.SLAScoring.AtRiskBoost))

	return tools.TextResult(sb.String()), nil
}

func handleAliasManage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action, _ := request.RequireString("action")
	entityType := request.GetString("type", "customer")
	store := clients.GetAliasStore()

	switch action {
	case "list":
		return listAliases(store, entityType)
	case "resolve":
		alias := request.GetString("alias", "")
		if alias == "" {
			return tools.ErrorResult(fmt.Errorf("alias required")), nil
		}
		return resolveAlias(store, entityType, alias)
	case "add":
		canonical := request.GetString("canonical", "")
		alias := request.GetString("alias", "")
		save := request.GetBool("save", false)
		if canonical == "" || alias == "" {
			return tools.ErrorResult(fmt.Errorf("canonical and alias required")), nil
		}
		return addAlias(store, entityType, canonical, alias, save)
	case "stats":
		stats := store.Stats()
		data, _ := json.MarshalIndent(stats, "", "  ")
		return tools.TextResult(fmt.Sprintf("# Alias Store Statistics\n```json\n%s\n```", string(data))), nil
	default:
		return tools.ErrorResult(fmt.Errorf("unknown action: %s", action)), nil
	}
}

func listAliases(store *clients.AliasStore, entityType string) (*mcp.CallToolResult, error) {
	var sb strings.Builder
	switch entityType {
	case "customer":
		customers := store.GetKnownCustomers()
		sort.Strings(customers)
		sb.WriteString(fmt.Sprintf("# Known Customers (%d)\n\n| Canonical | Aliases |\n|-----------|----------|\n", len(customers)))
		for _, c := range customers {
			aliases := store.GetCustomerAliases(c)
			if len(aliases) > 0 {
				sb.WriteString(fmt.Sprintf("| %s | %s |\n", c, strings.Join(aliases, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | - |\n", c))
			}
		}
	case "component":
		components := store.GetKnownComponents()
		sort.Strings(components)
		sb.WriteString(fmt.Sprintf("# Known Components (%d)\n\n| Canonical | Aliases |\n|-----------|----------|\n", len(components)))
		for _, c := range components {
			aliases := store.GetComponentAliases(c)
			if len(aliases) > 0 {
				sb.WriteString(fmt.Sprintf("| %s | %s |\n", c, strings.Join(aliases, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | - |\n", c))
			}
		}
	case "symptom":
		symptoms := store.GetKnownSymptoms()
		sort.Strings(symptoms)
		sb.WriteString(fmt.Sprintf("# Known Symptoms (%d)\n\n| Canonical | Aliases |\n|-----------|----------|\n", len(symptoms)))
		for _, s := range symptoms {
			aliases := store.GetSymptomAliases(s)
			if len(aliases) > 0 {
				sb.WriteString(fmt.Sprintf("| %s | %s |\n", s, strings.Join(aliases, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("| %s | - |\n", s))
			}
		}
	default:
		return tools.ErrorResult(fmt.Errorf("unknown type: %s", entityType)), nil
	}
	return tools.TextResult(sb.String()), nil
}

func resolveAlias(store *clients.AliasStore, entityType, input string) (*mcp.CallToolResult, error) {
	var resolved string
	switch entityType {
	case "customer":
		resolved = store.ResolveCustomer(input)
	case "component":
		resolved = store.ResolveComponent(input)
	case "symptom":
		resolved = store.ResolveSymptom(input)
	default:
		return tools.ErrorResult(fmt.Errorf("unknown type: %s", entityType)), nil
	}
	if resolved != input {
		return tools.TextResult(fmt.Sprintf("Resolved '%s' → '%s'", input, resolved)), nil
	}
	return tools.TextResult(fmt.Sprintf("'%s' (no alias found, using as-is)", input)), nil
}

func addAlias(store *clients.AliasStore, entityType, canonical, alias string, save bool) (*mcp.CallToolResult, error) {
	switch entityType {
	case "customer":
		store.AddCustomerAlias(canonical, alias)
	case "component":
		store.AddComponentAlias(canonical, alias)
	case "symptom":
		store.AddSymptomAlias(canonical, alias)
	default:
		return tools.ErrorResult(fmt.Errorf("unknown type: %s", entityType)), nil
	}
	msg := fmt.Sprintf("Added alias '%s' → '%s'", alias, canonical)
	if save {
		if err := store.SaveToVault(); err != nil {
			msg += fmt.Sprintf(" (save failed: %v)", err)
		} else {
			msg += " (saved to vault)"
		}
	}
	return tools.TextResult(msg), nil
}
