package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
	"github.com/hairglasses-studio/ralphglasses/internal/util"

	_ "modernc.org/sqlite"
)

var doctorJSON bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check required tools and environment",
	Long: `Verify that all required binaries are installed and the environment is
configured correctly for running ralphglasses.

Checks include:
  - Provider binaries (claude, gemini, codex, antigravity, cline)
  - Git binary and version (>= 2.20 for worktree support)
  - Config file (` + ralphpath.ConfigPathDefaultDescription() + `)
  - State directory (` + ralphpath.StateDirDefaultDescription() + `) permissions
  - SQLite store (` + ralphpath.SQLiteStoreDefaultDescription() + `)
  - Scan path validity and repo discovery
  - Disk space availability
  - API keys (ANTHROPIC_API_KEY, GEMINI_API_KEY, OPENAI_API_KEY)

Exit code 0 if all checks pass (warnings are OK), exit code 1 if any fail.`,
	Example: `  # Run all checks
  ralphglasses doctor

  # Machine-readable JSON output
  ralphglasses doctor --json

  # Run with debug output
  ralphglasses doctor --debug`,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output results as JSON")
	rootCmd.AddCommand(doctorCmd)
}

// doctorStatus represents the outcome severity of a check.
type doctorStatus string

const (
	statusPass doctorStatus = "pass"
	statusWarn doctorStatus = "warn"
	statusFail doctorStatus = "fail"
)

// doctorResult holds the outcome of a single doctor check.
type doctorResult struct {
	Name    string       `json:"name"`
	Status  doctorStatus `json:"status"`
	Message string       `json:"message"`
}

// doctorReport is the JSON-serialisable envelope for all check results.
type doctorReport struct {
	Results []doctorResult `json:"results"`
	Summary struct {
		Pass int  `json:"pass"`
		Warn int  `json:"warn"`
		Fail int  `json:"fail"`
		OK   bool `json:"ok"`
	} `json:"summary"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	sp := util.ExpandHome(scanPath)
	util.Debug.Debugf("doctor: scan-path=%s", sp)

	var results []doctorResult

	collect := func(r doctorResult) {
		results = append(results, r)
	}

	// --- Provider binaries ---
	collect(checkClaude())
	collect(checkGemini())
	collect(checkCodex())
	collect(checkAntigravity())
	collect(checkCline())

	// --- Git ---
	collect(checkGit())

	// --- Config & state ---
	collect(checkStateDir())
	collect(checkConfig())
	collect(checkSQLite())

	// --- Scan path ---
	collect(checkScanPath(cmd, sp))

	// --- Disk space ---
	collect(checkDiskSpaceCheck(sp))

	// --- API keys ---
	for _, r := range checkAPIKeys() {
		collect(r)
	}

	// Build the report.
	report := buildDoctorReport(results)

	if doctorJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	} else {
		printDoctorResults(results)
	}

	if !report.Summary.OK {
		return ErrChecksFailed
	}
	return nil
}

// buildDoctorReport tallies results into a summary.
func buildDoctorReport(results []doctorResult) doctorReport {
	var report doctorReport
	report.Results = results
	for _, r := range results {
		switch r.Status {
		case statusPass:
			report.Summary.Pass++
		case statusWarn:
			report.Summary.Warn++
		case statusFail:
			report.Summary.Fail++
		}
	}
	report.Summary.OK = report.Summary.Fail == 0
	return report
}

// printDoctorResults renders human-readable output with icons and colour.
func printDoctorResults(results []doctorResult) {
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))   // green
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // red

	for _, r := range results {
		var icon, styled string
		switch r.Status {
		case statusPass:
			icon = okStyle.Render("[/]")
			styled = okStyle.Render(r.Message)
		case statusWarn:
			icon = warnStyle.Render("[!]")
			styled = warnStyle.Render(r.Message)
		case statusFail:
			icon = errStyle.Render("[x]")
			styled = errStyle.Render(r.Message)
		}
		fmt.Printf(" %s %-22s %s\n", icon, r.Name, styled)
	}
}

// ---------------------------------------------------------------------------
// Individual check functions
// ---------------------------------------------------------------------------

// checkClaude verifies the claude binary is on PATH and reports its version.
func checkClaude() doctorResult {
	path, err := exec.LookPath("claude")
	if err != nil {
		return doctorResult{Name: "claude", Status: statusFail, Message: "claude not found in PATH"}
	}
	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err != nil {
		return doctorResult{Name: "claude", Status: statusPass, Message: path}
	}
	ver := strings.TrimSpace(string(out))
	// Take just the first line in case of multi-line output.
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}
	return doctorResult{Name: "claude", Status: statusPass, Message: ver}
}

// checkGemini verifies the gemini binary is on PATH (optional).
func checkGemini() doctorResult {
	path, err := exec.LookPath("gemini")
	if err != nil {
		return doctorResult{Name: "gemini", Status: statusWarn, Message: "gemini not found in PATH (optional)"}
	}
	return doctorResult{Name: "gemini", Status: statusPass, Message: path}
}

// checkCodex verifies the codex binary is on PATH (optional).
func checkCodex() doctorResult {
	path, err := exec.LookPath("codex")
	if err != nil {
		return doctorResult{Name: "codex", Status: statusWarn, Message: "codex not found in PATH (optional)"}
	}
	return doctorResult{Name: "codex", Status: statusPass, Message: path}
}

// checkAntigravity verifies the antigravity binary is on PATH (optional).
func checkAntigravity() doctorResult {
	path, err := exec.LookPath("antigravity")
	if err != nil {
		return doctorResult{Name: "antigravity", Status: statusWarn, Message: "antigravity not found in PATH (optional external-manager provider)"}
	}
	return doctorResult{Name: "antigravity", Status: statusPass, Message: path}
}

// checkCline verifies the cline binary is on PATH and reports its version.
// Cline uses WorkOS OAuth for auth (stored in ~/.cline/data/), not env var API keys.
func checkCline() doctorResult {
	path, err := exec.LookPath("cline")
	if err != nil {
		return doctorResult{Name: "cline", Status: statusWarn, Message: "cline not found in PATH (optional — free-tier fleet provider)"}
	}
	out, err := exec.Command("cline", "version").CombinedOutput()
	if err != nil {
		// Binary exists but version check failed — still usable.
		return doctorResult{Name: "cline", Status: statusPass, Message: path}
	}
	ver := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(ver, '\n'); idx >= 0 {
		ver = ver[:idx]
	}

	// Check if Cline has auth configured by looking for config directory.
	clineDir := os.Getenv("CLINE_DIR")
	if clineDir == "" {
		home, _ := os.UserHomeDir()
		clineDir = filepath.Join(home, ".cline", "data")
	}
	if _, statErr := os.Stat(clineDir); statErr != nil {
		return doctorResult{Name: "cline", Status: statusWarn, Message: ver + " (no config dir — run 'cline auth' to configure)"}
	}

	return doctorResult{Name: "cline", Status: statusPass, Message: ver}
}

// checkGit verifies git is on PATH and its version is >= 2.20 (worktree support).
func checkGit() doctorResult {
	_, err := exec.LookPath("git")
	if err != nil {
		return doctorResult{Name: "git", Status: statusFail, Message: "git not found in PATH"}
	}
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return doctorResult{Name: "git", Status: statusWarn, Message: "git found but could not determine version"}
	}
	ver := strings.TrimSpace(string(out))
	// Parse "git version 2.XX.Y ..."
	major, minor, ok := parseGitVersion(ver)
	if !ok {
		return doctorResult{Name: "git", Status: statusWarn, Message: "could not parse version: " + ver}
	}
	if major < 2 || (major == 2 && minor < 20) {
		return doctorResult{
			Name:    "git",
			Status:  statusFail,
			Message: fmt.Sprintf("git %d.%d (need >= 2.20 for worktree support)", major, minor),
		}
	}
	return doctorResult{Name: "git", Status: statusPass, Message: ver}
}

// parseGitVersion extracts major.minor from "git version X.Y.Z".
func parseGitVersion(raw string) (major, minor int, ok bool) {
	// git version 2.43.0
	parts := strings.Fields(raw)
	if len(parts) < 3 {
		return 0, 0, false
	}
	verStr := parts[2]
	// Strip Apple-specific suffix like "2.39.5 (Apple Git-154)"
	verStr = strings.SplitN(verStr, " ", 2)[0]
	segments := strings.SplitN(verStr, ".", 3)
	if len(segments) < 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(segments[0])
	if err != nil {
		return 0, 0, false
	}
	min, err := strconv.Atoi(segments[1])
	if err != nil {
		return 0, 0, false
	}
	return maj, min, true
}

// checkConfig validates the active runtime config file exists and is valid JSON.
func checkConfig() doctorResult {
	cfgPath := ralphpath.ConfigPath()
	info, err := os.Stat(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return doctorResult{Name: "config", Status: statusWarn, Message: cfgPath + " not found (will use defaults)"}
		}
		return doctorResult{Name: "config", Status: statusWarn, Message: "cannot stat config: " + err.Error()}
	}
	if info.IsDir() {
		return doctorResult{Name: "config", Status: statusFail, Message: cfgPath + " is a directory, expected file"}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return doctorResult{Name: "config", Status: statusFail, Message: "invalid config: " + err.Error()}
	}
	if warnings := cfg.Validate(); len(warnings) > 0 {
		msgs := make([]string, len(warnings))
		for i, w := range warnings {
			msgs[i] = w.Error()
		}
		return doctorResult{Name: "config", Status: statusWarn, Message: strings.Join(msgs, "; ")}
	}
	return doctorResult{Name: "config", Status: statusPass, Message: cfgPath}
}

// checkStateDir verifies the active state directory exists and is writable.
func checkStateDir() doctorResult {
	dir := ralphpath.StateDir()
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return doctorResult{Name: "state dir", Status: statusWarn, Message: dir + " does not exist (will be created on first run)"}
		}
		return doctorResult{Name: "state dir", Status: statusFail, Message: "cannot stat: " + err.Error()}
	}
	if !info.IsDir() {
		return doctorResult{Name: "state dir", Status: statusFail, Message: dir + " exists but is not a directory"}
	}
	// Check writability by attempting to create and remove a temp file.
	probe := filepath.Join(dir, ".doctor_probe")
	f, err := os.Create(probe)
	if err != nil {
		return doctorResult{Name: "state dir", Status: statusFail, Message: dir + " is not writable"}
	}
	f.Close()
	os.Remove(probe)
	return doctorResult{Name: "state dir", Status: statusPass, Message: dir}
}

// checkSQLite verifies the active SQLite store path can be opened.
func checkSQLite() doctorResult {
	dbPath := ralphpath.SQLiteStorePath()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return doctorResult{Name: "sqlite", Status: statusFail, Message: "cannot open: " + err.Error()}
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return doctorResult{Name: "sqlite", Status: statusFail, Message: "ping failed: " + err.Error()}
	}
	return doctorResult{Name: "sqlite", Status: statusPass, Message: dbPath}
}

// checkScanPath verifies the scan path exists and contains repos.
func checkScanPath(cmd *cobra.Command, sp string) doctorResult {
	info, err := os.Stat(sp)
	if err != nil {
		return doctorResult{Name: "scan path", Status: statusFail, Message: "path not found: " + sp}
	}
	if !info.IsDir() {
		return doctorResult{Name: "scan path", Status: statusFail, Message: "not a directory: " + sp}
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	repos, err := discovery.Scan(ctx, sp)
	if err != nil {
		return doctorResult{Name: "scan path", Status: statusWarn, Message: sp + " (scan error: " + err.Error() + ")"}
	}
	if len(repos) == 0 {
		return doctorResult{Name: "scan path", Status: statusWarn, Message: sp + " (no ralph-enabled repos found)"}
	}
	// Count repos with .ralphrc.
	rcCount := 0
	for _, r := range repos {
		if r.HasRC {
			rcCount++
		}
	}
	return doctorResult{
		Name:    "scan path",
		Status:  statusPass,
		Message: fmt.Sprintf("%s (%d repos, %d with .ralphrc)", sp, len(repos), rcCount),
	}
}

// checkDiskSpaceCheck warns if less than 1GB free on the filesystem containing path.
func checkDiskSpaceCheck(path string) doctorResult {
	free, err := diskFreeBytes(path)
	if err != nil {
		return doctorResult{Name: "disk space", Status: statusWarn, Message: "could not check: " + err.Error()}
	}
	gb := float64(free) / (1024 * 1024 * 1024)
	if gb < 1.0 {
		return doctorResult{
			Name:    "disk space",
			Status:  statusWarn,
			Message: fmt.Sprintf("%.1f GB free (< 1 GB)", gb),
		}
	}
	return doctorResult{Name: "disk space", Status: statusPass, Message: fmt.Sprintf("%.1f GB free", gb)}
}

// checkAPIKeys checks ANTHROPIC_API_KEY, GEMINI_API_KEY, and OPENAI_API_KEY
// with redacted display.
func checkAPIKeys() []doctorResult {
	keys := []struct {
		env      string
		name     string
		required bool
	}{
		{"ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY", false},
		{"GEMINI_API_KEY", "GEMINI_API_KEY", false},
		{"OPENAI_API_KEY", "OPENAI_API_KEY", false},
	}
	var results []doctorResult
	for _, k := range keys {
		val := os.Getenv(k.env)
		if val == "" {
			status := statusWarn
			msg := "not set"
			if k.env == "ANTHROPIC_API_KEY" {
				msg = "not set (Claude Code uses OAuth if missing)"
			}
			results = append(results, doctorResult{Name: k.name, Status: status, Message: msg})
		} else {
			results = append(results, doctorResult{Name: k.name, Status: statusPass, Message: redactKey(val)})
		}
	}
	return results
}

// redactKey shows the first 4 and last 4 characters of a key, masking the rest.
func redactKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

// ---------------------------------------------------------------------------
// Preserved helpers used by other checks (parseGoVersion, diskFreeBytes, etc.)
// ---------------------------------------------------------------------------

// parseGoVersion extracts major.minor from a Go version string like "1.22.1".
func parseGoVersion(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return maj, min, true
}

// diskFreeBytes returns the number of free bytes on the filesystem containing path.
// This is extracted as a variable so tests can override it.
var diskFreeBytes = diskFreeBytesImpl

func diskFreeBytesImpl(path string) (uint64, error) {
	return diskFreeBytesSyscall(path)
}

// diskFreeBytesSyscall is the platform-specific implementation.
// On non-unix systems it returns a safe default.
func diskFreeBytesSyscall(path string) (uint64, error) {
	if runtime.GOOS == "windows" {
		// Statfs not available on Windows; report a safe value.
		return 100 * 1024 * 1024 * 1024, nil // 100 GB placeholder
	}
	return statfsFreeBytes(path)
}

// checkRalphRC validates the first .ralphrc found in the scan path.
// Retained for use by other callers.
func checkRalphRC(cmd *cobra.Command, sp string) (string, string) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	repos, err := discovery.Scan(ctx, sp)
	if err != nil {
		return "WARN", "could not scan: " + err.Error()
	}
	for _, repo := range repos {
		if !repo.HasRC {
			continue
		}
		cfg, err := model.LoadConfig(context.Background(), repo.Path)
		if err != nil {
			return "WARN", fmt.Sprintf("%s: parse error: %s", repo.Name, err.Error())
		}
		if cfg.Get("PROJECT_NAME", "") == "" {
			return "WARN", fmt.Sprintf("%s: PROJECT_NAME not set", repo.Name)
		}
		return "OK", fmt.Sprintf("%s: valid", repo.Name)
	}
	return "OK", "no repos with .ralphrc found"
}
