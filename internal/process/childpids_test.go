package process

import (
	"testing"
	"time"
)

func TestChildPidsForRepo_NotRunning(t *testing.T) {
	m := NewManager()
	if pids := m.ChildPidsForRepo("/not/running"); pids != nil {
		t.Errorf("expected nil for unknown repo, got %v", pids)
	}
}

func TestChildPidsForRepo_RunningProcess(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)
	defer m.StopAll()

	if err := m.Start(repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Allow the process a moment to fully start.
	time.Sleep(50 * time.Millisecond)

	pids := m.ChildPidsForRepo(repoPath)
	// On Linux, pids contains at least the bash process itself (it shares the pgid).
	// On other platforms CollectChildPIDs is a no-op, so nil is acceptable.
	// Either way, the call must not panic and must update mp.ChildPids.
	_ = pids // platform-dependent; just ensure it doesn't panic

	// Confirm the stored snapshot matches what was returned.
	m.mu.Lock()
	mp, ok := m.procs[repoPath]
	m.mu.Unlock()
	if !ok {
		t.Fatal("process no longer tracked after ChildPidsForRepo")
	}

	// mp.ChildPids must be consistent with what ChildPidsForRepo returned.
	if len(mp.ChildPids) != len(pids) {
		t.Errorf("stored ChildPids len %d != returned len %d", len(mp.ChildPids), len(pids))
	}
}

func TestManagedProcess_ChildPidsField_ZeroValue(t *testing.T) {
	mp := &ManagedProcess{PID: 1234}
	if mp.ChildPids != nil {
		t.Errorf("zero-value ChildPids should be nil, got %v", mp.ChildPids)
	}
}
