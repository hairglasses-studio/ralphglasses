package hooks

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// CCHookEvent enumerates the Claude Code hook event types.
type CCHookEvent string

const (
	// Pre-tool events (can block/modify).
	CCPreToolUse   CCHookEvent = "PreToolUse"
	CCPostToolUse  CCHookEvent = "PostToolUse"
	CCNotification CCHookEvent = "Notification"

	// Prompt lifecycle events.
	CCUserPromptSubmit CCHookEvent = "UserPromptSubmit"
	CCStop             CCHookEvent = "Stop"
	CCSubagentStop     CCHookEvent = "SubagentStop"
)

// AllCCHookEvents returns every recognised Claude Code hook event.
func AllCCHookEvents() []CCHookEvent {
	return []CCHookEvent{
		CCPreToolUse,
		CCPostToolUse,
		CCNotification,
		CCUserPromptSubmit,
		CCStop,
		CCSubagentStop,
	}
}

// CCHookMatcher controls which tool invocations a hook fires for.
// Empty/nil fields mean "match everything".
type CCHookMatcher struct {
	// ToolName filters by exact tool name (e.g. "Bash", "Read").
	ToolName string `json:"tool_name,omitempty"`
	// ToolNamePattern is a glob pattern for tool names (e.g. "mcp__*").
	ToolNamePattern string `json:"tool_name_pattern,omitempty"`
}

// CCHookDef is a single Claude Code hook definition, matching the
// structure written to .claude/settings.json → hooks.
type CCHookDef struct {
	// Type is the hook event (PreToolUse, PostToolUse, etc.).
	Type CCHookEvent `json:"type"`
	// Matchers restrict which invocations trigger this hook.
	// For non-tool events (UserPromptSubmit, Stop) this is typically nil.
	Matchers []CCHookMatcher `json:"matchers,omitempty"`
	// Command is the shell command (or script path) to execute.
	Command string `json:"command"`
	// Timeout in seconds. 0 means Claude Code default (60s).
	Timeout int `json:"timeout,omitempty"`
}

// CCHookOutput is the JSON structure a hook command writes to stdout to
// communicate decisions back to Claude Code.
type CCHookOutput struct {
	// Decision: "approve", "block", or "ask" (PreToolUse / UserPromptSubmit).
	Decision string `json:"decision,omitempty"`
	// Reason shown to the user when decision is "block".
	Reason string `json:"reason,omitempty"`
	// Content replaces or appends to the tool output (PostToolUse).
	Content string `json:"content,omitempty"`
}

// CCHooksConfig is the top-level hooks section for .claude/settings.json.
type CCHooksConfig struct {
	Hooks []CCHookDef `json:"hooks"`
}

// CCHooks manages generation and validation of Claude Code hook configurations.
type CCHooks struct {
	hooks []CCHookDef
}

// NewCCHooks creates an empty CCHooks manager.
func NewCCHooks() *CCHooks {
	return &CCHooks{}
}

// Add appends a hook definition. Returns an error if the definition is invalid.
func (h *CCHooks) Add(def CCHookDef) error {
	if err := ValidateCCHookDef(def); err != nil {
		return err
	}
	h.hooks = append(h.hooks, def)
	return nil
}

// Hooks returns a copy of all registered hook definitions.
func (h *CCHooks) Hooks() []CCHookDef {
	out := make([]CCHookDef, len(h.hooks))
	copy(out, h.hooks)
	return out
}

// GenerateConfig produces a CCHooksConfig ready for JSON serialisation into
// .claude/settings.json.
func (h *CCHooks) GenerateConfig() CCHooksConfig {
	return CCHooksConfig{Hooks: h.Hooks()}
}

// GenerateJSON marshals the hook configuration to indented JSON.
func (h *CCHooks) GenerateJSON() ([]byte, error) {
	cfg := h.GenerateConfig()
	return json.MarshalIndent(cfg, "", "  ")
}

// ValidateChain checks that a sequence of hook definitions forms a valid
// chain: no duplicate event+matcher combinations, commands are non-empty,
// and events are recognised.
func (h *CCHooks) ValidateChain() []error {
	var errs []error
	seen := make(map[string]int) // key → index of first occurrence

	for i, def := range h.hooks {
		if err := ValidateCCHookDef(def); err != nil {
			errs = append(errs, fmt.Errorf("hook[%d]: %w", i, err))
			continue
		}

		key := chainKey(def)
		if prev, ok := seen[key]; ok {
			errs = append(errs, fmt.Errorf(
				"hook[%d]: duplicate of hook[%d] (event=%s, command=%s)",
				i, prev, def.Type, def.Command,
			))
		} else {
			seen[key] = i
		}
	}
	return errs
}

// ParseHookOutput parses the JSON stdout from a Claude Code hook command.
func ParseHookOutput(raw []byte) (*CCHookOutput, error) {
	raw = trimToJSON(raw)
	if len(raw) == 0 {
		return &CCHookOutput{}, nil
	}
	var out CCHookOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse hook output: %w", err)
	}
	return &out, nil
}

// ValidateCCHookDef checks a single hook definition for correctness.
func ValidateCCHookDef(def CCHookDef) error {
	if def.Type == "" {
		return fmt.Errorf("hook type is required")
	}
	if !isValidCCHookEvent(def.Type) {
		return fmt.Errorf("unknown hook event type %q", def.Type)
	}
	if strings.TrimSpace(def.Command) == "" {
		return fmt.Errorf("hook command is required")
	}
	if def.Timeout < 0 {
		return fmt.Errorf("hook timeout must be non-negative")
	}
	return nil
}

// DefaultRalphHooks returns a standard set of hook definitions that
// ralphglasses uses to integrate with Claude Code sessions.
func DefaultRalphHooks(ralphBin string) []CCHookDef {
	if ralphBin == "" {
		ralphBin = "ralphglasses"
	}
	return []CCHookDef{
		{
			Type:    CCPreToolUse,
			Command: ralphBin + " hook pre-tool",
			Matchers: []CCHookMatcher{
				{ToolNamePattern: "mcp__ralphglasses__*"},
			},
			Timeout: 10,
		},
		{
			Type:    CCPostToolUse,
			Command: ralphBin + " hook post-tool",
			Timeout: 10,
		},
		{
			Type:    CCUserPromptSubmit,
			Command: ralphBin + " hook prompt-submit",
			Timeout: 5,
		},
		{
			Type:    CCStop,
			Command: ralphBin + " hook stop",
			Timeout: 15,
		},
	}
}

// NewCCHookOutput creates a CCHookOutput with the given decision.
func NewCCHookOutput(decision, reason string) *CCHookOutput {
	return &CCHookOutput{
		Decision: decision,
		Reason:   reason,
	}
}

// ApproveOutput returns a hook output that approves the action.
func ApproveOutput() *CCHookOutput {
	return &CCHookOutput{Decision: "approve"}
}

// BlockOutput returns a hook output that blocks the action with a reason.
func BlockOutput(reason string) *CCHookOutput {
	return &CCHookOutput{Decision: "block", Reason: reason}
}

// AskOutput returns a hook output that asks the user for confirmation.
func AskOutput() *CCHookOutput {
	return &CCHookOutput{Decision: "ask"}
}

// JSON serialises the output for writing to stdout.
func (o *CCHookOutput) JSON() ([]byte, error) {
	return json.Marshal(o)
}

// --- internal helpers ---

func isValidCCHookEvent(e CCHookEvent) bool {
	return slices.Contains(AllCCHookEvents(), e)
}

func chainKey(def CCHookDef) string {
	// Unique by event type + command. Two hooks with the same event and
	// command but different matchers are still duplicates — the matchers
	// would conflict at runtime.
	return string(def.Type) + "\x00" + def.Command
}

// trimToJSON skips any leading non-JSON bytes (e.g. log lines before the
// JSON object) and returns the first {...} or empty slice.
func trimToJSON(data []byte) []byte {
	// Fast path: already starts with {.
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil
	}
	if data[0] == '{' {
		return data
	}
	// Scan for first '{'.
	idx := strings.IndexByte(string(data), '{')
	if idx < 0 {
		return nil
	}
	return data[idx:]
}

// CCHookTimeout returns the effective timeout for a hook definition,
// using the Claude Code default of 60 seconds when unset.
func CCHookTimeout(def CCHookDef) time.Duration {
	if def.Timeout > 0 {
		return time.Duration(def.Timeout) * time.Second
	}
	return 60 * time.Second
}
