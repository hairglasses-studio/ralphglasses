package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer/fewshot"
	"github.com/hairglasses-studio/ralphglasses/internal/promptdj"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handlePromptDJRoute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt is required"), nil
	}
	router := s.getOrCreateDJRouter()
	if router == nil {
		return codedError(ErrInternal, "Prompt DJ router not initialized"), nil
	}
	rreq := promptdj.RoutingRequest{
		Prompt: prompt, Repo: getStringArg(req, "repo"),
		Score: int(getNumberArg(req, "score", 0)),
	}
	if tt := getStringArg(req, "task_type"); tt != "" {
		rreq.TaskType = enhancer.TaskType(tt)
	}
	d, err := router.Route(ctx, rreq)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("routing failed: %v", err)), nil
	}
	return jsonResult(d), nil
}

func (s *Server) handlePromptDJDispatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	repo := getStringArg(req, "repo")
	if prompt == "" || repo == "" {
		return codedError(ErrInvalidParams, "prompt and repo are required"), nil
	}
	router := s.getOrCreateDJRouter()
	if router == nil {
		return codedError(ErrInternal, "Prompt DJ router not initialized"), nil
	}
	rreq := promptdj.RoutingRequest{Prompt: prompt, Repo: repo}
	if tt := getStringArg(req, "task_type"); tt != "" {
		rreq.TaskType = enhancer.TaskType(tt)
	}
	d, err := router.Route(ctx, rreq)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("routing failed: %v", err)), nil
	}
	result := map[string]any{
		"decision_id": d.DecisionID, "provider": d.Provider, "model": d.Model,
		"task_type": d.TaskType, "complexity": d.Complexity, "confidence": d.Confidence,
		"estimated_cost_usd": d.EstimatedCostUSD, "enhanced": d.WasEnhanced,
		"original_score": d.OriginalScore, "reasoning": d.Rationale,
		"dry_run": getBoolArg(req, "dry_run"),
	}
	if getBoolArg(req, "dry_run") {
		return jsonResult(result), nil
	}
	effectivePrompt := prompt
	if d.WasEnhanced && d.EnhancedPrompt != "" {
		effectivePrompt = d.EnhancedPrompt
	}
	opts := session.LaunchOptions{
		Provider: d.Provider, RepoPath: repo, Prompt: effectivePrompt,
		Model: d.Model, MaxBudgetUSD: getNumberArg(req, "budget_usd", 5.0),
	}
	sess, err := s.SessMgr.Launch(ctx, opts)
	if err != nil {
		result["launch_error"] = err.Error()
		return jsonResult(result), nil
	}
	result["session_id"] = sess.ID
	return jsonResult(result), nil
}

func (s *Server) handlePromptDJFeedback(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	decisionID := getStringArg(req, "decision_id")
	if decisionID == "" {
		return codedError(ErrInvalidParams, "decision_id is required"), nil
	}
	m := argsMap(req)
	if m == nil {
		return codedError(ErrInvalidParams, "arguments required"), nil
	}
	successRaw, ok := m["success"]
	if !ok {
		return codedError(ErrInvalidParams, "success (boolean) is required"), nil
	}
	success, _ := successRaw.(bool)
	router := s.getOrCreateDJRouter()
	if router == nil {
		return codedError(ErrInternal, "Prompt DJ router not initialized"), nil
	}
	log := router.GetDecisionLog()
	if log == nil {
		return codedError(ErrInternal, "decision log not available"), nil
	}
	if err := log.RecordOutcome(decisionID, success,
		getNumberArg(req, "cost_usd", 0), int(getNumberArg(req, "turns", 0)),
		getStringArg(req, "notes")); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("recording outcome failed: %v", err)), nil
	}
	return jsonResult(map[string]any{
		"decision_id": decisionID, "status": "recorded",
		"feedback_applied": []string{"decision_log"},
	}), nil
}

// handlePromptDJSimilar finds similar high-quality prompts from the registry.
func (s *Server) handlePromptDJSimilar(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt is required"), nil
	}
	retriever := s.getOrCreateRetriever()
	if retriever == nil {
		return codedError(ErrInternal, "retriever not initialized"), nil
	}
	repo := getStringArg(req, "repo")
	result, err := retriever.Retrieve(ctx, prompt, repo)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("retrieval failed: %v", err)), nil
	}
	return jsonResult(result), nil
}

// handlePromptDJSuggest provides routing-aware improvement suggestions.
func (s *Server) handlePromptDJSuggest(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt is required"), nil
	}

	// Analyze prompt quality
	ar := enhancer.Analyze(prompt)
	taskType := enhancer.Classify(prompt)

	// Route preview (lightweight, no session launch)
	router := s.getOrCreateDJRouter()
	var routeInfo map[string]any
	if router != nil {
		d, err := router.Route(context.Background(), promptdj.RoutingRequest{
			Prompt: prompt, Repo: getStringArg(req, "repo"),
		})
		if err == nil {
			routeInfo = map[string]any{
				"provider": d.Provider, "model": d.Model,
				"tier": d.CostTier, "confidence": d.Confidence,
			}
		}
	}

	// Build suggestions
	var suggestions []map[string]string
	score := ar.Score
	if ar.ScoreReport != nil {
		score = ar.ScoreReport.Overall
	}

	if score < 50 {
		suggestions = append(suggestions, map[string]string{
			"category": "quality", "priority": "high",
			"message":  fmt.Sprintf("Score %d/100 is low. Enhancement recommended before routing.", score),
			"action":   "Run /improve-prompt or prompt_improve to enhance.",
		})
	} else if score < 70 {
		suggestions = append(suggestions, map[string]string{
			"category": "quality", "priority": "medium",
			"message":  fmt.Sprintf("Score %d/100 is moderate. Enhancement would improve routing confidence.", score),
			"action":   "Consider running prompt_improve for better results.",
		})
	}

	if !ar.HasXML {
		suggestions = append(suggestions, map[string]string{
			"category": "structure", "priority": "medium",
			"message":  "No XML structure detected. Claude performs better with XML-tagged prompts.",
			"action":   "Add <role>, <instructions>, <constraints> tags.",
		})
	}
	if !ar.HasExamples {
		suggestions = append(suggestions, map[string]string{
			"category": "structure", "priority": "low",
			"message":  "No examples found. 3-5 few-shot examples improve output quality.",
			"action":   "Use promptdj_similar to find examples from the registry.",
		})
	}
	if ar.HasNegativeFrames {
		suggestions = append(suggestions, map[string]string{
			"category": "quality", "priority": "medium",
			"message":  "Negative framing detected. Claude 4.x responds better to positive instructions.",
			"action":   "Rewrite 'don't do X' as 'do Y instead'.",
		})
	}

	return jsonResult(map[string]any{
		"prompt_score":  score,
		"prompt_grade":  ar.ScoreReport.Grade,
		"task_type":     taskType,
		"suggestions":   suggestions,
		"would_route_to": routeInfo,
	}), nil
}

// handlePromptDJHistory returns routing decision history with optional summary mode.
func (s *Server) handlePromptDJHistory(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	router := s.getOrCreateDJRouter()
	if router == nil {
		return codedError(ErrInternal, "Prompt DJ router not initialized"), nil
	}
	log := router.GetDecisionLog()
	if log == nil {
		return codedError(ErrInternal, "decision log not available"), nil
	}

	filter := promptdj.DecisionFilter{
		Repo:     getStringArg(req, "repo"),
		Provider: getStringArg(req, "provider"),
		TaskType: getStringArg(req, "task_type"),
		Status:   getStringArg(req, "status"),
		Limit:    int(getNumberArg(req, "limit", 50)),
	}

	// Parse time window
	if since := getStringArg(req, "since"); since != "" {
		if d, err := time.ParseDuration(since); err == nil {
			filter.Since = time.Now().Add(-d)
		} else if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = t
		}
	}

	decisions := log.QueryDecisions(filter)
	summary := getBoolArg(req, "summary")

	if summary {
		// Aggregate summary
		var totalCost, successCount float64
		byProvider := map[string]int{}
		byTaskType := map[string]int{}
		byStatus := map[string]int{}
		for _, d := range decisions {
			totalCost += d.ActualCost
			byProvider[d.Provider]++
			byTaskType[d.TaskType]++
			byStatus[d.Status]++
			if d.Status == "succeeded" {
				successCount++
			}
		}
		var successRate float64
		if len(decisions) > 0 {
			successRate = successCount / float64(len(decisions))
		}
		return jsonResult(map[string]any{
			"total_decisions": len(decisions),
			"success_rate":    successRate,
			"total_cost_usd":  totalCost,
			"by_provider":     byProvider,
			"by_task_type":    byTaskType,
			"by_status":       byStatus,
		}), nil
	}

	return jsonResult(map[string]any{
		"decisions": decisions,
		"total":     len(decisions),
	}), nil
}

func (s *Server) getOrCreateRetriever() *fewshot.Retriever {
	if s.fewshotRetriever != nil {
		return s.fewshotRetriever
	}
	indexPath := filepath.Join(os.Getenv("HOME"), "hairglasses-studio", "docs", "prompts", ".prompt-index.jsonl")
	if _, err := os.Stat(indexPath); err != nil {
		return nil
	}
	cfg := fewshot.DefaultConfig()
	s.fewshotRetriever = fewshot.NewRetriever(indexPath, cfg)
	return s.fewshotRetriever
}

func (s *Server) getOrCreateDJRouter() *promptdj.PromptDJRouter {
	if s.djRouter != nil {
		return s.djRouter
	}
	cfg := promptdj.ConfigFromEnv()
	cfg.Enabled = true
	stateDir := s.ScanPath
	if stateDir == "" {
		stateDir = "."
	}
	s.djRouter = promptdj.NewPromptDJRouter(nil, nil, nil, s.Engine, nil, cfg, stateDir)
	return s.djRouter
}
