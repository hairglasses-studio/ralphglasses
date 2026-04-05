// Package recovery provides a thread-safe state machine for tracking and
// managing system recovery from various failure modes (server crashes,
// network timeouts, JSON parsing failures). The machine transitions between
// Healthy, Degraded, Recovering, and Failed states based on incoming events.
package recovery

import "sync"

// State represents the current recovery state of the system.
type State int

const (
	// StateHealthy indicates all servers are healthy and operational.
	StateHealthy State = iota
	// StateDegraded indicates some servers are unhealthy but the system
	// remains operational.
	StateDegraded
	// StateRecovering indicates the system is actively trying to restore
	// failed servers.
	StateRecovering
	// StateFailed indicates max retries have been exceeded and manual
	// intervention is required.
	StateFailed
)

// String returns the human-readable name for a State.
func (s State) String() string {
	switch s {
	case StateHealthy:
		return "Healthy"
	case StateDegraded:
		return "Degraded"
	case StateRecovering:
		return "Recovering"
	case StateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// Event represents an occurrence that may trigger a state transition.
type Event int

const (
	// EventServerDown indicates a server has become unreachable.
	EventServerDown Event = iota
	// EventServerUp indicates a previously failed server has recovered.
	EventServerUp
	// EventRetryExhausted indicates all retry attempts have been used.
	EventRetryExhausted
	// EventManualReset indicates an operator has manually reset the system.
	EventManualReset
)

// String returns the human-readable name for an Event.
func (e Event) String() string {
	switch e {
	case EventServerDown:
		return "ServerDown"
	case EventServerUp:
		return "ServerUp"
	case EventRetryExhausted:
		return "RetryExhausted"
	case EventManualReset:
		return "ManualReset"
	default:
		return "Unknown"
	}
}

// Machine is a thread-safe state machine that tracks recovery state and
// failure counts. It supports transition callbacks for observability.
type Machine struct {
	state        State
	failures     int
	maxFailures  int
	mu           sync.RWMutex
	onTransition func(from, to State)
}

// New creates a new recovery Machine starting in StateHealthy.
// maxFailures controls how many accumulated failures trigger the transition
// from Degraded to Recovering.
func New(maxFailures int) *Machine {
	if maxFailures < 1 {
		maxFailures = 1
	}
	return &Machine{
		state:       StateHealthy,
		maxFailures: maxFailures,
	}
}

// State returns the current state of the machine. Safe for concurrent use.
func (m *Machine) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Failures returns the current failure count. Safe for concurrent use.
func (m *Machine) Failures() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.failures
}

// Handle processes an event and returns the resulting state. The transition
// rules are:
//
//	Healthy  + ServerDown      → Degraded   (failures incremented)
//	Degraded + ServerDown      → Recovering (if failures >= maxFailures)
//	Degraded + ServerDown      → Degraded   (failures incremented)
//	Recovering + RetryExhausted → Failed
//	Failed   + ManualReset     → Healthy    (failures reset)
//	Any      + ServerUp        → move toward Healthy (failures decremented)
//	Any      + ManualReset     → Healthy    (failures reset)
func (m *Machine) Handle(event Event) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	from := m.state

	switch event {
	case EventServerDown:
		m.handleServerDown()
	case EventServerUp:
		m.handleServerUp()
	case EventRetryExhausted:
		m.handleRetryExhausted()
	case EventManualReset:
		m.handleManualReset()
	}

	to := m.state
	if from != to && m.onTransition != nil {
		m.onTransition(from, to)
	}
	return m.state
}

// OnTransition registers a callback that fires whenever the machine
// transitions between states. The callback receives the old and new states.
// Only one callback is active at a time; subsequent calls replace the
// previous callback.
func (m *Machine) OnTransition(fn func(from, to State)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onTransition = fn
}

// Reset unconditionally returns the machine to StateHealthy and zeroes the
// failure counter. This is equivalent to Handle(EventManualReset) but is
// provided as a convenience method.
func (m *Machine) Reset() {
	m.Handle(EventManualReset)
}

// --- internal transition handlers (caller must hold m.mu) ---

func (m *Machine) handleServerDown() {
	m.failures++
	switch m.state {
	case StateHealthy:
		m.state = StateDegraded
	case StateDegraded:
		if m.failures >= m.maxFailures {
			m.state = StateRecovering
		}
	case StateRecovering, StateFailed:
		// already in a bad state; just count the failure
	}
}

func (m *Machine) handleServerUp() {
	if m.failures > 0 {
		m.failures--
	}
	switch m.state {
	case StateDegraded:
		if m.failures == 0 {
			m.state = StateHealthy
		}
	case StateRecovering:
		if m.failures == 0 {
			m.state = StateHealthy
		} else {
			m.state = StateDegraded
		}
	case StateFailed:
		// ServerUp alone cannot exit Failed; require ManualReset.
	case StateHealthy:
		// already healthy
	}
}

func (m *Machine) handleRetryExhausted() {
	if m.state == StateRecovering {
		m.state = StateFailed
	}
}

func (m *Machine) handleManualReset() {
	m.failures = 0
	m.state = StateHealthy
}
