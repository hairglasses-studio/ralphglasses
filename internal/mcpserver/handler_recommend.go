package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
)

// handleCostRecommend analyzes cost history and returns actionable
// configuration recommendations (provider switches, budget pacing,
// anomaly responses, cache optimization, model downgrades).
func (s *Server) handleCostRecommend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.CostPredictor == nil {
		return jsonResult(map[string]any{
			"status":          "not_configured",
			"message":         "cost predictor not initialized — record cost samples first",
			"recommendations": []any{},
			"count":           0,
		}), nil
	}

	if s.CostPredictor.Len() < 2 {
		return jsonResult(map[string]any{
			"status":          "insufficient_data",
			"message":         "need at least 2 cost samples for analysis",
			"sample_count":    s.CostPredictor.Len(),
			"recommendations": []any{},
			"count":           0,
		}), nil
	}

	// Build recommender config from request parameters.
	cfg := fleet.DefaultRecommenderConfig()

	if v := getNumberArg(req, "budget_remaining", 0); v > 0 {
		cfg.BudgetRemaining = v
	}
	if v := int(getNumberArg(req, "concurrency", 0)); v > 0 {
		cfg.Concurrency = v
	}
	if v := getNumberArg(req, "budget_hours", 0); v > 0 {
		cfg.BudgetHours = v
	}
	if v := int(getNumberArg(req, "min_samples", 0)); v > 0 {
		cfg.MinSamplesPerProvider = v
	}

	recommender := fleet.NewRecommender(s.CostPredictor).WithConfig(cfg)
	recs := recommender.Analyze()

	if recs == nil {
		recs = []fleet.Recommendation{}
	}

	// Optional type filter.
	if typeFilter := getStringArg(req, "type"); typeFilter != "" {
		filtered := make([]fleet.Recommendation, 0, len(recs))
		for _, rec := range recs {
			if string(rec.Type) == typeFilter {
				filtered = append(filtered, rec)
			}
		}
		recs = filtered
	}

	return jsonResult(map[string]any{
		"recommendations": recs,
		"count":           len(recs),
		"sample_count":    s.CostPredictor.Len(),
	}), nil
}
