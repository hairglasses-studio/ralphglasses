package process

import (
	"testing"
)

func TestAuditOrphanedProcesses(t *testing.T) {
	tests := []struct {
		name          string
		scanned       []int           // PIDs returned by the stub scanner
		tracked       map[string]int  // repoPath -> PID entries in manager
		wantOrphans   int             // expected number of orphan log lines (approximate via side effect)
	}{
		{
			name:    "no-processes",
			scanned: nil,
			tracked: map[string]int{},
		},
		{
			name:    "all-known",
			scanned: []int{100, 200},
			tracked: map[string]int{"/repo/a": 100, "/repo/b": 200},
		},
		{
			name:        "single-orphan",
			scanned:     []int{100, 999},
			tracked:     map[string]int{"/repo/a": 100},
			wantOrphans: 1,
		},
		{
			name:        "multiple-orphans",
			scanned:     []int{100, 201, 302, 403},
			tracked:     map[string]int{"/repo/a": 100},
			wantOrphans: 3,
		},
		{
			name:        "empty-manager",
			scanned:     []int{111, 222},
			tracked:     map[string]int{},
			wantOrphans: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build manager with the tracked PIDs injected directly.
			m := NewManager()
			for repo, pid := range tc.tracked {
				m.procs[repo] = &ManagedProcess{PID: pid}
			}

			// Stub the scanner.
			origScan := *scanProcessesFnPtr.Load()
			defer func() { setScanProcessesFn(origScan) }()
			scanned := tc.scanned
			setScanProcessesFn(func() []int { return scanned })

			// Collect orphan PIDs by shadowing the log output via a counter.
			orphanCount := 0
			origLog := *orphanLogFnPtr.Load()
			defer func() { setOrphanLogFn(origLog) }()
			setOrphanLogFn(func(pid int) { orphanCount++ })

			m.auditOrphanedProcesses()

			if orphanCount != tc.wantOrphans {
				t.Errorf("orphan count: got %d, want %d", orphanCount, tc.wantOrphans)
			}
		})
	}
}
