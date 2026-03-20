package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// HookDef defines a single hook to execute on an event.
type HookDef struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Sync    bool   `yaml:"sync"`    // if true, blocks the action
	Timeout int    `yaml:"timeout"` // seconds (default: 5 sync, 30 async)
}

// HookConfig maps event types to hook definitions.
type HookConfig struct {
	Hooks map[events.EventType][]HookDef `yaml:"hooks"`
}

// Executor subscribes to the event bus and runs hooks.
type Executor struct {
	bus     *events.Bus
	configs map[string]*HookConfig // keyed by repo path
	mu      sync.RWMutex
	cancel  context.CancelFunc
}

// NewExecutor creates a hook executor wired to the given event bus.
func NewExecutor(bus *events.Bus) *Executor {
	return &Executor{
		bus:     bus,
		configs: make(map[string]*HookConfig),
	}
}

// LoadConfig reads .ralph/hooks.yaml for a repo.
func (e *Executor) LoadConfig(repoPath string) error {
	hooksFile := filepath.Join(repoPath, ".ralph", "hooks.yaml")
	data, err := os.ReadFile(hooksFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no hooks configured
		}
		return fmt.Errorf("read hooks config: %w", err)
	}

	var cfg HookConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse hooks config: %w", err)
	}

	e.mu.Lock()
	e.configs[repoPath] = &cfg
	e.mu.Unlock()
	return nil
}

// Start subscribes to the event bus and dispatches hooks.
func (e *Executor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel

	ch := e.bus.Subscribe("hooks-executor")
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				e.dispatch(event)
			}
		}
	}()
}

// Stop unsubscribes and shuts down the executor.
func (e *Executor) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.bus.Unsubscribe("hooks-executor")
}

func (e *Executor) dispatch(event events.Event) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for repoPath, cfg := range e.configs {
		hooks, ok := cfg.Hooks[event.Type]
		if !ok {
			continue
		}
		// Only run hooks for events matching this repo (or global events)
		if event.RepoPath != "" && event.RepoPath != repoPath {
			continue
		}
		for _, h := range hooks {
			e.runHook(h, event, repoPath)
		}
	}
}

func (e *Executor) runHook(h HookDef, event events.Event, repoPath string) {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout == 0 {
		if h.Sync {
			timeout = 5 * time.Second
		} else {
			timeout = 30 * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	run := func() {
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", h.Command)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(),
			"RALPH_EVENT_TYPE="+string(event.Type),
			"RALPH_REPO_NAME="+event.RepoName,
			"RALPH_REPO_PATH="+event.RepoPath,
			"RALPH_SESSION_ID="+event.SessionID,
			"RALPH_PROVIDER="+event.Provider,
		)
		_ = cmd.Run()
	}

	if h.Sync {
		run()
	} else {
		go run()
	}
}
