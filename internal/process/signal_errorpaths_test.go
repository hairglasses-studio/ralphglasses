package process

import (
	"fmt"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestKillSequence_KillReturnsESRCH(t *testing.T) {
	// Simulate ESRCH (no such process) — kill fails for a process that exited
	// between the alive check and the kill call.
	h := newHarness(100, 200)
	defer h.install()()

	esrchErr := fmt.Errorf("kill: %w", syscall.ESRCH)

	// Override killFn to return ESRCH for PID 200.
	setKillPid(func(pid int, sig syscall.Signal) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.signals = append(h.signals, signalRecord{PID: pid, Sig: sig})
		if pid == 200 {
			delete(h.alive, 200)
			return esrchErr
		}
		if sig == syscall.SIGKILL {
			delete(h.alive, pid)
		}
		return nil
	})

	// Should not panic even though kill returns an error for PID 200.
	runKillSequence(100, []int{200}, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Primary (100) should have received SIGTERM.
	var gotTermPrimary bool
	for _, s := range h.signals {
		if s.PID == 100 && s.Sig == syscall.SIGTERM {
			gotTermPrimary = true
		}
	}
	if !gotTermPrimary {
		t.Error("expected SIGTERM to primary PID 100")
	}
}

func TestKillSequence_AllDeadBeforeKill(t *testing.T) {
	// All PIDs die between SIGTERM and the alive check.
	h := newHarness(100, 200)
	defer h.install()()

	// Override killFn to kill all on SIGTERM.
	setKillPid(func(pid int, sig syscall.Signal) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.signals = append(h.signals, signalRecord{PID: pid, Sig: sig})
		if sig == syscall.SIGTERM {
			delete(h.alive, pid)
		}
		return nil
	})

	runKillSequence(100, []int{200}, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Should NOT reach SIGKILL since all died on SIGTERM.
	for _, s := range h.signals {
		if s.Sig == syscall.SIGKILL {
			t.Errorf("unexpected SIGKILL to PID %d — all should be dead after SIGTERM", s.PID)
		}
	}
}

func TestKillSequence_ConcurrentCalls(t *testing.T) {
	// Multiple concurrent runKillSequence calls should not panic or race.
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			h := newHarness(base, base+1)
			restore := h.install()
			defer restore()
			runKillSequence(base, []int{base + 1}, 50*time.Millisecond)
		}(1000 + i*10)
	}
	wg.Wait()
}
