package session

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestNewManagerWithStore(t *testing.T) {
	store := NewMemoryStore()
	bus := events.NewBus(100)

	m := NewManagerWithStore(store, bus)
	if m == nil {
		t.Fatal("NewManagerWithStore returned nil")
	}
	if m.store != store {
		t.Error("store should be the one passed in")
	}
	if m.bus != bus {
		t.Error("bus should be the one passed in")
	}
	if m.sessions == nil {
		t.Error("sessions map should be initialized")
	}
	if m.teams == nil {
		t.Error("teams map should be initialized")
	}
	if m.workflowRuns == nil {
		t.Error("workflowRuns map should be initialized")
	}
	if m.loops == nil {
		t.Error("loops map should be initialized")
	}
	if m.noopDetector == nil {
		t.Error("noopDetector should be initialized")
	}
}

func TestNewManagerWithStore_NilBus(t *testing.T) {
	store := NewMemoryStore()
	m := NewManagerWithStore(store, nil)
	if m == nil {
		t.Fatal("NewManagerWithStore with nil bus returned nil")
	}
	if m.bus != nil {
		t.Error("bus should be nil when nil is passed")
	}
	if m.store != store {
		t.Error("store should still be set")
	}
}

func TestNewManagerWithStore_NilStore(t *testing.T) {
	bus := events.NewBus(100)
	m := NewManagerWithStore(nil, bus)
	if m == nil {
		t.Fatal("NewManagerWithStore with nil store returned nil")
	}
	if m.store != nil {
		t.Error("store should be nil when nil is passed")
	}
}

func TestStore_Getter(t *testing.T) {
	m := NewManager()
	if m.Store() != nil {
		t.Error("Store should return nil for default manager")
	}

	store := NewMemoryStore()
	m.SetStore(store)
	if m.Store() != store {
		t.Error("Store should return the set store")
	}
}

func TestStore_SetStore(t *testing.T) {
	m := NewManager()
	s1 := NewMemoryStore()
	s2 := NewMemoryStore()

	m.SetStore(s1)
	if m.Store() != s1 {
		t.Error("Store should return s1")
	}

	m.SetStore(s2)
	if m.Store() != s2 {
		t.Error("Store should return s2 after second SetStore")
	}

	m.SetStore(nil)
	if m.Store() != nil {
		t.Error("Store should return nil after SetStore(nil)")
	}
}

func TestConsecutiveNoOps_Default(t *testing.T) {
	m := NewManager()
	count := m.ConsecutiveNoOps("nonexistent-loop")
	if count != 0 {
		t.Errorf("ConsecutiveNoOps for nonexistent loop should be 0, got %d", count)
	}
}

func TestConsecutiveNoOps_NilDetector(t *testing.T) {
	m := &Manager{
		sessions:     make(map[string]*Session),
		teams:        make(map[string]*TeamStatus),
		workflowRuns: make(map[string]*WorkflowRun),
		loops:        make(map[string]*LoopRun),
		noopDetector: nil,
	}
	count := m.ConsecutiveNoOps("any-loop")
	if count != 0 {
		t.Errorf("ConsecutiveNoOps with nil detector should be 0, got %d", count)
	}
}

func TestTotalPrunedThisSession_Default(t *testing.T) {
	m := NewManager()
	if m.TotalPrunedThisSession() != 0 {
		t.Errorf("TotalPrunedThisSession should be 0 initially, got %d", m.TotalPrunedThisSession())
	}
}

func TestNewManagerWithBus(t *testing.T) {
	bus := events.NewBus(100)
	m := NewManagerWithBus(bus)
	if m == nil {
		t.Fatal("NewManagerWithBus returned nil")
	}
	if m.bus != bus {
		t.Error("bus should be set")
	}
	if m.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestSetStateDir(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()
	m.SetStateDir(dir)
	// Verify it was set by checking the internal field
	m.mu.RLock()
	got := m.stateDir
	m.mu.RUnlock()
	if got != dir {
		t.Errorf("stateDir = %q, want %q", got, dir)
	}
}
