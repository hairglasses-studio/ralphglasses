package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSupervisorGatesAllPass(t *testing.T) {
	gates := DefaultSupervisorGates()
	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	findings, passed := gates.Evaluate(context.Background(), t.TempDir())
	if !passed {
		t.Fatalf("expected all gates to pass, got %d findings", len(findings))
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSupervisorGatesBuildFailure(t *testing.T) {
	gates := DefaultSupervisorGates()
	gates.RequireTest = false
	gates.RequireVet = false
	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "build" {
			return []byte("cannot find package"), fmt.Errorf("exit status 1")
		}
		return nil, nil
	}

	findings, passed := gates.Evaluate(context.Background(), t.TempDir())
	if passed {
		t.Fatal("expected gates to fail on build failure")
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Severity != "critical" {
		t.Errorf("expected severity critical, got %s", f.Severity)
	}
	if f.Category != "gate_failure" {
		t.Errorf("expected category gate_failure, got %s", f.Category)
	}
	if f.Source != "supervisor_gate" {
		t.Errorf("expected source supervisor_gate, got %s", f.Source)
	}
}

func TestSupervisorGatesTestFailure(t *testing.T) {
	gates := DefaultSupervisorGates()
	gates.RequireBuild = false
	gates.RequireVet = false
	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if slices.Contains(args, "test") {
			return []byte("--- FAIL: TestFoo"), fmt.Errorf("exit status 1")
		}
		return nil, nil
	}

	findings, passed := gates.Evaluate(context.Background(), t.TempDir())
	if passed {
		t.Fatal("expected gates to fail on test failure")
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != "high" {
		t.Errorf("expected severity high, got %s", findings[0].Severity)
	}
}

func TestSupervisorGatesVetWarning(t *testing.T) {
	gates := DefaultSupervisorGates()
	gates.RequireBuild = false
	gates.RequireTest = false
	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if slices.Contains(args, "vet") {
			return []byte("unreachable code"), fmt.Errorf("exit status 1")
		}
		return nil, nil
	}

	findings, passed := gates.Evaluate(context.Background(), t.TempDir())
	if passed {
		t.Fatal("expected gates to fail on vet warning")
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != "medium" {
		t.Errorf("expected severity medium, got %s", findings[0].Severity)
	}
}

func TestSupervisorGatesDisabled(t *testing.T) {
	gates := &SupervisorGates{
		RequireBuild: false,
		RequireTest:  false,
		RequireVet:   false,
		TestTimeout:  10 * time.Second,
	}
	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		t.Fatal("runCmd should not be called when all gates are disabled")
		return nil, nil
	}

	findings, passed := gates.Evaluate(context.Background(), t.TempDir())
	if !passed {
		t.Fatal("expected all gates to pass when disabled")
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSupervisorGatesContextCancellation(t *testing.T) {
	gates := DefaultSupervisorGates()
	gates.TestTimeout = 5 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, nil
	}

	findings, passed := gates.Evaluate(ctx, t.TempDir())
	if passed {
		t.Fatal("expected gates to fail on cancelled context")
	}
	if len(findings) == 0 {
		t.Fatal("expected at least one finding from cancelled context")
	}
}

func TestSupervisorGatesCoverageCheck(t *testing.T) {
	tmp := t.TempDir()
	ralphDir := filepath.Join(tmp, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "coverage.txt"), []byte("72.5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	gates := &SupervisorGates{
		RequireBuild: false,
		RequireTest:  false,
		RequireVet:   false,
		MinCoverage:  80.0,
		TestTimeout:  10 * time.Second,
	}

	findings, passed := gates.Evaluate(context.Background(), tmp)
	if passed {
		t.Fatal("expected gates to fail on low coverage")
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != "high" {
		t.Errorf("expected severity high, got %s", findings[0].Severity)
	}
}

func TestSupervisorGatesCoveragePass(t *testing.T) {
	tmp := t.TempDir()
	ralphDir := filepath.Join(tmp, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "coverage.txt"), []byte("85.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	gates := &SupervisorGates{
		RequireBuild: false,
		RequireTest:  false,
		RequireVet:   false,
		MinCoverage:  80.0,
		TestTimeout:  10 * time.Second,
	}

	findings, passed := gates.Evaluate(context.Background(), tmp)
	if !passed {
		t.Fatal("expected gates to pass when coverage meets minimum")
	}
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(findings))
	}
}

func TestSupervisorGatesTruncation(t *testing.T) {
	long := make([]byte, 600)
	for i := range long {
		long[i] = 'x'
	}

	gates := &SupervisorGates{
		RequireBuild: true,
		RequireTest:  false,
		RequireVet:   false,
		TestTimeout:  10 * time.Second,
	}
	gates.runCmd = func(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
		return long, fmt.Errorf("exit status 1")
	}

	findings, _ := gates.Evaluate(context.Background(), t.TempDir())
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if len(findings[0].Description) > 500 {
		t.Errorf("description not truncated: len=%d", len(findings[0].Description))
	}
}
