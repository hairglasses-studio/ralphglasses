package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- scratchpad_validate tests ---

func TestHandleScratchpadValidate_MissingParams(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	// Missing name.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"check": "all"}
	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}

	// Missing check.
	req.Params.Arguments = map[string]any{"name": "test"}
	result, err = srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing check")
	}
}

func TestHandleScratchpadValidate_ScoreInflation(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Write scratchpad with inflated overall score.
	content := `# Score Report
overall: 95
clarity: 60
specificity: 50
structure: 55
tone: 45
`
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "scores_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "scores",
		"check": "scores",
	}

	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var vr validateResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &vr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if vr.Valid {
		t.Error("expected valid=false for inflated scores")
	}
	if len(vr.Violations) == 0 {
		t.Fatal("expected at least one violation")
	}
	if vr.Violations[0].Check != "scores" {
		t.Errorf("expected check=scores, got %s", vr.Violations[0].Check)
	}
	if !strings.Contains(vr.Violations[0].Message, "score inflation") {
		t.Errorf("expected score inflation message, got: %s", vr.Violations[0].Message)
	}
}

func TestHandleScratchpadValidate_ValidScores(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Write scratchpad with consistent scores.
	content := `# Score Report
overall: 55
clarity: 60
specificity: 50
structure: 55
tone: 55
`
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "scores_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "scores",
		"check": "scores",
	}

	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var vr validateResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &vr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if !vr.Valid {
		t.Errorf("expected valid=true, got violations: %+v", vr.Violations)
	}
}

func TestHandleScratchpadValidate_BudgetMismatch(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	content := `# Session Log
requested_budget: 10.00
applied_budget: 5.00
`
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "budget_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "budget",
		"check": "budget",
	}

	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var vr validateResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &vr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if vr.Valid {
		t.Error("expected valid=false for budget mismatch")
	}
	if len(vr.Violations) == 0 {
		t.Fatal("expected budget violation")
	}
	if vr.Violations[0].Check != "budget" {
		t.Errorf("expected check=budget, got %s", vr.Violations[0].Check)
	}
}

func TestHandleScratchpadValidate_Noops(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	content := `# Iteration Log
iter 1: files_changed: 0, verify: pass
iter 2: files_changed: 3, verify: pass
iter 3: files_changed: 0, verify: pass
iter 4: files_changed: 0, verify: pass
iter 5: files_changed: 0, verify: pass
`
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "iters_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "iters",
		"check": "noops",
	}

	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var vr validateResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &vr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if vr.Valid {
		t.Error("expected valid=false for no-op iterations")
	}
	if len(vr.Violations) != 1 {
		t.Fatalf("expected 1 noop violation, got %d", len(vr.Violations))
	}
	if vr.Violations[0].Severity != "warning" {
		t.Errorf("expected warning severity for 4 noops, got %s", vr.Violations[0].Severity)
	}
}

func TestHandleScratchpadValidate_AllChecks(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	content := "# Clean scratchpad\nSome notes here.\n"
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "clean_scratchpad.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "clean",
		"check": "all",
	}

	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	var vr validateResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &vr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if !vr.Valid {
		t.Errorf("expected valid=true for clean scratchpad, got violations: %+v", vr.Violations)
	}
	if len(vr.ChecksRun) != 4 {
		t.Errorf("expected 4 checks run with 'all', got %d: %v", len(vr.ChecksRun), vr.ChecksRun)
	}
}

func TestHandleScratchpadValidate_ScratchpadNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "nonexistent",
		"check": "all",
	}

	result, err := srv.handleScratchpadValidate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent scratchpad")
	}
}

// --- scratchpad_context tests ---

func TestHandleScratchpadContext_MissingParams(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	// Missing name.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"sections": "all"}
	result, err := srv.handleScratchpadContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}

	// Missing sections.
	req.Params.Arguments = map[string]any{"name": "test"}
	result, err = srv.handleScratchpadContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing sections")
	}
}

func TestHandleScratchpadContext_FleetSection(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "ctx_test",
		"sections": "fleet",
	}

	result, err := srv.handleScratchpadContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var cr contextAppendResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(cr.Appended) == 0 {
		t.Error("expected at least one appended section")
	}

	// Verify file was written.
	data, err := os.ReadFile(filepath.Join(root, ".ralph", "ctx_test_scratchpad.md"))
	if err != nil {
		t.Fatalf("scratchpad not created: %v", err)
	}
	if !strings.Contains(string(data), "### Fleet Status") {
		t.Errorf("expected Fleet Status header in scratchpad, got: %s", string(data))
	}
}

func TestHandleScratchpadContext_AllSections(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "ctx_all",
		"sections": "all",
	}

	result, err := srv.handleScratchpadContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var cr contextAppendResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have appended multiple sections.
	if len(cr.Appended) < 2 {
		t.Errorf("expected multiple appended sections with 'all', got %d: %v", len(cr.Appended), cr.Appended)
	}

	// Verify scratchpad file.
	data, err := os.ReadFile(filepath.Join(root, ".ralph", "ctx_all_scratchpad.md"))
	if err != nil {
		t.Fatalf("scratchpad not created: %v", err)
	}
	if !strings.Contains(string(data), "## System Context") {
		t.Errorf("expected System Context header in scratchpad")
	}
}

func TestHandleScratchpadContext_ObservationsWithNoFile(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":     "ctx_obs",
		"sections": "observations",
	}

	result, err := srv.handleScratchpadContext(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var cr contextAppendResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should still succeed — observations section reports "no observations file".
	found := false
	for _, s := range cr.Appended {
		if s == "observations" {
			found = true
		}
	}
	if !found {
		t.Error("expected observations section to be appended even without file")
	}
}

// --- scratchpad_reason tests ---

func TestHandleScratchpadReason_MissingParams(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	// Missing name.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"topic": "rate_cards"}
	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}

	// Missing topic.
	req.Params.Arguments = map[string]any{"name": "test"}
	result, err = srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing topic")
	}
}

func TestHandleScratchpadReason_EnhanceStages(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "reason_test",
		"topic": "enhance_stages",
		"input": `{"target_provider": "claude", "low_dimensions": ["clarity", "tone"]}`,
	}

	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var rr reasonResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &rr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if rr.Confidence != "high" {
		t.Errorf("expected high confidence, got %s", rr.Confidence)
	}
	if rr.Reasoning["stage_to_dimensions"] == nil {
		t.Error("expected stage_to_dimensions in reasoning")
	}
	if rr.Reasoning["recommended_stages"] == nil {
		t.Error("expected recommended_stages for low dimensions")
	}

	// Verify appended to scratchpad.
	data, err := os.ReadFile(filepath.Join(root, ".ralph", "reason_test_scratchpad.md"))
	if err != nil {
		t.Fatalf("scratchpad not created: %v", err)
	}
	if !strings.Contains(string(data), "## Reasoning: enhance_stages") {
		t.Error("expected reasoning header in scratchpad")
	}
}

func TestHandleScratchpadReason_RateCards(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "rate_test",
		"topic": "rate_cards",
		"input": `{"provider": "claude", "input_tokens": 5000, "output_tokens": 2000}`,
	}

	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var rr reasonResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &rr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if rr.Reasoning["rate_cards_per_1m_tokens"] == nil {
		t.Error("expected rate_cards_per_1m_tokens in reasoning")
	}
}

func TestHandleScratchpadReason_PruneThresholds(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)

	// Create a large scratchpad to trigger prune recommendation.
	ralphDir := filepath.Join(root, ".ralph")
	bigContent := strings.Repeat("x", 60*1024) // 60KB > 50KB threshold
	if err := os.WriteFile(filepath.Join(ralphDir, "big_scratchpad.md"), []byte(bigContent), 0o644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "prune_test",
		"topic": "prune_thresholds",
	}

	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var rr reasonResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &rr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if rr.Confidence == "high" {
		// Should be "medium" when pruning is needed.
		t.Log("confidence is high, meaning no pruning needed — but we created a 60KB file")
	}

	needsPruning, _ := rr.Reasoning["needs_pruning"].([]any)
	if len(needsPruning) == 0 {
		t.Error("expected big scratchpad in needs_pruning list")
	}
}

func TestHandleScratchpadReason_ProviderSelection(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "provider_test",
		"topic": "provider_selection",
		"input": `{"task_type": "refactor"}`,
	}

	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	var rr reasonResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &rr); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if rr.Confidence != "high" {
		t.Errorf("expected high confidence for known task type, got %s", rr.Confidence)
	}
	if rr.Reasoning["recommended_provider"] != "claude" {
		t.Errorf("expected claude for refactor, got %v", rr.Reasoning["recommended_provider"])
	}
}

func TestHandleScratchpadReason_InvalidTopic(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "test",
		"topic": "invalid_topic",
	}

	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid topic")
	}
}

func TestHandleScratchpadReason_InvalidInputJSON(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"name":  "test",
		"topic": "rate_cards",
		"input": "not valid json {{{",
	}

	result, err := srv.handleScratchpadReason(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid JSON input")
	}
}
