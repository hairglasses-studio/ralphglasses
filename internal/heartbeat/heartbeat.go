// Package heartbeat provides a monitor that tracks the health of registered
// servers via periodic heartbeat signals. It is the foundation for the
// self-healing runtime: upstream packages use it to detect unresponsive
// servers and trigger recovery actions.
//
// The package has no dependencies beyond the Go standard library.
package heartbeat

import (
	"sync"
	"time"
)

// Status represents the current health state of a monitored server.
type Status struct {
	ServerName string
	LastSeen   time.Time
	Healthy    bool
	Latency    time.Duration
}

// Monitor tracks server health via periodic heartbeats.
// Servers must be registered before they can report heartbeats.
// A server is considered unhealthy if no heartbeat has been received
// within the configured timeout duration.
type Monitor struct {
	interval time.Duration
	timeout  time.Duration

	mu      sync.RWMutex
	servers map[string]*serverState
}

// serverState holds the internal tracking data for a single server.
type serverState struct {
	lastSeen   time.Time
	lastBeat   time.Time // timestamp of the previous heartbeat (for latency)
	registered time.Time
	healthy    bool
}

// New creates a Monitor that expects heartbeats at the given interval and
// marks a server as unhealthy after timeout elapses without a heartbeat.
//
// A typical configuration is interval=30s, timeout=90s (3x the interval).
func New(interval time.Duration, timeout time.Duration) *Monitor {
	return &Monitor{
		interval: interval,
		timeout:  timeout,
		servers:  make(map[string]*serverState),
	}
}

// Register adds a server to the monitor. The server starts in an unhealthy
// state until its first heartbeat is received.
func (m *Monitor) Register(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.servers[name] = &serverState{
		registered: now,
		healthy:    false,
	}
}

// Heartbeat records that the named server is alive. If the server has not
// been registered, the call is silently ignored.
func (m *Monitor) Heartbeat(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.servers[name]
	if !ok {
		return
	}

	now := time.Now()
	s.lastBeat = s.lastSeen
	s.lastSeen = now
	s.healthy = true
}

// Status returns the current health status of every registered server.
// A server is marked unhealthy if no heartbeat has been received within
// the timeout window. The returned slice is a snapshot; mutations do not
// affect the monitor's internal state.
func (m *Monitor) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.refreshHealthLocked(now)

	out := make([]Status, 0, len(m.servers))
	for name, s := range m.servers {
		var latency time.Duration
		if !s.lastBeat.IsZero() && !s.lastSeen.IsZero() {
			latency = s.lastSeen.Sub(s.lastBeat)
		}
		out = append(out, Status{
			ServerName: name,
			LastSeen:   s.lastSeen,
			Healthy:    s.healthy,
			Latency:    latency,
		})
	}
	return out
}

// Unhealthy returns the names of all servers that have not sent a heartbeat
// within the timeout window. The order is non-deterministic.
func (m *Monitor) Unhealthy() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.refreshHealthLocked(now)

	var names []string
	for name, s := range m.servers {
		if !s.healthy {
			names = append(names, name)
		}
	}
	return names
}

// refreshHealthLocked recalculates the healthy flag for every server.
// The caller must hold m.mu.
func (m *Monitor) refreshHealthLocked(now time.Time) {
	for _, s := range m.servers {
		if s.lastSeen.IsZero() {
			// Never received a heartbeat.
			s.healthy = false
			continue
		}
		if now.Sub(s.lastSeen) > m.timeout {
			s.healthy = false
		}
	}
}
