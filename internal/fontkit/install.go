package fontkit

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// InstallOpts configures font installation.
type InstallOpts struct {
	NerdFont bool // Install Monaspice (Nerd Font) instead of plain Monaspace
	DryRun   bool // Print what would be done without doing it
}

// InstallResult describes the outcome of a font installation.
type InstallResult struct {
	Cask      string // Homebrew cask that was installed
	DryRun    bool   // Whether this was a dry run
	Output    string // Command output
	AlreadyOK bool   // Font was already installed
}

// Install installs Monaspace or Monaspice fonts via Homebrew.
func Install(ctx context.Context, opts InstallOpts) (*InstallResult, error) {
	if _, err := exec.LookPath("brew"); err != nil {
		return nil, fmt.Errorf("homebrew not found: install from https://brew.sh")
	}

	cask := "font-monaspace"
	if opts.NerdFont {
		cask = "font-monaspice-nerd-font"
	}

	result := &InstallResult{
		Cask:   cask,
		DryRun: opts.DryRun,
	}

	if opts.DryRun {
		result.Output = fmt.Sprintf("would run: brew install --cask %s", cask)
		return result, nil
	}

	// Check if already installed
	checkCmd := exec.CommandContext(ctx, "brew", "list", "--cask", cask)
	if err := checkCmd.Run(); err == nil {
		result.AlreadyOK = true
		result.Output = fmt.Sprintf("%s is already installed", cask)
		return result, nil
	}

	// Install
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "brew", "install", "--cask", cask)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("brew install --cask %s failed: %w\n%s", cask, err, stderr.String())
	}

	result.Output = stdout.String()
	return result, nil
}
