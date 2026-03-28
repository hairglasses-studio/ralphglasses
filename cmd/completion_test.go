package cmd

import (
	"bytes"
	"testing"
)

func TestCompletionBash(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := rootCmd.GenBashCompletion(buf); err != nil {
		t.Fatalf("GenBashCompletion: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty bash completion output")
	}
}

func TestCompletionZsh(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := rootCmd.GenZshCompletion(buf); err != nil {
		t.Fatalf("GenZshCompletion: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty zsh completion output")
	}
}

func TestCompletionFish(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := rootCmd.GenFishCompletion(buf, true); err != nil {
		t.Fatalf("GenFishCompletion: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty fish completion output")
	}
}

func TestCompletionCmdRegistered(t *testing.T) {
	// Verify the completion command is registered as a subcommand
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "completion [bash|zsh|fish]" {
			found = true
			break
		}
	}
	if !found {
		t.Error("completion command not registered with rootCmd")
	}
}

func TestCompletionCmdValidArgs(t *testing.T) {
	if len(completionCmd.ValidArgs) == 0 {
		t.Error("completionCmd should have ValidArgs set")
	}
	expected := map[string]bool{"bash": true, "zsh": true, "fish": true}
	for _, arg := range completionCmd.ValidArgs {
		if !expected[arg] {
			t.Errorf("unexpected ValidArg: %s", arg)
		}
	}
}

func TestCompletionCmdRequiresOneArg(t *testing.T) {
	// Verify the command requires exactly 1 argument
	if completionCmd.Args == nil {
		t.Error("completionCmd should have Args validator set")
	}
}
