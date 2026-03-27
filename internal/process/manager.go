package process

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

const pidFileName = "ralphglasses.pid"

// ProcessErrorMsg is a tea.Msg delivered when a managed process exits with a
// non-zero status. The TUI can wrap ErrorChan in a tea.Cmd to receive it.
type ProcessErrorMsg struct {
	RepoPath string
	Err      error
}

// ProcessExitMsg is a tea.Msg delivered when any managed process exits,
// regardless of exit code. Use WaitForProcessExit to receive it in the TUI.
type ProcessExitMsg struct {
	RepoPath string
	ExitCode int
	Error    error
}

// WaitForProcessExit returns a tea.Cmd that blocks until the next ProcessExitMsg
// is available on ch. Wire this in Init and re-issue it in the Update handler.
func WaitForProcessExit(ch <-chan ProcessExitMsg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

// Manager tracks running ralph loop processes.
type Manager struct {
	mu          sync.Mutex
	procs       map[string]*ManagedProcess // keyed by repo path
	bus         *events.Bus
	errCh       chan ProcessErrorMsg
	exitCh      chan ProcessExitMsg
	KillTimeout time.Duration // timeout between kill sequence stages; defaults to 10s
	AutoRestart bool          // if true, crashed processes are restarted with backoff
	MaxRestarts int           // maximum restart attempts before giving up (default 3)
}

// ManagedProcess wraps an os/exec.Cmd for a ralph loop.
type ManagedProcess struct {
	Cmd          *exec.Cmd
	Paused       bool
	PID          int           // stored at start time; safe to read under mu without racing Wait()
	ChildPids    []int         // child PIDs collected at launch (best-effort, never nil)
	Recovered    bool          // true if re-adopted from PID file (no reaper goroutine)
	Stopping     bool          // true while Stop() owns this process's shutdown lifecycle
	ExitCode     int
	ExitError    string
	RestartCount int           // number of times this process has been auto-restarted
	KillTimeout  time.Duration // per-process kill timeout; zero means use Manager.KillTimeout
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

// DefaultKillTimeout is the default timeout between kill sequence stages.
const DefaultKillTimeout = 10 * time.Second

// DefaultMaxRestarts is the default maximum restart attempts for auto-restart.
const DefaultMaxRestarts = 3

// NewManager creates a new process manager.
func NewManager() *Manager {
	return &Manager{
		procs:       make(map[string]*ManagedProcess),
		errCh:       make(chan ProcessErrorMsg, 16),
		exitCh:      make(chan ProcessExitMsg, 16),
		KillTimeout: DefaultKillTimeout,
		MaxRestarts: DefaultMaxRestarts,
	}
}

// NewManagerWithBus creates a process manager wired to an event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	return &Manager{
		procs:       make(map[string]*ManagedProcess),
		bus:         bus,
		errCh:       make(chan ProcessErrorMsg, 16),
		exitCh:      make(chan ProcessExitMsg, 16),
		KillTimeout: DefaultKillTimeout,
		MaxRestarts: DefaultMaxRestarts,
	}
}

// maxRestartsLimit returns the effective max restarts, defaulting to DefaultMaxRestarts.
func (m *Manager) maxRestartsLimit() int {
	if m.MaxRestarts <= 0 {
		return DefaultMaxRestarts
	}
	return m.MaxRestarts
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
	if err := os.Remove(pidFilePath(repoPath)); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove PID file", "repo", repoPath, "error", err)
	}
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

// Package-level indirections for testing.
var (
	getpgid = syscall.Getpgid

	// killPidPtr is an atomic pointer to the kill function, allowing
	// tests to stub it without data races against background goroutines.
	killPidPtr atomic.Pointer[func(int, syscall.Signal) error]

	// aliveFnPtr is an atomic pointer to the process-alive check function,
	// allowing tests to stub it without data races against background goroutines.
	aliveFnPtr atomic.Pointer[func(int) bool]

	// sleepFnPtr is an atomic pointer to the sleep function, allowing
	// tests to stub it without data races against background goroutines.
	sleepFnPtr atomic.Pointer[func(time.Duration)]
)

func init() {
	killFn := func(pid int, sig syscall.Signal) error { return syscall.Kill(pid, sig) }
	killPidPtr.Store(&killFn)

	aFn := isProcessAlive
	aliveFnPtr.Store(&aFn)

	sleepFn := time.Sleep
	sleepFnPtr.Store(&sleepFn)
}

// killPid loads the current kill function atomically.
func killPid(pid int, sig syscall.Signal) error {
	return (*killPidPtr.Load())(pid, sig)
}

// setKillPid atomically replaces the kill function (for testing).
func setKillPid(fn func(int, syscall.Signal) error) {
	killPidPtr.Store(&fn)
}

// aliveFn loads the current alive-check function atomically.
func aliveFn(pid int) bool {
	return (*aliveFnPtr.Load())(pid)
}

// setAliveFn atomically replaces the alive-check function (for testing).
func setAliveFn(fn func(int) bool) {
	aliveFnPtr.Store(&fn)
}

// sleepFn loads the current sleep function atomically.
func sleepFn(d time.Duration) {
	(*sleepFnPtr.Load())(d)
}

// setSleepFn atomically replaces the sleep function (for testing).
func setSleepFn(fn func(time.Duration)) {
	sleepFnPtr.Store(&fn)
}

// sendSignal sends a signal to a process, trying process group first.
func sendSignal(pid int, sig syscall.Signal) error {
	pgid, err := getpgid(pid)
	if err != nil {
		slog.Warn("Getpgid failed, falling back to direct PID signal", "pid", pid, "err", err)
		return syscall.Kill(pid, sig)
	}
	return syscall.Kill(-pgid, sig)
}
