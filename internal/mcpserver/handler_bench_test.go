package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// benchSessionCounter ensures unique IDs across benchmark iterations.
var benchSessionCounter atomic.Int64

// setupBenchServer creates a minimal Server with pre-scanned repos and
// realistic fixture data, suitable for benchmarks. It avoids git init
// since that is expensive and not needed for handler hot paths.
func setupBenchServer(b *testing.B) (*Server, string) {
	b.Helper()
	root := b.TempDir()

	repoPath := filepath.Join(root, "bench-repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	logsDir := filepath.Join(ralphDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		b.Fatal(err)
	}

	// Write .ralphrc
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("MODEL=sonnet\nBUDGET=10\n"), 0644); err != nil {
		b.Fatal(err)
	}

	// Write status.json
	status := model.LoopStatus{
		LoopCount:       42,
		Status:          "running",
		CallsMadeThisHr: 15,
		MaxCallsPerHour: 100,
		LastAction:      "edit_file",
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(ralphDir, "status.json"), data, 0644); err != nil {
		b.Fatal(err)
	}

	// Write circuit breaker state
	cb := model.CircuitBreakerState{
		State:      "CLOSED",
		TotalOpens: 0,
	}
	data, _ = json.Marshal(cb)
	if err := os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), data, 0644); err != nil {
		b.Fatal(err)
	}

	// Write progress.json
	prog := model.Progress{
		Iteration:    5,
		Status:       "in_progress",
		CompletedIDs: []string{"task-1", "task-2", "task-3"},
	}
	data, _ = json.Marshal(prog)
	if err := os.WriteFile(filepath.Join(ralphDir, "progress.json"), data, 0644); err != nil {
		b.Fatal(err)
	}

	// Write log file
	if err := os.WriteFile(filepath.Join(logsDir, "ralph.log"), []byte("log line 1\nlog line 2\n"), 0644); err != nil {
		b.Fatal(err)
	}

	srv := NewServer(root)
	srv.SessMgr.SetStateDir(filepath.Join(root, ".session-state"))

	// Pre-scan so benchmarks don't measure scan overhead.
	if err := srv.scan(); err != nil {
		b.Fatalf("pre-scan: %v", err)
	}

	return srv, root
}

// injectBenchSession creates a fake session for benchmark use.
func injectBenchSession(b *testing.B, srv *Server, repoPath string) string {
	b.Helper()
	now := time.Now()
	seq := benchSessionCounter.Add(1)
	id := fmt.Sprintf("bench-%d-%d", now.UnixNano(), seq)
	sess := &session.Session{
		ID:           id,
		Provider:     session.ProviderClaude,
		RepoPath:     repoPath,
		RepoName:     filepath.Base(repoPath),
		Prompt:       "benchmark prompt",
		Model:        "sonnet",
		Status:       session.StatusRunning,
		LaunchedAt:   now,
		LastActivity: now,
		SpentUSD:     0.42,
		TurnCount:    10,
		OutputCh:     make(chan string, 1),
	}
	srv.SessMgr.AddSessionForTesting(sess)
	return id
}

func BenchmarkHandleList(b *testing.B) {
	srv, _ := setupBenchServer(b)
	ctx := context.Background()
	req := makeRequest(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := srv.handleList(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if result.IsError {
			b.Fatalf("handleList error: %s", getResultText(result))
		}
	}
}

func BenchmarkHandleStatus(b *testing.B) {
	srv, _ := setupBenchServer(b)
	ctx := context.Background()
	req := makeRequest(map[string]any{"repo": "bench-repo"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := srv.handleStatus(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if result.IsError {
			b.Fatalf("handleStatus error: %s", getResultText(result))
		}
	}
}

func BenchmarkHandleSessionList(b *testing.B) {
	srv, root := setupBenchServer(b)
	repoPath := filepath.Join(root, "bench-repo")

	// Inject a handful of sessions to make the list realistic.
	for i := 0; i < 5; i++ {
		injectBenchSession(b, srv, repoPath)
	}

	ctx := context.Background()
	req := makeRequest(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := srv.handleSessionList(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
		if result.IsError {
			b.Fatalf("handleSessionList error: %s", getResultText(result))
		}
	}
}

func BenchmarkJsonResult(b *testing.B) {
	// Build a payload representative of a typical handleList response.
	payload := []map[string]any{
		{"name": "repo-alpha", "status": "running", "loop_count": 42, "calls": "15/100", "circuit": "CLOSED", "managed": true},
		{"name": "repo-beta", "status": "idle", "loop_count": 0, "calls": "0/100", "circuit": "CLOSED", "managed": false},
		{"name": "repo-gamma", "status": "running", "loop_count": 99, "calls": "88/100", "circuit": "HALF_OPEN", "managed": true},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := jsonResult(payload)
		if result.IsError {
			b.Fatal("jsonResult returned error")
		}
	}
}
