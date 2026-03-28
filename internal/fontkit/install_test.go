package fontkit

import (
	"context"
	"os/exec"
	"testing"
)

func TestInstallDryRun(t *testing.T) {
	if _, err := exec.LookPath("brew"); err != nil {
		t.Skip("homebrew not available")
	}
	result, err := Install(context.Background(), InstallOpts{
		NerdFont: true,
		DryRun:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Error("DryRun should be true")
	}
	if result.Cask != "font-monaspice-nerd-font" {
		t.Errorf("Cask = %q, want font-monaspice-nerd-font", result.Cask)
	}
	if result.Output == "" {
		t.Error("Output should describe what would be done")
	}
}

func TestInstallDryRunMonaspace(t *testing.T) {
	if _, err := exec.LookPath("brew"); err != nil {
		t.Skip("homebrew not available")
	}
	result, err := Install(context.Background(), InstallOpts{
		NerdFont: false,
		DryRun:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Cask != "font-monaspace" {
		t.Errorf("Cask = %q, want font-monaspace", result.Cask)
	}
}
