package session

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestRunSessionOutput_ParseError(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-parse-error",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	// Feed invalid JSON.
	input := "this is not json\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.StreamParseErrors != 1 {
		t.Fatalf("expected 1 parse error, got %d", s.StreamParseErrors)
	}
	if s.LastEventType != "parse_error" {
		t.Fatalf("expected last_event_type=parse_error, got %s", s.LastEventType)
	}
}

func TestRunSessionOutput_SystemEvent(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-system",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"system","session_id":"abc123","content":"initialized"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ProviderSessionID != "abc123" {
		t.Fatalf("expected session_id=abc123, got %s", s.ProviderSessionID)
	}
	if s.TotalOutputCount != 1 {
		t.Fatalf("expected 1 output, got %d", s.TotalOutputCount)
	}
}

func TestRunSessionOutput_AssistantEvent(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-assistant",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"assistant","content":"hello world"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.LastOutput != "hello world" {
		t.Fatalf("expected 'hello world', got %q", s.LastOutput)
	}
}

func TestRunSessionOutput_ResultEvent(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-result",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"result","result":"done","num_turns":5,"session_id":"sess-1","cost_usd":0.42}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.LastOutput != "done" {
		t.Fatalf("expected 'done', got %q", s.LastOutput)
	}
	if s.TurnCount != 5 {
		t.Fatalf("expected 5 turns, got %d", s.TurnCount)
	}
	if s.ProviderSessionID != "sess-1" {
		t.Fatalf("expected session_id=sess-1, got %s", s.ProviderSessionID)
	}
	if s.SpentUSD != 0.42 {
		t.Fatalf("expected cost 0.42, got %f", s.SpentUSD)
	}
}

func TestRunSessionOutput_ResultError(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-result-err",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"result","is_error":true,"error":"something broke","session_id":"bad-id"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Error == "" {
		t.Fatal("expected error to be captured")
	}
	// Session ID from error responses should NOT be persisted.
	if s.ProviderSessionID == "bad-id" {
		t.Fatal("should not persist session ID from error response")
	}
}

func TestRunSessionOutput_CostTracking_Gemini(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-gemini-cost",
		Provider: ProviderGemini,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	// Gemini cost should be additive.
	input := `{"type":"result","text":"ok","cost_usd":0.10}` + "\n" +
		`{"type":"result","text":"ok2","cost_usd":0.20}` + "\n"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.SpentUSD < 0.29 || s.SpentUSD > 0.31 {
		t.Fatalf("expected ~0.30 cumulative cost, got %f", s.SpentUSD)
	}
}

func TestRunSessionOutput_CostTracking_Claude(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-claude-cost",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	// Claude cost is cumulative (overwritten).
	input := `{"type":"result","result":"r1","cost_usd":0.10}` + "\n" +
		`{"type":"result","result":"r2","cost_usd":0.30}` + "\n"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.SpentUSD != 0.30 {
		t.Fatalf("expected 0.30 (cumulative), got %f", s.SpentUSD)
	}
}

func TestRunSessionOutput_WithLogFile(t *testing.T) {
	t.Parallel()

	logPath := t.TempDir() + "/test.log"
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}

	s := &Session{
		ID:       "test-logfile",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"assistant","content":"logged output"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), logFile)
	logFile.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "logged output") {
		t.Fatalf("expected log to contain 'logged output', got %q", string(data))
	}
}

func TestRunSessionOutput_ContextCancelled(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-ctx-cancel",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Use a reader that blocks forever.
	pr, _ := io.Pipe()
	defer pr.Close()

	done := make(chan struct{})
	go func() {
		runSessionOutput(ctx, s, pr, nil)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runSessionOutput did not return after context cancellation")
	}
}

func TestRunSessionOutput_EmptyLines(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-empty",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := "\n\n\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.TotalOutputCount != 0 {
		t.Fatalf("expected 0 output for empty lines, got %d", s.TotalOutputCount)
	}
}

func TestRunSessionOutput_BudgetAlerts(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	s := &Session{
		ID:        "test-budget-alerts",
		Provider:  ProviderClaude,
		Status:    StatusRunning,
		BudgetUSD: 1.0,
		OutputCh:  make(chan string, 100),
		doneCh:    make(chan struct{}),
		bus:       bus,
	}

	// Cost that triggers 90% threshold.
	input := `{"type":"result","result":"r1","cost_usd":0.95}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.budgetAlertsEmitted == nil {
		t.Fatal("expected budget alerts map to be initialized")
	}
	// Should have emitted 50%, 75%, and 90% alerts.
	if !s.budgetAlertsEmitted["50%"] {
		t.Fatal("expected 50% alert")
	}
	if !s.budgetAlertsEmitted["75%"] {
		t.Fatal("expected 75% alert")
	}
	if !s.budgetAlertsEmitted["90%"] {
		t.Fatal("expected 90% alert")
	}
}

func TestRunSessionOutput_BudgetExceeded(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	s := &Session{
		ID:        "test-budget-exceeded",
		Provider:  ProviderClaude,
		Status:    StatusRunning,
		BudgetUSD: 0.50,
		OutputCh:  make(chan string, 100),
		doneCh:    make(chan struct{}),
		bus:       bus,
	}

	input := `{"type":"result","result":"r1","cost_usd":0.60}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)
}

func TestRunSessionOutput_DefaultEventType(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-default-type",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	// An event with an unknown type.
	input := `{"type":"custom_event","content":"custom data"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.TotalOutputCount != 1 {
		t.Fatalf("expected 1 output, got %d", s.TotalOutputCount)
	}
}

func TestRunSessionOutput_DefaultEventTypeError(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-default-err",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"custom_event","is_error":true,"error":"bad"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Error != "bad" {
		t.Fatalf("expected error 'bad', got %q", s.Error)
	}
}

func TestRunSessionOutput_ReplayRecorder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	recorder := NewRecorder("test-replay", dir+"/replay.jsonl")

	s := &Session{
		ID:       "test-replay",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
		recorder: recorder,
	}

	input := `{"type":"system","content":"init"}` + "\n" +
		`{"type":"assistant","content":"reply"}` + "\n" +
		`{"type":"tool_use","content":"tool output"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)
	_ = recorder.Close()
}

func TestRecordReplayEvent_NilRecorder(t *testing.T) {
	t.Parallel()
	// Should not panic.
	recordReplayEvent(nil, ReplayOutput, "test")
}

func TestRecordReplayEvent_EmptyData(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rec := NewRecorder("test", dir+"/test.jsonl")
	// Should be a no-op.
	recordReplayEvent(rec, ReplayOutput, "")
	_ = rec.Close()
}

func TestAppendSessionOutput_HistoryCap(t *testing.T) {
	t.Parallel()
	s := &Session{OutputCh: make(chan string, 200)}

	for i := 0; i < 150; i++ {
		appendSessionOutput(s, "line", nil)
	}

	if len(s.OutputHistory) > 100 {
		t.Fatalf("expected max 100 history entries, got %d", len(s.OutputHistory))
	}
	if s.TotalOutputCount != 150 {
		t.Fatalf("expected total=150, got %d", s.TotalOutputCount)
	}
}

func TestAppendSessionOutput_ChannelFullOverflow(t *testing.T) {
	t.Parallel()
	s := &Session{OutputCh: make(chan string, 1)}

	// Fill the channel.
	s.OutputCh <- "blocked"

	// Should not block.
	appendSessionOutput(s, "overflow", nil)
	if s.TotalOutputCount != 1 {
		t.Fatalf("expected 1, got %d", s.TotalOutputCount)
	}
}

func TestAppendSessionOutput_WithLogFile(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	s := &Session{OutputCh: make(chan string, 10)}

	// Use a fake "file" via bytes.Buffer (not a real *os.File, but the function only uses Fprintln).
	// Actually, appendSessionOutput expects *os.File. We need a real file.
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	_ = buf // unused, using real file
	defer f.Close()

	appendSessionOutput(s, "logged", f)

	f.Seek(0, 0)
	data, _ := io.ReadAll(f)
	if !strings.Contains(string(data), "logged") {
		t.Fatalf("expected 'logged' in file, got %q", string(data))
	}
}

func TestIsExtraUsageExhausted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		sess   *Session
		want   bool
	}{
		{
			"not exhausted",
			&Session{OutputHistory: []string{"normal output"}},
			false,
		},
		{
			"in output history",
			&Session{OutputHistory: []string{"You are out of extra usage for this period"}},
			true,
		},
		{
			"in error field",
			&Session{Error: "Out of Extra Usage"},
			true,
		},
		{
			"in last output",
			&Session{LastOutput: "out of extra usage"},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExtraUsageExhausted(tt.sess); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestTruncateStr_Coverage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is longer than five", 5, " five"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateStr(tt.input, tt.max)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestLaunch_EmptyRepoPath(t *testing.T) {
	t.Parallel()
	_, err := launch(context.Background(), LaunchOptions{})
	if err != ErrRepoPathRequired {
		t.Fatalf("expected ErrRepoPathRequired, got %v", err)
	}
}

func TestLaunch_NonExistentRepo(t *testing.T) {
	t.Parallel()
	_, err := launch(context.Background(), LaunchOptions{RepoPath: "/nonexistent/path"})
	if err == nil {
		t.Fatal("expected error for non-existent repo")
	}
}

func TestLaunch_NotADirectory(t *testing.T) {
	t.Parallel()
	f, err := os.CreateTemp("", "not-a-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	_, err = launch(context.Background(), LaunchOptions{RepoPath: f.Name()})
	if err == nil {
		t.Fatal("expected error for file path")
	}
}

func TestLaunch_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := launch(context.Background(), LaunchOptions{RepoPath: dir, Prompt: "test"})
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestLaunch_NoPrompt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(dir+"/.git", 0755)
	_, err := launch(context.Background(), LaunchOptions{RepoPath: dir})
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestRunSessionOutput_CostSource(t *testing.T) {
	t.Parallel()

	s := &Session{
		ID:       "test-cost-source",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
	}

	input := `{"type":"result","result":"ok","cost_usd":0.50,"cost_source":"api_key"}` + "\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runSessionOutput(ctx, s, strings.NewReader(input), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.CostSource != "structured" {
		t.Fatalf("expected cost_source=structured, got %s", s.CostSource)
	}
	if len(s.CostHistory) != 1 {
		t.Fatalf("expected 1 cost history entry, got %d", len(s.CostHistory))
	}
}
