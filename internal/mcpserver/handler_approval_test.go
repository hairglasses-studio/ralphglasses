package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// approvalTestServer returns a minimal Server with a session manager suitable
// for approval handler tests.
func approvalTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		ScanPath: t.TempDir(),
		SessMgr:  session.NewManager(),
	}
}

func parseJSON(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	text := result.Content[0].(mcp.TextContent).Text
	var m map[string]any
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("failed to parse JSON result: %v\ntext: %s", err, text)
	}
	return m
}

func TestHandleRequestApproval_Basic(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"action":  "merge PR #42",
		"context": "all CI checks passed",
		"urgency": "high",
	}

	result, err := srv.handleRequestApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	m := parseJSON(t, result)
	if m["approval_id"] == "" {
		t.Error("expected non-empty approval_id")
	}
	if m["status"] != "pending" {
		t.Errorf("status = %v, want pending", m["status"])
	}
	if m["action"] != "merge PR #42" {
		t.Errorf("action = %v, want 'merge PR #42'", m["action"])
	}
	if m["urgency"] != "high" {
		t.Errorf("urgency = %v, want high", m["urgency"])
	}
}

func TestHandleRequestApproval_MissingAction(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"context": "reason",
		"urgency": "low",
	}

	result, err := srv.handleRequestApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing action")
	}
}

func TestHandleRequestApproval_InvalidUrgency(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"action":  "deploy",
		"context": "ready",
		"urgency": "extreme",
	}

	result, err := srv.handleRequestApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid urgency")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS code, got: %s", text)
	}
}

func TestHandleRequestApproval_WithSession(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	// Create a mock session in the manager.
	sess := &session.Session{
		ID:     "test-sess-1",
		Status: session.StatusRunning,
	}
	srv.SessMgr.AddSessionForTesting(sess)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"action":     "deploy to prod",
		"context":    "release v2.0",
		"urgency":    "critical",
		"session_id": "test-sess-1",
	}

	result, err := srv.handleRequestApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	m := parseJSON(t, result)
	if m["session_paused"] != true {
		t.Error("expected session_paused=true")
	}

	// Verify the session was actually paused.
	got, ok := srv.SessMgr.Get("test-sess-1")
	if !ok {
		t.Fatal("session not found")
	}
	got.Lock()
	status := got.Status
	got.Unlock()
	if status != "paused" {
		t.Errorf("session status = %q, want paused", status)
	}
}

func TestHandleRequestApproval_WithNonexistentSession(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"action":     "deploy",
		"context":    "ready",
		"urgency":    "normal",
		"session_id": "nonexistent",
	}

	result, err := srv.handleRequestApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("request_approval should succeed even with nonexistent session")
	}

	m := parseJSON(t, result)
	if m["session_paused"] != false {
		t.Errorf("session_paused = %v, want false (session doesn't exist)", m["session_paused"])
	}
}

func TestHandleResolveApproval_Approved(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	// First create an approval.
	createReq := mcp.CallToolRequest{}
	createReq.Params.Arguments = map[string]any{
		"action":  "merge PR",
		"context": "tests pass",
		"urgency": "normal",
	}
	createResult, _ := srv.handleRequestApproval(context.Background(), createReq)
	created := parseJSON(t, createResult)
	approvalID := created["approval_id"].(string)

	// Now resolve it.
	resolveReq := mcp.CallToolRequest{}
	resolveReq.Params.Arguments = map[string]any{
		"approval_id": approvalID,
		"decision":    "approved",
		"reason":      "ship it",
	}

	result, err := srv.handleResolveApproval(context.Background(), resolveReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	m := parseJSON(t, result)
	if m["decision"] != "approved" {
		t.Errorf("decision = %v, want approved", m["decision"])
	}
	if m["reason"] != "ship it" {
		t.Errorf("reason = %v, want 'ship it'", m["reason"])
	}
	if m["status"] != "approved" {
		t.Errorf("status = %v, want approved", m["status"])
	}
}

func TestHandleResolveApproval_Rejected(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	// Create approval.
	createReq := mcp.CallToolRequest{}
	createReq.Params.Arguments = map[string]any{
		"action":  "deploy",
		"context": "untested",
		"urgency": "high",
	}
	createResult, _ := srv.handleRequestApproval(context.Background(), createReq)
	created := parseJSON(t, createResult)
	approvalID := created["approval_id"].(string)

	// Reject.
	resolveReq := mcp.CallToolRequest{}
	resolveReq.Params.Arguments = map[string]any{
		"approval_id": approvalID,
		"decision":    "rejected",
		"reason":      "needs more testing",
	}

	result, err := srv.handleResolveApproval(context.Background(), resolveReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	m := parseJSON(t, result)
	if m["decision"] != "rejected" {
		t.Errorf("decision = %v, want rejected", m["decision"])
	}
}

func TestHandleResolveApproval_ResumesSession(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	// Create a session and pause it via approval.
	sess := &session.Session{
		ID:     "resume-sess",
		Status: session.StatusRunning,
	}
	srv.SessMgr.AddSessionForTesting(sess)

	createReq := mcp.CallToolRequest{}
	createReq.Params.Arguments = map[string]any{
		"action":     "merge",
		"context":    "ready",
		"urgency":    "normal",
		"session_id": "resume-sess",
	}
	createResult, _ := srv.handleRequestApproval(context.Background(), createReq)
	created := parseJSON(t, createResult)
	approvalID := created["approval_id"].(string)

	// Verify session is paused.
	got, _ := srv.SessMgr.Get("resume-sess")
	got.Lock()
	if got.Status != "paused" {
		t.Fatalf("session status = %q before resolve, want paused", got.Status)
	}
	got.Unlock()

	// Resolve approval.
	resolveReq := mcp.CallToolRequest{}
	resolveReq.Params.Arguments = map[string]any{
		"approval_id": approvalID,
		"decision":    "approved",
	}
	result, err := srv.handleResolveApproval(context.Background(), resolveReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := parseJSON(t, result)
	if m["session_resumed"] != true {
		t.Error("expected session_resumed=true")
	}

	// Verify session is running again.
	got, _ = srv.SessMgr.Get("resume-sess")
	got.Lock()
	status := got.Status
	got.Unlock()
	if status != "running" {
		t.Errorf("session status = %q after resolve, want running", status)
	}
}

func TestHandleResolveApproval_NotFound(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"approval_id": "nonexistent",
		"decision":    "approved",
	}

	result, err := srv.handleResolveApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent approval")
	}
}

func TestHandleResolveApproval_InvalidDecision(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"approval_id": "abc",
		"decision":    "maybe",
	}

	result, err := srv.handleResolveApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid decision")
	}
}

func TestHandleResolveApproval_MissingParams(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleResolveApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing params")
	}
}

func TestHandleListApprovals_Empty(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleListApprovals(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content[0].(mcp.TextContent).Text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"status":"empty"`) {
		t.Errorf("expected empty result, got: %s", text)
	}
}

func TestHandleListApprovals_PendingOnly(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	// Create two approvals, resolve one.
	store := srv.getApprovalStore()
	store.Create("action1", "ctx1", "low", "")
	rec2 := store.Create("action2", "ctx2", "high", "")
	_, _ = store.Resolve(rec2.ID, "approved", "")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleListApprovals(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var records []map[string]any
	if err := json.Unmarshal([]byte(text), &records); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(records) != 1 {
		t.Errorf("pending count = %d, want 1", len(records))
	}
}

func TestHandleListApprovals_IncludeResolved(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	store := srv.getApprovalStore()
	store.Create("action1", "ctx1", "low", "")
	rec2 := store.Create("action2", "ctx2", "high", "")
	_, _ = store.Resolve(rec2.ID, "approved", "")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"include_resolved": true,
	}

	result, err := srv.handleListApprovals(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var records []map[string]any
	if err := json.Unmarshal([]byte(text), &records); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("total count = %d, want 2", len(records))
	}
}

func TestBuildApprovalGroup(t *testing.T) {
	t.Parallel()
	srv := approvalTestServer(t)

	group := srv.buildApprovalGroup()
	if group.Name != "approval" {
		t.Errorf("group name = %q, want approval", group.Name)
	}
	if len(group.Tools) != 3 {
		t.Errorf("tool count = %d, want 3", len(group.Tools))
	}

	names := make(map[string]bool)
	for _, entry := range group.Tools {
		names[entry.Tool.Name] = true
	}
	expected := []string{
		"ralphglasses_request_approval",
		"ralphglasses_resolve_approval",
		"ralphglasses_list_approvals",
	}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing tool %q in approval group", name)
		}
	}
}
