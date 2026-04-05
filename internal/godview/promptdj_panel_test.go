package godview

import (
	"strings"
	"testing"
)

func TestRenderPromptDJPanel_Empty(t *testing.T) {
	out := RenderPromptDJPanel(nil, 60)
	if !strings.Contains(out, "No routing decisions") {
		t.Error("expected empty state message")
	}
}

func TestRenderPromptDJPanel_WithData(t *testing.T) {
	stats := &PromptDJStats{
		TotalDecisions: 42,
		SuccessRate:    0.85,
		AvgConfidence:  0.78,
		AvgScore:       72,
		TotalCostUSD:   3.50,
		EnhancedPct:    0.15,
		ByProvider:     map[string]int64{"claude": 30, "gemini": 12},
		ByTaskType:     map[string]int64{"code": 25, "analysis": 17},
		ByConfidence:   map[string]int64{"high": 20, "medium": 15, "low": 7},
		RecentDecisions: []RecentDecision{
			{DecisionID: "abc12345-test", Provider: "claude", TaskType: "code", Score: 85, Confidence: 0.92, Status: "succeeded"},
			{DecisionID: "def67890-test", Provider: "gemini", TaskType: "workflow", Score: 60, Confidence: 0.55, Status: "failed"},
		},
	}

	out := RenderPromptDJPanel(stats, 80)
	if !strings.Contains(out, "PROMPT DJ") {
		t.Error("expected panel title")
	}
	if !strings.Contains(out, "42") {
		t.Error("expected decision count")
	}
	if !strings.Contains(out, "claude") {
		t.Error("expected provider name")
	}
	if !strings.Contains(out, "abc12345") {
		t.Error("expected recent decision hash")
	}
}
