package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

const raceConcurrency = 10

// TestConcurrentScan stresses the Server.mu RWMutex that guards the Repos
// slice. Multiple goroutines call handleScan simultaneously, each of which
// writes to s.Repos under a write lock via scan().
func TestConcurrentScan(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, raceConcurrency)
	results := make([]*mcp.CallToolResult, raceConcurrency)

	wg.Add(raceConcurrency)
	for i := 0; i < raceConcurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = srv.handleScan(ctx, makeRequest(nil))
		}(i)
	}
	wg.Wait()

	for i := 0; i < raceConcurrency; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: handleScan returned error: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Fatalf("goroutine %d: nil result", i)
		}
		if results[i].IsError {
			t.Errorf("goroutine %d: IsError: %s", i, getResultText(results[i]))
		}
		if text := getResultText(results[i]); !strings.Contains(text, "repos_found") {
			t.Errorf("goroutine %d: unexpected text: %s", i, text)
		}
	}
}

// TestConcurrentList stresses concurrent reads via handleList. Each call
// reads s.Repos under an RLock (reposNil, reposCopy) and iterates the
// copy. Concurrent with a simultaneous scan, this exercises read/write
// contention on the Repos slice.
func TestConcurrentList(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Pre-scan so list has data to read.
	if _, err := srv.handleScan(ctx, makeRequest(nil)); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(raceConcurrency)

	errs := make([]error, raceConcurrency)
	results := make([]*mcp.CallToolResult, raceConcurrency)

	for i := 0; i < raceConcurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = srv.handleList(ctx, makeRequest(nil))
		}(i)
	}
	wg.Wait()

	for i := 0; i < raceConcurrency; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: handleList error: %v", i, errs[i])
		}
		if results[i] == nil {
			t.Fatalf("goroutine %d: nil result", i)
		}
		if results[i].IsError {
			t.Errorf("goroutine %d: IsError: %s", i, getResultText(results[i]))
		}
		text := getResultText(results[i])
		if !strings.Contains(text, "test-repo") {
			t.Errorf("goroutine %d: expected test-repo in output, got: %s", i, text)
		}
	}
}

// TestConcurrentScanAndList exercises the most realistic contention pattern:
// simultaneous scans (write-lock) interleaved with list reads (read-lock).
// This targets the TOCTOU window in handleList where reposNil() and scan()
// are separate critical sections.
func TestConcurrentScanAndList(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	half := raceConcurrency / 2
	wg.Add(raceConcurrency)

	scanErrs := make([]error, half)
	listErrs := make([]error, half)
	listResults := make([]*mcp.CallToolResult, half)

	// Half scanners
	for i := 0; i < half; i++ {
		go func(idx int) {
			defer wg.Done()
			_, scanErrs[idx] = srv.handleScan(ctx, makeRequest(nil))
		}(i)
	}
	// Half listers
	for i := 0; i < half; i++ {
		go func(idx int) {
			defer wg.Done()
			listResults[idx], listErrs[idx] = srv.handleList(ctx, makeRequest(nil))
		}(i)
	}
	wg.Wait()

	for i := 0; i < half; i++ {
		if scanErrs[i] != nil {
			t.Errorf("scanner %d: %v", i, scanErrs[i])
		}
		if listErrs[i] != nil {
			t.Errorf("lister %d: %v", i, listErrs[i])
		}
		if listResults[i] != nil && listResults[i].IsError {
			t.Errorf("lister %d: IsError: %s", i, getResultText(listResults[i]))
		}
	}
}

// TestConcurrentFindRepo stresses findRepo (RLock) concurrent with scan
// (write-lock replacement of the Repos slice).
func TestConcurrentFindRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Pre-scan.
	if _, err := srv.handleScan(ctx, makeRequest(nil)); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(raceConcurrency)
	for i := 0; i < raceConcurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			if idx%2 == 0 {
				// Read path: findRepo
				r := srv.findRepo("test-repo")
				if r == nil {
					// Acceptable during a concurrent scan replacement
					return
				}
				if r.Name != "test-repo" {
					t.Errorf("findRepo returned wrong repo: %s", r.Name)
				}
			} else {
				// Write path: scan replaces Repos
				_ = srv.scan()
			}
		}(i)
	}
	wg.Wait()
}

// TestConcurrentHandlerSubtests uses parallel subtests to exercise
// multiple handler types concurrently against the same Server.
func TestConcurrentHandlerSubtests(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Pre-scan so all handlers have data.
	if _, err := srv.handleScan(ctx, makeRequest(nil)); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		req     mcp.CallToolRequest
		check   func(t *testing.T, r *mcp.CallToolResult)
	}{
		{
			// Stresses: reposCopy (RLock on Repos slice)
			name:    "list",
			handler: srv.handleList,
			req:     makeRequest(nil),
			check: func(t *testing.T, r *mcp.CallToolResult) {
				text := getResultText(r)
				if !strings.Contains(text, "test-repo") {
					t.Errorf("list missing test-repo: %s", text)
				}
			},
		},
		{
			// Stresses: scan (write-lock on Repos slice)
			name:    "scan",
			handler: srv.handleScan,
			req:     makeRequest(nil),
			check: func(t *testing.T, r *mcp.CallToolResult) {
				if !strings.Contains(getResultText(r), "repos_found") {
					t.Errorf("scan unexpected: %s", getResultText(r))
				}
			},
		},
		{
			// Stresses: findRepo (RLock) + repo field reads
			name:    "status",
			handler: srv.handleStatus,
			req:     makeRequest(map[string]any{"repo": "test-repo"}),
			check: func(t *testing.T, r *mcp.CallToolResult) {
				text := getResultText(r)
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("status not valid JSON: %v", err)
				}
			},
		},
		{
			// Stresses: findRepo + Config.Values map read
			name:    "config_get",
			handler: srv.handleConfig,
			req:     makeRequest(map[string]any{"repo": "test-repo", "key": "MODEL"}),
			check: func(t *testing.T, r *mcp.CallToolResult) {
				text := getResultText(r)
				if !strings.Contains(text, "MODEL") {
					t.Errorf("config_get unexpected: %s", text)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		// Run each case N times as parallel subtests.
		for i := 0; i < raceConcurrency; i++ {
			name := tc.name + "_" + strings.Repeat("g", i+1) // unique subtest names
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				result, err := tc.handler(ctx, tc.req)
				if err != nil {
					t.Fatalf("handler error: %v", err)
				}
				if result == nil {
					t.Fatal("nil result")
				}
				if result.IsError {
					t.Fatalf("IsError: %s", getResultText(result))
				}
				tc.check(t, result)
			})
		}
	}
}
