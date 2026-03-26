package process

import (
	"context"
	"errors"
	"fmt"
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
// The provided context is used with exec.CommandContext so that cancelling the
// context will kill the launched process and prevent auto-restarts.
func (m *Manager) Start(ctx context.Context, repoPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.procs[repoPath]; ok {
		return fmt.Errorf("%w: %s", ErrAlreadyRunning, filepath.Base(repoPath))
	}

	// Look for ralph_loop.sh in the repo directory.
	loopScript := filepath.Join(repoPath, "ralph_loop.sh")
	if _, err := os.Stat(loopScript); err != nil {
		return fmt.Errorf("%w: %s — use the native Go loop via session manager instead", ErrNoLoopScript, filepath.Base(repoPath))
	}
	cmd := exec.CommandContext(ctx, "bash", loopScript)
	cmd.Dir = repoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start loop: %w", err)
	}

	pid := cmd.Process.Pid
	if err := writePIDFile(repoPath, pid); err != nil {
		slog.Warn("failed to write PID file", "repo", repoPath, "pid", pid, "error", err)
	}

	m.procs[repoPath] = &ManagedProcess{Cmd: cmd, PID: pid, ChildPids: collectChildPIDs(pid)}

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
	go m.reaperLoop(ctx, rp, cmd)

	return nil
}

// reaperLoop waits for the process to exit and handles auto-restart with
// exponential backoff when AutoRestart is enabled. Context cancellation
// prevents auto-restarts and triggers cleanup.
func (m *Manager) reaperLoop(ctx context.Context, rp string, cmd *exec.Cmd) {
	for {
		waitErr := cmd.Wait()
		exitCode := 0
		exitErrStr := ""
		if waitErr != nil {
			exitErrStr = waitErr.Error()
		}
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		}
		lastExits.Lock()
		lastExits.m[rp] = exitStatus{Code: exitCode, Error: exitErrStr}
		lastExits.Unlock()

		// If Stop() has claimed this process, skip auto-restart and go
		// straight to cleanup. The stopping flag prevents the reaper from
		// racing with Stop's kill goroutine by ensuring we never attempt
		// to restart a process that is being intentionally shut down.
		m.mu.Lock()
		mpCheck, tracked := m.procs[rp]
		isStopping := tracked && mpCheck.Stopping
		m.mu.Unlock()
		if isStopping {
			goto cleanup
		}

		// Auto-restart: only for non-zero exit codes (not signal kills which yield -1).
		// Skip auto-restart if context is cancelled.
		if exitCode > 0 && m.AutoRestart && ctx.Err() == nil {
			m.mu.Lock()
			mp, stillTracked := m.procs[rp]
			if stillTracked && !mp.Stopping && mp.RestartCount < m.maxRestartsLimit() {
				restartCount := mp.RestartCount
				m.mu.Unlock()

				// Exponential backoff: 1s, 2s, 4s, 8s, ...
				backoff := time.Duration(1<<restartCount) * time.Second
				slog.Info("auto-restarting crashed process",
					"repo", filepath.Base(rp),
					"exit_code", exitCode,
					"restart", restartCount+1,
					"backoff", backoff,
				)
				sleepFn(backoff)

				// Check context again after backoff sleep.
				if ctx.Err() != nil {
					// Context cancelled during backoff — skip restart, fall through to cleanup.
					goto cleanup
				}

				// Re-check stopping flag after backoff — Stop() or StopAll()
				// may have been called while we were sleeping. If the entry
				// was deleted (StopAll) or marked as stopping, bail out to
				// avoid launching an orphaned process.
				m.mu.Lock()
				mp2, ok2 := m.procs[rp]
				if !ok2 || mp2.Stopping {
					m.mu.Unlock()
					goto cleanup
				}
				m.mu.Unlock()

				// Re-launch the same command with context.
				loopScript := filepath.Join(rp, "ralph_loop.sh")
				newCmd := exec.CommandContext(ctx, "bash", loopScript)
				newCmd.Dir = rp
				newCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

				if err := newCmd.Start(); err != nil {
					slog.Warn("auto-restart failed", "repo", filepath.Base(rp), "err", err)
					// Fall through to normal cleanup below.
				} else {
					newPID := newCmd.Process.Pid
					if err := writePIDFile(rp, newPID); err != nil {
						slog.Warn("failed to write PID file after restart", "repo", rp, "pid", newPID, "error", err)
					}

					m.mu.Lock()
					// Check that the entry still exists and Stop hasn't claimed it.
					// If the entry was removed (StopAll) or marked stopping while
					// we were launching, kill the newly started process to prevent
					// an untracked orphan.
					if existing, ok := m.procs[rp]; ok && !existing.Stopping {
						m.procs[rp] = &ManagedProcess{
							Cmd:          newCmd,
							PID:          newPID,
							ChildPids:    collectChildPIDs(newPID),
							RestartCount: restartCount + 1,
						}
					} else {
						// Entry gone or stopping — kill the orphan we just spawned.
						m.mu.Unlock()
						_ = killPid(newPID, syscall.SIGKILL)
						goto cleanup
					}
					m.mu.Unlock()

					// Publish restart event.
					if m.bus != nil {
						m.bus.Publish(events.Event{
							Type:     events.LoopRestarted,
							RepoPath: rp,
							RepoName: filepath.Base(rp),
							Data: map[string]any{
								"restart_count": restartCount + 1,
								"exit_code":     exitCode,
								"backoff_ms":    backoff.Milliseconds(),
							},
						})
					}

					// Continue reaper loop watching the new process.
					cmd = newCmd
					continue
				}
			} else {
				m.mu.Unlock()
			}
		}

	cleanup:
		// Normal cleanup: remove from map and PID file.
		m.mu.Lock()
		delete(m.procs, rp)
		m.mu.Unlock()
		removePIDFile(rp)

		// Notify TUI of every exit via ProcessExitMsg.
		exitCodeForMsg := 0
		if waitErr != nil {
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				exitCodeForMsg = exitErr.ExitCode()
			} else {
				exitCodeForMsg = -1
			}
		}
		select {
		case m.exitCh <- ProcessExitMsg{RepoPath: rp, ExitCode: exitCodeForMsg, Error: waitErr}:
		default: // drop if channel is full
		}

		// Notify TUI of unexpected failures (exit code > 0; signal kills yield -1).
		if exitCode > 0 {
			select {
			case m.errCh <- ProcessErrorMsg{RepoPath: rp, Err: fmt.Errorf("process exited %d: %s", exitCode, exitErrStr)}:
			default: // drop if channel is full
			}
		}

		// Publish loop stopped event.
		if m.bus != nil {
			m.bus.Publish(events.Event{
				Type:     events.LoopStopped,
				RepoPath: rp,
				RepoName: filepath.Base(rp),
				Data:     map[string]any{"exit_code": exitCode, "error": exitErrStr},
			})
		}

		return // exit the reaper loop
	}
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

// Stop initiates a graceful shutdown of the ralph loop process using a
// fallback kill sequence: SIGTERM primary → wait → SIGTERM children →
// wait → SIGKILL any survivors. The wait duration uses the per-process
// KillTimeout if set, otherwise the Manager.KillTimeout, defaulting to
// DefaultKillTimeout (10s). The sequence runs in a background goroutine
// so Stop returns immediately.
func (m *Manager) Stop(ctx context.Context, repoPath string) error {
	m.mu.Lock()
	mp, ok := m.procs[repoPath]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNotRunning, filepath.Base(repoPath))
	}
	pid := mp.PID
	childPids := append([]int(nil), mp.ChildPids...)
	recovered := mp.Recovered

	// Signal the reaper that Stop owns this process's lifecycle.
	// The reaper will skip cleanup/restart for processes marked as stopping.
	mp.Stopping = true

	// Prefer per-process KillTimeout over manager-level KillTimeout.
	procTimeout := mp.KillTimeout

	// For recovered processes, clean up immediately (no reaper goroutine).
	if recovered {
		removePIDFile(repoPath)
		delete(m.procs, repoPath)
	}
	m.mu.Unlock()

	timeout := m.killTimeoutWithOverride(procTimeout)
	go func() {
		runKillSequence(pid, childPids, timeout)
		// Post-stop orphan audit: detect lingering ralph_loop processes.
		m.auditOrphanedProcesses()
	}()
	return nil
}

// killTimeout returns the configured kill timeout, defaulting to DefaultKillTimeout if zero.
func (m *Manager) killTimeout() time.Duration {
	if m.KillTimeout <= 0 {
		return DefaultKillTimeout
	}
	return m.KillTimeout
}

// killTimeoutWithOverride returns the per-process timeout if set, otherwise
// falls back to the manager-level killTimeout.
func (m *Manager) killTimeoutWithOverride(perProcess time.Duration) time.Duration {
	if perProcess > 0 {
		return perProcess
	}
	return m.killTimeout()
}

// runKillSequence executes the escalating shutdown sequence:
//  1. SIGTERM to the primary PID
//  2. Wait timeout
//  3. SIGTERM to all known child PIDs
//  4. Wait timeout
//  5. SIGKILL to any PIDs still alive
func runKillSequence(pid int, childPids []int, timeout time.Duration) {
	if timeout <= 0 {
		timeout = DefaultKillTimeout
	}

	// Step 1: SIGTERM to primary PID.
	if aliveFn(pid) {
		_ = killPid(pid, syscall.SIGTERM)
	}

	// Step 2: Wait for primary to exit.
	sleepFn(timeout)

	// Step 3: SIGTERM to child PIDs.
	for _, cpid := range childPids {
		if aliveFn(cpid) {
			_ = killPid(cpid, syscall.SIGTERM)
		}
	}

	// Step 4: Wait for children to exit.
	sleepFn(timeout)

	// Step 5: SIGKILL any survivors.
	allPids := append([]int{pid}, childPids...)
	for _, p := range allPids {
		if aliveFn(p) {
			_ = killPid(p, syscall.SIGKILL)
		}
	}
}

// TogglePause sends SIGSTOP or SIGCONT to pause/resume a loop.
func (m *Manager) TogglePause(repoPath string) (paused bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	mp, ok := m.procs[repoPath]
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrNotRunning, filepath.Base(repoPath))
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
// It marks each process as Stopping before signaling so that reaper
// goroutines will not race to auto-restart processes being shut down.
func (m *Manager) StopAll(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for path, mp := range m.procs {
		mp.Stopping = true
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

