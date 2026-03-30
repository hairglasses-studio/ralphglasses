package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check required tools and environment",
	Long: `Verify that all required binaries are installed and the environment is
configured correctly for running ralphglasses.

Checks include:
  - Required binaries (ralph, claude, git, go)
  - API keys and environment variables
  - Scan path validity
  - Go and git version compatibility
  - Disk space availability
  - MCP server buildability
  - .ralphrc configuration validation`,
	Example: `  # Run all checks
  ralphglasses doctor

  # Run with debug output
  ralphglasses doctor --debug`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// doctorResult holds the outcome of a single doctor check.
type doctorResult struct {
	name     string
	status   string
	message  string
	required bool
}

func runDoctor(cmd *cobra.Command, args []string) error {
	sp := util.ExpandHome(scanPath)
	util.Debug.Debugf("doctor: scan-path=%s", sp)

	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))   // green
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // red

	var results []doctorResult
	anyFailed := false

	check := func(name string, required bool, fn func() (string, string)) {
		status, msg := fn()
		if required && status == "MISSING" {
			anyFailed = true
		}
		results = append(results, doctorResult{name, status, msg, required})
	}

	binaryCheck := func(bin string) func() (string, string) {
		return func() (string, string) {
			path, err := exec.LookPath(bin)
			if err != nil {
				return "MISSING", bin + " not found in PATH"
			}
			return "OK", path
		}
	}

	// Required checks
	check("ralph", true, binaryCheck("ralph"))
	check("claude", true, binaryCheck("claude"))
	check("git", true, binaryCheck("git"))
	check("ANTHROPIC_API_KEY", true, func() (string, string) {
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			return "OK", "set"
		}
		return "WARN", "not set (Claude Code uses OAuth if missing)"
	})
	check("scan-path exists", true, func() (string, string) {
		info, err := os.Stat(sp)
		if err != nil {
			return "MISSING", "path not found: " + sp
		}
		if !info.IsDir() {
			return "MISSING", "not a directory: " + sp
		}
		return "OK", sp
	})

	// Go version check
	check("go version", false, checkGoVersion)

	// Git version check
	check("git version", false, checkGitVersion)

	// Disk space check
	check("disk space", false, func() (string, string) {
		return checkDiskSpace(sp)
	})

	// MCP server buildability
	check("mcp build", false, checkMCPBuild)

	// .ralphrc validation
	check(".ralphrc valid", false, func() (string, string) {
		return checkRalphRC(cmd, sp)
	})

	// Optional binary checks
	check("gemini", false, binaryCheck("gemini"))
	check("codex", false, binaryCheck("codex"))
	check("goreleaser", false, binaryCheck("goreleaser"))
	check("golangci-lint", false, binaryCheck("golangci-lint"))

	// Print table
	fmt.Printf("%-25s  %-8s  %s\n", "CHECK", "STATUS", "DETAILS")
	fmt.Println(strings.Repeat("-", 65))
	for _, r := range results {
		var statusStr string
		switch r.status {
		case "OK":
			statusStr = okStyle.Render("OK")
		case "WARN":
			statusStr = warnStyle.Render("WARN")
		default:
			if r.required {
				statusStr = errStyle.Render("MISSING")
			} else {
				statusStr = warnStyle.Render("MISSING")
			}
		}
		fmt.Printf("%-25s  %-17s  %s\n", r.name, statusStr, r.message)
	}

	if anyFailed {
		return ErrChecksFailed
	}
	return nil
}

// checkGoVersion verifies Go >= 1.22 is installed.
func checkGoVersion() (string, string) {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return "WARN", "go not found or failed to run"
	}
	ver := strings.TrimSpace(string(out))
	// Parse "go version go1.XX.Y ..."
	parts := strings.Fields(ver)
	if len(parts) < 3 {
		return "WARN", "unexpected go version format: " + ver
	}
	goVer := strings.TrimPrefix(parts[2], "go")
	major, minor, ok := parseGoVersion(goVer)
	if !ok {
		return "WARN", "could not parse version: " + goVer
	}
	if major < 1 || (major == 1 && minor < 22) {
		return "WARN", fmt.Sprintf("go %s (need >= 1.22)", goVer)
	}
	return "OK", "go " + goVer
}

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

// checkGitVersion reports the installed git version.
func checkGitVersion() (string, string) {
	out, err := exec.Command("git", "--version").Output()
	if err != nil {
		return "WARN", "git not found or failed to run"
	}
	ver := strings.TrimSpace(string(out))
	return "OK", ver
}

// checkDiskSpace warns if less than 5GB free on the filesystem containing path.
func checkDiskSpace(path string) (string, string) {
	free, err := diskFreeBytes(path)
	if err != nil {
		return "WARN", "could not check disk space: " + err.Error()
	}
	gb := float64(free) / (1024 * 1024 * 1024)
	if gb < 5.0 {
		return "WARN", fmt.Sprintf("%.1f GB free (< 5 GB recommended)", gb)
	}
	return "OK", fmt.Sprintf("%.1f GB free", gb)
}

// diskFreeBytes returns the number of free bytes on the filesystem containing path.
// This is extracted as a variable so tests can override it.
var diskFreeBytes = diskFreeBytesImpl

func diskFreeBytesImpl(path string) (uint64, error) {
	return diskFreeBytesSyscall(path)
}

// checkMCPBuild tries to build the MCP server binary.
func checkMCPBuild() (string, string) {
	cmd := exec.Command("go", "build", "./cmd/ralphglasses-mcp/...")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 120 {
			msg = msg[:120] + "..."
		}
		return "WARN", "build failed: " + msg
	}
	return "OK", "builds successfully"
}

// checkRalphRC validates the first .ralphrc found in the scan path.
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

// diskFreeBytesSyscall is the platform-specific implementation.
// On non-unix systems it returns a safe default.
func diskFreeBytesSyscall(path string) (uint64, error) {
	if runtime.GOOS == "windows" {
		// Statfs not available on Windows; report a safe value.
		return 100 * 1024 * 1024 * 1024, nil // 100 GB placeholder
	}
	return statfsFreeBytes(path)
}
