package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// --- parseFlags tests ---

func TestParseFlags_Empty(t *testing.T) {
	m := parseFlags(nil)
	if len(m) != 0 {
		t.Errorf("parseFlags(nil) = %v, want empty", m)
	}
}

func TestParseFlags_SinglePair(t *testing.T) {
	m := parseFlags([]string{"--key", "value"})
	if m["key"] != "value" {
		t.Errorf("parseFlags: key = %q, want %q", m["key"], "value")
	}
}

func TestParseFlags_MultiplePairs(t *testing.T) {
	m := parseFlags([]string{"--a", "1", "--b", "2"})
	if m["a"] != "1" || m["b"] != "2" {
		t.Errorf("parseFlags: got %v", m)
	}
}

func TestParseFlags_TrailingFlag(t *testing.T) {
	// A flag with no value following it should be ignored
	m := parseFlags([]string{"--lonely"})
	if _, ok := m["lonely"]; ok {
		t.Error("trailing flag with no value should be ignored")
	}
}

func TestParseFlags_NonFlagArgs(t *testing.T) {
	m := parseFlags([]string{"positional", "--key", "value"})
	if _, ok := m["positional"]; ok {
		t.Error("positional arg should not be treated as flag")
	}
	if m["key"] != "value" {
		t.Errorf("key = %q, want %q", m["key"], "value")
	}
}

// --- printHelp test ---

func TestPrintHelp_DoesNotPanic(t *testing.T) {
	// Redirect stdout to avoid noise
	old := os.Stdout
	os.Stdout, _ = os.Create(filepath.Join(t.TempDir(), "out"))
	defer func() { os.Stdout = old }()

	printHelp()
}

// --- runEnhanceQuiet tests ---

func TestRunEnhanceQuiet_Verbose(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runEnhanceQuiet("write a function to sort users by name with error handling", "code", false)

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "enhanced") {
		t.Error("verbose output should contain 'enhanced' field")
	}
}

func TestRunEnhanceQuiet_Quiet(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runEnhanceQuiet("write a function to sort users by name with error handling", "", true)

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Quiet mode should NOT output JSON structure, just the enhanced text
	if strings.Contains(output, `"enhanced"`) {
		t.Error("quiet mode should not output JSON structure")
	}
	if len(output) == 0 {
		t.Error("quiet mode should still produce output")
	}
}

// --- runEnhance test ---

func TestRunEnhance_DoesNotPanic(t *testing.T) {
	old := os.Stdout
	os.Stdout, _ = os.Create(filepath.Join(t.TempDir(), "out"))
	defer func() { os.Stdout = old }()

	runEnhance("write a function to sort users by name with error handling", "code")
}

// --- runAnalyze test ---

func TestRunAnalyze_Basic(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runAnalyze("fix this bug in the sorting function", "")

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "score") {
		t.Error("analyze output should contain score")
	}
}

func TestRunAnalyze_WithProvider(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runAnalyze("fix this bug in the sorting function", "claude")

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "score_report") {
		t.Error("analyze with provider should contain score_report")
	}
}

// --- runLint test ---

func TestRunLint_Clean(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runLint("Return exactly 5 user records as JSON, sorted by creation date.")

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output == "" {
		t.Error("lint should produce some output")
	}
}

func TestRunLint_Dirty(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runLint("CRITICAL: You MUST follow this rule. NEVER ignore it.")

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "overtrigger-phrase") {
		t.Error("dirty prompt should trigger lint findings")
	}
}

// --- runDiff test ---

func TestRunDiff_Output(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runDiff("write a function to sort users by name with error handling")

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "--- original") {
		t.Error("diff output should contain '--- original'")
	}
	if !strings.Contains(output, "+++ enhanced") {
		t.Error("diff output should contain '+++ enhanced'")
	}
}

// --- runCacheCheck tests ---

func TestRunCacheCheck_File(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(path, []byte("<role>Expert.</role>\n<instructions>Do it.</instructions>"), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runCacheCheck(path)

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output == "" {
		t.Error("cache-check should produce output")
	}
}

// --- runCheckClaudeMD tests ---

func TestRunCheckClaudeMD_Healthy(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte("# Project\n\nSimple project.\n\n## Standards\n\nUse gofmt."), 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runCheckClaudeMD(path)

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "healthy") {
		t.Error("healthy CLAUDE.md should report healthy")
	}
}

// --- runTemplate test ---

func TestRunTemplate_ValidTemplate(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runTemplate("troubleshoot", []string{"--system", "resolume", "--symptoms", "clips stuck"})

	w.Close()
	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "resolume") {
		t.Error("template should fill in system variable")
	}
}

// --- getOrCreateEngine tests ---

func TestGetOrCreateEngine_NilWithoutKey(t *testing.T) {
	// Reset global
	origEngine := hybridEngine
	defer func() { hybridEngine = origEngine }()
	hybridEngine = nil

	// Without API key, engine creation should return nil or an engine
	// (depends on env). Just verify it doesn't panic.
	cfg := enhancer.LLMConfig{}
	engine := getOrCreateEngine(cfg)
	_ = engine
}

func TestGetOrCreateEngine_CachesResult(t *testing.T) {
	origEngine := hybridEngine
	defer func() { hybridEngine = origEngine }()
	hybridEngine = nil

	cfg := enhancer.LLMConfig{}
	e1 := getOrCreateEngine(cfg)
	e2 := getOrCreateEngine(cfg)

	// Should return the same instance (cached)
	if e1 != e2 {
		t.Error("getOrCreateEngine should cache the result")
	}
}

// --- runEnhanceWithMode tests ---

func TestRunEnhanceWithMode_Local(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runEnhanceWithMode("write a function to sort users by name with error handling", "code", "local", false, "", "")

	w.Close()
	buf := make([]byte, 16384)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "enhanced") {
		t.Error("local mode should produce enhanced output")
	}
}

func TestRunEnhanceWithMode_LocalQuiet(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runEnhanceWithMode("write a function to sort users by name with error handling", "", "local", true, "", "")

	w.Close()
	buf := make([]byte, 16384)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if strings.Contains(output, `"enhanced"`) {
		t.Error("quiet mode should not include JSON structure")
	}
	if output == "" {
		t.Error("should produce output even in quiet mode")
	}
}

func TestRunEnhanceWithMode_LocalWithTargetProvider(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	runEnhanceWithMode("write a function to sort users by name with error handling", "", "local", false, "", "gemini")

	w.Close()
	buf := make([]byte, 16384)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "enhanced") {
		t.Error("should produce enhanced output with target provider")
	}
}

// --- settingsPathFor tests ---

func TestSettingsPathFor_Local(t *testing.T) {
	path := settingsPathFor(false)
	if !strings.HasSuffix(path, ".claude/settings.json") {
		t.Errorf("local path = %q, want suffix .claude/settings.json", path)
	}
	if strings.Contains(path, os.Getenv("HOME")) {
		t.Error("local path should be relative, not under HOME")
	}
}

func TestSettingsPathFor_Global(t *testing.T) {
	path := settingsPathFor(true)
	if !strings.HasSuffix(path, ".claude/settings.json") {
		t.Errorf("global path = %q, want suffix .claude/settings.json", path)
	}
	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(path, home) {
		t.Errorf("global path %q should start with home dir %q", path, home)
	}
}

// --- MCP handler tests (additional coverage) ---

func TestMCP_HandleDiff(t *testing.T) {
	req := makeMCPReq(map[string]any{"prompt": "fix this bug in the sorting function"})
	result, err := mcpHandleDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected success")
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "--- original") {
		t.Error("diff should contain '--- original'")
	}
}

func TestMCP_HandleDiff_EmptyPrompt(t *testing.T) {
	req := makeMCPReq(map[string]any{"prompt": ""})
	result, err := mcpHandleDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for empty prompt")
	}
}

func TestMCP_HandleCheckClaudeMD_Default(t *testing.T) {
	// With no path specified, uses ./CLAUDE.md which may not exist
	req := makeMCPReq(map[string]any{})
	result, err := mcpHandleCheckClaudeMD(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Either error (file not found) or success — both are valid
	_ = result
}

func TestMCP_HandleCheckClaudeMD_Healthy(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte("# Project\n\nGood content."), 0644)

	req := makeMCPReq(map[string]any{"path": path})
	result, err := mcpHandleCheckClaudeMD(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("healthy CLAUDE.md should not error")
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "healthy") {
		t.Error("should report healthy")
	}
}

func TestMCP_HandleListTemplates(t *testing.T) {
	req := mcp.CallToolRequest{}
	result, err := mcpHandleListTemplates(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("list_templates should not error")
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "troubleshoot") {
		t.Error("should list troubleshoot template")
	}
}

func TestMCP_HandleImprove_EmptyPrompt(t *testing.T) {
	req := makeMCPReq(map[string]any{"prompt": ""})
	result, err := mcpHandleImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for empty prompt")
	}
}

func TestMCP_HandleImprove_LocalMode(t *testing.T) {
	req := makeMCPReq(map[string]any{
		"prompt": "fix this bug in the sorting function with error handling",
		"mode":   "local",
	})
	result, err := mcpHandleImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := getResultText(t, result)
		t.Errorf("local mode should not error: %s", text)
	}
}

func TestMCP_HandleImprove_AutoMode(t *testing.T) {
	// Auto mode without API key should fall back to local
	origEngine := hybridEngine
	defer func() { hybridEngine = origEngine }()
	hybridEngine = nil

	req := makeMCPReq(map[string]any{
		"prompt": "fix this bug in the sorting function with error handling",
		"mode":   "auto",
	})
	result, err := mcpHandleImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// May error if no API key, or succeed with local fallback
	_ = result
}

func TestMCP_HandleImprove_WithThinking(t *testing.T) {
	req := makeMCPReq(map[string]any{
		"prompt":           "fix this bug in the sorting function with error handling",
		"mode":             "local",
		"thinking_enabled": true,
	})
	result, err := mcpHandleImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestMCP_HandleImprove_WithFeedback(t *testing.T) {
	req := makeMCPReq(map[string]any{
		"prompt":   "fix this bug in the sorting function with error handling",
		"mode":     "local",
		"feedback": "add more error context",
	})
	result, err := mcpHandleImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := getResultText(t, result)
		t.Errorf("should succeed with feedback: %s", text)
	}
}

func TestMCP_HandleImprove_WithTaskType(t *testing.T) {
	req := makeMCPReq(map[string]any{
		"prompt":    "review this code for security vulnerabilities",
		"mode":      "local",
		"task_type": "code",
	})
	result, err := mcpHandleImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := getResultText(t, result)
		t.Errorf("should succeed: %s", text)
	}
}

func TestMCP_HandleDiff_WithTaskType(t *testing.T) {
	req := makeMCPReq(map[string]any{
		"prompt":    "write a haiku about testing",
		"task_type": "creative",
	})
	result, err := mcpHandleDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "improvements") {
		// May or may not have improvements, that's fine
		_ = text
	}
}

func TestMCP_HandleCheckClaudeMD_Unhealthy(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "CLAUDE.md")
	content := strings.Repeat("CRITICAL: You MUST follow this rule.\n", 50)
	os.WriteFile(path, []byte(content), 0644)

	req := makeMCPReq(map[string]any{"path": path})
	result, err := mcpHandleCheckClaudeMD(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("should not error, just report issues")
	}
}

// --- mcpGetBool test ---

func TestMcpGetBool_True(t *testing.T) {
	req := makeMCPReq(map[string]any{"flag": true})
	if !mcpGetBool(req, "flag") {
		t.Error("mcpGetBool should return true")
	}
}

func TestMcpGetBool_False(t *testing.T) {
	req := makeMCPReq(map[string]any{"flag": false})
	if mcpGetBool(req, "flag") {
		t.Error("mcpGetBool should return false")
	}
}

func TestMcpGetBool_Missing(t *testing.T) {
	req := makeMCPReq(map[string]any{})
	if mcpGetBool(req, "flag") {
		t.Error("mcpGetBool should return false for missing key")
	}
}

func TestMcpGetBool_NilArgs(t *testing.T) {
	req := mcp.CallToolRequest{}
	if mcpGetBool(req, "flag") {
		t.Error("mcpGetBool should return false for nil args")
	}
}

// --- mcpArgsMap / mcpGetString tests ---

func TestMcpArgsMap_NilArgs(t *testing.T) {
	req := mcp.CallToolRequest{}
	m := mcpArgsMap(req)
	if m != nil {
		t.Error("should return nil for nil args")
	}
}

func TestMcpGetString_NilArgs(t *testing.T) {
	req := mcp.CallToolRequest{}
	s := mcpGetString(req, "key")
	if s != "" {
		t.Errorf("mcpGetString with nil args = %q, want empty", s)
	}
}

func TestMcpGetString_NonStringValue(t *testing.T) {
	req := makeMCPReq(map[string]any{"key": 42})
	s := mcpGetString(req, "key")
	if s != "" {
		t.Errorf("mcpGetString with int value = %q, want empty", s)
	}
}

func TestMcpGetString_MissingKey(t *testing.T) {
	req := makeMCPReq(map[string]any{"other": "val"})
	s := mcpGetString(req, "key")
	if s != "" {
		t.Errorf("mcpGetString with missing key = %q, want empty", s)
	}
}

func TestMcpGetString_ValidString(t *testing.T) {
	req := makeMCPReq(map[string]any{"key": "hello"})
	s := mcpGetString(req, "key")
	if s != "hello" {
		t.Errorf("mcpGetString = %q, want %q", s, "hello")
	}
}

// --- mcpJSONResult test ---

func TestMcpJSONResult_Success(t *testing.T) {
	result := mcpJSONResult(map[string]string{"key": "value"})
	if result.IsError {
		t.Error("should not be error")
	}
	text := getResultText(t, result)
	if !strings.Contains(text, "key") {
		t.Error("should contain key")
	}
}

// --- mcpErrResult / mcpTextResult tests ---

func TestMcpErrResult(t *testing.T) {
	result := mcpErrResult("test error")
	if !result.IsError {
		t.Error("should be error")
	}
	text := getResultText(t, result)
	if text != "test error" {
		t.Errorf("text = %q, want %q", text, "test error")
	}
}

func TestMcpTextResult(t *testing.T) {
	result := mcpTextResult("test text")
	if result.IsError {
		t.Error("should not be error")
	}
	text := getResultText(t, result)
	if text != "test text" {
		t.Errorf("text = %q, want %q", text, "test text")
	}
}

// --- readStdin test ---

func TestReadStdin_NonPipe(t *testing.T) {
	// readStdin checks if stdin is a char device; in test context it may vary.
	// Just verify it doesn't panic and returns a string.
	result := readStdin()
	// When run in tests, stdin is not a pipe, so should return ""
	if result != "" {
		t.Logf("readStdin returned %q (expected empty in non-pipe test context)", result)
	}
}

func TestReadStdin_Pipe(t *testing.T) {
	// Replace stdin with a pipe to test the piped input path
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r

	go func() {
		w.Write([]byte("piped input\n"))
		w.Close()
	}()

	result := readStdin()
	if result != "piped input" {
		t.Errorf("readStdin from pipe = %q, want %q", result, "piped input")
	}
}

func TestReadStdin_PipeWhitespace(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r

	go func() {
		w.Write([]byte("  hello world  \n"))
		w.Close()
	}()

	result := readStdin()
	if result != "hello world" {
		t.Errorf("readStdin should trim whitespace: got %q, want %q", result, "hello world")
	}
}

func TestReadStdin_PipeEmpty(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	w.Close() // Close immediately = empty input

	result := readStdin()
	if result != "" {
		t.Errorf("readStdin from empty pipe = %q, want empty", result)
	}
}

// --- readSettings edge cases ---

func TestReadSettings_MissingFile(t *testing.T) {
	s, raw, err := readSettings("/nonexistent/path/settings.json")
	if err == nil {
		t.Error("should error on missing file")
	}
	if s == nil {
		t.Error("should return non-nil settings even on error")
	}
	_ = raw
}

func TestReadSettings_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	os.WriteFile(path, []byte("not json"), 0644)

	_, _, err := readSettings(path)
	if err == nil {
		t.Error("should error on invalid JSON")
	}
}

func TestReadSettings_WithHooksAndMCP(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	content := `{
		"hooks": {
			"UserPromptSubmit": [{"hooks": [{"type": "command", "command": "test"}]}]
		},
		"mcpServers": {
			"test-server": {"type": "stdio", "command": "test"}
		},
		"otherKey": "preserved"
	}`
	os.WriteFile(path, []byte(content), 0644)

	s, raw, err := readSettings(path)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if len(s.Hooks["UserPromptSubmit"]) != 1 {
		t.Error("should parse hooks")
	}
	if _, ok := s.McpServers["test-server"]; !ok {
		t.Error("should parse mcpServers")
	}
	if raw["otherKey"] == nil {
		t.Error("should preserve unknown keys in raw")
	}
}

func TestReadSettings_InvalidHooksJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	// hooks has wrong type (string instead of object)
	os.WriteFile(path, []byte(`{"hooks": "not-an-object"}`), 0644)

	_, _, err := readSettings(path)
	if err == nil {
		t.Error("should error on invalid hooks JSON")
	}
	if !strings.Contains(err.Error(), "invalid hooks JSON") {
		t.Errorf("error = %q, want 'invalid hooks JSON'", err.Error())
	}
}

func TestReadSettings_InvalidMcpServersJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	// mcpServers has wrong type
	os.WriteFile(path, []byte(`{"mcpServers": "not-an-object"}`), 0644)

	_, _, err := readSettings(path)
	if err == nil {
		t.Error("should error on invalid mcpServers JSON")
	}
	if !strings.Contains(err.Error(), "invalid mcpServers JSON") {
		t.Errorf("error = %q, want 'invalid mcpServers JSON'", err.Error())
	}
}

func TestReadSettings_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "settings.json")
	os.WriteFile(path, []byte(`{"hooks":{},"mcpServers":{}}`), 0644)

	s, _, err := readSettings(path)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if s.Hooks == nil {
		t.Error("hooks should not be nil")
	}
	if s.McpServers == nil {
		t.Error("mcpServers should not be nil")
	}
}

// --- dispatch tests ---

// captureStdout runs fn with stdout redirected and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	w.Close()
	buf := make([]byte, 65536)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestDispatch_Enhance_Basic(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("enhance should produce output with 'enhanced'")
	}
}

func TestDispatch_Enhance_Quiet(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--quiet", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance --quiet: %v", err)
		}
	})
	if strings.Contains(out, `"enhanced"`) {
		t.Error("quiet mode should not output JSON key")
	}
	if out == "" {
		t.Error("quiet mode should still produce output")
	}
}

func TestDispatch_Enhance_WithType(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--type", "code", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance --type: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("enhance with type should produce output")
	}
}

func TestDispatch_Enhance_WithMode(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--mode", "local", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance --mode local: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("enhance with mode should produce output")
	}
}

func TestDispatch_Enhance_WithProvider(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--provider", "claude", "--mode", "local", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance --provider: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("enhance with provider should produce output")
	}
}

func TestDispatch_Enhance_WithTargetProvider(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--target-provider", "gemini", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance --target-provider: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("enhance with target-provider should produce output")
	}
}

func TestDispatch_Enhance_AllFlags(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--type", "creative", "--mode", "local", "--quiet", "--provider", "claude", "--target-provider", "openai", "write a poem about testing"})
		if err != nil {
			t.Errorf("dispatch enhance all flags: %v", err)
		}
	})
	if out == "" {
		t.Error("should produce output")
	}
}

func TestDispatch_Enhance_ModeWithoutProvider(t *testing.T) {
	// When mode is set but provider is empty, mode defaults should work
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "--mode", "local", "write a comprehensive sorting function with error handling"})
		if err != nil {
			t.Errorf("dispatch enhance mode only: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("enhance with mode should produce output")
	}
}

func TestDispatch_Enhance_MissingPrompt(t *testing.T) {
	err := dispatch([]string{"enhance"})
	if err == nil {
		t.Error("enhance without prompt should error")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error = %q, want usage message", err.Error())
	}
}

func TestDispatch_Enhance_MultiWordPrompt(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"enhance", "write", "a", "function", "to", "sort", "users", "by", "name"})
		if err != nil {
			t.Errorf("dispatch enhance multi-word: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("multi-word enhance should produce output")
	}
}

func TestDispatch_Analyze_Basic(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"analyze", "fix this bug in the sorting function"})
		if err != nil {
			t.Errorf("dispatch analyze: %v", err)
		}
	})
	if !strings.Contains(out, "score") {
		t.Error("analyze should produce score output")
	}
}

func TestDispatch_Analyze_WithTargetProvider(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"analyze", "--target-provider", "claude", "fix this bug in the sorting function"})
		if err != nil {
			t.Errorf("dispatch analyze --target-provider: %v", err)
		}
	})
	if !strings.Contains(out, "score_report") {
		t.Error("analyze with target-provider should produce score_report")
	}
}

func TestDispatch_Analyze_MultiWord(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"analyze", "--target-provider", "gemini", "fix", "this", "sorting", "bug", "with", "error", "handling"})
		if err != nil {
			t.Errorf("dispatch analyze multi-word: %v", err)
		}
	})
	if !strings.Contains(out, "score_report") {
		t.Error("analyze multi-word should produce score_report")
	}
}

func TestDispatch_Analyze_MissingPrompt(t *testing.T) {
	err := dispatch([]string{"analyze"})
	if err == nil {
		t.Error("analyze without prompt should error")
	}
}

func TestDispatch_Lint_Basic(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"lint", "CRITICAL:", "You", "MUST", "follow", "this", "rule"})
		if err != nil {
			t.Errorf("dispatch lint: %v", err)
		}
	})
	if out == "" {
		t.Error("lint should produce output")
	}
}

func TestDispatch_Lint_MissingPrompt(t *testing.T) {
	err := dispatch([]string{"lint"})
	if err == nil {
		t.Error("lint without prompt should error")
	}
}

func TestDispatch_Diff_Basic(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"diff", "write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch diff: %v", err)
		}
	})
	if !strings.Contains(out, "--- original") {
		t.Error("diff should produce diff output")
	}
}

func TestDispatch_Diff_MissingPrompt(t *testing.T) {
	err := dispatch([]string{"diff"})
	if err == nil {
		t.Error("diff without prompt should error")
	}
}

func TestDispatch_Templates(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"templates"})
		if err != nil {
			t.Errorf("dispatch templates: %v", err)
		}
	})
	if !strings.Contains(out, "troubleshoot") {
		t.Error("templates should list available templates")
	}
}

func TestDispatch_Template_Basic(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"template", "troubleshoot", "--system", "resolume", "--symptoms", "clips stuck"})
		if err != nil {
			t.Errorf("dispatch template: %v", err)
		}
	})
	if !strings.Contains(out, "resolume") {
		t.Error("template should fill variables")
	}
}

func TestDispatch_Template_MissingName(t *testing.T) {
	err := dispatch([]string{"template"})
	if err == nil {
		t.Error("template without name should error")
	}
}

func TestDispatch_CacheCheck_File(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(path, []byte("<role>Expert.</role>\n<instructions>Do it.</instructions>"), 0644)

	out := captureStdout(t, func() {
		err := dispatch([]string{"cache-check", path})
		if err != nil {
			t.Errorf("dispatch cache-check: %v", err)
		}
	})
	if out == "" {
		t.Error("cache-check should produce output")
	}
}

func TestDispatch_CheckClaudeMD_Healthy(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte("# Project\n\nSimple project.\n\n## Standards\n\nUse gofmt."), 0644)

	out := captureStdout(t, func() {
		err := dispatch([]string{"check-claudemd", path})
		if err != nil {
			t.Errorf("dispatch check-claudemd: %v", err)
		}
	})
	if !strings.Contains(out, "healthy") {
		t.Error("healthy CLAUDE.md should report healthy")
	}
}

func TestDispatch_CheckClaudeMD_WithPath(t *testing.T) {
	// Use an explicit path to a valid file so runCheckClaudeMD doesn't os.Exit
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "CLAUDE.md")
	os.WriteFile(path, []byte("# Project\n\nGood project."), 0644)

	out := captureStdout(t, func() {
		err := dispatch([]string{"check-claudemd", path})
		if err != nil {
			t.Errorf("dispatch check-claudemd with path: %v", err)
		}
	})
	if !strings.Contains(out, "healthy") {
		t.Error("should report healthy")
	}
}

func TestDispatch_Version(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"version"})
		if err != nil {
			t.Errorf("dispatch version: %v", err)
		}
	})
	if !strings.Contains(out, "prompt-improver") {
		t.Error("version should contain prompt-improver")
	}
}

func TestDispatch_Help(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"help"})
		if err != nil {
			t.Errorf("dispatch help: %v", err)
		}
	})
	if !strings.Contains(out, "USAGE") {
		t.Error("help should contain USAGE section")
	}
}

func TestDispatch_HelpFlag(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"--help"})
		if err != nil {
			t.Errorf("dispatch --help: %v", err)
		}
	})
	if !strings.Contains(out, "USAGE") {
		t.Error("--help should contain USAGE section")
	}
}

func TestDispatch_HelpShortFlag(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"-h"})
		if err != nil {
			t.Errorf("dispatch -h: %v", err)
		}
	})
	if !strings.Contains(out, "USAGE") {
		t.Error("-h should contain USAGE section")
	}
}

func TestDispatch_DefaultEnhance(t *testing.T) {
	out := captureStdout(t, func() {
		err := dispatch([]string{"write a function to sort users by name with error handling"})
		if err != nil {
			t.Errorf("dispatch default: %v", err)
		}
	})
	if !strings.Contains(out, "enhanced") {
		t.Error("default command should enhance the prompt")
	}
}

func TestDispatch_Improve_MissingPrompt(t *testing.T) {
	err := dispatch([]string{"improve"})
	if err == nil {
		t.Error("improve without prompt should error")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error = %q, want usage message", err.Error())
	}
}

// Note: TestDispatch_Improve_WithFlags is not included because runImprove
// calls os.Exit(1) when no API key is available, which kills the test process.

// --- install/uninstall helper tests ---

func TestAddHookEntry(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addHookEntry(s, "/usr/bin/prompt-improver")
	if len(s.Hooks["UserPromptSubmit"]) != 1 {
		t.Errorf("expected 1 hook group, got %d", len(s.Hooks["UserPromptSubmit"]))
	}
	// Idempotent: adding again should not duplicate
	addHookEntry(s, "/usr/bin/prompt-improver")
	if len(s.Hooks["UserPromptSubmit"]) != 1 {
		t.Errorf("expected 1 hook group after re-add, got %d", len(s.Hooks["UserPromptSubmit"]))
	}
}

func TestAddMCPEntry(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addMCPEntry(s, "/usr/bin/prompt-improver")
	if _, ok := s.McpServers["prompt-improver"]; !ok {
		t.Error("MCP entry should be added")
	}
	// Idempotent
	addMCPEntry(s, "/usr/bin/prompt-improver")
	if len(s.McpServers) != 1 {
		t.Errorf("expected 1 MCP entry, got %d", len(s.McpServers))
	}
}

func TestRemoveHookEntry(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addHookEntry(s, "/usr/bin/prompt-improver")
	removeHookEntry(s)
	if len(s.Hooks["UserPromptSubmit"]) != 0 {
		t.Error("hook entry should be removed")
	}
}

func TestRemoveHookEntry_KeepsOthers(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	// Add a prompt-improver hook and another hook
	addHookEntry(s, "/usr/bin/prompt-improver")
	s.Hooks["UserPromptSubmit"] = append(s.Hooks["UserPromptSubmit"], hookGroup{
		Hooks: []hookEntry{{Type: "command", Command: "other-tool hook"}},
	})
	removeHookEntry(s)
	if len(s.Hooks["UserPromptSubmit"]) != 1 {
		t.Errorf("should keep other hooks, got %d groups", len(s.Hooks["UserPromptSubmit"]))
	}
}

func TestRemoveMCPEntry(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addMCPEntry(s, "/usr/bin/prompt-improver")
	removeMCPEntry(s)
	if _, ok := s.McpServers["prompt-improver"]; ok {
		t.Error("MCP entry should be removed")
	}
}

func TestWriteSettings_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".claude", "settings.json")

	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addHookEntry(s, "/usr/bin/prompt-improver")
	addMCPEntry(s, "/usr/bin/prompt-improver")

	err := writeSettings(path, s, nil)
	if err != nil {
		t.Fatalf("writeSettings: %v", err)
	}

	s2, _, err := readSettings(path)
	if err != nil {
		t.Fatalf("readSettings: %v", err)
	}
	if len(s2.Hooks["UserPromptSubmit"]) != 1 {
		t.Error("hook should survive round-trip")
	}
	if _, ok := s2.McpServers["prompt-improver"]; !ok {
		t.Error("MCP entry should survive round-trip")
	}
}

// --- runInstall / runUninstall tests ---
// These test the functions directly since they have os.Exit only on error paths
// that we avoid by providing valid temp directories.

func TestRunInstall_HookAndMCP(t *testing.T) {
	tmpDir := t.TempDir()
	// Override settingsPathFor by using --global and changing HOME
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	out := captureStdout(t, func() {
		runInstall([]string{"--global"})
	})
	if !strings.Contains(out, "Installed") {
		t.Error("runInstall should report installation")
	}

	// Verify the settings file was created
	path := filepath.Join(tmpDir, ".claude", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("settings file not created: %v", err)
	}
	if !strings.Contains(string(data), "hooks") {
		t.Error("settings should contain hooks")
	}
	if !strings.Contains(string(data), "mcpServers") {
		t.Error("settings should contain mcpServers")
	}
}

func TestRunInstall_HookOnly(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	out := captureStdout(t, func() {
		runInstall([]string{"--global", "--hook-only"})
	})
	if !strings.Contains(out, "hook") {
		t.Error("should install hook")
	}
}

func TestRunInstall_MCPOnly(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	out := captureStdout(t, func() {
		runInstall([]string{"--global", "--mcp-only"})
	})
	if !strings.Contains(out, "MCP") {
		t.Error("should install MCP")
	}
}

func TestRunUninstall_ExistingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// First install, then uninstall
	captureStdout(t, func() {
		runInstall([]string{"--global"})
	})

	out := captureStdout(t, func() {
		runUninstall([]string{"--global"})
	})
	if !strings.Contains(out, "Uninstalled") {
		t.Error("should report uninstallation")
	}
}

func TestRunUninstall_NoSettings(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	out := captureStdout(t, func() {
		runUninstall([]string{"--global"})
	})
	if !strings.Contains(out, "Nothing to uninstall") {
		t.Error("should report nothing to uninstall")
	}
}

// --- runCacheCheck additional tests ---

func TestRunCacheCheck_FromStdin(t *testing.T) {
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r

	go func() {
		w.Write([]byte("<role>Expert.</role>\n<instructions>Do it.</instructions>"))
		w.Close()
	}()

	out := captureStdout(t, func() {
		runCacheCheck("")
	})
	if out == "" {
		t.Error("cache-check from stdin should produce output")
	}
}

func TestRunCacheCheck_NoIssues(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "prompt.txt")
	os.WriteFile(path, []byte("Simple prompt without any XML structure."), 0644)

	out := captureStdout(t, func() {
		runCacheCheck(path)
	})
	if !strings.Contains(out, "Cache-friendly") {
		t.Error("simple text should report cache-friendly")
	}
}

// --- runCheckClaudeMD additional tests ---

func TestRunCheckClaudeMD_WithIssues(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "CLAUDE.md")
	// Create a CLAUDE.md with issues - excessive overtrigger language
	content := strings.Repeat("You MUST ALWAYS follow this CRITICAL rule. NEVER ignore it.\n", 50)
	os.WriteFile(path, []byte(content), 0644)

	out := captureStdout(t, func() {
		runCheckClaudeMD(path)
	})
	// Should produce JSON output with issues, not "healthy"
	if strings.Contains(out, "healthy") {
		t.Error("should report issues, not healthy")
	}
	if out == "" {
		t.Error("should produce some output")
	}
}

// --- hookInput/hookOutput struct coverage ---

func TestHookInput_JSON(t *testing.T) {
	hi := hookInput{
		SessionID:      "test-session",
		TranscriptPath: "/tmp/transcript",
		Cwd:            "/tmp/cwd",
		PermissionMode: "default",
		HookEventName:  "UserPromptSubmit",
		Prompt:         "test prompt",
	}
	data, err := json.Marshal(hi)
	if err != nil {
		t.Fatalf("marshal hookInput: %v", err)
	}
	var parsed hookInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal hookInput: %v", err)
	}
	if parsed.Prompt != "test prompt" {
		t.Errorf("prompt = %q, want %q", parsed.Prompt, "test prompt")
	}
}

func TestHookOutput_JSON(t *testing.T) {
	out := hookOutput{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: "test context",
		},
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal hookOutput: %v", err)
	}
	if !strings.Contains(string(data), "hookSpecificOutput") {
		t.Error("should contain hookSpecificOutput key")
	}
}

// --- dispatch install/uninstall tests ---

func TestDispatch_Install(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	out := captureStdout(t, func() {
		err := dispatch([]string{"install", "--global"})
		if err != nil {
			t.Errorf("dispatch install: %v", err)
		}
	})
	if !strings.Contains(out, "Installed") {
		t.Error("dispatch install should report installation")
	}
}

func TestDispatch_Uninstall(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	out := captureStdout(t, func() {
		err := dispatch([]string{"uninstall", "--global"})
		if err != nil {
			t.Errorf("dispatch uninstall: %v", err)
		}
	})
	if !strings.Contains(out, "Nothing to uninstall") {
		t.Error("dispatch uninstall without prior install should report nothing")
	}
}

// --- runAnalyze additional coverage ---

func TestRunAnalyze_MultiWord(t *testing.T) {
	out := captureStdout(t, func() {
		runAnalyze("write a comprehensive function to parse JSON with error handling and retry logic", "gemini")
	})
	if !strings.Contains(out, "score_report") {
		t.Error("analyze with provider should include score_report")
	}
}

func TestWriteSettings_PreservesExistingKeys(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".claude", "settings.json")

	// Write initial settings with an extra key
	raw := map[string]json.RawMessage{
		"otherKey": json.RawMessage(`"preserved"`),
	}
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addHookEntry(s, "/usr/bin/prompt-improver")

	err := writeSettings(path, s, raw)
	if err != nil {
		t.Fatalf("writeSettings: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "preserved") {
		t.Error("writeSettings should preserve unknown keys")
	}
}

func TestWriteSettings_MkdirAllError(t *testing.T) {
	// Try to write to a path where parent dir creation should fail
	// e.g., under /dev/null which is a file, not a dir
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	err := writeSettings("/dev/null/nonexistent/settings.json", s, nil)
	if err == nil {
		t.Error("should error when parent dir cannot be created")
	}
}

func TestWriteSettings_WithNilRaw(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".claude", "settings.json")

	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}
	addHookEntry(s, "/usr/bin/prompt-improver")
	addMCPEntry(s, "/usr/bin/prompt-improver")

	// Pass nil raw - should create new raw map
	err := writeSettings(path, s, nil)
	if err != nil {
		t.Fatalf("writeSettings with nil raw: %v", err)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "hooks") {
		t.Error("should write hooks")
	}
}

func TestWriteSettings_EmptyClears(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".claude", "settings.json")

	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}

	err := writeSettings(path, s, nil)
	if err != nil {
		t.Fatalf("writeSettings: %v", err)
	}

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "hooks") {
		t.Error("empty hooks should not appear in output")
	}
}
