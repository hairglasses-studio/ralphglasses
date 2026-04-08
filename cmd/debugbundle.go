package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var debugBundleOutput string

var debugBundleCmd = &cobra.Command{
	Use:   "debug-bundle",
	Short: "Collect diagnostic information into a text bundle",
	Long: `Gather system information, configuration, and logs into a single diagnostic
file for troubleshooting. API keys and secrets are automatically masked.

The bundle includes:
  - Go and git versions, OS info
  - ralphglasses version and configuration
  - .ralphrc contents (sanitized)
  - Recent log output
  - Doctor check results`,
	Example: `  # Create a debug bundle in the current directory
  ralphglasses debug-bundle

  # Specify output path
  ralphglasses debug-bundle --output /tmp/diag.txt`,
	RunE: runDebugBundle,
}

func init() {
	debugBundleCmd.Flags().StringVarP(&debugBundleOutput, "output", "o", "",
		"Output file path (default: ralph-debug-TIMESTAMP.txt in current directory)")
	rootCmd.AddCommand(debugBundleCmd)
}

func runDebugBundle(cmd *cobra.Command, args []string) error {
	sp := util.ExpandHome(scanPath)

	outPath := debugBundleOutput
	if outPath == "" {
		outPath = parity.DefaultDebugBundlePath(".", time.Now())
	}

	content, err := parity.BuildDebugBundle(context.Background(), parity.DebugBundleOptions{
		ScanPath:  sp,
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	})
	if err != nil {
		return fmt.Errorf("build bundle: %w", err)
	}

	if err := parity.WriteDebugBundle(outPath, content); err != nil {
		return err
	}

	fmt.Printf("Debug bundle written to %s (%d bytes)\n", outPath, len(content))
	return nil
}

func writeSection(b *strings.Builder, title, content string) {
	b.WriteString("=== " + title + " ===\n")
	b.WriteString(content)
	b.WriteString("\n\n")
}

func collectSystemInfo() string {
	return fmt.Sprintf("OS: %s\nArch: %s\nNumCPU: %d",
		runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

func collectCommandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// sanitizeSecrets masks API keys and tokens in text. Exported for testing.
func sanitizeSecrets(s string) string {
	// Pattern: KEY=value where KEY contains API, TOKEN, SECRET, PASSWORD
	re := regexp.MustCompile(`(?i)((?:API|TOKEN|SECRET|PASSWORD|KEY)[A-Z_]*)=(\S{4})\S+`)
	s = re.ReplaceAllString(s, "${1}=${2}...")

	// Also mask sk-ant-, sk-, gsk_, AIza patterns directly
	keyPrefixes := regexp.MustCompile(`(sk-ant-[a-zA-Z0-9]{4})[a-zA-Z0-9-]+`)
	s = keyPrefixes.ReplaceAllString(s, "${1}...")

	skPrefixes := regexp.MustCompile(`(sk-[a-zA-Z0-9]{4})[a-zA-Z0-9-]+`)
	s = skPrefixes.ReplaceAllString(s, "${1}...")

	geminiPrefixes := regexp.MustCompile(`(AIza[a-zA-Z0-9]{4})[a-zA-Z0-9-]+`)
	s = geminiPrefixes.ReplaceAllString(s, "${1}...")

	return s
}

func collectSanitizedEnv() string {
	var relevant []string
	for _, env := range os.Environ() {
		key := strings.SplitN(env, "=", 2)[0]
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "RALPH") ||
			strings.Contains(upper, "ANTHROPIC") ||
			strings.Contains(upper, "GEMINI") ||
			strings.Contains(upper, "OPENAI") ||
			strings.Contains(upper, "CLAUDE") {
			relevant = append(relevant, sanitizeSecrets(env))
		}
	}
	if len(relevant) == 0 {
		return "(no relevant environment variables found)"
	}
	return strings.Join(relevant, "\n")
}

func collectRalphRC(sp string) string {
	rcPath := filepath.Join(sp, ".ralphrc")
	data, err := os.ReadFile(rcPath)
	if err == nil {
		return sanitizeSecrets(string(data))
	}
	entries, dirErr := os.ReadDir(sp)
	if dirErr != nil {
		return "(scan path not readable)"
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(sp, entry.Name(), ".ralphrc")
		data, err = os.ReadFile(candidate)
		if err == nil {
			return sanitizeSecrets(string(data))
		}
	}
	return "(no .ralphrc found)"
}

func collectRecentLogs(sp string) string {
	logPath := filepath.Join(sp, ".ralph", "logs", "ralphglasses.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "(no log file found at " + logPath + ")"
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 50 {
		lines = lines[len(lines)-50:]
	}
	return sanitizeSecrets(strings.Join(lines, "\n"))
}

func collectDoctorOutput() string {
	report := parity.RunDoctor(context.Background(), parity.DoctorOptions{
		ScanPath:        util.ExpandHome(scanPath),
		IncludeOptional: true,
	})
	return sanitizeSecrets(parity.FormatDoctorResults(report.Results))
}
