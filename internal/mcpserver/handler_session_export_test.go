package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func writeTestReplay(t *testing.T, repoPath, sessionID string) {
	t.Helper()
	dir := filepath.Join(repoPath, ".ralph", "replays")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sessionID+".jsonl")
	rec := session.NewRecorder(sessionID, path)
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	_ = rec.Record(session.ReplayEvent{Timestamp: t0, Type: session.ReplayInput, Data: "implement feature"})
	_ = rec.Record(session.ReplayEvent{Timestamp: t0.Add(2 * time.Second), Type: session.ReplayOutput, Data: "starting implementation"})
	_ = rec.Record(session.ReplayEvent{Timestamp: t0.Add(5 * time.Second), Type: session.ReplayTool, Data: "edit_file main.go"})
	_ = rec.Record(session.ReplayEvent{Timestamp: t0.Add(8 * time.Second), Type: session.ReplayStatus, Data: "completed"})
	_ = rec.Close()
}

func TestHandleSessionExport(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing session_id")
		}
		text := getResultText(result)
		if !strings.Contains(text, "session_id is required") {
			t.Errorf("unexpected error: %s", text)
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{
			"session_id": "test-sess",
			"format":     "xml",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid format")
		}
		text := getResultText(result)
		if !strings.Contains(text, "invalid format") {
			t.Errorf("unexpected error: %s", text)
		}
	})

	t.Run("replay not found", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{
			"session_id": "nonexistent-sess",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing replay")
		}
		text := getResultText(result)
		if !strings.Contains(text, "replay file not found") {
			t.Errorf("unexpected error: %s", text)
		}
	})

	t.Run("markdown export", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")
		writeTestReplay(t, repoPath, "export-md-sess")

		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{
			"session_id": "export-md-sess",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}

		text := getResultText(result)
		if !strings.Contains(text, "# Session Replay Export") {
			t.Error("missing markdown header")
		}
		if !strings.Contains(text, "implement feature") {
			t.Error("missing input data in markdown")
		}
		if !strings.Contains(text, "[TOOL]") {
			t.Error("missing tool marker in markdown")
		}
		if !strings.Contains(text, "edit_file main.go") {
			t.Error("missing tool data in markdown")
		}
	})

	t.Run("json export", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")
		writeTestReplay(t, repoPath, "export-json-sess")

		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{
			"session_id": "export-json-sess",
			"format":     "json",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}

		text := getResultText(result)
		var doc session.ExportJSONDocument
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("JSON parse: %v", err)
		}
		if doc.Metadata.TotalEvents != 4 {
			t.Errorf("total_events = %d, want 4", doc.Metadata.TotalEvents)
		}
		if doc.Version != "1.0" {
			t.Errorf("version = %q, want 1.0", doc.Version)
		}
	})

	t.Run("event type filter", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")
		writeTestReplay(t, repoPath, "export-filter-sess")

		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{
			"session_id":  "export-filter-sess",
			"format":      "json",
			"event_types": "tool,status",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}

		text := getResultText(result)
		var doc session.ExportJSONDocument
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("JSON parse: %v", err)
		}
		if doc.Metadata.TotalEvents != 2 {
			t.Errorf("total_events = %d, want 2 (tool + status)", doc.Metadata.TotalEvents)
		}
		for _, ev := range doc.Events {
			if ev.Type != session.ReplayTool && ev.Type != session.ReplayStatus {
				t.Errorf("unexpected event type: %s", ev.Type)
			}
		}
	})

	t.Run("time range filter", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")
		writeTestReplay(t, repoPath, "export-time-sess")

		// Events are at t0, t0+2s, t0+5s, t0+8s. Filter to t0+1s..t0+6s.
		result, err := srv.handleSessionExport(context.Background(), makeRequest(map[string]any{
			"session_id": "export-time-sess",
			"format":     "json",
			"after":      "2025-06-01T12:00:01Z",
			"before":     "2025-06-01T12:00:06Z",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected tool error: %s", getResultText(result))
		}

		text := getResultText(result)
		var doc session.ExportJSONDocument
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			t.Fatalf("JSON parse: %v", err)
		}
		if doc.Metadata.TotalEvents != 2 {
			t.Errorf("total_events = %d, want 2 (output + tool)", doc.Metadata.TotalEvents)
		}
	})
}
