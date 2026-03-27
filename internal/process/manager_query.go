package process

// IsRunning checks if a loop is managed for the given repo.
func (m *Manager) IsRunning(repoPath string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.procs[repoPath]
	return ok
}

// IsPaused checks if a managed loop is paused.
func (m *Manager) IsPaused(repoPath string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	mp, ok := m.procs[repoPath]
	return ok && mp.Paused
}

// AddProcForTesting inserts a stub ManagedProcess entry so IsRunning/IsPaused
// return expected values in tests. The entry has no real OS process.
func (m *Manager) AddProcForTesting(repoPath string, paused bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.procs[repoPath] = &ManagedProcess{Paused: paused}
}

// PidForRepo returns the PID of the managed process for a repo, or 0 if not managed.
func (m *Manager) PidForRepo(repoPath string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	mp, ok := m.procs[repoPath]
	if !ok {
		return 0
	}
	return mp.PID
}

// LastExitStatus returns the exit code and error for a previously reaped process.
func (m *Manager) LastExitStatus(repoPath string) (int, string, bool) {
	lastExits.Lock()
	defer lastExits.Unlock()
	es, ok := lastExits.m[repoPath]
	if !ok {
		return 0, "", false
	}
	return es.Code, es.Error, true
}

// CleanStalePIDFiles removes PID files for dead processes across the given repo paths.
func CleanStalePIDFiles(repoPaths []string) int {
	cleaned := 0
	for _, repoPath := range repoPaths {
		pid := readPIDFile(repoPath)
		if pid == 0 {
			continue
		}
		if !isProcessAlive(pid) {
			removePIDFile(repoPath)
			cleaned++
		}
	}
	return cleaned
}

// RunningPaths returns the paths of all running loops.
func (m *Manager) RunningPaths() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	paths := make([]string, 0, len(m.procs))
	for p := range m.procs {
		paths = append(paths, p)
	}
	return paths
}

// ErrorChan returns a channel that receives ProcessErrorMsg when a managed
// process exits with a non-zero status. Wrap with a tea.Cmd to handle in the TUI.
func (m *Manager) ErrorChan() <-chan ProcessErrorMsg {
	return m.errCh
}

// ExitChan returns a channel that receives ProcessExitMsg on every managed
// process exit. Use WaitForProcessExit to consume it as a tea.Cmd.
func (m *Manager) ExitChan() <-chan ProcessExitMsg {
	return m.exitCh
}
