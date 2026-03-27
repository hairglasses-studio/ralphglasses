package session

import (
	"context"
	"strings"
	"testing"
)

func TestBuildCmd(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		opts     LaunchOptions
		wantArgs []string
	}{
		{
			name: "basic",
			opts: LaunchOptions{
				RepoPath: "/tmp/repo",
				Prompt:   "hello",
			},
			wantArgs: []string{"-p", "--output-format", "stream-json"},
		},
		{
			name: "with model and budget",
			opts: LaunchOptions{
				RepoPath:     "/tmp/repo",
				Prompt:       "hello",
				Model:        "opus",
				MaxBudgetUSD: 5.0,
			},
			wantArgs: []string{"--model", "opus", "--max-budget-usd", "5.00"},
		},
		{
			name: "with agent and max turns",
			opts: LaunchOptions{
				RepoPath: "/tmp/repo",
				Prompt:   "hello",
				Agent:    "reviewer",
				MaxTurns: 10,
			},
			wantArgs: []string{"--agent", "reviewer", "--max-turns", "10"},
		},
		{
			name: "with resume",
			opts: LaunchOptions{
				RepoPath: "/tmp/repo",
				Resume:   "abc-123",
			},
			wantArgs: []string{"--resume", "abc-123"},
		},
		{
			name: "with continue",
			opts: LaunchOptions{
				RepoPath: "/tmp/repo",
				Continue: true,
			},
			wantArgs: []string{"--continue"},
		},
		{
			name: "with worktree auto",
			opts: LaunchOptions{
				RepoPath: "/tmp/repo",
				Prompt:   "hello",
				Worktree: "true",
			},
			wantArgs: []string{"-w"},
		},
		{
			name: "with worktree branch",
			opts: LaunchOptions{
				RepoPath: "/tmp/repo",
				Prompt:   "hello",
				Worktree: "feature-branch",
			},
			wantArgs: []string{"-w", "feature-branch"},
		},
		{
			name: "with allowed tools",
			opts: LaunchOptions{
				RepoPath:     "/tmp/repo",
				Prompt:       "hello",
				AllowedTools: []string{"Bash", "Read", "Edit"},
			},
			wantArgs: []string{"--allowedTools", "Bash,Read,Edit"},
		},
		{
			name: "with system prompt",
			opts: LaunchOptions{
				RepoPath:     "/tmp/repo",
				Prompt:       "hello",
				SystemPrompt: "Be concise",
			},
			wantArgs: []string{"--append-system-prompt", "Be concise"},
		},
		{
			name: "with session name (internal only, no CLI flag)",
			opts: LaunchOptions{
				RepoPath:    "/tmp/repo",
				Prompt:      "hello",
				SessionName: "my-session",
			},
			wantArgs: []string{}, // SessionName is tracked internally; Claude CLI has no --name flag
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildCmd(ctx, tt.opts)
			cmdStr := strings.Join(cmd.Args, " ")

			for _, want := range tt.wantArgs {
				if !strings.Contains(cmdStr, want) {
					t.Errorf("command %q missing expected arg %q", cmdStr, want)
				}
			}

			if cmd.Dir != tt.opts.RepoPath {
				t.Errorf("cmd.Dir = %q, want %q", cmd.Dir, tt.opts.RepoPath)
			}
		})
	}
}

func TestBuildCmdResumeTakesPrecedenceOverContinue(t *testing.T) {
	ctx := context.Background()
	cmd := buildCmd(ctx, LaunchOptions{
		RepoPath: "/tmp/repo",
		Resume:   "abc-123",
		Continue: true, // should be ignored when Resume is set
	})

	cmdStr := strings.Join(cmd.Args, " ")
	if !strings.Contains(cmdStr, "--resume abc-123") {
		t.Error("expected --resume flag")
	}
	if strings.Contains(cmdStr, "--continue") {
		t.Error("--continue should not be present when --resume is set")
	}
}

func TestTruncateStr(t *testing.T) {
	s := "hello world"
	if got := truncateStr(s, 100); got != s {
		t.Errorf("truncateStr(%q, 100) = %q", s, got)
	}
	if got := truncateStr(s, 5); got != "world" {
		t.Errorf("truncateStr(%q, 5) = %q, want 'world'", s, got)
	}
}

func TestRunSessionStreamParsing(t *testing.T) {
	// Simulate streaming JSON output
	streamData := `{"type":"system","session_id":"sess-abc"}
{"type":"assistant","content":"Working on it..."}
{"type":"result","result":"Done!","cost_usd":0.05,"num_turns":3,"session_id":"sess-abc"}
`
	s := &Session{
		ID:     "test",
		Status: StatusRunning,
	}

	stdout := strings.NewReader(streamData)
	stderr := strings.NewReader("")

	// Run in a goroutine that we can wait for
	// We need to mock cmd.Wait(), so we test the scanner logic directly
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSessionOutput(context.Background(), s, stdout, nil)
	}()
	<-done

	// drain stderr
	_ = stderr

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ProviderSessionID != "sess-abc" {
		t.Errorf("ProviderSessionID = %q, want sess-abc", s.ProviderSessionID)
	}
	if s.SpentUSD != 0.05 {
		t.Errorf("SpentUSD = %f, want 0.05", s.SpentUSD)
	}
	if s.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", s.TurnCount)
	}
	if s.LastOutput != "Done!" {
		t.Errorf("LastOutput = %q, want 'Done!'", s.LastOutput)
	}
}

func TestRunSessionOutputRecordsParseErrors(t *testing.T) {
	s := &Session{
		ID:       "parse-test",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		OutputCh: make(chan string, 10),
	}

	runSessionOutput(context.Background(), s, strings.NewReader("not-json\n"), nil)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.StreamParseErrors != 1 {
		t.Fatalf("StreamParseErrors = %d, want 1", s.StreamParseErrors)
	}
	if s.LastEventType != "parse_error" {
		t.Errorf("LastEventType = %q", s.LastEventType)
	}
	if s.LastOutput != "not-json" {
		t.Errorf("LastOutput = %q", s.LastOutput)
	}
	if len(s.OutputHistory) != 1 {
		t.Errorf("OutputHistory len = %d, want 1", len(s.OutputHistory))
	}
}
