package fontkit

import (
	"context"
	"os/exec"
	"strings"
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

func TestInstallDryRun_OutputDescribesAction(t *testing.T) {
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
	if result.Output == "" {
		t.Error("DryRun output should describe what would be done")
	}
	if !strings.Contains(result.Output, "brew install") {
		t.Error("DryRun output should mention brew install")
	}
}

func TestInstallDryRun_CaskField(t *testing.T) {
	if _, err := exec.LookPath("brew"); err != nil {
		t.Skip("homebrew not available")
	}
	// NerdFont = true
	r1, _ := Install(context.Background(), InstallOpts{NerdFont: true, DryRun: true})
	if r1.Cask != "font-monaspice-nerd-font" {
		t.Errorf("NerdFont cask = %q, want font-monaspice-nerd-font", r1.Cask)
	}

	// NerdFont = false
	r2, _ := Install(context.Background(), InstallOpts{NerdFont: false, DryRun: true})
	if r2.Cask != "font-monaspace" {
		t.Errorf("Monaspace cask = %q, want font-monaspace", r2.Cask)
	}
}
