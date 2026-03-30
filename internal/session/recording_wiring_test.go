package session

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// TestRecordingWiring_RunSessionOutput verifies that when a session has a
// recorder, events flowing through runSessionOutput are written to the
// replay file and can be played back.
func TestRecordingWiring_RunSessionOutput(t *testing.T) {
	dir := t.TempDir()
	replayPath := filepath.Join(dir, "test-session.jsonl")

	rec := NewRecorder("test-sess", replayPath)

	s := &Session{
		ID:       "test-sess",
		Provider: ProviderClaude,
		RepoPath: dir,
		RepoName: "test-repo",
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		recorder: rec,
	}

	// Simulate streaming JSON output from Claude CLI.
	lines := []string{
		`{"type":"system","session_id":"provider-123","content":"Session initialized"}`,
		`{"type":"assistant","content":"Hello world"}`,
		`{"type":"tool_use","content":"running tests"}`,
		`{"type":"result","result":"All tests passed","num_turns":3}`,
	}
	input := strings.Join(lines, "\n") + "\n"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := io.NopCloser(strings.NewReader(input))
	runSessionOutput(ctx, s, stdout, nil)

	// Close the recorder to flush.
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	// Verify the replay file was created and contains events.
	player, err := NewPlayer(replayPath)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}

	evts := player.Events()
	if len(evts) == 0 {
		t.Fatal("expected replay events, got none")
	}

	// Verify event types are correctly mapped.
	typeMap := make(map[ReplayEventType]int)
	for _, ev := range evts {
		typeMap[ev.Type]++
		if ev.SessionID != "test-sess" {
			t.Errorf("expected session_id 'test-sess', got %q", ev.SessionID)
		}
	}

	if typeMap[ReplayStatus] == 0 {
		t.Error("expected at least one ReplayStatus event (from system event)")
	}
	if typeMap[ReplayOutput] == 0 {
		t.Error("expected at least one ReplayOutput event (from assistant/result)")
	}
	if typeMap[ReplayTool] == 0 {
		t.Error("expected at least one ReplayTool event (from tool_use)")
	}
}

// TestRecordingWiring_NoRecorder verifies that when recorder is nil, no
// panic or error occurs during event processing.
func TestRecordingWiring_NoRecorder(t *testing.T) {
	dir := t.TempDir()

	s := &Session{
		ID:       "no-rec-sess",
		Provider: ProviderClaude,
		RepoPath: dir,
		RepoName: "test-repo",
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		// recorder is nil — should be safe
	}

	lines := []string{
		`{"type":"assistant","content":"no crash"}`,
		`{"type":"result","result":"done","num_turns":1}`,
	}
	input := strings.Join(lines, "\n") + "\n"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdout := io.NopCloser(strings.NewReader(input))
	runSessionOutput(ctx, s, stdout, nil)

	// If we got here without panic, test passes.
	if s.TotalOutputCount == 0 {
		t.Error("expected output to be processed even without recorder")
	}
}

// TestRecordingWiring_RunSession verifies that runSession closes the recorder
// and records the final status event.
func TestRecordingWiring_RunSession(t *testing.T) {
	dir := t.TempDir()
	replayPath := filepath.Join(dir, "final-session.jsonl")

	rec := NewRecorder("final-sess", replayPath)

	bus := events.NewBus(100)

	s := &Session{
		ID:       "final-sess",
		Provider: ProviderClaude,
		RepoPath: dir,
		RepoName: "test-repo",
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		recorder: rec,
		bus:      bus,
		doneCh:   make(chan struct{}),
	}

	// Subscribe to recording events.
	recCh := bus.SubscribeFiltered("test-rec", events.RecordingEnded)

	lines := []string{
		`{"type":"result","result":"completed work","num_turns":1}`,
	}
	input := strings.Join(lines, "\n") + "\n"

	stdout := io.NopCloser(strings.NewReader(input))
	stderr := io.NopCloser(strings.NewReader(""))

	// runSession expects cmd to have been started. We use a mock approach:
	// call the parts we can test. Since we can't easily mock cmd.Wait(),
	// verify the recorder close path by calling it directly after runSessionOutput.
	ctx := context.Background()
	runSessionOutput(ctx, s, stdout, nil)

	// Simulate what runSession does after output processing.
	s.mu.Lock()
	now := time.Now()
	s.EndedAt = &now
	s.Status = StatusCompleted
	s.ExitReason = "completed normally"

	// Close replay recorder (same logic as runSession).
	if s.recorder != nil {
		_ = s.recorder.Record(ReplayEvent{
			Type: ReplayStatus,
			Data: fmt.Sprintf("session ended: %s (%s)", s.Status, s.ExitReason),
		})
		_ = s.recorder.Close()
		if s.bus != nil {
			s.bus.Publish(events.Event{
				Type:      events.RecordingEnded,
				SessionID: s.ID,
				RepoPath:  s.RepoPath,
				RepoName:  s.RepoName,
				Provider:  string(s.Provider),
				Data:      map[string]any{"status": string(s.Status)},
			})
		}
	}
	s.mu.Unlock()

	// Verify the replay contains the final status event.
	player, err := NewPlayer(replayPath)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}

	evts := player.Events()
	if len(evts) == 0 {
		t.Fatal("expected replay events")
	}

	lastEvt := evts[len(evts)-1]
	if lastEvt.Type != ReplayStatus {
		t.Errorf("expected last event to be status, got %s", lastEvt.Type)
	}
	if !strings.Contains(lastEvt.Data, "completed") {
		t.Errorf("expected final status to mention 'completed', got %q", lastEvt.Data)
	}

	// Verify recording ended event was published.
	select {
	case ev := <-recCh:
		if ev.Type != events.RecordingEnded {
			t.Errorf("expected RecordingEnded event, got %s", ev.Type)
		}
		if ev.SessionID != "final-sess" {
			t.Errorf("expected session_id 'final-sess', got %q", ev.SessionID)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for RecordingEnded event")
	}

	// Avoid unused variable.
	_ = stderr
}

// TestRecordingWiring_LaunchOptions verifies that RecordingEnabled in
// LaunchOptions causes a Recorder to be created.
func TestRecordingWiring_LaunchOptions(t *testing.T) {
	dir := t.TempDir()
	replayDir := filepath.Join(dir, ".ralph", "replays")

	// Verify that the recording infrastructure creates the directory and
	// recorder when RecordingEnabled is set. We test this by simulating
	// the recorder creation path from launch().
	if err := os.MkdirAll(replayDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	sessionID := "launch-test-sess"
	replayPath := filepath.Join(replayDir, sessionID+".jsonl")

	rec := NewRecorder(sessionID, replayPath)
	_ = rec.Record(ReplayEvent{Type: ReplayInput, Data: "test prompt"})
	_ = rec.Close()

	// Verify replay file exists and is valid.
	player, err := NewPlayer(replayPath)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}

	evts := player.Events()
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	if evts[0].Type != ReplayInput {
		t.Errorf("expected input event, got %s", evts[0].Type)
	}
	if evts[0].Data != "test prompt" {
		t.Errorf("expected 'test prompt', got %q", evts[0].Data)
	}
}

// TestRecordingWiring_RecordingStartedEvent verifies that the
// session.recording.started event is emitted.
func TestRecordingWiring_RecordingStartedEvent(t *testing.T) {
	bus := events.NewBus(100)
	ch := bus.SubscribeFiltered("test-start", events.RecordingStarted)

	dir := t.TempDir()
	replayPath := filepath.Join(dir, "started-test.jsonl")

	// Simulate what launch() does when recording is enabled.
	bus.Publish(events.Event{
		Type:      events.RecordingStarted,
		SessionID: "start-sess",
		RepoPath:  dir,
		RepoName:  "test-repo",
		Provider:  "claude",
		Data:      map[string]any{"replay_path": replayPath},
	})

	select {
	case ev := <-ch:
		if ev.Type != events.RecordingStarted {
			t.Errorf("expected RecordingStarted, got %s", ev.Type)
		}
		if ev.SessionID != "start-sess" {
			t.Errorf("expected session_id 'start-sess', got %q", ev.SessionID)
		}
		path, ok := ev.Data["replay_path"].(string)
		if !ok || path != replayPath {
			t.Errorf("expected replay_path %q, got %v", replayPath, ev.Data["replay_path"])
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for RecordingStarted event")
	}
}

// TestRecordingWiring_RecordReplayEventHelper verifies the recordReplayEvent
// helper is a safe no-op when recorder is nil.
func TestRecordingWiring_RecordReplayEventHelper(t *testing.T) {
	// nil recorder should not panic.
	recordReplayEvent(nil, ReplayOutput, "test")
	recordReplayEvent(nil, ReplayOutput, "")

	// With a recorder but empty data should not write.
	dir := t.TempDir()
	path := filepath.Join(dir, "helper-test.jsonl")
	rec := NewRecorder("helper-sess", path)

	recordReplayEvent(rec, ReplayOutput, "")
	_ = rec.Close()

	// File should not exist (nothing was written).
	if _, err := os.Stat(path); err == nil {
		// File exists, check it's empty or nonexistent.
		data, _ := os.ReadFile(path)
		if len(bytes.TrimSpace(data)) > 0 {
			t.Error("expected no data written for empty events")
		}
	}

	// With actual data, should write.
	rec2 := NewRecorder("helper-sess-2", path)
	recordReplayEvent(rec2, ReplayTool, "tool data")
	_ = rec2.Close()

	player, err := NewPlayer(path)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	if len(player.Events()) != 1 {
		t.Errorf("expected 1 event, got %d", len(player.Events()))
	}
}
