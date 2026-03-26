package process

import (
	"sync"
	"syscall"
	"testing"
	"time"
)

// signalRecord captures a single signal delivery for test assertions.
type signalRecord struct {
	PID int
	Sig syscall.Signal
}

// killSequenceHarness stubs the package-level indirections so that
// runKillSequence can be exercised without real processes or sleeps.
type killSequenceHarness struct {
	mu      sync.Mutex
	signals []signalRecord
	alive   map[int]bool // which PIDs are "alive"
	sleeps  int          // how many sleeps have occurred
}

func newHarness(alivePids ...int) *killSequenceHarness {
	h := &killSequenceHarness{alive: make(map[int]bool)}
	for _, p := range alivePids {
		h.alive[p] = true
	}
	return h
}

func (h *killSequenceHarness) killFn(pid int, sig syscall.Signal) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signals = append(h.signals, signalRecord{PID: pid, Sig: sig})
	// SIGKILL always kills; SIGTERM kills only if we haven't set it to survive.
	if sig == syscall.SIGKILL {
		delete(h.alive, pid)
	}
	return nil
}

func (h *killSequenceHarness) aliveFn(pid int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.alive[pid]
}

func (h *killSequenceHarness) sleepFn(_ time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sleeps++
}

// install replaces the package-level stubs and returns a restore function.
func (h *killSequenceHarness) install() func() {
	origKill := *killPidPtr.Load()
	origSleep := *sleepFnPtr.Load()
	origAlive := *aliveFnPtr.Load()

	setKillPid(h.killFn)
	setSleepFn(h.sleepFn)
	setAliveFn(h.aliveFn)

	return func() {
		setKillPid(origKill)
		setSleepFn(origSleep)
		setAliveFn(origAlive)
	}
}

func TestKillSequence_FullEscalation(t *testing.T) {
	// All PIDs stay alive through SIGTERM so we reach SIGKILL.
	h := newHarness(100, 200, 201)
	defer h.install()()

	runKillSequence(100, []int{200, 201}, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Expect: SIGTERM(100), SIGTERM(200), SIGTERM(201), SIGKILL(100), SIGKILL(200), SIGKILL(201)
	expected := []signalRecord{
		{100, syscall.SIGTERM},
		{200, syscall.SIGTERM},
		{201, syscall.SIGTERM},
		{100, syscall.SIGKILL},
		{200, syscall.SIGKILL},
		{201, syscall.SIGKILL},
	}

	if len(h.signals) != len(expected) {
		t.Fatalf("expected %d signals, got %d: %+v", len(expected), len(h.signals), h.signals)
	}
	for i, want := range expected {
		got := h.signals[i]
		if got != want {
			t.Errorf("signal[%d]: got %+v, want %+v", i, got, want)
		}
	}

	if h.sleeps != 2 {
		t.Errorf("expected 2 sleeps, got %d", h.sleeps)
	}
}

func TestKillSequence_PrimaryExitsAfterSIGTERM(t *testing.T) {
	// Primary dies after SIGTERM, children are alive.
	h := newHarness(100, 200)
	defer h.install()()

	// Override killFn to also mark primary as dead on SIGTERM.
	origKill := h.killFn
	setKillPid(func(pid int, sig syscall.Signal) error {
		err := origKill(pid, sig)
		if pid == 100 && sig == syscall.SIGTERM {
			h.mu.Lock()
			delete(h.alive, 100)
			h.mu.Unlock()
		}
		return err
	})

	runKillSequence(100, []int{200}, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Primary got SIGTERM. After sleep, child 200 gets SIGTERM.
	// After second sleep, primary is dead so skipped, child 200 gets SIGKILL.
	expected := []signalRecord{
		{100, syscall.SIGTERM},
		{200, syscall.SIGTERM},
		{200, syscall.SIGKILL},
	}

	if len(h.signals) != len(expected) {
		t.Fatalf("expected %d signals, got %d: %+v", len(expected), len(h.signals), h.signals)
	}
	for i, want := range expected {
		got := h.signals[i]
		if got != want {
			t.Errorf("signal[%d]: got %+v, want %+v", i, got, want)
		}
	}
}

func TestKillSequence_AllExitBeforeSIGKILL(t *testing.T) {
	// All PIDs die on SIGTERM — no SIGKILL sent.
	h := newHarness(100, 200)
	defer h.install()()

	origKill := h.killFn
	setKillPid(func(pid int, sig syscall.Signal) error {
		err := origKill(pid, sig)
		if sig == syscall.SIGTERM {
			h.mu.Lock()
			delete(h.alive, pid)
			h.mu.Unlock()
		}
		return err
	})

	runKillSequence(100, []int{200}, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	// Only two SIGTERMs, no SIGKILLs.
	expected := []signalRecord{
		{100, syscall.SIGTERM},
		{200, syscall.SIGTERM},
	}

	if len(h.signals) != len(expected) {
		t.Fatalf("expected %d signals, got %d: %+v", len(expected), len(h.signals), h.signals)
	}
	for i, want := range expected {
		if h.signals[i] != want {
			t.Errorf("signal[%d]: got %+v, want %+v", i, h.signals[i], want)
		}
	}
}

func TestKillSequence_NoChildren(t *testing.T) {
	// Only a primary PID, no children.
	h := newHarness(100)
	defer h.install()()

	runKillSequence(100, nil, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	// SIGTERM(100), then SIGKILL(100).
	expected := []signalRecord{
		{100, syscall.SIGTERM},
		{100, syscall.SIGKILL},
	}

	if len(h.signals) != len(expected) {
		t.Fatalf("expected %d signals, got %d: %+v", len(expected), len(h.signals), h.signals)
	}
	for i, want := range expected {
		if h.signals[i] != want {
			t.Errorf("signal[%d]: got %+v, want %+v", i, h.signals[i], want)
		}
	}
}

func TestKillSequence_AlreadyDead(t *testing.T) {
	// Primary is already dead at the start — no signals sent.
	h := newHarness() // nothing alive
	defer h.install()()

	runKillSequence(100, []int{200}, DefaultKillTimeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.signals) != 0 {
		t.Errorf("expected 0 signals for dead PIDs, got %d: %+v", len(h.signals), h.signals)
	}
}
