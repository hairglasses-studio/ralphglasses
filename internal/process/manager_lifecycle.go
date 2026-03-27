package process

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

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

// Stop initiates a graceful shutdown of the ralph loop process using a
// fallback kill sequence: SIGTERM primary -> wait -> SIGTERM children ->
// wait -> SIGKILL any survivors. The wait duration uses the per-process
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
