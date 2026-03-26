package session

import (
	"os/exec"
	"syscall"
	"time"
)

// DefaultStallThreshold is the default duration after which a running session
// with no activity is considered stalled.
const DefaultStallThreshold = 5 * time.Minute

// DetectStalls returns the IDs of sessions that are in "running" state but
// have not recorded any activity within the given threshold duration.
func (m *Manager) DetectStalls(threshold time.Duration) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var stalled []string
	for _, s := range m.sessions {
		s.mu.Lock()
		isRunning := s.Status == StatusRunning
		elapsed := time.Since(s.LastActivity)
		s.mu.Unlock()

		if isRunning && elapsed > threshold {
			stalled = append(stalled, s.ID)
		}
	}
	return stalled
}

// killWithEscalation sends SIGTERM, waits up to timeout, then sends SIGKILL if still alive.
// Returns true if SIGKILL was needed.
//
// The done channel should be closed when the process has exited (typically by the
// runner goroutine that calls cmd.Wait()). If done is nil, killWithEscalation
// spawns its own Wait() goroutine internally.
func killWithEscalation(cmd *exec.Cmd, timeout time.Duration, done <-chan struct{}) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}

	// If no external done channel, create one by calling Wait() ourselves.
	if done == nil {
		ch := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(ch)
		}()
		done = ch
	}

	// Send SIGTERM to process group.
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	} else {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	}

	// Wait for the process to exit or timeout.
	select {
	case <-done:
		return false
	case <-time.After(timeout):
		// Escalate to SIGKILL.
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		return true
	}
}
