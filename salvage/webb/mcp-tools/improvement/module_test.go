package improvement

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleRegistration(t *testing.T) {
	m := &Module{}
	assert.Equal(t, "improvement", m.Name())
	assert.NotEmpty(t, m.Description())
	assert.NotEmpty(t, m.Tools())
}

func TestToolDefinitions(t *testing.T) {
	m := &Module{}
	tools := m.Tools()

	// Check expected tools exist
	expectedTools := []string{
		"webb_self_improve",
		"webb_improvement_analyze",
		"webb_improvement_suggest",
		"webb_improvement_scaffold",
		"webb_improvement_noise_report",
		"webb_improvement_track",
		"webb_usage_aggregate",
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Tool.Name] = true
	}

	for _, expected := range expectedTools {
		assert.True(t, toolNames[expected], "Expected tool %s not found", expected)
	}
}

func TestHandleSelfImprove(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"mode": "quick",
		"save": false,
	}

	result, err := handleSelfImprove(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should contain summary
	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "Self-Improvement")
		}
	}
}

func TestHandleAnalyze(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"focus":  "all",
		"output": "markdown",
	}

	result, err := handleAnalyze(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestHandleSuggest(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"min_frequency": 5,
		"min_savings":   10,
		"limit":         5,
	}

	result, err := handleSuggest(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "Consolidation Suggestions")
		}
	}
}

func TestHandleScaffold(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"source_tools":  "webb_k8s_pods, webb_k8s_events",
		"name":          "webb_k8s_combined",
		"include_tests": true,
	}

	result, err := handleScaffold(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "webb_k8s_combined")
			assert.Contains(t, tc.Text, "func handle")
		}
	}
}

func TestHandleScaffoldMissingParams(t *testing.T) {
	ctx := context.Background()

	// Missing source_tools
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"name": "webb_test",
	}

	result, err := handleScaffold(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Missing name
	req.Params.Arguments = map[string]interface{}{
		"source_tools": "webb_a, webb_b",
	}

	result, err = handleScaffold(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)
}

func TestHandleNoiseReport(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"threshold": 50,
	}

	result, err := handleNoiseReport(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "Noise Report")
		}
	}
}

func TestHandleTrack(t *testing.T) {
	ctx := context.Background()

	// Test history action
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"action": "history",
	}

	result, err := handleTrack(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "Available Snapshots")
		}
	}
}

func TestHandleUsageAggregate(t *testing.T) {
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"time_range": "7d",
		"group_by":   "tool",
	}

	result, err := handleUsageAggregate(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	for _, content := range result.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			assert.Contains(t, tc.Text, "Usage Aggregate")
		}
	}
}

func TestBestPracticesCompliance(t *testing.T) {
	m := &Module{}
	tools := m.Tools()

	for _, tool := range tools {
		// DESC-001: Description starts with action verb
		desc := tool.Tool.Description
		assert.NotEmpty(t, desc, "Tool %s has no description", tool.Tool.Name)

		// SCHEMA-001: Has category
		assert.NotEmpty(t, tool.Category, "Tool %s has no category", tool.Tool.Name)

		// SCHEMA-002: Has tags
		assert.NotEmpty(t, tool.Tags, "Tool %s has no tags", tool.Tool.Name)

		// SCHEMA-003: Has use cases
		assert.NotEmpty(t, tool.UseCases, "Tool %s has no use cases", tool.Tool.Name)
	}
}

func TestConsolidationSuggestionStruct(t *testing.T) {
	s := ConsolidationSuggestion{
		SourceTools:  []string{"webb_a", "webb_b"},
		ProposedName: "webb_combined",
		TokenSavings: 50,
		Priority:     10,
	}

	assert.Equal(t, "webb_combined", s.ProposedName)
	assert.Equal(t, 50, s.TokenSavings)
	assert.Len(t, s.SourceTools, 2)
}
