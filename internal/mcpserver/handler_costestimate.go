package mcpserver

import (
	"context"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// CostEstimate holds the result of a pre-launch cost prediction.
type CostEstimate struct {
	Provider      string              `json:"provider"`
	Model         string              `json:"model"`
	Mode          string              `json:"mode"`
	Estimate      CostEstimateRange   `json:"estimate"`
	Breakdown     CostEstimateBreak   `json:"breakdown"`
	HistoricalAvg *float64            `json:"historical_avg_usd,omitempty"`
	Confidence    string              `json:"confidence"`
}

// CostEstimateRange holds low/mid/high USD estimates.
type CostEstimateRange struct {
	LowUSD  float64 `json:"low_usd"`
	MidUSD  float64 `json:"mid_usd"`
	HighUSD float64 `json:"high_usd"`
}

// CostEstimateBreak holds the per-component cost breakdown.
type CostEstimateBreak struct {
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	InputCostUSD  float64 `json:"input_cost_usd"`
	OutputCostUSD float64 `json:"output_cost_usd"`
	Turns         int     `json:"turns"`
	Iterations    int     `json:"iterations,omitempty"`
}

// defaultModels maps provider names to their default model identifiers.
var defaultModels = map[string]string{
	"claude": "claude-sonnet-4-6",
	"gemini": "gemini-2.5-flash",
	"codex":  "gpt-5.4",
}

// providerRateKeys maps provider names to the CostRates map key used for lookup.
var providerRateKeys = map[string]string{
	"claude": "claude_sonnet",
	"gemini": "gemini_flash",
	"codex":  "codex",
}

// estimateSessionCost computes a CostEstimate from the given parameters.
// This is a pure function suitable for unit testing without a Server.
func estimateSessionCost(
	provider, model string,
	promptTokens, outputTokensPerTurn, turns int,
	mode string, iterations int,
	rates *session.CostRates,
	historicalAvg *float64,
) CostEstimate {
	if model == "" {
		model = defaultModels[provider]
	}
	if mode == "" {
		mode = "session"
	}

	// Look up rates for the provider.
	rateKey := providerRateKeys[provider]
	inputRate := rates.InputPerMToken[rateKey]   // USD per 1M tokens
	outputRate := rates.OutputPerMToken[rateKey]  // USD per 1M tokens

	// Total tokens across all turns.
	totalInputTokens := promptTokens * turns
	totalOutputTokens := outputTokensPerTurn * turns

	// For loop mode, multiply by iterations and add planner overhead.
	if mode == "loop" {
		totalInputTokens *= iterations
		totalOutputTokens *= iterations
	}

	inputCost := float64(totalInputTokens) * inputRate / 1_000_000
	outputCost := float64(totalOutputTokens) * outputRate / 1_000_000
	baseCost := inputCost + outputCost

	// Add 20% planner overhead for loop mode.
	if mode == "loop" {
		baseCost *= 1.20
		inputCost *= 1.20
		outputCost *= 1.20
	}

	// Determine confidence level.
	confidence := "medium"
	if historicalAvg != nil {
		confidence = "high"
	}

	// If we have historical data, calibrate: blend estimated with historical.
	midCost := baseCost
	if historicalAvg != nil && *historicalAvg > 0 {
		// Weight: 60% estimate, 40% historical.
		midCost = baseCost*0.6 + (*historicalAvg)*0.4
	}

	est := CostEstimate{
		Provider: provider,
		Model:    model,
		Mode:     mode,
		Estimate: CostEstimateRange{
			LowUSD:  midCost * 0.7,
			MidUSD:  midCost,
			HighUSD: midCost * 1.5,
		},
		Breakdown: CostEstimateBreak{
			InputTokens:   totalInputTokens,
			OutputTokens:  totalOutputTokens,
			InputCostUSD:  inputCost,
			OutputCostUSD: outputCost,
			Turns:         turns,
		},
		HistoricalAvg: historicalAvg,
		Confidence:    confidence,
	}

	if mode == "loop" {
		est.Breakdown.Iterations = iterations
	}

	return est
}

func (s *Server) handleCostEstimate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	provider := getStringArg(req, "provider")
	if provider == "" {
		return invalidParams("provider is required (claude, gemini, codex)"), nil
	}
	if _, ok := providerRateKeys[provider]; !ok {
		return invalidParams("provider must be one of: claude, gemini, codex"), nil
	}

	model := getStringArg(req, "model")
	promptTokens := int(getNumberArg(req, "prompt_tokens", 5000))
	turns := int(getNumberArg(req, "turns", 5))
	outputTokensPerTurn := int(getNumberArg(req, "output_tokens_per_turn", 2000))
	mode := getStringArg(req, "mode")
	if mode == "" {
		mode = "session"
	}
	if mode != "session" && mode != "loop" {
		return invalidParams("mode must be 'session' or 'loop'"), nil
	}
	iterations := int(getNumberArg(req, "iterations", 3))
	repo := getStringArg(req, "repo")

	// Load cost rates.
	rates := session.DefaultCostRates()

	// Try to load historical observations for calibration.
	var historicalAvg *float64
	if repo != "" {
		if s.reposNil() {
			_ = s.scan()
		}
		if r := s.findRepo(repo); r != nil {
			obsPath := filepath.Join(r.Path, ".ralph", "logs", "loop_observations.jsonl")
			obs, err := session.LoadObservations(obsPath, time.Now().Add(-30*24*time.Hour))
			if err == nil && len(obs) > 0 {
				var total float64
				var count int
				for _, o := range obs {
					workerProv := o.WorkerProvider
					plannerProv := o.PlannerProvider
					if workerProv == provider || plannerProv == provider {
						total += o.TotalCostUSD
						count++
					}
				}
				if count > 0 {
					avg := total / float64(count)
					historicalAvg = &avg
				}
			}
		}
	}

	est := estimateSessionCost(
		provider, model,
		promptTokens, outputTokensPerTurn, turns,
		mode, iterations,
		rates, historicalAvg,
	)

	return jsonResult(est), nil
}
