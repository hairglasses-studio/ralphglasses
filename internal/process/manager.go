package process

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

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

	m.procs[repoPath] = &ManagedProcess{Cmd: cmd}

	// Publish loop started event
	if m.bus != nil {
		m.bus.Publish(events.Event{
			Type:     events.LoopStarted,
			RepoPath: repoPath,
			RepoName: filepath.Base(repoPath),
		})
	}

	// Reap the process in the background.
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

		// Publish loop stopped event
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

// Stop sends SIGTERM to the ralph loop process group.
func (m *Manager) Stop(repoPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	mp, ok := m.procs[repoPath]
	if !ok {
		return fmt.Errorf("no running loop for %s", filepath.Base(repoPath))
	}

	pgid, err := syscall.Getpgid(mp.Cmd.Process.Pid)
	if err != nil {
		return mp.Cmd.Process.Signal(syscall.SIGTERM)
	}
	return syscall.Kill(-pgid, syscall.SIGTERM)
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
		err = mp.Cmd.Process.Signal(syscall.SIGCONT)
		if err == nil {
			mp.Paused = false
		}
		return false, err
	}

	err = mp.Cmd.Process.Signal(syscall.SIGSTOP)
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
		pgid, err := syscall.Getpgid(mp.Cmd.Process.Pid)
		if err != nil {
			_ = mp.Cmd.Process.Signal(syscall.SIGTERM)
		} else {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		}
		delete(m.procs, path)
	}
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
