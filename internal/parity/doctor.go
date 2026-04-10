package parity

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
	"github.com/hairglasses-studio/ralphglasses/internal/session"

	_ "modernc.org/sqlite"
)

var discoverDoctorOllamaInventory = session.DiscoverOllamaInventory

type DoctorStatus string

const (
	StatusPass DoctorStatus = "pass"
	StatusWarn DoctorStatus = "warn"
	StatusFail DoctorStatus = "fail"
)

type DoctorResult struct {
	Name    string       `json:"name"`
	Status  DoctorStatus `json:"status"`
	Message string       `json:"message"`
}

type DoctorReport struct {
	Results []DoctorResult `json:"results"`
	Summary struct {
		Pass int  `json:"pass"`
		Warn int  `json:"warn"`
		Fail int  `json:"fail"`
		OK   bool `json:"ok"`
	} `json:"summary"`
}

type DoctorOptions struct {
	ScanPath        string
	Checks          []string
	IncludeOptional bool
}

func RunDoctor(ctx context.Context, opts DoctorOptions) DoctorReport {
	results := CollectDoctorResults(ctx, opts)
	return BuildDoctorReport(results)
}

func CollectDoctorResults(ctx context.Context, opts DoctorOptions) []DoctorResult {
	enabled := make(map[string]bool)
	for _, check := range opts.Checks {
		check = strings.TrimSpace(strings.ToLower(check))
		if check != "" {
			enabled[check] = true
		}
	}
	isEnabled := func(name string) bool {
		if len(enabled) == 0 {
			return true
		}
		_, ok := enabled[strings.ToLower(name)]
		return ok
	}

	includeOptional := opts.IncludeOptional
	if len(enabled) == 0 {
		includeOptional = true
	}

	var results []DoctorResult
	add := func(name string, fn func(context.Context, DoctorOptions) DoctorResult) {
		if isEnabled(name) {
			results = append(results, fn(ctx, opts))
		}
	}

	add("claude", checkClaude)
	if includeOptional || isEnabled("gemini") {
		add("gemini", checkGemini)
	}
	if includeOptional || isEnabled("codex") {
		add("codex", checkCodex)
	}
	if includeOptional || isEnabled("ollama") {
		add("ollama", checkOllama)
	}
	add("git", checkGit)
	add("state_dir", checkStateDir)
	add("config", checkConfig)
	add("sqlite", checkSQLite)
	add("scan_path", checkScanPath)
	add("disk_space", checkDiskSpace)
	if isEnabled("api_keys") || isEnabled("anthropic_api_key") || isEnabled("google_api_key") || isEnabled("openai_api_key") || len(enabled) == 0 {
		results = append(results, checkAPIKeys(includeOptional)...)
	}
	return results
}

func BuildDoctorReport(results []DoctorResult) DoctorReport {
	var report DoctorReport
	report.Results = results
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			report.Summary.Pass++
		case StatusWarn:
			report.Summary.Warn++
		case StatusFail:
			report.Summary.Fail++
		}
	}
	report.Summary.OK = report.Summary.Fail == 0
	return report
}

func FormatDoctorResults(results []DoctorResult) string {
	var b strings.Builder
	for _, r := range results {
		fmt.Fprintf(&b, "%-22s %-5s %s\n", r.Name, strings.ToUpper(string(r.Status)), r.Message)
	}
	return strings.TrimRight(b.String(), "\n")
}

func checkClaude(_ context.Context, _ DoctorOptions) DoctorResult {
	path, err := exec.LookPath("claude")
	if err != nil {
		return DoctorResult{Name: "claude", Status: StatusFail, Message: "claude not found in PATH"}
	}
	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err != nil {
		return DoctorResult{Name: "claude", Status: StatusPass, Message: path}
	}
	ver := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	return DoctorResult{Name: "claude", Status: StatusPass, Message: ver}
}

func checkGemini(_ context.Context, _ DoctorOptions) DoctorResult {
	path, err := exec.LookPath("gemini")
	if err != nil {
		return DoctorResult{Name: "gemini", Status: StatusWarn, Message: "gemini not found in PATH (optional)"}
	}
	return DoctorResult{Name: "gemini", Status: StatusPass, Message: path}
}

func checkCodex(_ context.Context, _ DoctorOptions) DoctorResult {
	path, err := exec.LookPath("codex")
	if err != nil {
		return DoctorResult{Name: "codex", Status: StatusWarn, Message: "codex not found in PATH (optional)"}
	}
	return DoctorResult{Name: "codex", Status: StatusPass, Message: path}
}

func checkOllama(ctx context.Context, _ DoctorOptions) DoctorResult {
	inventory := discoverDoctorOllamaInventory(ctx, 5*time.Second)
	if !inventory.Reachable {
		msg := fmt.Sprintf("%s not reachable", inventory.BaseURL)
		if inventory.Error != "" {
			msg = fmt.Sprintf("%s: %s", msg, inventory.Error)
		}
		return DoctorResult{Name: "ollama", Status: StatusWarn, Message: msg + " (optional)"}
	}
	if len(inventory.MissingRequiredModels) > 0 {
		msg := fmt.Sprintf("ready %d/%d required lanes; missing %s",
			inventory.ReadyRequiredCount(), len(inventory.RequiredModels), strings.Join(inventory.MissingRequiredModels, ", "))
		return DoctorResult{Name: "ollama", Status: StatusWarn, Message: msg}
	}
	aliasIssues := inventory.AliasIssueNames()
	if len(aliasIssues) > 0 {
		return DoctorResult{
			Name:   "ollama",
			Status: StatusWarn,
			Message: fmt.Sprintf("ready %d/%d required lanes; alias drift on %s; run hg-ollama-sync-aliases.sh",
				inventory.ReadyRequiredCount(), len(inventory.RequiredModels), strings.Join(aliasIssues, ", ")),
		}
	}
	modelCount := inventory.AvailableModelCount
	return DoctorResult{
		Name:    "ollama",
		Status:  StatusPass,
		Message: fmt.Sprintf("%s (%d models, %d/%d required lanes ready)", inventory.BaseURL, modelCount, inventory.ReadyRequiredCount(), len(inventory.RequiredModels)),
	}
}

func checkGit(_ context.Context, _ DoctorOptions) DoctorResult {
	if _, err := exec.LookPath("git"); err != nil {
		return DoctorResult{Name: "git", Status: StatusFail, Message: "git not found in PATH"}
	}
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return DoctorResult{Name: "git", Status: StatusWarn, Message: "git found but could not determine version"}
	}
	ver := strings.TrimSpace(string(out))
	major, minor, ok := ParseGitVersion(ver)
	if !ok {
		return DoctorResult{Name: "git", Status: StatusWarn, Message: "could not parse version: " + ver}
	}
	if major < 2 || (major == 2 && minor < 20) {
		return DoctorResult{
			Name:    "git",
			Status:  StatusFail,
			Message: fmt.Sprintf("git %d.%d (need >= 2.20 for worktree support)", major, minor),
		}
	}
	return DoctorResult{Name: "git", Status: StatusPass, Message: ver}
}

func ParseGitVersion(raw string) (major, minor int, ok bool) {
	parts := strings.Fields(raw)
	if len(parts) < 3 {
		return 0, 0, false
	}
	segments := strings.SplitN(parts[2], ".", 3)
	if len(segments) < 2 {
		return 0, 0, false
	}
	major, errMajor := strconv.Atoi(segments[0])
	minor, errMinor := strconv.Atoi(segments[1])
	return major, minor, errMajor == nil && errMinor == nil
}

func checkConfig(_ context.Context, _ DoctorOptions) DoctorResult {
	cfgPath := ralphpath.ConfigPath()
	info, err := os.Stat(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DoctorResult{Name: "config", Status: StatusWarn, Message: cfgPath + " not found (will use defaults)"}
		}
		return DoctorResult{Name: "config", Status: StatusWarn, Message: "cannot stat config: " + err.Error()}
	}
	if info.IsDir() {
		return DoctorResult{Name: "config", Status: StatusFail, Message: cfgPath + " is a directory, expected file"}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return DoctorResult{Name: "config", Status: StatusFail, Message: "invalid config: " + err.Error()}
	}
	var msgs []string
	for _, warn := range cfg.Validate() {
		msgs = append(msgs, warn.Error())
	}
	if len(msgs) > 0 {
		return DoctorResult{Name: "config", Status: StatusWarn, Message: strings.Join(msgs, "; ")}
	}
	return DoctorResult{Name: "config", Status: StatusPass, Message: cfgPath}
}

func checkStateDir(_ context.Context, _ DoctorOptions) DoctorResult {
	dir := ralphpath.StateDir()
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return DoctorResult{Name: "state dir", Status: StatusWarn, Message: dir + " does not exist (will be created on first run)"}
		}
		return DoctorResult{Name: "state dir", Status: StatusFail, Message: "cannot stat: " + err.Error()}
	}
	if !info.IsDir() {
		return DoctorResult{Name: "state dir", Status: StatusFail, Message: dir + " exists but is not a directory"}
	}
	probe := filepath.Join(dir, ".doctor_probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return DoctorResult{Name: "state dir", Status: StatusFail, Message: dir + " is not writable"}
	}
	_ = os.Remove(probe)
	return DoctorResult{Name: "state dir", Status: StatusPass, Message: dir}
}

func checkSQLite(_ context.Context, _ DoctorOptions) DoctorResult {
	dbPath := ralphpath.SQLiteStorePath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return DoctorResult{Name: "sqlite", Status: StatusFail, Message: "cannot open: " + err.Error()}
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return DoctorResult{Name: "sqlite", Status: StatusFail, Message: "ping failed: " + err.Error()}
	}
	return DoctorResult{Name: "sqlite", Status: StatusPass, Message: dbPath}
}

func checkScanPath(ctx context.Context, opts DoctorOptions) DoctorResult {
	sp := opts.ScanPath
	info, err := os.Stat(sp)
	if err != nil {
		return DoctorResult{Name: "scan path", Status: StatusFail, Message: "path not found: " + sp}
	}
	if !info.IsDir() {
		return DoctorResult{Name: "scan path", Status: StatusFail, Message: "not a directory: " + sp}
	}
	repos, err := discovery.Scan(ctx, sp)
	if err != nil {
		return DoctorResult{Name: "scan path", Status: StatusWarn, Message: sp + " (scan error: " + err.Error() + ")"}
	}
	if len(repos) == 0 {
		return DoctorResult{Name: "scan path", Status: StatusWarn, Message: sp + " (no ralph-enabled repos found)"}
	}
	return DoctorResult{Name: "scan path", Status: StatusPass, Message: fmt.Sprintf("%s (%d repos)", sp, len(repos))}
}

func checkDiskSpace(_ context.Context, opts DoctorOptions) DoctorResult {
	free, err := freeBytes(opts.ScanPath)
	if err != nil {
		return DoctorResult{Name: "disk space", Status: StatusWarn, Message: "could not check: " + err.Error()}
	}
	gb := float64(free) / (1024 * 1024 * 1024)
	if gb < 1.0 {
		return DoctorResult{Name: "disk space", Status: StatusFail, Message: fmt.Sprintf("%.1f GB free", gb)}
	}
	if gb < 5.0 {
		return DoctorResult{Name: "disk space", Status: StatusWarn, Message: fmt.Sprintf("%.1f GB free", gb)}
	}
	return DoctorResult{Name: "disk space", Status: StatusPass, Message: fmt.Sprintf("%.1f GB free", gb)}
}

func freeBytes(path string) (uint64, error) {
	if runtime.GOOS == "windows" {
		return 0, fmt.Errorf("disk space check unsupported on windows")
	}
	cmd := exec.Command("df", "-Pk", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output")
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df fields")
	}
	availKB, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return 0, err
	}
	return availKB * 1024, nil
}

func checkAPIKeys(includeOptional bool) []DoctorResult {
	keys := []struct {
		env      string
		required bool
	}{
		{env: "ANTHROPIC_API_KEY", required: true},
		{env: "GOOGLE_API_KEY", required: false},
		{env: "OPENAI_API_KEY", required: false},
	}
	results := make([]DoctorResult, 0, len(keys))
	for _, key := range keys {
		if !includeOptional && !key.required {
			continue
		}
		val := os.Getenv(key.env)
		if val == "" {
			status := StatusWarn
			msg := "not set (optional)"
			if key.required {
				status = StatusFail
				msg = "not set"
			}
			results = append(results, DoctorResult{Name: key.env, Status: status, Message: msg})
			continue
		}
		results = append(results, DoctorResult{Name: key.env, Status: StatusPass, Message: RedactKey(val)})
	}
	return results
}

func RedactKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func LoadRepoConfig(repoPath string) (*model.RalphConfig, error) {
	return model.LoadConfig(context.Background(), repoPath)
}
