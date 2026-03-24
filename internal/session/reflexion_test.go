package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractReflection_VerifyFailed(t *testing.T) {
	rs := NewReflexionStore("")
	iter := LoopIteration{
		Number: 3,
		Status: "failed",
		Task:   LoopTask{Title: "implement auth middleware"},
		Verification: []LoopVerification{
			{
				Command:  "go test ./...",
				Status:   "failed",
				ExitCode: 1,
				Output:   "FAIL github.com/example/pkg [build failed]\nerror: undefined: AuthMiddleware",
			},
		},
		Error: "verification failed",
	}

	r := rs.ExtractReflection("loop-1", iter)
	if r == nil {
		t.Fatal("expected non-nil reflection")
	}
	if r.FailureMode != "verify_failed" {
		t.Errorf("expected failure_mode=verify_failed, got %s", r.FailureMode)
	}
	if r.RootCause == "" {
		t.Error("expected non-empty root cause")
	}
	if !strings.Contains(r.Correction, "verification commands pass") {
		t.Errorf("expected correction about verification, got: %s", r.Correction)
	}
	if r.TaskTitle != "implement auth middleware" {
		t.Errorf("expected task title preserved, got: %s", r.TaskTitle)
	}
	if r.LoopID != "loop-1" {
		t.Errorf("expected loop_id=loop-1, got %s", r.LoopID)
	}
	if r.IterationNum != 3 {
		t.Errorf("expected iteration=3, got %d", r.IterationNum)
	}
}

func TestExtractReflection_WorkerError(t *testing.T) {
	rs := NewReflexionStore("")
	iter := LoopIteration{
		Number: 1,
		Status: "failed",
		Task:   LoopTask{Title: "refactor database layer"},
		Error:  "worker process exited: error: cannot connect to database",
		WorkerOutput: `processing files...
error: cannot connect to database at localhost:5432
panic: runtime error in worker`,
	}

	r := rs.ExtractReflection("loop-2", iter)
	if r == nil {
		t.Fatal("expected non-nil reflection")
	}
	if r.FailureMode != "worker_error" {
		t.Errorf("expected failure_mode=worker_error, got %s", r.FailureMode)
	}
	if r.RootCause == "" {
		t.Error("expected non-empty root cause")
	}
	if !strings.Contains(r.Correction, "worker encountered") {
		t.Errorf("expected correction about worker error, got: %s", r.Correction)
	}
}

func TestExtractReflection_PlannerError(t *testing.T) {
	rs := NewReflexionStore("")
	iter := LoopIteration{
		Number: 1,
		Status: "failed",
		Task:   LoopTask{Title: "add logging"},
		Error:  "failed to parse planner output",
	}

	r := rs.ExtractReflection("loop-3", iter)
	if r == nil {
		t.Fatal("expected non-nil reflection")
	}
	if r.FailureMode != "planner_error" {
		t.Errorf("expected failure_mode=planner_error, got %s", r.FailureMode)
	}
	if !strings.Contains(r.Correction, "planner could not parse") {
		t.Errorf("expected correction about planner, got: %s", r.Correction)
	}
}

func TestExtractReflection_NotFailed(t *testing.T) {
	rs := NewReflexionStore("")
	iter := LoopIteration{
		Number: 1,
		Status: "idle",
		Task:   LoopTask{Title: "some task"},
	}

	r := rs.ExtractReflection("loop-4", iter)
	if r != nil {
		t.Errorf("expected nil for non-failed iteration, got %+v", r)
	}
}

func TestReflexionStorePersistence(t *testing.T) {
	dir := t.TempDir()

	// Create store and add reflections.
	store1 := NewReflexionStore(dir)
	store1.Store(Reflection{
		Timestamp:    time.Now(),
		LoopID:       "loop-a",
		IterationNum: 1,
		TaskTitle:    "first task",
		FailureMode:  "verify_failed",
		RootCause:    "test failed",
		Correction:   "fix the test",
	})
	store1.Store(Reflection{
		Timestamp:    time.Now(),
		LoopID:       "loop-a",
		IterationNum: 2,
		TaskTitle:    "second task",
		FailureMode:  "worker_error",
		RootCause:    "timeout",
		Correction:   "increase timeout",
	})

	// Verify JSONL file exists.
	path := filepath.Join(dir, "reflections.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected reflections.jsonl to exist: %v", err)
	}

	// Load from same directory in a new store.
	store2 := NewReflexionStore(dir)
	store2.mu.Lock()
	count := len(store2.reflections)
	store2.mu.Unlock()

	if count != 2 {
		t.Errorf("expected 2 loaded reflections, got %d", count)
	}
}

func TestRecentForTask(t *testing.T) {
	rs := NewReflexionStore("")

	// Store reflections with different titles.
	rs.Store(Reflection{
		Timestamp:    time.Now().Add(-3 * time.Hour),
		LoopID:       "loop-1",
		IterationNum: 1,
		TaskTitle:    "implement auth middleware",
		FailureMode:  "verify_failed",
		RootCause:    "test failed",
		Correction:   "fix auth test",
	})
	rs.Store(Reflection{
		Timestamp:    time.Now().Add(-2 * time.Hour),
		LoopID:       "loop-1",
		IterationNum: 2,
		TaskTitle:    "implement database layer",
		FailureMode:  "worker_error",
		RootCause:    "connection error",
		Correction:   "fix connection",
	})
	rs.Store(Reflection{
		Timestamp:    time.Now().Add(-1 * time.Hour),
		LoopID:       "loop-1",
		IterationNum: 3,
		TaskTitle:    "fix auth middleware tests",
		FailureMode:  "verify_failed",
		RootCause:    "missing import",
		Correction:   "add import",
	})

	// Query for "auth middleware" — should match reflections 1 and 3.
	results := rs.RecentForTask("auth middleware", 10)
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'auth middleware', got %d", len(results))
	}

	// Newest first.
	if len(results) >= 2 && results[0].IterationNum != 3 {
		t.Errorf("expected newest first (iteration 3), got iteration %d", results[0].IterationNum)
	}

	// Query for "database" — should match only reflection 2.
	results = rs.RecentForTask("database", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'database', got %d", len(results))
	}

	// Limit test.
	results = rs.RecentForTask("auth middleware", 1)
	if len(results) != 1 {
		t.Errorf("expected 1 result with limit=1, got %d", len(results))
	}
}

func TestFormatForPrompt(t *testing.T) {
	rs := NewReflexionStore("")

	reflections := []Reflection{
		{
			FailureMode: "verify_failed",
			RootCause:   "undefined: AuthMiddleware",
			Correction:  "Add missing AuthMiddleware definition",
		},
		{
			FailureMode: "worker_error",
			RootCause:   "timeout connecting to DB",
			Correction:  "Increase connection timeout",
		},
	}

	output := rs.FormatForPrompt(reflections)

	if !strings.Contains(output, "## Lessons from Previous Attempts") {
		t.Error("expected header in output")
	}
	if !strings.Contains(output, "Attempt 1 failed") {
		t.Error("expected 'Attempt 1 failed' in output")
	}
	if !strings.Contains(output, "Attempt 2 failed") {
		t.Error("expected 'Attempt 2 failed' in output")
	}
	if !strings.Contains(output, "verify_failed") {
		t.Error("expected failure mode in output")
	}
	if !strings.Contains(output, "AuthMiddleware") {
		t.Error("expected root cause in output")
	}
	if !strings.Contains(output, "Apply these lessons") {
		t.Error("expected closing instruction in output")
	}

	// Empty case.
	empty := rs.FormatForPrompt(nil)
	if empty != "" {
		t.Errorf("expected empty string for nil reflections, got: %s", empty)
	}
}

func TestExtractReflection_FilesInvolved(t *testing.T) {
	rs := NewReflexionStore("")
	iter := LoopIteration{
		Number: 1,
		Status: "failed",
		Task:   LoopTask{Title: "fix compile errors"},
		Error:  "build failed in internal/session/manager.go and cmd/main.go",
		WorkerOutput: `compiling...
error in pkg/util/helpers.py: syntax error`,
	}

	r := rs.ExtractReflection("loop-5", iter)
	if r == nil {
		t.Fatal("expected non-nil reflection")
	}
	if len(r.FilesInvolved) == 0 {
		t.Error("expected files to be extracted")
	}
	// Check that at least one known file path was found.
	found := false
	for _, f := range r.FilesInvolved {
		if strings.HasSuffix(f, ".go") || strings.HasSuffix(f, ".py") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected .go or .py file paths, got: %v", r.FilesInvolved)
	}
}
