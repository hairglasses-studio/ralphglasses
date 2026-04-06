package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// EnrichedContext holds deterministically pre-fetched data gathered before
// an LLM invocation. Each field includes its estimated token count so the
// caller can deduct from the ContextBudget.
type EnrichedContext struct {
	RepoPath   string `json:"repo_path"`
	CommitHash string `json:"commit_hash"`

	GitLog      string `json:"git_log,omitempty"`
	GitLogTokens int64 `json:"git_log_tokens"`

	GitStatus      string `json:"git_status,omitempty"`
	GitStatusTokens int64 `json:"git_status_tokens"`

	RecentErrors      string `json:"recent_errors,omitempty"`
	RecentErrorsTokens int64 `json:"recent_errors_tokens"`

	TotalTokens int64     `json:"total_tokens"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// prefetchCache provides a short-lived cache keyed by repo+commit.
var prefetchCache = struct {
	sync.RWMutex
	entries map[string]*EnrichedContext
}{
	entries: make(map[string]*EnrichedContext),
}

const prefetchCacheTTL = 5 * time.Minute

// PrefetchContext deterministically gathers all anticipated context data
// before invoking the LLM. This follows 12-Factor Agent principle #13:
// pre-fetch everything the model will need so it focuses on reasoning,
// not data fetching.
func PrefetchContext(ctx context.Context, repoPath string, provider Provider, recentErrors []LoopError) (*EnrichedContext, error) {
	if repoPath == "" {
		return &EnrichedContext{FetchedAt: time.Now()}, nil
	}

	// Check cache.
	commitHash := getHeadCommit(ctx, repoPath)
	cacheKey := repoPath + ":" + commitHash

	prefetchCache.RLock()
	if cached, ok := prefetchCache.entries[cacheKey]; ok {
		if time.Since(cached.FetchedAt) < prefetchCacheTTL {
			prefetchCache.RUnlock()
			return cached, nil
		}
	}
	prefetchCache.RUnlock()

	ec := &EnrichedContext{
		RepoPath:   repoPath,
		CommitHash: commitHash,
		FetchedAt:  time.Now(),
	}

	// Fetch git log and status in parallel.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ec.GitLog = prefetchGitLog(ctx, repoPath, 20)
		ec.GitLogTokens = EstimateTokensForProvider(ec.GitLog, provider)
	}()

	go func() {
		defer wg.Done()
		ec.GitStatus = prefetchGitStatus(ctx, repoPath)
		ec.GitStatusTokens = EstimateTokensForProvider(ec.GitStatus, provider)
	}()

	wg.Wait()

	// Compact recent errors if any.
	if len(recentErrors) > 0 {
		ec.RecentErrors = CompactErrors(recentErrors, MaxCompactedTokens)
		ec.RecentErrorsTokens = EstimateTokensForProvider(ec.RecentErrors, provider)
	}

	ec.TotalTokens = ec.GitLogTokens + ec.GitStatusTokens + ec.RecentErrorsTokens

	// Update cache.
	prefetchCache.Lock()
	prefetchCache.entries[cacheKey] = ec
	prefetchCache.Unlock()

	return ec, nil
}

// FormatForContext renders the enriched context as a structured markdown block
// suitable for inclusion in an LLM prompt.
func (ec *EnrichedContext) FormatForContext() string {
	if ec == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Pre-fetched Repository Context\n\n")

	if ec.GitLog != "" {
		b.WriteString("### Recent Commits\n```\n")
		b.WriteString(ec.GitLog)
		b.WriteString("```\n\n")
	}

	if ec.GitStatus != "" {
		b.WriteString("### Working Tree Status\n```\n")
		b.WriteString(ec.GitStatus)
		b.WriteString("```\n\n")
	}

	if ec.RecentErrors != "" {
		b.WriteString(ec.RecentErrors)
		b.WriteString("\n")
	}

	return b.String()
}

func getHeadCommit(ctx context.Context, repoPath string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func prefetchGitLog(ctx context.Context, repoPath string, n int) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", repoPath, "log",
		fmt.Sprintf("-%d", n), "--oneline").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func prefetchGitStatus(ctx context.Context, repoPath string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--short").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
