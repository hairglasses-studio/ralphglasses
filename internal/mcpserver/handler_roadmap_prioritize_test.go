package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleRoadmapPrioritize_PrioritizesReadySmallHighImpactWork(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	roadmapContent := "# Test Repo Roadmap\n\n" +
		"## Phase Alpha\n\n" +
		"### Ready Slice\n" +
		"- [ ] 1.1.1 — P1 `S` Fix planner panic\n" +
		"- [ ] 1.1.2 — P2 `L` Rewrite the whole prompt pipeline\n\n" +
		"### Blocked Slice [BLOCKED BY 9.9]\n" +
		"- [ ] 1.2.1 — P3 `L` Blocked rollout fix\n"
	if err := os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte(roadmapContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleRoadmapPrioritize(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"top_n": float64(2),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}

	payload := decodeRoadmapPrioritizePayload(t, result)
	if payload["total_remaining"] != float64(3) {
		t.Fatalf("total_remaining = %v, want 3", payload["total_remaining"])
	}

	items := decodeRoadmapPrioritizeItems(t, payload["prioritized_items"])
	if len(items) != 2 {
		t.Fatalf("prioritized_items len = %d, want 2", len(items))
	}
	if items[0]["task_id"] != "1.1.1" {
		t.Fatalf("first task_id = %v, want 1.1.1", items[0]["task_id"])
	}
	if items[1]["task_id"] != "1.1.2" {
		t.Fatalf("second task_id = %v, want 1.1.2", items[1]["task_id"])
	}
	if blocked, _ := items[1]["blocked"].(bool); blocked {
		t.Fatalf("expected second item to be ready, got blocked=true")
	}

	sprint := decodeRoadmapPrioritizeItems(t, payload["recommended_next_sprint"])
	if len(sprint) != 2 {
		t.Fatalf("recommended_next_sprint len = %d, want 2", len(sprint))
	}
}

func TestHandleRoadmapPrioritize_RespectsPhaseFilterAndWeights(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	roadmapContent := "# Test Repo Roadmap\n\n" +
		"## Phase Alpha\n\n" +
		"### Alpha Work\n" +
		"- [ ] 1.1.1 — P1 `L` Large alpha rewrite\n" +
		"- [ ] 1.1.2 — P2 `S` Small alpha polish\n\n" +
		"## Phase Beta\n\n" +
		"### Beta Work\n" +
		"- [ ] 2.1.1 — P1 `S` Beta launch blocker\n"
	if err := os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte(roadmapContent), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleRoadmapPrioritize(context.Background(), makeRequest(map[string]any{
		"repo":         "test-repo",
		"phase_filter": "alpha",
		"weights":      `{"impact":0.1,"effort":0.8,"dependency":0.1}`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}

	payload := decodeRoadmapPrioritizePayload(t, result)
	items := decodeRoadmapPrioritizeItems(t, payload["prioritized_items"])
	if len(items) != 2 {
		t.Fatalf("prioritized_items len = %d, want 2", len(items))
	}
	if items[0]["task_id"] != "1.1.2" {
		t.Fatalf("first filtered task_id = %v, want 1.1.2", items[0]["task_id"])
	}
	for _, item := range items {
		if item["phase"] != "Phase Alpha" {
			t.Fatalf("phase = %v, want only Phase Alpha items", item["phase"])
		}
	}
}

func decodeRoadmapPrioritizePayload(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal roadmap prioritize payload: %v", err)
	}
	return payload
}

func decodeRoadmapPrioritizeItems(t *testing.T, raw any) []map[string]any {
	t.Helper()

	rows, ok := raw.([]any)
	if !ok {
		t.Fatalf("items payload is %T, want []any", raw)
	}

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item, ok := row.(map[string]any)
		if !ok {
			t.Fatalf("item row is %T, want map[string]any", row)
		}
		items = append(items, item)
	}
	return items
}
