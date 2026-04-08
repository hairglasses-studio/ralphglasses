package parity

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
)

type DebugBundleOptions struct {
	ScanPath   string
	Version    string
	Commit     string
	BuildDate  string
	OutputPath string
	Sections   []string
}

func BuildDebugBundle(ctx context.Context, opts DebugBundleOptions) (string, error) {
	var b strings.Builder
	selected := make(map[string]bool, len(opts.Sections))
	for _, section := range opts.Sections {
		section = strings.TrimSpace(strings.ToLower(section))
		if section != "" {
			selected[section] = true
		}
	}
	include := func(names ...string) bool {
		if len(selected) == 0 {
			return true
		}
		for _, name := range names {
			if selected[strings.ToLower(name)] {
				return true
			}
		}
		return false
	}

	writeDebugSection(&b, "RALPHGLASSES DEBUG BUNDLE", fmt.Sprintf(
		"Generated: %s\nVersion: %s (commit: %s, built: %s)",
		time.Now().Format(time.RFC3339), opts.Version, opts.Commit, opts.BuildDate))
	if include("system", "system_info") {
		writeDebugSection(&b, "SYSTEM INFO", collectSystemInfo())
	}
	if include("go", "go_version") {
		writeDebugSection(&b, "GO VERSION", collectCommandOutput("go", "version"))
	}
	if include("git", "git_version") {
		writeDebugSection(&b, "GIT VERSION", collectCommandOutput("git", "--version"))
	}
	if include("environment", "env") {
		writeDebugSection(&b, "ENVIRONMENT", collectSanitizedEnv())
	}
	if include("scan_path", "scan") {
		writeDebugSection(&b, "SCAN PATH", opts.ScanPath)
	}
	if include("ralphrc", "config") {
		writeDebugSection(&b, "RALPHRC", collectRalphRC(opts.ScanPath))
	}
	if include("logs", "recent_logs") {
		writeDebugSection(&b, "RECENT LOGS", collectRecentLogs(opts.ScanPath))
	}
	if include("doctor", "doctor_output") {
		writeDebugSection(&b, "DOCTOR OUTPUT", sanitizeSecrets(FormatDoctorResults(RunDoctor(ctx, DoctorOptions{
			ScanPath:        opts.ScanPath,
			IncludeOptional: true,
		}).Results)))
	}
	return b.String(), nil
}

func WriteDebugBundle(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write bundle: %w", err)
	}
	return nil
}

func DefaultDebugBundlePath(baseDir string, now time.Time) string {
	return filepath.Join(baseDir, fmt.Sprintf("ralph-debug-%s.txt", now.Format("20060102-150405")))
}

func writeDebugSection(b *strings.Builder, title, content string) {
	b.WriteString("=== " + title + " ===\n")
	b.WriteString(content)
	b.WriteString("\n\n")
}

func collectSystemInfo() string {
	return fmt.Sprintf("OS: %s\nArch: %s\nNumCPU: %d", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

func collectCommandOutput(name string, args ...string) string {
	out, err := execCommand(name, args...)
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return strings.TrimSpace(out)
}

func execCommand(name string, args ...string) (string, error) {
	out, err := osExecCommand(name, args...)
	return string(out), err
}

var osExecCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func sanitizeSecrets(s string) string {
	re := regexp.MustCompile(`(?i)((?:API|TOKEN|SECRET|PASSWORD|KEY)[A-Z_]*)=(\S{4})\S+`)
	s = re.ReplaceAllString(s, "${1}=${2}...")

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

func collectRalphRC(scanPath string) string {
	rcPath := filepath.Join(scanPath, ".ralphrc")
	data, err := os.ReadFile(rcPath)
	if err == nil {
		return sanitizeSecrets(string(data))
	}
	entries, dirErr := os.ReadDir(scanPath)
	if dirErr != nil {
		return "(scan path not readable)"
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(scanPath, entry.Name(), ".ralphrc")
		data, err = os.ReadFile(candidate)
		if err == nil {
			return sanitizeSecrets(string(data))
		}
	}
	return "(no .ralphrc found)"
}

func collectRecentLogs(scanPath string) string {
	logPath := filepath.Join(scanPath, ".ralph", "logs", "ralphglasses.log")
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
