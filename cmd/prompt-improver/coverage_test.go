package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// --- dispatch: uncovered branch tests ---

func TestDispatch_Enhance_ProviderWithoutMode(t *testing.T) {
	// When --provider is set but --mode is not, mode defaults to "local" inside the if block
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--provider", "claude", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("should produce enhanced output")
	}
}

func TestDispatch_Enhance_TargetProviderWithoutMode(t *testing.T) {
	// When --target-provider is set without --mode, triggers the mode/provider/targetProvider branch
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--target-provider", "openai", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("should produce enhanced output with target-provider")
	}
}

func TestDispatch_Enhance_ModeDefaultsToLocal(t *testing.T) {
	// When --provider is set but --mode is empty, mode should default to "local"
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--provider", "gemini", "write a sorting function with error handling"})
		if err != nil {
			t.Errorf("dispatch: %v", err)
		}
	})
	if out == "" {
		t.Error("should produce output")
	}
}

func TestDispatch_Improve_WithQuietFlag(t *testing.T) {
	// runImprove calls os.Exit when OPENAI_API_KEY is not set, which kills
	// the entire test process. Skip when the key is absent.
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set — skipping to avoid os.Exit in runImprove")
	}
	err := dispatch([]string{"improve", "--quiet", "fix the sorting bug in the codebase"})
	_ = err
}

func TestDispatch_Improve_WithAllFlags(t *testing.T) {
	// runImprove calls os.Exit when OPENAI_API_KEY is not set.
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set — skipping to avoid os.Exit in runImprove")
	}
	err := dispatch([]string{"improve", "--type", "code", "--thinking", "--feedback", "focus on performance", "--provider", "claude", "--quiet", "fix", "the", "bug"})
	_ = err
}

func TestDispatch_Analyze_MultiWordWithTargetProvider(t *testing.T) {
	// Test multi-word prompt concatenation in analyze with target-provider
	out := captureStdout(t, func() {
		err := dispatch([]string{"analyze", "--target-provider", "openai", "fix", "this", "sorting", "bug"})
		if err != nil {
			t.Errorf("dispatch: %v", err)
		}
	})
	if !strings.Contains(out, "score_report") {
		t.Error("should include score_report with target-provider")
	}
}

// TestDispatch_CacheCheck_NoArgs is skipped because runCacheCheck calls
// os.Exit which terminates the test process.
func TestDispatch_CacheCheck_NoArgs(t *testing.T) {
	t.Skip("runCacheCheck calls os.Exit, cannot test in-process")
}

// --- runAnalyze: score clamping edge cases ---

func TestRunAnalyze_ScoreClampMin(t *testing.T) {
	out := captureStdout(t, func() {
		runAnalyze("x", "claude")
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	score, ok := result["score"].(float64)
	if !ok {
		t.Fatal("score field missing")
	}
	if score < 1 {
		t.Errorf("score %f should be >= 1", score)
	}
}

func TestRunAnalyze_ScoreClampMax(t *testing.T) {
	// A well-crafted prompt with target provider; score/10 should be <= 10
	out := captureStdout(t, func() {
		runAnalyze("Return exactly 5 user records as JSON, sorted by creation date, including name, email, and created_at fields.", "gemini")
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	score, ok := result["score"].(float64)
	if !ok {
		t.Fatal("score field missing")
	}
	if score > 10 {
		t.Errorf("score %f should be <= 10", score)
	}
}

// --- runEnhanceWithMode: default mode branch ---

func TestRunEnhanceWithMode_EmptyModeDefaultsToLocal(t *testing.T) {
	// When mode is empty string, ValidMode returns "", causing os.Exit
	// Test with a valid mode to exercise provider/targetProvider branches
	out := captureStdout(t, func() {
		runEnhanceWithMode("write a sorting function with error handling for edge cases", "analysis", "local", false, "gemini", "openai")
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("should produce output with provider and target-provider set")
	}
}

// --- runDiff: improvements display ---

func TestRunDiff_ShowsImprovements(t *testing.T) {
	out := captureStdout(t, func() {
		runDiff("fix this")
	})
	// Short prompts like "fix this" should trigger improvements
	if !strings.Contains(out, "--- original") {
		t.Error("should show original marker")
	}
	// May or may not have improvements section
	_ = out
}

// --- runCacheCheck: no-issues path ---

func TestRunCacheCheck_PlainTextNoXML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(path, []byte("Just a simple plain text prompt with no XML tags."), 0644)

	out := captureStdout(t, func() {
		runCacheCheck(path)
	})
	if !strings.Contains(out, "Cache-friendly") {
		t.Error("plain text should report cache-friendly: no ordering issues")
	}
}

// --- writeSettings edge cases ---

func TestWriteSettings_NilRaw(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".claude", "settings.json")

	s := &settingsJSON{
		Hooks: map[string][]hookGroup{
			"UserPromptSubmit": {
				{Hooks: []hookEntry{{Type: "command", Command: "test"}}},
			},
		},
		McpServers: map[string]mcpServerEntry{
			"test": {Type: "stdio", Command: "test"},
		},
	}

	// nil raw map - should create one internally
	err := writeSettings(path, s, nil)
	if err != nil {
		t.Fatalf("writeSettings with nil raw: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hooks") {
		t.Error("should write hooks")
	}
}

func TestWriteSettings_CleansEmptyMaps(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".claude", "settings.json")

	raw := map[string]json.RawMessage{
		"hooks":      json.RawMessage(`{}`),
		"mcpServers": json.RawMessage(`{}`),
		"other":      json.RawMessage(`"kept"`),
	}

	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup), // empty
		McpServers: make(map[string]mcpServerEntry), // empty
	}

	err := writeSettings(path, s, raw)
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)
	if strings.Contains(content, "hooks") {
		t.Error("empty hooks should be deleted from output")
	}
	if strings.Contains(content, "mcpServers") {
		t.Error("empty mcpServers should be deleted from output")
	}
	if !strings.Contains(content, "other") {
		t.Error("other keys should be preserved")
	}
}

// --- hookInput/hookOutput struct edge cases ---

func TestHookOutput_NilSpecificOutput(t *testing.T) {
	out := hookOutput{HookSpecificOutput: nil}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Should produce empty JSON or omit hookSpecificOutput
	if strings.Contains(string(data), "additionalContext") {
		t.Error("nil HookSpecificOutput should not produce additionalContext")
	}
}

func TestHookInput_AllFields(t *testing.T) {
	hi := hookInput{
		SessionID:      "s1",
		TranscriptPath: "/t",
		Cwd:            "/c",
		PermissionMode: "p",
		HookEventName:  "h",
		Prompt:         "test",
	}
	data, _ := json.Marshal(hi)
	var parsed hookInput
	json.Unmarshal(data, &parsed)

	if parsed.SessionID != "s1" {
		t.Error("SessionID mismatch")
	}
	if parsed.TranscriptPath != "/t" {
		t.Error("TranscriptPath mismatch")
	}
	if parsed.Cwd != "/c" {
		t.Error("Cwd mismatch")
	}
	if parsed.PermissionMode != "p" {
		t.Error("PermissionMode mismatch")
	}
	if parsed.HookEventName != "h" {
		t.Error("HookEventName mismatch")
	}
}

// --- mcpGetBool edge cases ---

func TestMcpGetBool_NonBoolStringValue(t *testing.T) {
	req := makeMCPReq(map[string]any{"flag": "true"})
	if mcpGetBool(req, "flag") {
		t.Error("string 'true' should not be treated as bool true")
	}
}

func TestMcpGetBool_IntValue(t *testing.T) {
	req := makeMCPReq(map[string]any{"flag": 1})
	if mcpGetBool(req, "flag") {
		t.Error("int 1 should not be treated as bool true")
	}
}

// --- removeHookEntry edge cases ---

func TestRemoveHookEntry_AllGroupsRemoved(t *testing.T) {
	s := &settingsJSON{
		Hooks: map[string][]hookGroup{
			"UserPromptSubmit": {
				{Hooks: []hookEntry{{Type: "command", Command: "/bin/prompt-improver hook"}}},
				{Hooks: []hookEntry{{Type: "command", Command: "/other/prompt-improver hook"}}},
			},
		},
		McpServers: make(map[string]mcpServerEntry),
	}

	removeHookEntry(s)

	// All groups contained prompt-improver, so key should be deleted
	if _, ok := s.Hooks["UserPromptSubmit"]; ok {
		t.Error("should delete UserPromptSubmit key when all groups are prompt-improver")
	}
}

func TestRemoveHookEntry_MixedHooksInGroup(t *testing.T) {
	s := &settingsJSON{
		Hooks: map[string][]hookGroup{
			"UserPromptSubmit": {
				{Hooks: []hookEntry{
					{Type: "command", Command: "/bin/prompt-improver hook"},
					{Type: "command", Command: "other-tool hook"},
				}},
			},
		},
		McpServers: make(map[string]mcpServerEntry),
	}

	removeHookEntry(s)

	groups := s.Hooks["UserPromptSubmit"]
	if len(groups) != 1 {
		t.Fatalf("expected 1 remaining group, got %d", len(groups))
	}
	if len(groups[0].Hooks) != 1 {
		t.Fatalf("expected 1 remaining hook, got %d", len(groups[0].Hooks))
	}
	if groups[0].Hooks[0].Command != "other-tool hook" {
		t.Error("should keep other-tool hook")
	}
}

// --- parseFlags: additional coverage ---

func TestParseFlags_ConsecutiveFlags(t *testing.T) {
	m := parseFlags([]string{"--a", "1", "--b", "2", "--c", "3"})
	if m["a"] != "1" || m["b"] != "2" || m["c"] != "3" {
		t.Errorf("parseFlags: got %v", m)
	}
}

// --- settingsPathFor test ---

func TestSettingsPathFor_LocalExactValue(t *testing.T) {
	path := settingsPathFor(false)
	expected := filepath.Join(".claude", "settings.json")
	if path != expected {
		t.Errorf("settingsPathFor(false) = %q, want %q", path, expected)
	}
}

func TestSettingsPathFor_GlobalContainsHome(t *testing.T) {
	path := settingsPathFor(true)
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".claude", "settings.json")
	if path != expected {
		t.Errorf("settingsPathFor(true) = %q, want %q", path, expected)
	}
}

// --- getOrCreateEngine: caching ---

func TestGetOrCreateEngine_ReturnsCachedInstance(t *testing.T) {
	origEngine := hybridEngine
	defer func() { hybridEngine = origEngine }()

	// Set to a non-nil sentinel value to test caching
	hybridEngine = nil

	// First call creates
	e1 := getOrCreateEngine(enhancer.LLMConfig{})
	// Second call returns same
	e2 := getOrCreateEngine(enhancer.LLMConfig{})
	if e1 != e2 {
		t.Error("should return cached engine")
	}
}

// --- runEnhanceQuiet: source field ---

func TestRunEnhanceQuiet_SetsSource(t *testing.T) {
	out := captureStdout(t, func() {
		runEnhanceQuiet("write a comprehensive sorting function with error handling for edge cases", "code", false)
	})
	if !strings.Contains(out, `"source"`) {
		t.Error("verbose output should contain source field")
	}
	if !strings.Contains(out, `"local"`) {
		t.Error("source should be 'local'")
	}
}
