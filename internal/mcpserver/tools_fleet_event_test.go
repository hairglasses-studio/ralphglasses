package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestHandleEventList_NilBus(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	srv.EventBus = nil

	result, err := srv.handleEventList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when EventBus is nil")
	}
	text := getResultText(result)
	if !contains(text, "NOT_RUNNING") {
		t.Errorf("expected NOT_RUNNING error code, got: %s", text)
	}
}

func TestHandleEventList_EmptyBus(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	result, err := srv.handleEventList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp struct {
		Events     []any `json:"events"`
		TotalCount int   `json:"total_count"`
		HasMore    bool  `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(resp.Events))
	}
	if resp.TotalCount != 0 {
		t.Errorf("expected total_count=0, got %d", resp.TotalCount)
	}
}

func TestHandleEventList_WithEvents(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	bus.Publish(events.Event{
		Type:      events.SessionStarted,
		Timestamp: time.Now(),
		RepoName:  "repo-a",
		SessionID: "sess-1",
	})
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		Timestamp: time.Now(),
		RepoName:  "repo-b",
		SessionID: "sess-2",
	})

	result, err := srv.handleEventList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp struct {
		Events     []json.RawMessage `json:"events"`
		TotalCount int               `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(resp.Events))
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected total_count=2, got %d", resp.TotalCount)
	}
}

func TestHandleEventList_TypeFilter(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now()})
	bus.Publish(events.Event{Type: events.CostUpdate, Timestamp: time.Now()})
	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now()})

	result, err := srv.handleEventList(context.Background(), makeRequest(map[string]any{
		"type": "session.started",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp struct {
		Events     []json.RawMessage `json:"events"`
		TotalCount int               `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected 2 filtered events, got %d", resp.TotalCount)
	}
}

func TestHandleEventList_MultiTypeFilter(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now()})
	bus.Publish(events.Event{Type: events.CostUpdate, Timestamp: time.Now()})
	bus.Publish(events.Event{Type: events.LoopStarted, Timestamp: time.Now()})

	result, err := srv.handleEventList(context.Background(), makeRequest(map[string]any{
		"types": "session.started,cost.update",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := getResultText(result)
	var resp struct {
		TotalCount int `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("expected 2 events for multi-type filter, got %d", resp.TotalCount)
	}
}

func TestHandleEventList_SinceUntil(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Hour)
	t2 := t0.Add(2 * time.Hour)

	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: t0})
	bus.Publish(events.Event{Type: events.CostUpdate, Timestamp: t1})
	bus.Publish(events.Event{Type: events.LoopStarted, Timestamp: t2})

	// Query events since t0+30m until t2 (should only get the t1 event).
	since := t0.Add(30 * time.Minute).Format(time.RFC3339)
	until := t2.Format(time.RFC3339)

	result, err := srv.handleEventList(context.Background(), makeRequest(map[string]any{
		"since": since,
		"until": until,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp struct {
		TotalCount int `json:"total_count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Errorf("expected 1 event in time range, got %d", resp.TotalCount)
	}
}

func TestHandleEventList_InvalidSince(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	result, err := srv.handleEventList(context.Background(), makeRequest(map[string]any{
		"since": "not-a-date",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid since timestamp")
	}
}

func TestHandleEventPoll_NilBus_ErrorCode(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	srv.EventBus = nil

	result, err := srv.handleEventPoll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when EventBus is nil")
	}
	text := getResultText(result)
	if !contains(text, "NOT_RUNNING") {
		t.Errorf("expected NOT_RUNNING in error, got: %s", text)
	}
}

func TestHandleEventPoll_EmptyBus(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp struct {
		Events []any  `json:"events"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(resp.Events))
	}
}

func TestHandleEventPoll_WithCursor(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now(), RepoName: "r1"})
	bus.Publish(events.Event{Type: events.CostUpdate, Timestamp: time.Now(), RepoName: "r2"})

	// First poll from cursor 0.
	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "0",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := getResultText(result)
	var resp struct {
		Events []json.RawMessage `json:"events"`
		Cursor string            `json:"cursor"`
		Count  int               `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(resp.Events))
	}
	if resp.Cursor == "0" {
		t.Error("cursor should have advanced")
	}

	// Second poll from new cursor should return no events.
	result2, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": resp.Cursor,
	}))
	if err != nil {
		t.Fatal(err)
	}
	text2 := getResultText(result2)
	var resp2 struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal([]byte(text2), &resp2); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp2.Events) != 0 {
		t.Errorf("expected 0 events on second poll, got %d", len(resp2.Events))
	}
}

func TestHandleEventPoll_TypeFilter(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now()})
	bus.Publish(events.Event{Type: events.CostUpdate, Timestamp: time.Now()})
	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now()})

	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "0",
		"type":   "session.started",
	}))
	if err != nil {
		t.Fatal(err)
	}

	text := getResultText(result)
	var resp struct {
		Events []json.RawMessage `json:"events"`
		Count  int               `json:"count"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Errorf("expected 2 session.started events, got %d", len(resp.Events))
	}
}

func TestHandleEventPoll_InvalidCursor(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "not-a-number",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid cursor")
	}
}

func TestHandleEventPoll_DefaultCursor(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	bus.Publish(events.Event{Type: events.SessionStarted, Timestamp: time.Now()})

	// No cursor param should default to 0.
	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp struct {
		Events []json.RawMessage `json:"events"`
	}
	_ = json.Unmarshal([]byte(text), &resp)
	if len(resp.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(resp.Events))
	}
}

func TestHandleFleetAnalytics_EmptySessions(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus("/tmp/test", bus)

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if total, ok := resp["total_sessions"].(float64); !ok || total != 0 {
		t.Errorf("expected total_sessions=0, got %v", resp["total_sessions"])
	}
}

// testCallToolRequest builds a CallToolRequest with given args (for readability).
func testCallToolRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}
