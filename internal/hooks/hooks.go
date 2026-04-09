package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// SECURITY: The Command field in hooks.yaml is executed directly by the system shell
// without sanitization. The .ralph/hooks.yaml file must only contain trusted content
// and its modification should be tightly controlled.
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

	for eventType, hooks := range cfg.Hooks {
		for i, h := range hooks {
			if strings.ContainsAny(h.Command, ";|&`$(){}") {
				return fmt.Errorf("hook %q for event %s contains shell metacharacters", h.Name, eventType)
			}
			if h.Name == "" {
				return fmt.Errorf("hook at index %d for event %s has empty name", i, eventType)
			}
		}
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
				if ok == false {
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
		if ok == false {
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
	payload, hasPayload := toolHookPayload(event)
	payloadBody := []byte(nil)
	if hasPayload {
		body, err := payload.JSON()
		if err != nil {
			slog.Warn("hook payload marshal failed", "hook", h.Name, "error", err)
		} else {
			payloadBody = body
		}
	}

	sanitize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r < 32 || r == '=' || r == '\'' || r == '"' || r == '`' || r == '$' {
				return '_'
			}
			return r
		}, s)
	}

	run := func() {
		defer cancel()
		cmd := exec.Command("sh", "-c", h.Command)
		setCommandProcessGroup(cmd)
		cmd.Dir = repoPath

		env := append(os.Environ(),
			"RALPH_EVENT_TYPE="+sanitize(string(event.Type)),
			"RALPH_REPO_NAME="+sanitize(event.RepoName),
			"RALPH_REPO_PATH="+sanitize(event.RepoPath),
			"RALPH_SESSION_ID="+sanitize(event.SessionID),
			"RALPH_PROVIDER="+sanitize(event.Provider),
		)
		if hasPayload {
			for key, value := range payload.Env() {
				env = append(env, key+"="+value)
			}
		}
		cmd.Env = env
		if len(payloadBody) > 0 {
			cmd.Stdin = bytes.NewReader(payloadBody)
		}

		var stdout strings.Builder
		var stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Start(); err != nil {
			slog.Error("hook failed", "hook", h.Name, "error", err)
			return
		}
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		select {
		case err := <-done:
			if err != nil {
				if hasPayload && e.handleToolHookExit(err, h, event, repoPath, stdout.String(), stderr.String()) {
					return
				}
				slog.Error("hook failed", "hook", h.Name, "error", err)
				return
			}
			e.handleHookStdout(h, event, repoPath, stdout.String())
		case <-ctx.Done():
			_ = killCommandProcessGroup(cmd)
			<-done
			if ctx.Err() == context.DeadlineExceeded {
				slog.Error("hook failed", "hook", h.Name, "error", fmt.Errorf("hook %q timed out after %s: %w", h.Name, timeout, ctx.Err()))
				return
			}
			slog.Error("hook failed", "hook", h.Name, "error", ctx.Err())
		}
	}

	if h.Sync {
		run()
	} else {
		go run()
	}
}

func (e *Executor) handleHookStdout(h HookDef, event events.Event, repoPath, raw string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return
	}
	if strings.HasPrefix(trimmed, "{") == false {
		return
	}
	verdict, parseErr := ParseHookOutput([]byte(trimmed))
	if parseErr != nil {
		slog.Warn("hook stdout not valid verdict JSON", "hook", h.Name, "error", parseErr)
		return
	}
	if verdict.Decision == "block" {
		reason := verdict.Reason
		if reason == "" {
			reason = fmt.Sprintf("hook %q blocked event %s", h.Name, event.Type)
		}
		slog.Warn("hook verdict: blocked", "hook", h.Name, "reason", reason)
		e.publishHookBlocked(repoPath, event.RepoName, h.Name, reason)
		return
	}
	if verdict.Decision != "" {
		slog.Info("hook verdict", "hook", h.Name, "decision", verdict.Decision)
	}
}

func (e *Executor) handleToolHookExit(err error, h HookDef, event events.Event, repoPath, stdout, stderr string) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) == false {
		return false
	}
	message := strings.TrimSpace(stdout)
	if message == "" {
		message = strings.TrimSpace(stderr)
	}
	switch exitErr.ExitCode() {
	case 2:
		if message == "" {
			message = fmt.Sprintf("hook %q denied tool execution", h.Name)
		}
		slog.Warn("hook verdict: blocked", "hook", h.Name, "reason", message, "event", event.Type)
		e.publishHookBlocked(repoPath, event.RepoName, h.Name, message)
		return true
	default:
		if message == "" {
			message = fmt.Sprintf("hook %q exited with status %d; allowing execution to continue", h.Name, exitErr.ExitCode())
		}
		slog.Warn("hook returned non-zero status", "hook", h.Name, "status", exitErr.ExitCode(), "message", message)
		return true
	}
}

func (e *Executor) publishHookBlocked(repoPath, repoName, hookName, reason string) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(events.Event{
		Type:     events.HookBlocked,
		RepoPath: repoPath,
		RepoName: repoName,
		Data: map[string]any{
			"hook":   hookName,
			"reason": reason,
		},
	})
}

type toolHookPayloadData struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInputJSON string `json:"tool_input_json"`
	ToolOutput    string `json:"tool_output,omitempty"`
	ToolIsError   bool   `json:"tool_result_is_error"`
}

func newToolHookPayload(toolName, toolInputJSON, toolOutput string, toolIsError bool) toolHookPayloadData {
	return toolHookPayloadData{
		HookEventName: "PostToolUse",
		ToolName:      toolName,
		ToolInputJSON: toolInputJSON,
		ToolOutput:    toolOutput,
		ToolIsError:   toolIsError,
	}
}

func (p toolHookPayloadData) JSON() ([]byte, error) {
	return json.Marshal(p)
}

func (p toolHookPayloadData) Env() map[string]string {
	isError := "0"
	if p.ToolIsError {
		isError = "1"
	}
	return map[string]string{
		"HOOK_EVENT_NAME":    p.HookEventName,
		"HOOK_TOOL_NAME":     p.ToolName,
		"HOOK_TOOL_INPUT":    p.ToolInputJSON,
		"HOOK_TOOL_INPUT_JSON": p.ToolInputJSON,
		"HOOK_TOOL_OUTPUT":   p.ToolOutput,
		"HOOK_TOOL_IS_ERROR": isError,
	}
}

func toolHookPayload(event events.Event) (toolHookPayloadData, bool) {
	if event.Type != events.ToolCalled {
		return toolHookPayloadData{}, false
	}
	toolName := hookDataString(event.Data, "tool")
	if toolName == "" {
		toolName = hookDataString(event.Data, "name")
	}
	toolInputJSON := hookDataString(event.Data, "tool_input_json")
	if toolInputJSON == "" {
		if raw, ok := event.Data["tool_input"]; ok {
			data, err := json.Marshal(raw)
			if err == nil {
				toolInputJSON = string(data)
			}
		}
	}
	if toolInputJSON == "" {
		toolInputJSON = "{}"
	}
	payload := newToolHookPayload(
		toolName,
		toolInputJSON,
		hookDataString(event.Data, "tool_output"),
		hookDataBool(event.Data, "tool_result_is_error"),
	)
	return payload, true
}

func hookDataString(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	value, ok := data[key]
	if ok == false {
		return ""
	}
	asString, ok := value.(string)
	if ok == false {
		return ""
	}
	return asString
}

func hookDataBool(data map[string]any, key string) bool {
	if len(data) == 0 {
		return false
	}
	value, ok := data[key]
	if ok == false {
		return false
	}
	asBool, ok := value.(bool)
	if ok == false {
		return false
	}
	return asBool
}
