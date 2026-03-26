package cmd

import (
	"testing"
)

func TestSelftestCmd_Defaults(t *testing.T) {
	// Verify the command is registered and has correct defaults.
	cmd := selftestCmd

	if cmd.Use != "selftest" {
		t.Errorf("Use = %q, want %q", cmd.Use, "selftest")
	}

	// Check flag defaults
	f := cmd.Flags()

	iterations, err := f.GetInt("iterations")
	if err != nil {
		t.Fatal(err)
	}
	if iterations != 2 {
		t.Errorf("iterations default = %d, want 2", iterations)
	}

	budget, err := f.GetFloat64("budget")
	if err != nil {
		t.Fatal(err)
	}
	if budget != 2.0 {
		t.Errorf("budget default = %f, want 2.0", budget)
	}

	repoPath, err := f.GetString("repo-path")
	if err != nil {
		t.Fatal(err)
	}
	if repoPath != "." {
		t.Errorf("repo-path default = %q, want %q", repoPath, ".")
	}

	jsonFlag, err := f.GetBool("json")
	if err != nil {
		t.Fatal(err)
	}
	if jsonFlag {
		t.Error("json default should be false")
	}

	gate, err := f.GetBool("gate")
	if err != nil {
		t.Fatal(err)
	}
	if gate {
		t.Error("gate default should be false")
	}
}

func TestSelftestCmd_DryRunFlag(t *testing.T) {
	f := selftestCmd.Flags()

	dryRun, err := f.GetBool("dry-run")
	if err != nil {
		t.Fatalf("dry-run flag not found: %v", err)
	}
	if dryRun {
		t.Error("dry-run default should be false")
	}
}

func TestSelftestCmd_Registration(t *testing.T) {
	// Verify the command is registered on rootCmd.
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "selftest" {
			found = true
			break
		}
	}
	if !found {
		t.Error("selftest command not registered on rootCmd")
	}
}
