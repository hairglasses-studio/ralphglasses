package mcpserver

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleCostEstimateMissingProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing provider")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected %s error code, got: %s", ErrInvalidParams, text)
	}
}

func TestHandleCostEstimateClaude(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var est CostEstimate
	if err := json.Unmarshal([]byte(getResultText(result)), &est); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if est.Provider != "claude" {
		t.Errorf("provider = %q, want %q", est.Provider, "claude")
	}
	if est.Model != session.ProviderDefaults(session.ProviderClaude) {
		t.Errorf("model = %q, want %q", est.Model, session.ProviderDefaults(session.ProviderClaude))
	}
	if est.Mode != "session" {
		t.Errorf("mode = %q, want %q", est.Mode, "session")
	}
	if est.Estimate.MidUSD <= 0 {
		t.Errorf("mid estimate should be positive, got %f", est.Estimate.MidUSD)
	}
	if est.Estimate.LowUSD >= est.Estimate.MidUSD {
		t.Errorf("low (%f) should be less than mid (%f)", est.Estimate.LowUSD, est.Estimate.MidUSD)
	}
	if est.Estimate.HighUSD <= est.Estimate.MidUSD {
		t.Errorf("high (%f) should be greater than mid (%f)", est.Estimate.HighUSD, est.Estimate.MidUSD)
	}
	if est.Confidence != "medium" {
		t.Errorf("confidence = %q, want %q", est.Confidence, "medium")
	}
	if est.Breakdown.Turns != 5 {
		t.Errorf("turns = %d, want 5", est.Breakdown.Turns)
	}
}

func TestHandleCostEstimateGemini(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "gemini",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var est CostEstimate
	if err := json.Unmarshal([]byte(getResultText(result)), &est); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if est.Provider != "gemini" {
		t.Errorf("provider = %q, want %q", est.Provider, "gemini")
	}
	if est.Model != "gemini-3.1-flash" {
		t.Errorf("model = %q, want %q", est.Model, "gemini-3.1-flash")
	}

	// Gemini should be cheaper than Claude for same defaults.
	claudeResult, _ := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
	}))
	var claudeEst CostEstimate
	_ = json.Unmarshal([]byte(getResultText(claudeResult)), &claudeEst)

	if est.Estimate.MidUSD >= claudeEst.Estimate.MidUSD {
		t.Errorf("gemini mid (%f) should be less than claude mid (%f)", est.Estimate.MidUSD, claudeEst.Estimate.MidUSD)
	}
}

func TestHandleCostEstimateLoopMode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sessionResult, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
		"mode":     "session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loopResult, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider":   "claude",
		"mode":       "loop",
		"iterations": float64(3),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sessEst, loopEst CostEstimate
	_ = json.Unmarshal([]byte(getResultText(sessionResult)), &sessEst)
	_ = json.Unmarshal([]byte(getResultText(loopResult)), &loopEst)

	// Loop with 3 iterations + 20% overhead should be ~3.6x session cost.
	ratio := loopEst.Estimate.MidUSD / sessEst.Estimate.MidUSD
	if ratio < 3.0 || ratio > 4.0 {
		t.Errorf("loop/session ratio = %f, expected ~3.6 (3 iterations * 1.2 overhead)", ratio)
	}

	if loopEst.Breakdown.Iterations != 3 {
		t.Errorf("iterations = %d, want 3", loopEst.Breakdown.Iterations)
	}
}

func TestHandleCostEstimateCustomTokens(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider":               "claude",
		"prompt_tokens":          float64(10000),
		"output_tokens_per_turn": float64(4000),
		"turns":                  float64(10),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var est CostEstimate
	if err := json.Unmarshal([]byte(getResultText(result)), &est); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if est.Breakdown.InputTokens != 100000 {
		t.Errorf("input_tokens = %d, want 100000 (10000 * 10)", est.Breakdown.InputTokens)
	}
	if est.Breakdown.OutputTokens != 40000 {
		t.Errorf("output_tokens = %d, want 40000 (4000 * 10)", est.Breakdown.OutputTokens)
	}
	if est.Breakdown.Turns != 10 {
		t.Errorf("turns = %d, want 10", est.Breakdown.Turns)
	}
}

func TestHandleCostEstimateInvalidProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "gpt4",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleCostEstimate", result, "INVALID_PARAMS")
}

func TestHandleCostEstimateInvalidMode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
		"mode":     "batch",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleCostEstimate", result, "INVALID_PARAMS")
}

func TestHandleCostEstimateCodex(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "codex",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var est CostEstimate
	if err := json.Unmarshal([]byte(getResultText(result)), &est); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if est.Provider != "codex" {
		t.Errorf("provider = %q, want codex", est.Provider)
	}
}

func TestHandleCostEstimateOllama(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "ollama",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var est CostEstimate
	if err := json.Unmarshal([]byte(getResultText(result)), &est); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if est.Provider != "ollama" {
		t.Fatalf("provider = %q, want ollama", est.Provider)
	}
	if est.Model != session.ProviderDefaults(session.ProviderOllama) {
		t.Fatalf("model = %q, want %q", est.Model, session.ProviderDefaults(session.ProviderOllama))
	}
	if est.Estimate.MidUSD != 0 {
		t.Fatalf("mid estimate = %f, want 0", est.Estimate.MidUSD)
	}
}

func TestHandleCostEstimateWithRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	// Scan first so repos are populated
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
		"repo":     "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

func TestEstimateSessionCostConfidenceDowngrade(t *testing.T) {
	t.Parallel()
	rates := session.DefaultCostRates()

	// Model estimate for Claude defaults: ~$0.225
	// Historical avg much lower: $0.04 — ratio = 0.225/0.04 = 5.625 (>2.0)
	hist := 0.04
	est := estimateSessionCost("claude", "", 5000, 2000, 5, "session", 3, rates, &hist)

	if est.Confidence != "low" {
		t.Errorf("confidence = %q, want %q (model/historical ratio > 2.0)", est.Confidence, "low")
	}
	if est.CalibrationRatio == nil {
		t.Fatal("expected calibration_ratio to be set")
	}
	if *est.CalibrationRatio < 2.0 {
		t.Errorf("calibration_ratio = %f, expected > 2.0", *est.CalibrationRatio)
	}

	// When ratio is within bounds, confidence should stay "high".
	histClose := 0.20
	estClose := estimateSessionCost("claude", "", 5000, 2000, 5, "session", 3, rates, &histClose)
	if estClose.Confidence != "high" {
		t.Errorf("confidence = %q, want %q when estimates are close", estClose.Confidence, "high")
	}
	if estClose.CalibrationRatio == nil {
		t.Fatal("expected calibration_ratio to be set even when close")
	}
	if *estClose.CalibrationRatio < 0.5 || *estClose.CalibrationRatio > 2.0 {
		t.Errorf("calibration_ratio = %f, expected between 0.5 and 2.0", *estClose.CalibrationRatio)
	}
}

func TestEstimateSessionCostPure(t *testing.T) {
	t.Parallel()
	rates := session.DefaultCostRates()

	// Claude session: 5000 prompt tokens * 5 turns = 25000 input tokens
	// 2000 output * 5 turns = 10000 output tokens
	// Input cost: 25000 * 3.00 / 1M = 0.075
	// Output cost: 10000 * 15.00 / 1M = 0.15
	// Total: 0.225
	est := estimateSessionCost("claude", "", 5000, 2000, 5, "session", 3, rates, nil)

	expectedInput := 0.075
	expectedOutput := 0.15
	expectedTotal := 0.225

	if math.Abs(est.Breakdown.InputCostUSD-expectedInput) > 0.001 {
		t.Errorf("input cost = %f, want %f", est.Breakdown.InputCostUSD, expectedInput)
	}
	if math.Abs(est.Breakdown.OutputCostUSD-expectedOutput) > 0.001 {
		t.Errorf("output cost = %f, want %f", est.Breakdown.OutputCostUSD, expectedOutput)
	}
	if math.Abs(est.Estimate.MidUSD-expectedTotal) > 0.001 {
		t.Errorf("mid estimate = %f, want %f", est.Estimate.MidUSD, expectedTotal)
	}
	if est.Confidence != "medium" {
		t.Errorf("confidence = %q, want %q", est.Confidence, "medium")
	}

	// With historical data, confidence should be "high".
	hist := 0.30
	estHist := estimateSessionCost("claude", "", 5000, 2000, 5, "session", 3, rates, &hist)
	if estHist.Confidence != "high" {
		t.Errorf("confidence with history = %q, want %q", estHist.Confidence, "high")
	}
	// Mid should be blended: 0.225 * 0.6 + 0.30 * 0.4 = 0.135 + 0.12 = 0.255
	expectedBlend := 0.255
	if math.Abs(estHist.Estimate.MidUSD-expectedBlend) > 0.001 {
		t.Errorf("blended mid = %f, want %f", estHist.Estimate.MidUSD, expectedBlend)
	}
}
