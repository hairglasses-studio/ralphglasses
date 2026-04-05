// Package sandbox provides container isolation and resource limit enforcement for LLM sessions.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Sentinel errors returned by ResourceLimiter.
var (
	ErrCPUTimeExceeded   = errors.New("cpu time limit exceeded")
	ErrMemoryExceeded    = errors.New("memory limit exceeded")
	ErrFileCountExceeded = errors.New("file count limit exceeded")
)

// ResourceLimits specifies the configurable limits for a ResourceLimiter.
type ResourceLimits struct {
	// CPUTimeout is the maximum wall-clock time the wrapped command may run.
	// Enforced via context deadline. Zero means no CPU time limit.
	CPUTimeout time.Duration

	// MemoryLimitBytes is the maximum RSS (resident set size) in bytes.
	// Monitored via /proc/<pid>/status on Linux or ps on macOS.
	// Zero means no memory limit.
	MemoryLimitBytes int64

	// MaxFiles is the maximum number of files the process may create inside
	// the tracked directory. Zero means no file count limit.
	MaxFiles int

	// PollInterval controls how often memory and file counts are sampled.
	// Defaults to 250ms if zero.
	PollInterval time.Duration
}

// ResourceUsage captures the observed resource consumption of a completed run.
type ResourceUsage struct {
	// PeakRSSBytes is the highest RSS observed during polling.
	PeakRSSBytes int64

	// FilesCreated is the number of new files detected in the tracked directory.
	FilesCreated int

	// Duration is the wall-clock time the command ran.
	Duration time.Duration

	// LimitHit is non-nil if the process was killed for exceeding a limit.
	LimitHit error
}

// ResourceLimiter wraps an exec.Cmd and enforces configurable resource limits.
type ResourceLimiter struct {
	limits   ResourceLimits
	trackDir string // directory to monitor for file creation; empty disables

	// mu protects usage during concurrent polling.
	mu    sync.Mutex
	usage ResourceUsage

	// killed is set atomically when the process is terminated for a limit violation.
	killed atomic.Bool

	// rssReader is overridable for testing (returns RSS in bytes for a given PID).
	rssReader func(pid int) (int64, error)

	// fileCounter is overridable for testing (returns file count in trackDir).
	fileCounter func(dir string) (int, error)
}

// NewResourceLimiter creates a ResourceLimiter with the given limits.
// trackDir is the directory monitored for file creation; pass "" to disable file tracking.
func NewResourceLimiter(limits ResourceLimits, trackDir string) *ResourceLimiter {
	if limits.PollInterval <= 0 {
		limits.PollInterval = 250 * time.Millisecond
	}
	return &ResourceLimiter{
		limits:      limits,
		trackDir:    trackDir,
		rssReader:   readRSS,
		fileCounter: countFiles,
	}
}

// RunCmd executes the given command name and args under the configured resource
// limits. Internally it creates the process with exec.CommandContext so context
// cancellation (including the CPU timeout) terminates the child process.
func (rl *ResourceLimiter) RunCmd(ctx context.Context, name string, args ...string) (ResourceUsage, error) {
	// Apply CPU timeout via context deadline.
	if rl.limits.CPUTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, rl.limits.CPUTimeout)
		defer cancel()
	}

	// Snapshot initial file count in trackDir.
	var baseFileCount int
	if rl.limits.MaxFiles > 0 && rl.trackDir != "" {
		n, err := rl.fileCounter(rl.trackDir)
		if err != nil {
			return ResourceUsage{}, fmt.Errorf("counting files in %s: %w", rl.trackDir, err)
		}
		baseFileCount = n
	}

	cmd := exec.CommandContext(ctx, name, args...)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return ResourceUsage{}, fmt.Errorf("starting command: %w", err)
	}

	// Start background monitor goroutine.
	monitorDone := make(chan struct{})
	go rl.monitor(ctx, cmd, baseFileCount, monitorDone)

	// Wait for the command to finish.
	cmdErr := cmd.Wait()
	close(monitorDone)

	rl.mu.Lock()
	rl.usage.Duration = time.Since(start)
	usage := rl.usage
	rl.mu.Unlock()

	// If we killed the process, return the limit error.
	if usage.LimitHit != nil {
		return usage, usage.LimitHit
	}

	// If the context timed out, report CPU time exceeded.
	if ctx.Err() == context.DeadlineExceeded {
		usage.LimitHit = ErrCPUTimeExceeded
		return usage, ErrCPUTimeExceeded
	}

	return usage, cmdErr
}

// monitor polls resource usage and kills the process if limits are exceeded.
func (rl *ResourceLimiter) monitor(ctx context.Context, cmd *exec.Cmd, baseFileCount int, done <-chan struct{}) {
	ticker := time.NewTicker(rl.limits.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cmd.Process == nil {
				continue
			}
			pid := cmd.Process.Pid

			// Check memory.
			if rl.limits.MemoryLimitBytes > 0 {
				rss, err := rl.rssReader(pid)
				if err == nil {
					rl.mu.Lock()
					if rss > rl.usage.PeakRSSBytes {
						rl.usage.PeakRSSBytes = rss
					}
					rl.mu.Unlock()

					if rss > rl.limits.MemoryLimitBytes {
						rl.killForLimit(cmd, ErrMemoryExceeded)
						return
					}
				}
				// Ignore transient read errors (process may have exited).
			}

			// Check file count.
			if rl.limits.MaxFiles > 0 && rl.trackDir != "" {
				n, err := rl.fileCounter(rl.trackDir)
				if err == nil {
					created := max(n-baseFileCount, 0)

					rl.mu.Lock()
					rl.usage.FilesCreated = created
					rl.mu.Unlock()

					if created > rl.limits.MaxFiles {
						rl.killForLimit(cmd, ErrFileCountExceeded)
						return
					}
				}
			}
		}
	}
}

// killForLimit terminates the process and records which limit was hit.
func (rl *ResourceLimiter) killForLimit(cmd *exec.Cmd, limitErr error) {
	if !rl.killed.CompareAndSwap(false, true) {
		return // already killed
	}
	rl.mu.Lock()
	rl.usage.LimitHit = limitErr
	rl.mu.Unlock()

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Usage returns the current resource usage snapshot. Safe to call after Run returns.
func (rl *ResourceLimiter) Usage() ResourceUsage {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.usage
}

// --- Platform-specific RSS reading ---

// readRSS returns the RSS (resident set size) in bytes for the given PID.
// On Linux it reads /proc/<pid>/status; on macOS it uses ps.
func readRSS(pid int) (int64, error) {
	switch runtime.GOOS {
	case "linux":
		return readRSSLinux(pid)
	case "darwin":
		return readRSSDarwin(pid)
	default:
		return 0, fmt.Errorf("unsupported platform for RSS reading: %s", runtime.GOOS)
	}
}

// readRSSLinux reads VmRSS from /proc/<pid>/status (in kB, returned as bytes).
func readRSSLinux(pid int) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, fmt.Errorf("reading /proc/%d/status: %w", pid, err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("unexpected VmRSS format: %q", line)
			}
			kb, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parsing VmRSS value %q: %w", fields[1], err)
			}
			return kb * 1024, nil // kB to bytes
		}
	}
	return 0, fmt.Errorf("VmRSS not found in /proc/%d/status", pid)
}

// readRSSDarwin reads RSS via ps (in bytes; ps -o rss= reports kB on macOS).
func readRSSDarwin(pid int) (int64, error) {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, fmt.Errorf("ps for pid %d: %w", pid, err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("empty ps output for pid %d", pid)
	}
	kb, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing ps rss %q: %w", s, err)
	}
	return kb * 1024, nil // kB to bytes
}

// --- File counting ---

// countFiles returns the number of regular files in dir (non-recursive).
func countFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading directory %s: %w", dir, err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	return count, nil
}
