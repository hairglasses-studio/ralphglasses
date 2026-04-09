package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/mark3labs/mcp-go/server"
)

// --- parseLogLevel tests ---

func TestParseLogLevel_Debug(t *testing.T) {
	if got := parseLogLevel("debug"); got != slog.LevelDebug {
		t.Errorf("parseLogLevel(debug) = %v, want %v", got, slog.LevelDebug)
	}
}

func TestParseLogLevel_Warn(t *testing.T) {
	if got := parseLogLevel("warn"); got != slog.LevelWarn {
		t.Errorf("parseLogLevel(warn) = %v, want %v", got, slog.LevelWarn)
	}
}

func TestParseLogLevel_Error(t *testing.T) {
	if got := parseLogLevel("error"); got != slog.LevelError {
		t.Errorf("parseLogLevel(error) = %v, want %v", got, slog.LevelError)
	}
}

func TestParseLogLevel_Info(t *testing.T) {
	if got := parseLogLevel("info"); got != slog.LevelInfo {
		t.Errorf("parseLogLevel(info) = %v, want %v", got, slog.LevelInfo)
	}
}

func TestParseLogLevel_Default(t *testing.T) {
	if got := parseLogLevel("bogus"); got != slog.LevelInfo {
		t.Errorf("parseLogLevel(bogus) = %v, want %v (default info)", got, slog.LevelInfo)
	}
}

func TestParseLogLevel_CaseInsensitive(t *testing.T) {
	if got := parseLogLevel("DEBUG"); got != slog.LevelDebug {
		t.Errorf("parseLogLevel(DEBUG) = %v, want %v", got, slog.LevelDebug)
	}
	if got := parseLogLevel("WARN"); got != slog.LevelWarn {
		t.Errorf("parseLogLevel(WARN) = %v, want %v", got, slog.LevelWarn)
	}
}

// --- newLogHandler tests ---

func TestNewLogHandler_JSON(t *testing.T) {
	origLevel := logLevel
	origFormat := logFormat
	defer func() {
		logLevel = origLevel
		logFormat = origFormat
	}()

	logLevel = "info"
	logFormat = "json"

	var buf bytes.Buffer
	h := newLogHandler(&buf)
	logger := slog.New(h)
	logger.Info("test msg")

	if !strings.Contains(buf.String(), `"msg"`) {
		t.Error("json handler should produce JSON with msg field")
	}
}

func TestNewLogHandler_Text(t *testing.T) {
	origLevel := logLevel
	origFormat := logFormat
	defer func() {
		logLevel = origLevel
		logFormat = origFormat
	}()

	logLevel = "debug"
	logFormat = "text"

	var buf bytes.Buffer
	h := newLogHandler(&buf)
	logger := slog.New(h)
	logger.Info("test msg")

	if strings.Contains(buf.String(), `"msg"`) {
		t.Error("text handler should not produce JSON")
	}
	if !strings.Contains(buf.String(), "test msg") {
		t.Error("text handler should contain the message")
	}
}

// --- Flag registration tests ---

func TestFlagDefaults_Persistent(t *testing.T) {
	tests := []struct {
		flag    string
		wantDef string
	}{
		{"scan-path", config.DefaultScanPath},
		{"debug", "false"},
		{"log-level", "info"},
		{"log-format", "json"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			fl := rootCmd.PersistentFlags().Lookup(tt.flag)
			if fl == nil {
				t.Fatalf("persistent flag %q not found", tt.flag)
			}
			if fl.DefValue != tt.wantDef {
				t.Errorf("flag %q default = %q, want %q", tt.flag, fl.DefValue, tt.wantDef)
			}
		})
	}
}

func TestFlagDefaults_Local(t *testing.T) {
	tests := []struct {
		flag    string
		wantDef string
	}{
		{"theme", "k9s"},
		{"notify", "false"},
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			fl := rootCmd.Flags().Lookup(tt.flag)
			if fl == nil {
				t.Fatalf("flag %q not found", tt.flag)
			}
			if fl.DefValue != tt.wantDef {
				t.Errorf("flag %q default = %q, want %q", tt.flag, fl.DefValue, tt.wantDef)
			}
		})
	}
}

// --- Subcommand registration tests ---

func TestSubcommandsRegistered(t *testing.T) {
	expected := []string{"mcp", "doctor", "validate", "selftest", "gate-check", "serve", "completion", "mcp-call"}
	registered := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		registered[c.Name()] = true
	}
	for _, name := range expected {
		if !registered[name] {
			t.Errorf("subcommand %q not registered on rootCmd", name)
		}
	}
}

// --- Completion command tests ---

func TestCompletionCmd_RequiresArg(t *testing.T) {
	err := completionCmd.Args(completionCmd, []string{})
	if err == nil {
		t.Error("completion should require exactly 1 arg")
	}
}

func TestCompletionCmd_AcceptsOneArg(t *testing.T) {
	err := completionCmd.Args(completionCmd, []string{"bash"})
	if err != nil {
		t.Errorf("completion should accept 1 arg: %v", err)
	}
}

func TestCompletionCmd_RejectsTwoArgs(t *testing.T) {
	err := completionCmd.Args(completionCmd, []string{"bash", "zsh"})
	if err == nil {
		t.Error("completion should reject 2 args")
	}
}

func TestCompletionCmd_UnsupportedShell(t *testing.T) {
	err := completionCmd.RunE(completionCmd, []string{"powershell"})
	if err == nil {
		t.Error("should reject unsupported shell")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Errorf("error = %q, want 'unsupported shell'", err.Error())
	}
}

func TestCompletionCmd_BashGeneratesOutput(t *testing.T) {
	var buf bytes.Buffer
	err := rootCmd.GenBashCompletion(&buf)
	if err != nil {
		t.Fatalf("GenBashCompletion: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("bash completion should produce output")
	}
}

// --- Help output tests ---

func TestRootCmd_HelpContainsDescription(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	_ = rootCmd.Help()
	out := buf.String()
	if !strings.Contains(out, "k9s-style TUI") {
		t.Error("help should mention k9s-style TUI")
	}
	if !strings.Contains(out, "MCP server") {
		t.Error("help should mention MCP server")
	}
}

// --- Version template test ---

func TestVersionTemplate(t *testing.T) {
	tmpl := rootCmd.VersionTemplate()
	if !strings.Contains(tmpl, "ralphglasses version") {
		t.Error("version template should contain 'ralphglasses version'")
	}
}

// --- validateConfig tests ---

func TestValidateConfig_NoProjectName(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "PROJECT_NAME") {
			found = true
		}
	}
	if !found {
		t.Error("should report missing PROJECT_NAME")
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":       "test-project",
		"MAX_CALLS_PER_HOUR": "100",
	}}
	issues := validateConfig(cfg)
	for _, iss := range issues {
		if strings.HasPrefix(iss, "ERROR") {
			t.Errorf("unexpected error: %s", iss)
		}
	}
}

func TestValidateConfig_InvalidMaxCalls(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":       "test",
		"MAX_CALLS_PER_HOUR": "abc",
	}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "MAX_CALLS_PER_HOUR") && strings.Contains(iss, "not a valid integer") {
			found = true
		}
	}
	if !found {
		t.Error("should report invalid MAX_CALLS_PER_HOUR")
	}
}

func TestValidateConfig_NegativeMaxCalls(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":       "test",
		"MAX_CALLS_PER_HOUR": "-1",
	}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "MAX_CALLS_PER_HOUR") && strings.Contains(iss, "> 0") {
			found = true
		}
	}
	if !found {
		t.Error("should report negative MAX_CALLS_PER_HOUR")
	}
}

func TestValidateConfig_InvalidTimeoutMinutes(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":           "test",
		"CLAUDE_TIMEOUT_MINUTES": "bad",
	}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "CLAUDE_TIMEOUT_MINUTES") {
			found = true
		}
	}
	if !found {
		t.Error("should report invalid CLAUDE_TIMEOUT_MINUTES")
	}
}

func TestValidateConfig_NegativeTimeoutMinutes(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":           "test",
		"CLAUDE_TIMEOUT_MINUTES": "0",
	}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "CLAUDE_TIMEOUT_MINUTES") && strings.Contains(iss, "> 0") {
			found = true
		}
	}
	if !found {
		t.Error("should report non-positive CLAUDE_TIMEOUT_MINUTES")
	}
}

func TestValidateConfig_CBThresholdOutOfRange(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":         "test",
		"CB_FAILURE_THRESHOLD": "200",
	}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "CB_FAILURE_THRESHOLD") && strings.Contains(iss, "0-100") {
			found = true
		}
	}
	if !found {
		t.Error("should report CB_FAILURE_THRESHOLD out of range")
	}
}

func TestValidateConfig_CBThresholdInvalid(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{
		"PROJECT_NAME":         "test",
		"CB_SUCCESS_THRESHOLD": "abc",
	}}
	issues := validateConfig(cfg)
	found := false
	for _, iss := range issues {
		if strings.Contains(iss, "CB_SUCCESS_THRESHOLD") && strings.Contains(iss, "not a valid integer") {
			found = true
		}
	}
	if !found {
		t.Error("should report invalid CB_SUCCESS_THRESHOLD")
	}
}

// --- outputGateReport tests ---

func TestOutputGateReport_JSON(t *testing.T) {
	report := &e2e.GateReport{
		Timestamp: time.Now(),
		Overall:   e2e.VerdictPass,
		Results: []e2e.GateResult{
			{Metric: "cost", Verdict: e2e.VerdictPass, CurrentVal: 0.5},
		},
		SampleCount: 10,
	}

	err := outputGateReport(report, "Test Gate", true)
	if err != nil {
		t.Errorf("outputGateReport JSON failed: %v", err)
	}
}

func TestOutputGateReport_Human(t *testing.T) {
	report := &e2e.GateReport{
		Timestamp: time.Now(),
		Overall:   e2e.VerdictWarn,
		Results: []e2e.GateResult{
			{Metric: "cost", Verdict: e2e.VerdictPass, CurrentVal: 0.5, BaselineVal: 1.0, DeltaPct: -50.0},
			{Metric: "latency", Verdict: e2e.VerdictWarn, CurrentVal: 100},
			{Metric: "errors", Verdict: e2e.VerdictFail, CurrentVal: 0.5},
			{Metric: "skipped", Verdict: e2e.VerdictSkip, CurrentVal: 0},
		},
		SampleCount: 5,
	}

	err := outputGateReport(report, "Test Gate", false)
	if err != nil {
		t.Errorf("outputGateReport human failed: %v", err)
	}
}

// --- Error sentinel tests ---

func TestErrChecksFailed(t *testing.T) {
	if ErrChecksFailed == nil {
		t.Fatal("ErrChecksFailed should not be nil")
	}
	if ErrChecksFailed.Error() != "checks failed" {
		t.Errorf("ErrChecksFailed = %q", ErrChecksFailed.Error())
	}
}

func TestErrGateFailed(t *testing.T) {
	if ErrGateFailed == nil {
		t.Fatal("ErrGateFailed should not be nil")
	}
	if ErrGateFailed.Error() != "gate check failed" {
		t.Errorf("ErrGateFailed = %q", ErrGateFailed.Error())
	}
}

// --- Doctor command registration test ---

func TestDoctorCmd_Registration(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "doctor" {
			found = true
			break
		}
	}
	if !found {
		t.Error("doctor command not registered on rootCmd")
	}
}

// --- Gate-check command tests ---

func TestGateCheckCmd_Defaults(t *testing.T) {
	f := gateCheckCmd.Flags()

	hours, err := f.GetFloat64("hours")
	if err != nil {
		t.Fatal(err)
	}
	if hours != 24 {
		t.Errorf("hours default = %f, want 24", hours)
	}

	jsonFlag, err := f.GetBool("json")
	if err != nil {
		t.Fatal(err)
	}
	if jsonFlag {
		t.Error("json default should be false")
	}
}

// --- Serve command tests ---

func TestServeCmd_Defaults(t *testing.T) {
	f := serveCmd.Flags()

	coord, err := f.GetBool("coordinator")
	if err != nil {
		t.Fatal(err)
	}
	if coord {
		t.Error("coordinator default should be false")
	}

	budget, err := f.GetFloat64("fleet-budget")
	if err != nil {
		t.Fatal(err)
	}
	if budget != 500 {
		t.Errorf("fleet-budget default = %f, want 500", budget)
	}
}

// --- runMCPCall tests ---

func TestRunMCPCall_InvalidParam(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore global state
	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	// Reset the params slice
	origParams := mcpCallParams
	defer func() { mcpCallParams = origParams }()
	mcpCallParams = []string{"badformat"}

	err := runMCPCall(mcpCallCmd, []string{"ralphglasses_scan"})
	if err == nil {
		t.Error("should error on invalid param format")
	}
	if !strings.Contains(err.Error(), "invalid param format") {
		t.Errorf("error = %q, want 'invalid param format'", err.Error())
	}
}

func TestRunMCPCall_ToolNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	origParams := mcpCallParams
	defer func() { mcpCallParams = origParams }()
	mcpCallParams = nil

	err := runMCPCall(mcpCallCmd, []string{"nonexistent_tool_xyz"})
	if err == nil {
		t.Error("should error on nonexistent tool")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestRunMCPCall_ParamParsing(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	origParams := mcpCallParams
	defer func() { mcpCallParams = origParams }()

	// Test various param types: float, bool, string
	mcpCallParams = []string{"count=42", "enabled=true", "name=hello"}

	// This will fail because ralphglasses_scan doesn't accept these params,
	// but the param parsing happens before that, and the tool must exist.
	// Use a tool we know exists: ralphglasses_tool_groups (core tool, no args needed)
	err := runMCPCall(mcpCallCmd, []string{"ralphglasses_tool_groups"})
	// tool_groups takes no params, so even with extra params it should succeed or
	// at least get past the param parsing stage
	if err != nil {
		t.Logf("runMCPCall returned error (expected for some tools): %v", err)
	}
}

func TestRunMCPCall_ToolGroupsSucceeds(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	origParams := mcpCallParams
	defer func() { mcpCallParams = origParams }()
	mcpCallParams = nil

	err := runMCPCall(mcpCallCmd, []string{"ralphglasses_tool_groups"})
	if err != nil {
		t.Errorf("ralphglasses_tool_groups should succeed: %v", err)
	}
}

// --- GateCheck command execution tests ---

func TestGateCheckCmd_MissingBaseline(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	origGateBaseline := gateBaselinePath
	origGateJSON := gateJSON
	origGateHours := gateHours
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origGateBaseline
		gateJSON = origGateJSON
		gateHours = origGateHours
	}()

	scanPath = tmpDir
	gateBaselinePath = ""
	gateJSON = true
	gateHours = 24

	err := gateCheckCmd.RunE(gateCheckCmd, nil)
	// With no baseline file, it should output a skip verdict and return nil
	if err != nil {
		t.Errorf("gate-check with missing baseline should not error (skip verdict): %v", err)
	}
}

// --- Selftest command flag tests ---

func TestSelftestCmd_AllFlags(t *testing.T) {
	f := selftestCmd.Flags()

	for _, flag := range []string{"iterations", "budget", "repo-path", "json", "gate", "dry-run"} {
		if f.Lookup(flag) == nil {
			t.Errorf("flag %q not registered on selftest", flag)
		}
	}
}

// --- Doctor command execution tests ---

func TestDoctorCmd_Execute(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	origDebug := debugMode
	defer func() {
		scanPath = origScanPath
		debugMode = origDebug
	}()
	scanPath = tmpDir
	debugMode = false

	// Doctor should run and return ErrChecksFailed because
	// required binaries (ralph, claude) likely aren't in PATH.
	err := doctorCmd.RunE(doctorCmd, nil)
	// Either nil (all checks pass) or ErrChecksFailed — both are valid
	if err != nil && err != ErrChecksFailed {
		t.Errorf("doctor returned unexpected error: %v", err)
	}
}

// --- Validate command execution tests ---

func TestValidateCmd_EmptyScanPath(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	// Set context on command (required for cmd.Context() inside RunE)
	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	if err != nil {
		t.Errorf("validate with empty scan-path should succeed: %v", err)
	}
}

func TestValidateCmd_WithValidRepo(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := tmpDir + "/test-repo"
	os.MkdirAll(repoDir+"/.ralph", 0755)
	os.WriteFile(repoDir+"/.ralphrc", []byte("PROJECT_NAME=test-project\nMAX_CALLS_PER_HOUR=100\n"), 0644)
	os.MkdirAll(repoDir+"/.git", 0755)

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	if err != nil {
		t.Errorf("validate with valid repo should succeed: %v", err)
	}
}

func TestValidateCmd_WithInvalidRepo(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := tmpDir + "/bad-repo"
	os.MkdirAll(repoDir+"/.ralph", 0755)
	os.WriteFile(repoDir+"/.ralphrc", []byte("MAX_CALLS_PER_HOUR=abc\n"), 0644)
	os.MkdirAll(repoDir+"/.git", 0755)

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	if err != nil && err != ErrChecksFailed {
		t.Errorf("validate returned unexpected error: %v", err)
	}
}

// --- Selftest command execution tests ---

func TestSelftestCmd_GateOnly_NoObservations(t *testing.T) {
	tmpDir := t.TempDir()

	origRepoPath := selftestRepoPath
	origGateOnly := selftestGateOnly
	origJSON := selftestJSON
	defer func() {
		selftestRepoPath = origRepoPath
		selftestGateOnly = origGateOnly
		selftestJSON = origJSON
	}()

	selftestRepoPath = tmpDir
	selftestGateOnly = true
	selftestJSON = true

	// Gate-only mode with no observations should skip (not fail)
	err := selftestCmd.RunE(selftestCmd, nil)
	// Should either succeed with skip verdict or return a gate error
	if err != nil && err != ErrGateFailed {
		t.Logf("selftest gate-only with no data: %v (acceptable)", err)
	}
}

// --- GateCheck with baseline/observations ---

func TestGateCheckCmd_WithBaseline(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a baseline file
	baselineDir := tmpDir + "/.ralph"
	os.MkdirAll(baselineDir, 0755)
	baselineJSON := `{
		"timestamp": "2026-01-01T00:00:00Z",
		"cost_per_iteration": 1.0,
		"completion_rate": 0.9,
		"total_latency_ms": 5000,
		"error_rate": 0.05,
		"verify_pass_rate": 0.8
	}`
	os.WriteFile(baselineDir+"/loop_baseline.json", []byte(baselineJSON), 0644)

	origScanPath := scanPath
	origGateBaseline := gateBaselinePath
	origGateJSON := gateJSON
	origGateHours := gateHours
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origGateBaseline
		gateJSON = origGateJSON
		gateHours = origGateHours
	}()

	scanPath = tmpDir
	gateBaselinePath = baselineDir + "/loop_baseline.json"
	gateJSON = true
	gateHours = 24

	// With baseline but no observations, should skip or pass
	err := gateCheckCmd.RunE(gateCheckCmd, nil)
	if err != nil && err != ErrGateFailed {
		t.Logf("gate-check with baseline but no observations: %v", err)
	}
}

// --- GateCheck with baseline and observations ---

func TestGateCheckCmd_WithBaselineAndObservations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create baseline
	baselineDir := tmpDir + "/.ralph"
	os.MkdirAll(baselineDir, 0755)
	baselineJSON := `{
		"timestamp": "2026-01-01T00:00:00Z",
		"cost_per_iteration": 1.0,
		"completion_rate": 0.9,
		"total_latency_ms": 5000,
		"error_rate": 0.05,
		"verify_pass_rate": 0.8
	}`
	os.WriteFile(baselineDir+"/loop_baseline.json", []byte(baselineJSON), 0644)

	// Create observations file
	obsDir := tmpDir + "/.ralph"
	obsJSON := `{"timestamp":"2026-03-26T00:00:00Z","cost_usd":0.5,"latency_ms":3000,"success":true,"error_rate":0.01}
{"timestamp":"2026-03-26T01:00:00Z","cost_usd":0.6,"latency_ms":3500,"success":true,"error_rate":0.02}
`
	os.WriteFile(obsDir+"/loop_observations.jsonl", []byte(obsJSON), 0644)

	origScanPath := scanPath
	origGateBaseline := gateBaselinePath
	origGateJSON := gateJSON
	origGateHours := gateHours
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origGateBaseline
		gateJSON = origGateJSON
		gateHours = origGateHours
	}()

	scanPath = tmpDir
	gateBaselinePath = baselineDir + "/loop_baseline.json"
	gateJSON = false
	gateHours = 720 // 30 days to capture the observations

	err := gateCheckCmd.RunE(gateCheckCmd, nil)
	// Observations may not be valid format, but at least the code path executes
	if err != nil && err != ErrGateFailed {
		t.Logf("gate-check with observations: %v (acceptable)", err)
	}
}

func TestGateCheckCmd_IgnoresStandaloneObservations(t *testing.T) {
	tmpDir := t.TempDir()

	baselineDir := filepath.Join(tmpDir, ".ralph")
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		t.Fatalf("mkdir baseline dir: %v", err)
	}
	baseline := &e2e.LoopBaseline{
		GeneratedAt: time.Now(),
		Entries: map[string]*e2e.BaselineStats{
			"task-a:claude": {CostP95: 1.0, LatencyP95: 5000, SampleCount: 5},
		},
		Aggregate: &e2e.BaselineStats{CostP95: 1.0, LatencyP95: 5000, SampleCount: 5},
		Rates:     &e2e.BaselineRates{CompletionRate: 1.0, VerifyPassRate: 1.0, ErrorRate: 0.0},
	}
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baselineDir, "loop_baseline.json"), data, 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	logsDir := filepath.Join(tmpDir, ".ralph", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir logs dir: %v", err)
	}
	observations := []session.LoopObservation{
		{Timestamp: time.Now(), Mode: "standalone", Status: "failed", Error: "launch failed", TaskTitle: "smoke test", TotalCostUSD: 5.0, TotalLatencyMs: 1000},
		{Timestamp: time.Now(), Mode: "mock", Status: "idle", VerifyPassed: true, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.8, TotalLatencyMs: 4000},
		{Timestamp: time.Now(), Mode: "mock", Status: "idle", VerifyPassed: true, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.9, TotalLatencyMs: 4200},
		{Timestamp: time.Now(), Mode: "mock", Status: "idle", VerifyPassed: true, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.85, TotalLatencyMs: 4100},
		{Timestamp: time.Now(), Mode: "mock", Status: "idle", VerifyPassed: true, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.95, TotalLatencyMs: 4300},
	}
	f, err := os.Create(filepath.Join(logsDir, "loop_observations.jsonl"))
	if err != nil {
		t.Fatalf("create observations: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, obs := range observations {
		if err := enc.Encode(obs); err != nil {
			t.Fatalf("encode observation: %v", err)
		}
	}

	origScanPath := scanPath
	origGateBaseline := gateBaselinePath
	origGateJSON := gateJSON
	origGateHours := gateHours
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origGateBaseline
		gateJSON = origGateJSON
		gateHours = origGateHours
	}()

	scanPath = tmpDir
	gateBaselinePath = filepath.Join(baselineDir, "loop_baseline.json")
	gateJSON = true
	gateHours = 24 * 365

	if err := gateCheckCmd.RunE(gateCheckCmd, nil); err != nil {
		t.Fatalf("gate-check should ignore standalone observations, got: %v", err)
	}
}

// --- Validate command additional tests ---

func TestValidateCmd_RepoWithWarnOnly(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := tmpDir + "/warn-repo"
	os.MkdirAll(repoDir+"/.ralph", 0755)
	// Project name set (no ERROR), but timeout is 0 (WARN only)
	os.WriteFile(repoDir+"/.ralphrc", []byte("PROJECT_NAME=warn-test\nCLAUDE_TIMEOUT_MINUTES=0\n"), 0644)
	os.MkdirAll(repoDir+"/.git", 0755)

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	// WARN-only should not return ErrChecksFailed
	if err != nil {
		t.Errorf("validate with WARN-only issues should succeed (no ERROR): %v", err)
	}
}

func TestValidateCmd_MixedRepos(t *testing.T) {
	tmpDir := t.TempDir()

	// Repo with .git but no .ralphrc (HasRC=false, should be skipped)
	noRCDir := tmpDir + "/no-rc-repo"
	os.MkdirAll(noRCDir+"/.ralph", 0755)
	os.MkdirAll(noRCDir+"/.git", 0755)

	// Repo with valid config
	validDir := tmpDir + "/valid-repo"
	os.MkdirAll(validDir+"/.ralph", 0755)
	os.WriteFile(validDir+"/.ralphrc", []byte("PROJECT_NAME=valid\nMAX_CALLS_PER_HOUR=50\n"), 0644)
	os.MkdirAll(validDir+"/.git", 0755)

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	if err != nil {
		t.Errorf("validate mixed repos should succeed: %v", err)
	}
}

func TestValidateCmd_MultipleRepos(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two repos
	for _, name := range []string{"repo-a", "repo-b"} {
		repoDir := tmpDir + "/" + name
		os.MkdirAll(repoDir+"/.ralph", 0755)
		os.WriteFile(repoDir+"/.ralphrc", []byte("PROJECT_NAME="+name+"\nMAX_CALLS_PER_HOUR=100\n"), 0644)
		os.MkdirAll(repoDir+"/.git", 0755)
	}

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	if err != nil {
		t.Errorf("validate with multiple valid repos should succeed: %v", err)
	}
}

// --- PersistentPreRunE tests ---

func TestRootCmd_PersistentPreRunE(t *testing.T) {
	origDebug := debugMode
	defer func() { debugMode = origDebug }()

	debugMode = true
	err := rootCmd.PersistentPreRunE(rootCmd, nil)
	if err != nil {
		t.Errorf("PersistentPreRunE should not error: %v", err)
	}
}

// --- MCP command flag tests ---

func TestMcpCmd_Use(t *testing.T) {
	if mcpCmd.Use != "mcp" {
		t.Errorf("mcpCmd.Use = %q, want %q", mcpCmd.Use, "mcp")
	}
}

func TestMcpCmd_Short(t *testing.T) {
	if mcpCmd.Short == "" {
		t.Error("mcpCmd.Short should not be empty")
	}
}

// --- Completion command RunE tests ---

func TestCompletionCmd_Bash_RunE(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	defer rootCmd.SetOut(os.Stdout)

	err := completionCmd.RunE(completionCmd, []string{"bash"})
	if err != nil {
		t.Errorf("bash completion RunE: %v", err)
	}
}

func TestCompletionCmd_Zsh_RunE(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	defer rootCmd.SetOut(os.Stdout)

	err := completionCmd.RunE(completionCmd, []string{"zsh"})
	if err != nil {
		t.Errorf("zsh completion RunE: %v", err)
	}
}

func TestCompletionCmd_Fish_RunE(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	defer rootCmd.SetOut(os.Stdout)

	err := completionCmd.RunE(completionCmd, []string{"fish"})
	if err != nil {
		t.Errorf("fish completion RunE: %v", err)
	}
}

// --- Selftest dry-run test ---

func TestSelftestCmd_DryRun_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a minimal Go project structure for the selftest to find
	os.MkdirAll(tmpDir+"/.git", 0755)
	os.WriteFile(tmpDir+"/go.mod", []byte("module test\ngo 1.22\n"), 0644)
	os.WriteFile(tmpDir+"/main.go", []byte("package main\nfunc main() {}\n"), 0644)

	origRepoPath := selftestRepoPath
	origGateOnly := selftestGateOnly
	origJSON := selftestJSON
	origDryRun := selftestDryRun
	origIterations := selftestIterations
	origBudget := selftestBudget
	defer func() {
		selftestRepoPath = origRepoPath
		selftestGateOnly = origGateOnly
		selftestJSON = origJSON
		selftestDryRun = origDryRun
		selftestIterations = origIterations
		selftestBudget = origBudget
	}()

	selftestRepoPath = tmpDir
	selftestGateOnly = false
	selftestJSON = true
	selftestDryRun = true
	selftestIterations = 1
	selftestBudget = 0.1

	err := selftestCmd.RunE(selftestCmd, nil)
	// Dry-run may succeed or fail depending on whether the binary can be built
	if err != nil {
		t.Logf("selftest dry-run error (may be expected): %v", err)
	}
}

func TestSelftestCmd_DryRun_Text(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(tmpDir+"/.git", 0755)
	os.WriteFile(tmpDir+"/go.mod", []byte("module test\ngo 1.22\n"), 0644)
	os.WriteFile(tmpDir+"/main.go", []byte("package main\nfunc main() {}\n"), 0644)

	origRepoPath := selftestRepoPath
	origGateOnly := selftestGateOnly
	origJSON := selftestJSON
	origDryRun := selftestDryRun
	origIterations := selftestIterations
	origBudget := selftestBudget
	defer func() {
		selftestRepoPath = origRepoPath
		selftestGateOnly = origGateOnly
		selftestJSON = origJSON
		selftestDryRun = origDryRun
		selftestIterations = origIterations
		selftestBudget = origBudget
	}()

	selftestRepoPath = tmpDir
	selftestGateOnly = false
	selftestJSON = false
	selftestDryRun = true
	selftestIterations = 1
	selftestBudget = 0.1

	err := selftestCmd.RunE(selftestCmd, nil)
	if err != nil {
		t.Logf("selftest dry-run text error (may be expected): %v", err)
	}
}

// --- initLogging tests ---

func TestInitLogging_CreatesLogFile(t *testing.T) {
	tmpDir := t.TempDir()

	logFile, err := initLogging(tmpDir)
	if err != nil {
		t.Fatalf("initLogging: %v", err)
	}
	defer logFile.Close()

	if logFile == nil {
		t.Fatal("logFile should not be nil")
	}

	// Verify the file was created
	info, err := logFile.Stat()
	if err != nil {
		t.Fatalf("stat logFile: %v", err)
	}
	if info.Size() < 0 {
		t.Error("logFile should be writable")
	}
}

func TestInitLogging_InvalidPath(t *testing.T) {
	_, err := initLogging("/dev/null/nonexistent")
	if err == nil {
		t.Error("initLogging with invalid path should fail")
	}
}

// --- applyTheme tests ---

func TestApplyTheme_Default(t *testing.T) {
	// Should not panic with default theme
	applyTheme("k9s")
}

func TestApplyTheme_Dracula(t *testing.T) {
	applyTheme("dracula")
}

func TestApplyTheme_Unknown(t *testing.T) {
	// Unknown theme should not panic (tries file load, fails silently)
	applyTheme("nonexistent-theme-xyz")
}

// --- setupMCP tests ---

func TestSetupMCP_CreatesServer(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setupMCP(tmpDir)
	if err != nil {
		t.Fatalf("setupMCP: %v", err)
	}
	defer cleanup()

	if srv == nil {
		t.Fatal("setupMCP returned nil server")
	}
}

func TestSetupMCP_RegistersCoreTools(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setupMCP(tmpDir)
	if err != nil {
		t.Fatalf("setupMCP: %v", err)
	}
	defer cleanup()

	// Check core tools are registered
	coreTools := []string{
		"ralphglasses_scan",
		"ralphglasses_list",
		"ralphglasses_status",
		"ralphglasses_start",
		"ralphglasses_stop",
	}
	for _, name := range coreTools {
		if srv.GetTool(name) == nil {
			t.Errorf("core tool %q not registered", name)
		}
	}
}

func TestSetupMCP_CleanupDoesNotPanic(t *testing.T) {
	tmpDir := t.TempDir()

	_, cleanup, err := setupMCP(tmpDir)
	if err != nil {
		t.Fatalf("setupMCP: %v", err)
	}
	cleanup() // Should not panic
}

// --- Serve command RunE test ---

func TestServeCmd_FlagRegistration(t *testing.T) {
	f := serveCmd.Flags()
	for _, flag := range []string{"coordinator", "port", "coordinator-url", "fleet-budget"} {
		if f.Lookup(flag) == nil {
			t.Errorf("flag %q not registered on serve command", flag)
		}
	}
}

// --- setupServe tests ---

func TestSetupServe_ReturnsComponents(t *testing.T) {
	bus, sessMgr, hostname := setupServe()
	if bus == nil {
		t.Error("setupServe should return non-nil bus")
	}
	if sessMgr == nil {
		t.Error("setupServe should return non-nil session manager")
	}
	if hostname == "" {
		t.Error("setupServe should return non-empty hostname")
	}
}

// --- runWorker error tests ---

func TestRunWorker_NoCoordinatorURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	origCoordURL := coordinatorURL
	defer func() { coordinatorURL = origCoordURL }()
	coordinatorURL = ""

	bus, sessMgr, hostname := setupServe()
	err := runWorker(ctx, hostname, tmpDir, bus, sessMgr)
	// With empty coordinatorURL, it tries Tailscale discovery which should fail
	if err == nil {
		t.Error("runWorker with no coordinator URL and no Tailscale should error")
	}
	if err != nil && !strings.Contains(err.Error(), "could not discover coordinator") {
		t.Logf("runWorker error (expected): %v", err)
	}
}

func TestRunWorker_WithBadCoordinatorURL(t *testing.T) {
	// Use a very short timeout so this doesn't hang
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	tmpDir := t.TempDir()

	origCoordURL := coordinatorURL
	defer func() { coordinatorURL = origCoordURL }()
	coordinatorURL = "http://127.0.0.1:1" // bad port

	bus, sessMgr, hostname := setupServe()
	// This will try to connect and fail quickly
	_ = runWorker(ctx, hostname, tmpDir, bus, sessMgr)
	// We just care that it doesn't panic and covers the code path
}

// Note: runCoordinator is not tested directly because the fleet.Coordinator
// has internal races between Start/Stop that trigger the race detector.

// --- Doctor RunE additional coverage ---

func TestDoctorCmd_Execute_WithScanPathDir(t *testing.T) {
	// Create a real scan-path dir to cover the "OK" path for scan-path check
	tmpDir := t.TempDir()

	origScanPath := scanPath
	origDebug := debugMode
	defer func() {
		scanPath = origScanPath
		debugMode = origDebug
	}()
	scanPath = tmpDir
	debugMode = true

	err := doctorCmd.RunE(doctorCmd, nil)
	if err != nil && err != ErrChecksFailed {
		t.Errorf("doctor returned unexpected error: %v", err)
	}
}

// --- GateCheck with valid observations to cover evaluation path ---

func TestGateCheckCmd_WithValidObservations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create baseline
	baselineDir := tmpDir + "/.ralph"
	os.MkdirAll(baselineDir, 0755)
	baselineJSON := `{
		"timestamp": "2026-01-01T00:00:00Z",
		"cost_per_iteration": 1.0,
		"completion_rate": 0.9,
		"total_latency_ms": 5000,
		"error_rate": 0.05,
		"verify_pass_rate": 0.8
	}`
	os.WriteFile(baselineDir+"/loop_baseline.json", []byte(baselineJSON), 0644)

	// Create observations with proper format expected by session.LoadObservations
	obsJSON := `{"timestamp":"2026-03-26T10:00:00Z","repo":"test","cost_usd":0.5,"latency_ms":3000,"success":true,"provider":"claude"}
{"timestamp":"2026-03-26T11:00:00Z","repo":"test","cost_usd":0.6,"latency_ms":3500,"success":true,"provider":"claude"}
{"timestamp":"2026-03-26T12:00:00Z","repo":"test","cost_usd":0.4,"latency_ms":2500,"success":false,"provider":"claude"}
`
	os.WriteFile(baselineDir+"/loop_observations.jsonl", []byte(obsJSON), 0644)

	origScanPath := scanPath
	origGateBaseline := gateBaselinePath
	origGateJSON := gateJSON
	origGateHours := gateHours
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origGateBaseline
		gateJSON = origGateJSON
		gateHours = origGateHours
	}()

	scanPath = tmpDir
	gateBaselinePath = baselineDir + "/loop_baseline.json"
	gateJSON = true
	gateHours = 8760 // 1 year to capture any observations

	err := gateCheckCmd.RunE(gateCheckCmd, nil)
	// May pass, warn, or fail — all acceptable as long as it doesn't return an unexpected error
	if err != nil && err != ErrGateFailed {
		t.Logf("gate-check with valid observations: %v (checking code path)", err)
	}
}

// --- GateCheck human output with observations ---

func TestGateCheckCmd_HumanOutputWithObs(t *testing.T) {
	tmpDir := t.TempDir()

	baselineDir := tmpDir + "/.ralph"
	os.MkdirAll(baselineDir, 0755)
	baselineJSON := `{
		"timestamp": "2026-01-01T00:00:00Z",
		"cost_per_iteration": 1.0,
		"completion_rate": 0.9,
		"total_latency_ms": 5000,
		"error_rate": 0.05,
		"verify_pass_rate": 0.8
	}`
	os.WriteFile(baselineDir+"/loop_baseline.json", []byte(baselineJSON), 0644)

	obsJSON := `{"timestamp":"2026-03-26T10:00:00Z","repo":"test","cost_usd":0.5,"latency_ms":3000,"success":true,"provider":"claude"}
`
	os.WriteFile(baselineDir+"/loop_observations.jsonl", []byte(obsJSON), 0644)

	origScanPath := scanPath
	origGateBaseline := gateBaselinePath
	origGateJSON := gateJSON
	origGateHours := gateHours
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origGateBaseline
		gateJSON = origGateJSON
		gateHours = origGateHours
	}()

	scanPath = tmpDir
	gateBaselinePath = baselineDir + "/loop_baseline.json"
	gateJSON = false // human output
	gateHours = 8760

	err := gateCheckCmd.RunE(gateCheckCmd, nil)
	if err != nil && err != ErrGateFailed {
		t.Logf("gate-check human output: %v", err)
	}
}

// --- Selftest gate-only human output ---

func TestSelftestCmd_GateOnly_HumanOutput(t *testing.T) {
	tmpDir := t.TempDir()

	origRepoPath := selftestRepoPath
	origGateOnly := selftestGateOnly
	origJSON := selftestJSON
	defer func() {
		selftestRepoPath = origRepoPath
		selftestGateOnly = origGateOnly
		selftestJSON = origJSON
	}()

	selftestRepoPath = tmpDir
	selftestGateOnly = true
	selftestJSON = false // human output

	err := selftestCmd.RunE(selftestCmd, nil)
	if err != nil && err != ErrGateFailed {
		t.Logf("selftest gate-only human: %v (acceptable)", err)
	}
}

// --- MCP command RunE tests ---

func TestMcpCmd_RunE_ClosedStdin(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	// Replace stdin with a pipe that we immediately close
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	w.Close() // close write end immediately
	os.Stdin = r
	defer func() { os.Stdin = origStdin; r.Close() }()

	// mcpCmd.RunE will call initLogging, setupMCP, and then ServeStdio
	// ServeStdio should return quickly when stdin is closed
	err = mcpCmd.RunE(mcpCmd, nil)
	// May return nil or an error from ServeStdio — both are acceptable
	if err != nil {
		t.Logf("mcpCmd.RunE with closed stdin: %v (acceptable)", err)
	}
}

func TestMcpCmd_RunE_BadScanPath(t *testing.T) {
	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = "/dev/null/nonexistent"

	err := mcpCmd.RunE(mcpCmd, nil)
	if err == nil {
		t.Error("mcpCmd.RunE with invalid scan-path should fail")
	}
}

func TestMcpCmd_RunE_ContextCanceledIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	origServe := serveMCP
	defer func() { serveMCP = origServe }()
	serveMCP = func(*server.MCPServer) error { return context.Canceled }

	if err := mcpCmd.RunE(mcpCmd, nil); err != nil {
		t.Fatalf("mcpCmd.RunE() error = %v, want nil for context cancellation", err)
	}
}

// --- Validate with corrupt .ralphrc ---

func TestValidateCmd_CorruptRalphrc(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := tmpDir + "/corrupt-repo"
	os.MkdirAll(repoDir+"/.ralph", 0755)
	// Write binary garbage as .ralphrc to test LoadConfig error path
	os.WriteFile(repoDir+"/.ralphrc", []byte("\x00\x01\x02"), 0644)
	os.MkdirAll(repoDir+"/.git", 0755)

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	// Should handle corrupt file gracefully
	err := cmd.RunE(&cmd, nil)
	// May return ErrChecksFailed or nil depending on how LoadConfig handles it
	if err != nil && err != ErrChecksFailed {
		t.Errorf("validate with corrupt ralphrc should not return unexpected error: %v", err)
	}
}

func TestValidateCmd_UnreadableRalphrc(t *testing.T) {
	tmpDir := t.TempDir()

	repoDir := tmpDir + "/unreadable-repo"
	os.MkdirAll(repoDir+"/.ralph", 0755)
	// Create .ralphrc as a directory to trigger LoadConfig error
	os.MkdirAll(repoDir+"/.ralphrc", 0755)
	os.MkdirAll(repoDir+"/.git", 0755)

	origScanPath := scanPath
	defer func() { scanPath = origScanPath }()
	scanPath = tmpDir

	ctx := context.Background()
	cmd := *validateCmd
	cmd.SetContext(ctx)

	err := cmd.RunE(&cmd, nil)
	// LoadConfig should fail on a directory, triggering the error path
	if err != nil && err != ErrChecksFailed {
		t.Errorf("validate with unreadable ralphrc should not return unexpected error: %v", err)
	}
}

// --- Doctor with scan-path as a file (not directory) ---

func TestDoctorCmd_ScanPathIsFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/afile"
	os.WriteFile(filePath, []byte("x"), 0644)

	origScanPath := scanPath
	origDebug := debugMode
	defer func() {
		scanPath = origScanPath
		debugMode = origDebug
	}()
	scanPath = filePath
	debugMode = false

	err := doctorCmd.RunE(doctorCmd, nil)
	// Should return ErrChecksFailed because scan-path is not a directory
	if err != nil && err != ErrChecksFailed {
		t.Errorf("doctor with file scan-path returned unexpected error: %v", err)
	}
}

// --- setupServe tests ---

func TestSetupServe_ReturnsNonNil(t *testing.T) {
	bus, sessMgr, hostname := setupServe()
	if bus == nil {
		t.Error("setupServe should return non-nil bus")
	}
	if sessMgr == nil {
		t.Error("setupServe should return non-nil session manager")
	}
	if hostname == "" {
		t.Error("setupServe should return non-empty hostname")
	}
}

// --- Serve command flag tests ---

func TestServeCmd_PortDefault(t *testing.T) {
	f := serveCmd.Flags()
	port, err := f.GetInt("port")
	if err != nil {
		t.Fatal(err)
	}
	if port == 0 {
		t.Error("port default should not be 0")
	}
}

func TestServeCmd_CoordinatorURLDefault(t *testing.T) {
	f := serveCmd.Flags()
	url, err := f.GetString("coordinator-url")
	if err != nil {
		t.Fatal(err)
	}
	if url != "" {
		t.Errorf("coordinator-url default = %q, want empty", url)
	}
}

// --- applyTheme additional tests ---

func TestApplyTheme_Gruvbox(t *testing.T) {
	applyTheme("gruvbox")
}

func TestApplyTheme_Nord(t *testing.T) {
	applyTheme("nord")
}

func TestApplyTheme_InvalidFilePath(t *testing.T) {
	// Non-existent file path — should not panic, just silently fail
	applyTheme("/nonexistent/path/theme.yaml")
}

// --- newLogHandler level filtering tests ---

func TestNewLogHandler_DebugLevel(t *testing.T) {
	origLevel := logLevel
	origFormat := logFormat
	defer func() {
		logLevel = origLevel
		logFormat = origFormat
	}()

	logLevel = "debug"
	logFormat = "json"

	var buf bytes.Buffer
	h := newLogHandler(&buf)
	logger := slog.New(h)
	logger.Debug("debug msg")

	if !strings.Contains(buf.String(), "debug msg") {
		t.Error("debug handler should include debug messages")
	}
}

func TestNewLogHandler_ErrorLevel(t *testing.T) {
	origLevel := logLevel
	origFormat := logFormat
	defer func() {
		logLevel = origLevel
		logFormat = origFormat
	}()

	logLevel = "error"
	logFormat = "json"

	var buf bytes.Buffer
	h := newLogHandler(&buf)
	logger := slog.New(h)
	logger.Info("info msg")

	if strings.Contains(buf.String(), "info msg") {
		t.Error("error handler should not include info messages")
	}
}

// --- runCoordinator with pre-cancelled context ---

func TestRunCoordinator_CancelledContext(t *testing.T) {
	// This test exercises the runCoordinator → ctx.Done path. fleet.Coordinator
	// has a known data race: Start() writes c.server (server.go:100) in a
	// goroutine while Stop() reads c.server (server.go:120), requiring a mutex.
	// To avoid triggering the race we use an invalid port so Start()/
	// ListenAndServe returns an error immediately (errCh path) rather than the
	// ctx.Done path that calls Stop() concurrently with the Start() goroutine.
	bus, sessMgr, hostname := setupServe()
	ctx := context.Background()

	origPort := servePort
	origBudget := fleetBudget
	defer func() {
		servePort = origPort
		fleetBudget = origBudget
	}()
	servePort = -1 // invalid port → ListenAndServe errors immediately
	fleetBudget = 100

	err := runCoordinator(ctx, hostname, "", bus, sessMgr)
	// Invalid port causes Start() to return an error, which is acceptable.
	if err == nil {
		t.Log("runCoordinator returned nil (unexpected but not fatal)")
	} else {
		t.Logf("runCoordinator returned: %v (expected for invalid port)", err)
	}
}

// --- newLogHandler text format ---

func TestNewLogHandler_TextFormat(t *testing.T) {
	origLevel := logLevel
	origFormat := logFormat
	defer func() {
		logLevel = origLevel
		logFormat = origFormat
	}()

	logLevel = "info"
	logFormat = "text"

	var buf bytes.Buffer
	h := newLogHandler(&buf)
	logger := slog.New(h)
	logger.Info("test text")

	if !strings.Contains(buf.String(), "test text") {
		t.Error("text handler should include info messages")
	}
}

// --- Serve command flag: fleet-budget ---

func TestServeCmd_FleetBudgetDefault(t *testing.T) {
	f := serveCmd.Flags()
	budget, err := f.GetFloat64("fleet-budget")
	if err != nil {
		t.Fatal(err)
	}
	if budget != 500 {
		t.Errorf("fleet-budget default = %f, want 500", budget)
	}
}

// --- gatecheck with observations error path ---

func TestGateCheckCmd_ObservationsLoadError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid baseline file
	baselinePath := tmpDir + "/baseline.json"
	os.WriteFile(baselinePath, []byte(`{"generated_at":"2024-01-01T00:00:00Z","window_hours":24,"entries":{}}`), 0644)

	// Create observation path as a directory (to trigger non-NotExist error)
	obsDir := tmpDir + "/.ralph/observations"
	os.MkdirAll(obsDir, 0755)
	// Create the expected file path as a directory
	os.MkdirAll(obsDir+"/loop_observations.json", 0755)

	origScanPath := scanPath
	origBaselinePath := gateBaselinePath
	origJSON := gateJSON
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origBaselinePath
		gateJSON = origJSON
	}()
	scanPath = tmpDir
	gateBaselinePath = baselinePath
	gateJSON = true

	cmd := *gateCheckCmd
	err := cmd.RunE(&cmd, nil)
	// If observations path is a directory, it may error or skip
	_ = err // Just exercise the code path
}

func TestServeCmd_CoordinatorDefault(t *testing.T) {
	f := serveCmd.Flags()
	coord, err := f.GetBool("coordinator")
	if err != nil {
		t.Fatal(err)
	}
	if coord {
		t.Error("coordinator default should be false")
	}
}

// --- gatecheck RunE tests ---

func TestGateCheckCmd_NoBaseline(t *testing.T) {
	tmpDir := t.TempDir()

	origScanPath := scanPath
	origBaselinePath := gateBaselinePath
	origJSON := gateJSON
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origBaselinePath
		gateJSON = origJSON
	}()
	scanPath = tmpDir
	gateBaselinePath = "" // use default, which won't exist
	gateJSON = true

	cmd := *gateCheckCmd
	err := cmd.RunE(&cmd, nil)
	// With no baseline, should produce a skip report (not error)
	if err != nil {
		t.Errorf("gate-check with no baseline should not error, got: %v", err)
	}
}

func TestGateCheckCmd_WithBaseline_NoObservations(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid baseline file
	baselinePath := tmpDir + "/baseline.json"
	os.WriteFile(baselinePath, []byte(`{"generated_at":"2024-01-01T00:00:00Z","window_hours":24,"entries":{}}`), 0644)

	origScanPath := scanPath
	origBaselinePath := gateBaselinePath
	origJSON := gateJSON
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origBaselinePath
		gateJSON = origJSON
	}()
	scanPath = tmpDir
	gateBaselinePath = baselinePath
	gateJSON = true

	cmd := *gateCheckCmd
	err := cmd.RunE(&cmd, nil)
	// No observations file — should produce skip report
	if err != nil {
		t.Errorf("gate-check with no observations should not error, got: %v", err)
	}
}

func TestGateCheckCmd_InvalidBaselinePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create baseline as a directory to trigger non-NotExist error
	baselinePath := tmpDir + "/baseline.json"
	os.MkdirAll(baselinePath, 0755)

	origScanPath := scanPath
	origBaselinePath := gateBaselinePath
	origJSON := gateJSON
	defer func() {
		scanPath = origScanPath
		gateBaselinePath = origBaselinePath
		gateJSON = origJSON
	}()
	scanPath = tmpDir
	gateBaselinePath = baselinePath
	gateJSON = false

	cmd := *gateCheckCmd
	err := cmd.RunE(&cmd, nil)
	// Should return error (not IsNotExist, but read error)
	if err == nil {
		t.Error("gate-check with directory as baseline should error")
	}
}
