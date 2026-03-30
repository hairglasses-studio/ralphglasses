package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SupervisorGates runs acceptance checks after cycle completion.
type SupervisorGates struct {
	RequireBuild bool          // default true — run go build ./...
	RequireTest  bool          // default true — run go test ./...
	RequireVet   bool          // default true — run go vet ./...
	MinCoverage  float64       // 0 = disabled; check .ralph/coverage.txt
	TestTimeout  time.Duration // default 120s

	// runCmd is a pluggable command runner for testing.
	// Signature: func(ctx, dir, name, args...) (output, error).
	runCmd func(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// DefaultSupervisorGates returns gates with sensible defaults.
func DefaultSupervisorGates() *SupervisorGates {
	return &SupervisorGates{
		RequireBuild: true,
		RequireTest:  true,
		RequireVet:   true,
		TestTimeout:  120 * time.Second,
	}
}

// Evaluate runs the configured gates and returns findings for any failures.
// The bool return is true if all gates passed.
func (sg *SupervisorGates) Evaluate(ctx context.Context, repoPath string) ([]CycleFinding, bool) {
	var findings []CycleFinding
	timeout := sg.TestTimeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	gateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if sg.RequireBuild {
		if out, err := sg.run(gateCtx, repoPath, "go", "build", "./..."); err != nil {
			findings = append(findings, CycleFinding{
				ID:          fmt.Sprintf("gate-build-%d", time.Now().UnixNano()),
				Category:    "gate_failure",
				Severity:    "critical",
				Description: truncateGateOutput(fmt.Sprintf("go build failed: %s", combineOutput(out, err)), 500),
				Source:      "supervisor_gate",
			})
		}
	}

	if sg.RequireTest {
		if out, err := sg.run(gateCtx, repoPath, "go", "test", "./...", "-count=1"); err != nil {
			findings = append(findings, CycleFinding{
				ID:          fmt.Sprintf("gate-test-%d", time.Now().UnixNano()),
				Category:    "gate_failure",
				Severity:    "high",
				Description: truncateGateOutput(fmt.Sprintf("go test failed: %s", combineOutput(out, err)), 500),
				Source:      "supervisor_gate",
			})
		}
	}

	if sg.RequireVet {
		if out, err := sg.run(gateCtx, repoPath, "go", "vet", "./..."); err != nil {
			findings = append(findings, CycleFinding{
				ID:          fmt.Sprintf("gate-vet-%d", time.Now().UnixNano()),
				Category:    "gate_failure",
				Severity:    "medium",
				Description: truncateGateOutput(fmt.Sprintf("go vet warnings: %s", combineOutput(out, err)), 500),
				Source:      "supervisor_gate",
			})
		}
	}

	if sg.MinCoverage > 0 {
		covPath := filepath.Join(repoPath, ".ralph", "coverage.txt")
		if data, err := os.ReadFile(covPath); err == nil {
			var pct float64
			if _, parseErr := fmt.Sscanf(strings.TrimSpace(string(data)), "%f", &pct); parseErr == nil {
				if pct < sg.MinCoverage {
					findings = append(findings, CycleFinding{
						ID:          fmt.Sprintf("gate-coverage-%d", time.Now().UnixNano()),
						Category:    "gate_failure",
						Severity:    "high",
						Description: truncateGateOutput(fmt.Sprintf("coverage %.1f%% below minimum %.1f%%", pct, sg.MinCoverage), 500),
						Source:      "supervisor_gate",
					})
				}
			}
		}
	}

	return findings, len(findings) == 0
}

// run executes a command, using the pluggable runner if set.
func (sg *SupervisorGates) run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if sg.runCmd != nil {
		return sg.runCmd(ctx, dir, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = SetSelfTestEnv(os.Environ())
	return cmd.CombinedOutput()
}

// combineOutput merges command output and error into a single string.
func combineOutput(out []byte, err error) string {
	msg := string(out)
	if err != nil && msg == "" {
		msg = err.Error()
	}
	return msg
}

// truncateGateOutput limits a string to maxLen characters.
func truncateGateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}


