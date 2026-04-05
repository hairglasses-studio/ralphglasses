package session

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestRunSessionOutput_ClaudeResumedCacheAnomaly(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(10)
	ch := bus.SubscribeFiltered("cache-anomaly", events.SessionError)

	s := &Session{
		ID:       "claude-resume",
		Provider: ProviderClaude,
		Status:   StatusRunning,
		Resumed:  true,
		RepoPath: "/tmp/repo",
		RepoName: "repo",
		bus:      bus,
	}

	input := `{"type":"result","result":"done","usage":{"cache_creation_input_tokens":1200}}` + "\n"
	runSessionOutput(context.Background(), s, strings.NewReader(input), nil)

	if s.CacheWriteTokens != 1200 {
		t.Fatalf("CacheWriteTokens = %d, want 1200", s.CacheWriteTokens)
	}
	if s.CacheReadTokens != 0 {
		t.Fatalf("CacheReadTokens = %d, want 0", s.CacheReadTokens)
	}
	if s.CacheAnomaly == "" {
		t.Fatal("expected cache anomaly to be recorded")
	}

	select {
	case ev := <-ch:
		if ev.Type != events.SessionError {
			t.Fatalf("event type = %s, want %s", ev.Type, events.SessionError)
		}
		if got, _ := ev.Data["cache_write_tokens"].(int); got != 1200 {
			t.Fatalf("cache_write_tokens = %v, want 1200", ev.Data["cache_write_tokens"])
		}
	default:
		t.Fatal("expected session.error event for Claude cache anomaly")
	}
}
