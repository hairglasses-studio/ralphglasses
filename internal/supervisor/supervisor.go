// Package supervisor provides process crash detection and automatic restart
// with exponential backoff. It is designed for supervising MCP server processes
// managed by ralphglasses.
package supervisor

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Config controls restart behavior for supervised processes.
type Config struct {
	MaxRestarts   int           // max restarts before giving up (0 = never restart)
	RestartDelay  time.Duration // initial delay between restarts
	BackoffFactor float64       // exponential backoff multiplier (e.g., 2.0)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRestarts:   3,
		RestartDelay:  time.Second,
		BackoffFactor: 2.0,
	}
}

// processState tracks the state of a single supervised process.
type processState struct {
	name         string
	cmdFactory   func() *exec.Cmd // creates a fresh Cmd for restarts
	cmd          *exec.Cmd        // current running command
	restarts     int
	stopped      bool   // true when Stop() has been called
	done         chan struct{} // closed when the supervision goroutine exits
}

// Supervisor watches processes and restarts them on crash with exponential backoff.
type Supervisor struct {
	cfg   Config
	mu    sync.Mutex
	procs map[string]*processState
}

// New creates a Supervisor with the given configuration.
func New(cfg Config) *Supervisor {
	if cfg.BackoffFactor < 1.0 {
		cfg.BackoffFactor = 1.0
	}
	return &Supervisor{
		cfg:   cfg,
		procs: make(map[string]*processState),
	}
}

// ErrMaxRestartsReached is returned when a process has crashed more times than
// Config.MaxRestarts allows.
var ErrMaxRestartsReached = errors.New("supervisor: max restarts reached")

// ErrAlreadyWatched is returned when Watch is called with a name that is
// already being supervised.
var ErrAlreadyWatched = errors.New("supervisor: process already watched")

// ErrNotFound is returned when Stop or RestartCount is called with an unknown name.
var ErrNotFound = errors.New("supervisor: process not found")

// Watch starts the given command and supervises it. If the process exits with a
// non-zero status, it is restarted up to Config.MaxRestarts times with
// exponential backoff. If the process exits with status 0, it is considered a
// clean exit and is not restarted.
//
// The provided cmd is used for the initial start. For subsequent restarts, a new
// exec.Cmd is created from the same Path and Args.
//
// Watch returns immediately after starting the process. Use the done channel
// or Running() to observe state.
func (s *Supervisor) Watch(name string, cmd *exec.Cmd) error {
	s.mu.Lock()
	if _, exists := s.procs[name]; exists {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrAlreadyWatched, name)
	}

	// Capture path and args so we can construct fresh Cmds for restarts.
	path := cmd.Path
	args := cmd.Args

	factory := func() *exec.Cmd {
		return exec.Command(path, args[1:]...)
	}

	ps := &processState{
		name:       name,
		cmdFactory: factory,
		cmd:        cmd,
		done:       make(chan struct{}),
	}
	s.procs[name] = ps
	s.mu.Unlock()

	// Start the initial process.
	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		delete(s.procs, name)
		s.mu.Unlock()
		return fmt.Errorf("supervisor: start %s: %w", name, err)
	}

	go s.supervise(ps)
	return nil
}

// supervise runs in a goroutine and waits for the process to exit, restarting
// it on crash up to MaxRestarts times.
func (s *Supervisor) supervise(ps *processState) {
	defer close(ps.done)

	delay := s.cfg.RestartDelay

	for {
		// Wait for the current process to exit.
		err := ps.cmd.Wait()

		s.mu.Lock()
		if ps.stopped {
			// Stop() was called; do not restart.
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		// Clean exit (status 0): no restart needed.
		if err == nil {
			return
		}

		// Process crashed. Check if we can restart.
		s.mu.Lock()
		if ps.restarts >= s.cfg.MaxRestarts {
			s.mu.Unlock()
			return
		}
		ps.restarts++
		currentDelay := delay
		s.mu.Unlock()

		// Backoff delay before restart.
		time.Sleep(currentDelay)
		delay = time.Duration(float64(delay) * s.cfg.BackoffFactor)

		// Check again after sleeping — Stop() may have been called.
		s.mu.Lock()
		if ps.stopped {
			s.mu.Unlock()
			return
		}

		// Create and start a fresh command.
		newCmd := ps.cmdFactory()
		ps.cmd = newCmd
		s.mu.Unlock()

		if startErr := newCmd.Start(); startErr != nil {
			// Cannot restart; give up.
			return
		}
	}
}

// Stop stops supervising the named process and kills it if running. It blocks
// until the supervision goroutine has exited.
func (s *Supervisor) Stop(name string) error {
	s.mu.Lock()
	ps, ok := s.procs[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	ps.stopped = true
	cmd := ps.cmd
	s.mu.Unlock()

	// Kill the process. Ignore errors — it may have already exited.
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}

	// Wait for the supervision goroutine to finish.
	<-ps.done

	// Remove from tracked processes.
	s.mu.Lock()
	delete(s.procs, name)
	s.mu.Unlock()

	return nil
}

// RestartCount returns how many times the named process has been restarted.
func (s *Supervisor) RestartCount(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, ok := s.procs[name]
	if !ok {
		return 0
	}
	return ps.restarts
}

// Running returns the names of all currently supervised processes.
func (s *Supervisor) Running() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.procs))
	for name := range s.procs {
		names = append(names, name)
	}
	return names
}
