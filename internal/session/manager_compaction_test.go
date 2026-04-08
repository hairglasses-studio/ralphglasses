package session

import (
	"context"
	"strings"
	"testing"
)

func TestResume_InjectsClearedToolResultsIntoSystemPrompt(t *testing.T) {
	t.Parallel()

	m := NewManager()
	m.SetStateDir(t.TempDir())

	repoPath := t.TempDir()
	m.AddSessionForTesting(&Session{
		ID:       "resume-source",
		RepoPath: repoPath,
		Provider: ProviderCodex,
		MessageHistory: []Message{
			{Role: "user", Content: "inspect the logs"},
			{Role: "tool", Content: strings.Repeat("large tool output ", 20000), ToolName: "FileRead"},
			{Role: "assistant", Content: "I inspected them"},
			{Role: "user", Content: "open the build trace"},
			{Role: "tool", Content: strings.Repeat("large build trace ", 20000), ToolName: "Bash"},
			{Role: "assistant", Content: "I opened it"},
			{Role: "user", Content: "recent turn 1"},
			{Role: "assistant", Content: "recent reply 1"},
			{Role: "user", Content: "recent turn 2"},
			{Role: "assistant", Content: "recent reply 2"},
			{Role: "user", Content: "recent turn 3"},
			{Role: "assistant", Content: "recent reply 3"},
			{Role: "user", Content: "recent turn 4"},
			{Role: "assistant", Content: "recent reply 4"},
			{Role: "user", Content: "recent turn 5"},
			{Role: "assistant", Content: "recent reply 5"},
		},
	})

	var captured LaunchOptions
	m.SetHooksForTesting(func(_ context.Context, opts LaunchOptions) (*Session, error) {
		captured = opts
		return &Session{
			ID:       "resumed-session",
			RepoPath: opts.RepoPath,
			Provider: opts.Provider,
			Status:   StatusCompleted,
			TenantID: DefaultTenantID,
		}, nil
	}, nil)

	if _, err := m.Resume(context.Background(), repoPath, ProviderCodex, "resume-source", "continue"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if !strings.Contains(captured.SystemPrompt, "[Old tool result content cleared: FileRead]") {
		t.Fatalf("SystemPrompt missing cleared FileRead placeholder: %q", captured.SystemPrompt)
	}
	if !strings.Contains(captured.SystemPrompt, "[Old tool result content cleared: Bash]") {
		t.Fatalf("SystemPrompt missing cleared Bash placeholder: %q", captured.SystemPrompt)
	}
	if strings.Contains(captured.SystemPrompt, "large tool output") || strings.Contains(captured.SystemPrompt, "large build trace") {
		t.Fatalf("SystemPrompt should not contain original large tool output: %q", captured.SystemPrompt)
	}
}

func TestResumeCompactionPrompt_CircuitBreakerTripsAndSkips(t *testing.T) {
	t.Parallel()

	m := NewManager()
	repoPath := t.TempDir()
	m.AddSessionForTesting(&Session{
		ID:       "resume-circuit",
		RepoPath: repoPath,
		Provider: ProviderCodex,
		MessageHistory: []Message{
			{Role: "user", Content: strings.Repeat("single gigantic message ", 30000)},
		},
	})

	opts := LaunchOptions{
		Provider: ProviderCodex,
		RepoPath: repoPath,
		Resume:   "resume-circuit",
	}

	for want := 1; want <= maxConsecutiveCompactFailures; want++ {
		summary, applied, attempted := m.resumeCompactionPrompt(context.Background(), opts)
		if summary != "" || applied {
			t.Fatalf("attempt %d unexpectedly applied compaction: summary=%q applied=%v", want, summary, applied)
		}
		if !attempted {
			t.Fatalf("attempt %d should have tried compaction before the breaker opened", want)
		}
		if got := m.resumeCompactionFailureCount("resume-circuit"); got != want {
			t.Fatalf("failure count after attempt %d = %d, want %d", want, got, want)
		}
	}

	summary, applied, attempted := m.resumeCompactionPrompt(context.Background(), opts)
	if summary != "" || applied || attempted {
		t.Fatalf("circuit-open call = (%q, %v, %v), want empty/false/false", summary, applied, attempted)
	}
	if got := m.resumeCompactionFailureCount("resume-circuit"); got != maxConsecutiveCompactFailures {
		t.Fatalf("failure count after breaker opened = %d, want %d", got, maxConsecutiveCompactFailures)
	}
}

func TestResumeCompactionPrompt_SuccessResetsFailureCounter(t *testing.T) {
	t.Parallel()

	m := NewManager()
	repoPath := t.TempDir()
	m.AddSessionForTesting(&Session{
		ID:       "resume-reset",
		RepoPath: repoPath,
		Provider: ProviderCodex,
		MessageHistory: []Message{
			{Role: "user", Content: "inspect the logs"},
			{Role: "tool", Content: strings.Repeat("large tool output ", 20000), ToolName: "FileRead"},
			{Role: "assistant", Content: "I inspected them"},
			{Role: "user", Content: "open the build trace"},
			{Role: "tool", Content: strings.Repeat("large build trace ", 20000), ToolName: "Bash"},
			{Role: "assistant", Content: "I opened it"},
			{Role: "user", Content: "recent turn 1"},
			{Role: "assistant", Content: "recent reply 1"},
			{Role: "user", Content: "recent turn 2"},
			{Role: "assistant", Content: "recent reply 2"},
			{Role: "user", Content: "recent turn 3"},
			{Role: "assistant", Content: "recent reply 3"},
			{Role: "user", Content: "recent turn 4"},
			{Role: "assistant", Content: "recent reply 4"},
			{Role: "user", Content: "recent turn 5"},
			{Role: "assistant", Content: "recent reply 5"},
		},
	})
	m.setResumeCompactionFailures("resume-reset", maxConsecutiveCompactFailures-1)

	summary, applied, attempted := m.resumeCompactionPrompt(context.Background(), LaunchOptions{
		Provider: ProviderCodex,
		RepoPath: repoPath,
		Resume:   "resume-reset",
	})
	if !attempted || !applied {
		t.Fatalf("resumeCompactionPrompt = (%q, %v, %v), want applied success", summary, applied, attempted)
	}
	if !strings.Contains(summary, "[Old tool result content cleared: FileRead]") {
		t.Fatalf("summary missing cleared placeholder: %q", summary)
	}
	if got := m.resumeCompactionFailureCount("resume-reset"); got != 0 {
		t.Fatalf("failure count = %d, want 0 after successful compaction", got)
	}
}
