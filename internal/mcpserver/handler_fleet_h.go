package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
)

// handleBlackboardQuery returns entries in a blackboard namespace.
func (s *Server) handleBlackboardQuery(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.Blackboard == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "blackboard not initialized",
		}), nil
	}

	ns := getStringArg(req, "namespace")
	if ns == "" {
		return codedError(ErrInvalidParams, "namespace is required"), nil
	}

	entries := s.Blackboard.Query(ns)
	return jsonResult(map[string]any{
		"namespace": ns,
		"count":     len(entries),
		"entries":   entries,
	}), nil
}

// handleBlackboardPut writes an entry to the blackboard.
func (s *Server) handleBlackboardPut(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.Blackboard == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "blackboard not initialized",
		}), nil
	}

	ns := getStringArg(req, "namespace")
	if ns == "" {
		return codedError(ErrInvalidParams, "namespace is required"), nil
	}
	key := getStringArg(req, "key")
	if key == "" {
		return codedError(ErrInvalidParams, "key is required"), nil
	}
	valueStr := getStringArg(req, "value")
	if valueStr == "" {
		return codedError(ErrInvalidParams, "value is required"), nil
	}

	var value map[string]any
	if err := json.Unmarshal([]byte(valueStr), &value); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("value must be valid JSON object: %v", err)), nil
	}

	writerID := getStringArg(req, "writer_id")
	ttlSeconds := getNumberArg(req, "ttl_seconds", 0)

	var ttl time.Duration
	if ttlSeconds > 0 {
		ttl = time.Duration(ttlSeconds) * time.Second
	}

	entry := blackboard.Entry{
		Namespace: ns,
		Key:       key,
		Value:     value,
		WriterID:  writerID,
		TTL:       ttl,
	}
	if err := s.Blackboard.Put(entry); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("blackboard put: %v", err)), nil
	}

	stored, _ := s.Blackboard.Get(ns, key)
	return jsonResult(stored), nil
}

// handleA2AOffers lists open agent-to-agent task offers.
func (s *Server) handleA2AOffers(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.A2A == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "a2a adapter not initialized",
		}), nil
	}

	offers := s.A2A.ListOpenOffers()
	return jsonResult(map[string]any{
		"count":  len(offers),
		"offers": offers,
	}), nil
}

// handleCostForecast returns cost burn rate, anomalies, and exhaustion ETA.
func (s *Server) handleCostForecast(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.CostPredictor == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "cost predictor not initialized",
		}), nil
	}

	budgetRemaining := getNumberArg(req, "budget_remaining", 0)
	forecast := s.CostPredictor.Forecast(budgetRemaining)
	return jsonResult(forecast), nil
}
