package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

const pidFileName = "ralphglasses.pid"

// Manager tracks running ralph loop processes.
type Manager struct {
	mu    sync.Mutex
	procs map[string]*ManagedProcess // keyed by repo path
	bus   *events.Bus
}

// ManagedProcess wraps an os/exec.Cmd for a ralph loop.
type ManagedProcess struct {
	Cmd       *exec.Cmd
	Paused    bool
	PID       int  // stored at start time; safe to read under mu without racing Wait()
	Recovered bool // true if re-adopted from PID file (no reaper goroutine)
	ExitCode  int
	ExitError string
}

// lastExits tracks exit status after reaping (keyed by repo path).
var lastExits = struct {
	sync.Mutex
	m map[string]exitStatus
}{m: make(map[string]exitStatus)}

type exitStatus struct {
	Code  int
	Error string
}

// NewManager creates a new process manager.
func NewManager() *Manager {
	return &Manager{
		procs: make(map[string]*ManagedProcess),
	}
}

// NewManagerWithBus creates a process manager wired to an event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	return &Manager{
		procs: make(map[string]*ManagedProcess),
		bus:   bus,
	}
}

// pidFilePath returns the path to the PID file for a repo.
func pidFilePath(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", pidFileName)
}

// writePIDFile writes the PID to .ralph/ralphglasses.pid.
func writePIDFile(repoPath string, pid int) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(pidFilePath(repoPath), []byte(strconv.Itoa(pid)+"\n"), 0644)
}

// removePIDFile removes the PID file for a repo.
func removePIDFile(repoPath string) {
	_ = os.Remove(pidFilePath(repoPath))
}

// readPIDFile reads the PID from a repo's PID file. Returns 0 if not found or invalid.
func readPIDFile(repoPath string) int {
	data, err := os.ReadFile(pidFilePath(repoPath))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Use signal 0 to check liveness.
	return proc.Signal(syscall.Signal(0)) == nil
}

// Recover scans the given repo paths for stale PID files and re-adopts
// processes that are still alive. Removes PID files for dead processes.
func (m *Manager) Recover(repoPaths []string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	recovered := 0
	for _, repoPath := range repoPaths {
		if _, ok := m.procs[repoPath]; ok {
			continue // already managed
		}
		pid := readPIDFile(repoPath)
		if pid == 0 {
			continue
		}
		if !isProcessAlive(pid) {
			removePIDFile(repoPath)
			continue
		}
		m.procs[repoPath] = &ManagedProcess{
			PID:       pid,
			Recovered: true,
		}
		recovered++
	}
	return recovered
}

// Start launches a ralph loop in the given repo directory.
func (m *Manager) Start(repoPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.procs[repoPath]; ok {
		return fmt.Errorf("loop already running for %s", filepath.Base(repoPath))
	}

	// Look for ralph_loop.sh in the repo, then fall back to `ralph` on PATH.
	var cmd *exec.Cmd
	loopScript := filepath.Join(repoPath, "ralph_loop.sh")
	if _, err := os.Stat(loopScript); err == nil {
		cmd = exec.Command("bash", loopScript)
	} else {
		cmd = exec.Command("ralph")
	}
	cmd.Dir = repoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start loop: %w", err)
	}

	pid := cmd.Process.Pid
	_ = writePIDFile(repoPath, pid)

	m.procs[repoPath] = &ManagedProcess{Cmd: cmd, PID: pid}

	// Publish loop started event.
	if m.bus != nil {
		m.bus.Publish(events.Event{
			Type:     events.LoopStarted,
			RepoPath: repoPath,
			RepoName: filepath.Base(repoPath),
		})
	}

	// Reap the process in the background and clean up PID file.
	rp := repoPath
	go func() {
		err := cmd.Wait()
		exitCode := 0
		exitErr := ""
		if err != nil {
			exitErr = err.Error()
		}
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		lastExits.Lock()
		lastExits.m[rp] = exitStatus{Code: exitCode, Error: exitErr}
		lastExits.Unlock()

		m.mu.Lock()
		delete(m.procs, rp)
		m.mu.Unlock()
		removePIDFile(rp)

		// Publish loop stopped event.
		if m.bus != nil {
			m.bus.Publish(events.Event{
				Type:     events.LoopStopped,
				RepoPath: rp,
				RepoName: filepath.Base(rp),
				Data:     map[string]any{"exit_code": exitCode, "error": exitErr},
			})
		}
	}()

	return nil
}

// sendSignal sends a signal to a process, trying process group first.
func sendSignal(pid int, sig syscall.Signal) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return syscall.Kill(pid, sig)
	}
	return syscall.Kill(-pgid, sig)
}

// Stop sends SIGTERM to the ralph loop process group.
func (m *Manager) Stop(repoPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mp, ok := m.procs[repoPath]
	if !ok {
		return fmt.Errorf("no running loop for %s", filepath.Base(repoPath))
	}

	err := sendSignal(mp.PID, syscall.SIGTERM)

	// For recovered processes, clean up immediately (no reaper goroutine).
	if mp.Recovered {
		removePIDFile(repoPath)
		delete(m.procs, repoPath)
	}

	return err
}

// TogglePause sends SIGSTOP or SIGCONT to pause/resume a loop.
func (m *Manager) TogglePause(repoPath string) (paused bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mp, ok := m.procs[repoPath]
	if !ok {
		return false, fmt.Errorf("no running loop for %s", filepath.Base(repoPath))
	}

	if mp.Paused {
		err = syscall.Kill(mp.PID, syscall.SIGCONT)
		if err == nil {
			mp.Paused = false
		}
		return false, err
	}

	err = syscall.Kill(mp.PID, syscall.SIGSTOP)
	if err == nil {
		mp.Paused = true
	}
	return true, err
}

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

// StopAll sends SIGTERM to all managed processes.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for path, mp := range m.procs {
		_ = sendSignal(mp.PID, syscall.SIGTERM)
		removePIDFile(path)
		delete(m.procs, path)
	}
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
