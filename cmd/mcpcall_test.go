package cmd

import (
	"testing"
)

func TestMCPCallCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "mcp-call" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("mcp-call subcommand not registered on rootCmd")
	}
}

func TestMCPCallCmd_Defaults(t *testing.T) {
	cmd := mcpCallCmd

	if cmd.Use != "mcp-call <tool-name> [--param key=value ...]" {
		t.Errorf("Use = %q, unexpected", cmd.Use)
	}

	f := cmd.Flags().Lookup("param")
	if f == nil {
		t.Fatal("param flag not registered")
	}
	if f.Shorthand != "p" {
		t.Errorf("param shorthand = %q, want %q", f.Shorthand, "p")
	}
}

func TestMCPCallCmd_RequiresArgs(t *testing.T) {
	// cobra.MinimumNArgs(1) should reject zero args
	err := mcpCallCmd.Args(mcpCallCmd, []string{})
	if err == nil {
		t.Error("expected error when no args provided, got nil")
	}
}

func TestMCPCallCmd_AcceptsArgs(t *testing.T) {
	err := mcpCallCmd.Args(mcpCallCmd, []string{"ralphglasses_scan"})
	if err != nil {
		t.Errorf("unexpected error with one arg: %v", err)
	}
}
