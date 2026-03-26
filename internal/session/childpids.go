package session

import "github.com/hairglasses-studio/ralphglasses/internal/process"

// ChildPIDs collects the PIDs of all processes sharing the same process group
// as this session's CLI process, stores them in s.ChildPids, and returns them.
// Returns nil when the session has no live process.
func (s *Session) ChildPIDs() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}
	pids := process.CollectChildPIDs(s.cmd.Process.Pid)
	s.ChildPids = pids
	return pids
}
