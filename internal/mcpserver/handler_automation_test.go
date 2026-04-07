package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHandleAutomationPolicy_SetDisabledWithoutResetConfig(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleAutomationPolicy(context.Background(), makeRequest(map[string]any{
		"repo":    "test-repo",
		"action":  "set",
		"enabled": false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	policy, ok := data["policy"].(map[string]any)
	if !ok {
		t.Fatalf("policy payload missing: %#v", data["policy"])
	}
	if policy["enabled"] != false {
		t.Fatalf("policy.enabled = %v, want false", policy["enabled"])
	}
}

func TestHandleAutomationPolicy_SetRejectsConcurrentSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleAutomationPolicy(context.Background(), makeRequest(map[string]any{
		"repo":                    "test-repo",
		"action":                  "set",
		"enabled":                 true,
		"timezone":                "UTC",
		"reset_anchor":            "2026-04-06T00:00:00Z",
		"reset_window_hours":      24,
		"max_concurrent_sessions": 2,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected invalid params error")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
	if !strings.Contains(text, "max_concurrent_sessions") {
		t.Fatalf("expected max_concurrent_sessions validation, got: %s", text)
	}
}

func TestHandleAutomationQueue_CRUD(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	enqueue, err := srv.handleAutomationQueue(context.Background(), makeRequest(map[string]any{
		"repo":       "test-repo",
		"action":     "enqueue",
		"prompt":     "run the overnight automation cycle",
		"priority":   9,
		"budget_usd": 4.5,
		"source":     "manual",
	}))
	if err != nil {
		t.Fatalf("enqueue error: %v", err)
	}
	if enqueue.IsError {
		t.Fatalf("enqueue returned error: %s", getResultText(enqueue))
	}

	var enqueueData map[string]any
	if err := json.Unmarshal([]byte(getResultText(enqueue)), &enqueueData); err != nil {
		t.Fatalf("unmarshal enqueue: %v", err)
	}
	item, ok := enqueueData["item"].(map[string]any)
	if !ok {
		t.Fatalf("enqueue item missing: %#v", enqueueData["item"])
	}
	itemID, _ := item["id"].(string)
	if itemID == "" {
		t.Fatal("expected queue item id")
	}
	if item["prompt"] != "run the overnight automation cycle" {
		t.Fatalf("prompt = %v", item["prompt"])
	}

	list, err := srv.handleAutomationQueue(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if list.IsError {
		t.Fatalf("list returned error: %s", getResultText(list))
	}
	var listData map[string]any
	if err := json.Unmarshal([]byte(getResultText(list)), &listData); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if int(listData["count"].(float64)) != 1 {
		t.Fatalf("count = %v, want 1", listData["count"])
	}

	reprioritize, err := srv.handleAutomationQueue(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"action":   "reprioritize",
		"id":       itemID,
		"priority": 3,
	}))
	if err != nil {
		t.Fatalf("reprioritize error: %v", err)
	}
	if reprioritize.IsError {
		t.Fatalf("reprioritize returned error: %s", getResultText(reprioritize))
	}
	var reprioritizeData map[string]any
	if err := json.Unmarshal([]byte(getResultText(reprioritize)), &reprioritizeData); err != nil {
		t.Fatalf("unmarshal reprioritize: %v", err)
	}
	reprioritizedItem, ok := reprioritizeData["item"].(map[string]any)
	if !ok {
		t.Fatalf("reprioritized item missing: %#v", reprioritizeData["item"])
	}
	if int(reprioritizedItem["priority"].(float64)) != 3 {
		t.Fatalf("priority = %v, want 3", reprioritizedItem["priority"])
	}

	remove, err := srv.handleAutomationQueue(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "remove",
		"id":     itemID,
	}))
	if err != nil {
		t.Fatalf("remove error: %v", err)
	}
	if remove.IsError {
		t.Fatalf("remove returned error: %s", getResultText(remove))
	}
	var removeData map[string]any
	if err := json.Unmarshal([]byte(getResultText(remove)), &removeData); err != nil {
		t.Fatalf("unmarshal remove: %v", err)
	}
	if removeData["removed_id"] != itemID {
		t.Fatalf("removed_id = %v, want %s", removeData["removed_id"], itemID)
	}
}
