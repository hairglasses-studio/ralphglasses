package hooks

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- CCHookDef validation ---

func TestValidateCCHookDef_Valid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		def  CCHookDef
	}{
		{
			name: "minimal PreToolUse",
			def:  CCHookDef{Type: CCPreToolUse, Command: "echo ok"},
		},
		{
			name: "PostToolUse with matcher",
			def: CCHookDef{
				Type:    CCPostToolUse,
				Command: "/usr/local/bin/audit",
				Matchers: []CCHookMatcher{
					{ToolName: "Bash"},
				},
				Timeout: 30,
			},
		},
		{
			name: "UserPromptSubmit",
			def:  CCHookDef{Type: CCUserPromptSubmit, Command: "validate-prompt"},
		},
		{
			name: "Stop hook",
			def:  CCHookDef{Type: CCStop, Command: "cleanup.sh"},
		},
		{
			name: "SubagentStop hook",
			def:  CCHookDef{Type: CCSubagentStop, Command: "cleanup.sh"},
		},
		{
			name: "Notification hook",
			def:  CCHookDef{Type: CCNotification, Command: "notify.sh"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateCCHookDef(tt.def); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}

func TestValidateCCHookDef_Invalid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		def     CCHookDef
		wantSub string // substring expected in error
	}{
		{
			name:    "empty type",
			def:     CCHookDef{Command: "echo"},
			wantSub: "type is required",
		},
		{
			name:    "unknown type",
			def:     CCHookDef{Type: "FakeEvent", Command: "echo"},
			wantSub: "unknown hook event type",
		},
		{
			name:    "empty command",
			def:     CCHookDef{Type: CCPreToolUse},
			wantSub: "command is required",
		},
		{
			name:    "whitespace-only command",
			def:     CCHookDef{Type: CCPreToolUse, Command: "   "},
			wantSub: "command is required",
		},
		{
			name:    "negative timeout",
			def:     CCHookDef{Type: CCPreToolUse, Command: "echo", Timeout: -1},
			wantSub: "timeout must be non-negative",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCCHookDef(tt.def)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error %q should contain %q", err, tt.wantSub)
			}
		})
	}
}

// --- CCHooks Add / Hooks / GenerateConfig ---

func TestCCHooks_AddAndHooks(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	if len(h.Hooks()) != 0 {
		t.Fatal("new CCHooks should be empty")
	}

	err := h.Add(CCHookDef{Type: CCPreToolUse, Command: "echo pre"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	err = h.Add(CCHookDef{Type: CCStop, Command: "echo stop"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	hooks := h.Hooks()
	if len(hooks) != 2 {
		t.Fatalf("Hooks() len = %d, want 2", len(hooks))
	}

	// Returned slice is a copy — mutating it should not affect internal state.
	hooks[0].Command = "mutated"
	if h.Hooks()[0].Command == "mutated" {
		t.Error("Hooks() should return a copy, not a reference")
	}
}

func TestCCHooks_AddRejectsInvalid(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	err := h.Add(CCHookDef{Type: "BadType", Command: "echo"})
	if err == nil {
		t.Error("Add should reject invalid hook def")
	}
	if len(h.Hooks()) != 0 {
		t.Error("invalid hook should not be stored")
	}
}

func TestCCHooks_GenerateConfig(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	_ = h.Add(CCHookDef{Type: CCUserPromptSubmit, Command: "prompt-guard"})
	_ = h.Add(CCHookDef{Type: CCPostToolUse, Command: "audit-tool", Timeout: 10})

	cfg := h.GenerateConfig()
	if len(cfg.Hooks) != 2 {
		t.Fatalf("config hooks len = %d, want 2", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Type != CCUserPromptSubmit {
		t.Errorf("first hook type = %s, want %s", cfg.Hooks[0].Type, CCUserPromptSubmit)
	}
}

func TestCCHooks_GenerateJSON(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	_ = h.Add(CCHookDef{
		Type:    CCPreToolUse,
		Command: "guard.sh",
		Matchers: []CCHookMatcher{
			{ToolName: "Bash"},
		},
	})

	data, err := h.GenerateJSON()
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	// Should be valid JSON.
	var cfg CCHooksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if len(cfg.Hooks) != 1 {
		t.Fatalf("hooks len = %d, want 1", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Matchers[0].ToolName != "Bash" {
		t.Errorf("matcher tool_name = %q, want Bash", cfg.Hooks[0].Matchers[0].ToolName)
	}
}

// --- ValidateChain ---

func TestCCHooks_ValidateChain_Clean(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	_ = h.Add(CCHookDef{Type: CCPreToolUse, Command: "guard"})
	_ = h.Add(CCHookDef{Type: CCPostToolUse, Command: "audit"})
	_ = h.Add(CCHookDef{Type: CCStop, Command: "cleanup"})

	errs := h.ValidateChain()
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestCCHooks_ValidateChain_Duplicates(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	_ = h.Add(CCHookDef{Type: CCPreToolUse, Command: "guard"})
	// Force a duplicate by appending directly (bypassing Add validation won't
	// help since the def is valid — the duplicate is structural).
	h.hooks = append(h.hooks, CCHookDef{Type: CCPreToolUse, Command: "guard"})

	errs := h.ValidateChain()
	if len(errs) != 1 {
		t.Fatalf("expected 1 duplicate error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "duplicate") {
		t.Errorf("error should mention duplicate: %v", errs[0])
	}
}

func TestCCHooks_ValidateChain_SameEventDifferentCommand(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	_ = h.Add(CCHookDef{Type: CCPreToolUse, Command: "guard-a"})
	_ = h.Add(CCHookDef{Type: CCPreToolUse, Command: "guard-b"})

	errs := h.ValidateChain()
	if len(errs) != 0 {
		t.Errorf("same event with different commands should be valid, got %v", errs)
	}
}

func TestCCHooks_ValidateChain_InvalidDef(t *testing.T) {
	t.Parallel()
	h := &CCHooks{
		hooks: []CCHookDef{
			{Type: "", Command: "oops"}, // invalid: empty type
		},
	}

	errs := h.ValidateChain()
	if len(errs) != 1 {
		t.Fatalf("expected 1 validation error, got %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "hook[0]") {
		t.Errorf("error should reference index: %v", errs[0])
	}
}

// --- ParseHookOutput ---

func TestParseHookOutput_Approve(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"decision":"approve"}`)
	out, err := ParseHookOutput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Decision != "approve" {
		t.Errorf("decision = %q, want approve", out.Decision)
	}
}

func TestParseHookOutput_Block(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"decision":"block","reason":"dangerous operation"}`)
	out, err := ParseHookOutput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Decision != "block" || out.Reason != "dangerous operation" {
		t.Errorf("got decision=%q reason=%q", out.Decision, out.Reason)
	}
}

func TestParseHookOutput_Empty(t *testing.T) {
	t.Parallel()
	out, err := ParseHookOutput([]byte(""))
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if out.Decision != "" {
		t.Errorf("empty input should yield empty output, got decision=%q", out.Decision)
	}
}

func TestParseHookOutput_WhitespaceOnly(t *testing.T) {
	t.Parallel()
	out, err := ParseHookOutput([]byte("   \n\t  "))
	if err != nil {
		t.Fatalf("parse whitespace: %v", err)
	}
	if out.Decision != "" {
		t.Errorf("whitespace should yield empty output")
	}
}

func TestParseHookOutput_LeadingGarbage(t *testing.T) {
	t.Parallel()
	// Simulate a hook that prints log lines before JSON.
	raw := []byte("DEBUG: starting hook\nINFO: checking\n{\"decision\":\"ask\"}")
	out, err := ParseHookOutput(raw)
	if err != nil {
		t.Fatalf("parse with leading garbage: %v", err)
	}
	if out.Decision != "ask" {
		t.Errorf("decision = %q, want ask", out.Decision)
	}
}

func TestParseHookOutput_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParseHookOutput([]byte("{not json}"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse hook output") {
		t.Errorf("error should wrap with context: %v", err)
	}
}

func TestParseHookOutput_ContentField(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"content":"appended output"}`)
	out, err := ParseHookOutput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out.Content != "appended output" {
		t.Errorf("content = %q, want 'appended output'", out.Content)
	}
}

// --- Output helpers ---

func TestApproveOutput(t *testing.T) {
	t.Parallel()
	out := ApproveOutput()
	if out.Decision != "approve" {
		t.Errorf("decision = %q", out.Decision)
	}
}

func TestBlockOutput(t *testing.T) {
	t.Parallel()
	out := BlockOutput("too dangerous")
	if out.Decision != "block" || out.Reason != "too dangerous" {
		t.Errorf("got %+v", out)
	}
}

func TestAskOutput(t *testing.T) {
	t.Parallel()
	out := AskOutput()
	if out.Decision != "ask" {
		t.Errorf("decision = %q", out.Decision)
	}
}

func TestNewCCHookOutput(t *testing.T) {
	t.Parallel()
	out := NewCCHookOutput("block", "safety concern")
	if out.Decision != "block" || out.Reason != "safety concern" {
		t.Errorf("got %+v", out)
	}
}

func TestCCHookOutput_JSON(t *testing.T) {
	t.Parallel()
	out := BlockOutput("test reason")
	data, err := out.JSON()
	if err != nil {
		t.Fatalf("JSON: %v", err)
	}

	var roundtrip CCHookOutput
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundtrip.Decision != "block" || roundtrip.Reason != "test reason" {
		t.Errorf("round-trip mismatch: %+v", roundtrip)
	}
}

// --- DefaultRalphHooks ---

func TestDefaultRalphHooks(t *testing.T) {
	t.Parallel()
	hooks := DefaultRalphHooks("")
	if len(hooks) != 4 {
		t.Fatalf("expected 4 default hooks, got %d", len(hooks))
	}

	// All should use "ralphglasses" as the binary name.
	for _, h := range hooks {
		if !strings.HasPrefix(h.Command, "ralphglasses ") {
			t.Errorf("command %q should start with 'ralphglasses '", h.Command)
		}
	}

	// Should have one of each expected type.
	types := make(map[CCHookEvent]bool)
	for _, h := range hooks {
		types[h.Type] = true
	}
	for _, want := range []CCHookEvent{CCPreToolUse, CCPostToolUse, CCUserPromptSubmit, CCStop} {
		if !types[want] {
			t.Errorf("missing default hook for event %s", want)
		}
	}
}

func TestDefaultRalphHooks_CustomBin(t *testing.T) {
	t.Parallel()
	hooks := DefaultRalphHooks("/opt/bin/ralph")
	for _, h := range hooks {
		if !strings.HasPrefix(h.Command, "/opt/bin/ralph ") {
			t.Errorf("command %q should use custom binary path", h.Command)
		}
	}
}

// --- AllCCHookEvents ---

func TestAllCCHookEvents(t *testing.T) {
	t.Parallel()
	events := AllCCHookEvents()
	if len(events) < 5 {
		t.Errorf("expected at least 5 hook events, got %d", len(events))
	}
	// Every event should validate.
	for _, e := range events {
		if !isValidCCHookEvent(e) {
			t.Errorf("event %q not recognised by isValidCCHookEvent", e)
		}
	}
}

// --- CCHookTimeout ---

func TestCCHookTimeout_Default(t *testing.T) {
	t.Parallel()
	def := CCHookDef{Type: CCPreToolUse, Command: "echo"}
	if got := CCHookTimeout(def); got != 60*time.Second {
		t.Errorf("default timeout = %s, want 60s", got)
	}
}

func TestCCHookTimeout_Custom(t *testing.T) {
	t.Parallel()
	def := CCHookDef{Type: CCPreToolUse, Command: "echo", Timeout: 15}
	if got := CCHookTimeout(def); got != 15*time.Second {
		t.Errorf("custom timeout = %s, want 15s", got)
	}
}

// --- JSON round-trip for config ---

func TestCCHooksConfig_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	h := NewCCHooks()
	_ = h.Add(CCHookDef{
		Type:    CCPreToolUse,
		Command: "guard --strict",
		Matchers: []CCHookMatcher{
			{ToolName: "Bash"},
			{ToolNamePattern: "mcp__*"},
		},
		Timeout: 20,
	})
	_ = h.Add(CCHookDef{
		Type:    CCPostToolUse,
		Command: "audit-log",
	})

	data, err := h.GenerateJSON()
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	var cfg CCHooksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Hooks) != 2 {
		t.Fatalf("hooks len = %d, want 2", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Timeout != 20 {
		t.Errorf("timeout = %d, want 20", cfg.Hooks[0].Timeout)
	}
	if len(cfg.Hooks[0].Matchers) != 2 {
		t.Errorf("matchers len = %d, want 2", len(cfg.Hooks[0].Matchers))
	}
	// Second hook should omit timeout (zero value) and matchers (nil).
	if cfg.Hooks[1].Timeout != 0 {
		t.Errorf("second hook timeout = %d, want 0", cfg.Hooks[1].Timeout)
	}
	if cfg.Hooks[1].Matchers != nil {
		t.Errorf("second hook matchers should be nil")
	}
}

// --- trimToJSON edge cases ---

func TestTrimToJSON_NoJSON(t *testing.T) {
	t.Parallel()
	result := trimToJSON([]byte("no json here at all"))
	if result != nil {
		t.Errorf("expected nil, got %q", result)
	}
}

func TestTrimToJSON_DirectJSON(t *testing.T) {
	t.Parallel()
	input := []byte(`{"key":"value"}`)
	result := trimToJSON(input)
	if string(result) != `{"key":"value"}` {
		t.Errorf("got %q", result)
	}
}

func TestTrimToJSON_LeadingWhitespace(t *testing.T) {
	t.Parallel()
	input := []byte(`  {"key":"value"}`)
	result := trimToJSON(input)
	if string(result) != `{"key":"value"}` {
		t.Errorf("got %q", result)
	}
}
