package session

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/resource"
)

const validateMinDiskFreeBytes = 5 * 1024 * 1024 * 1024

var (
	validateClaudeLookPath = func() error {
		_, err := exec.LookPath("claude")
		return err
	}
	validateTmuxLookPath = func() error {
		_, err := exec.LookPath("tmux")
		return err
	}
	validateTmuxList = func() ([]byte, error) {
		return exec.Command("tmux", "ls").Output()
	}
	validateGitStatus = func(repoPath string) ([]byte, error) {
		return exec.Command("git", "-C", repoPath, "status", "--porcelain").Output()
	}
	validateResourceStatus = resource.Check
)

type ValidationResult struct {
	Errors   []string
	Warnings []string
}

func (vr ValidationResult) OK() bool { return len(vr.Errors) == 0 }

func ValidateConfig(repoPath string) ValidationResult {
	var vr ValidationResult

	info, err := os.Stat(repoPath)
	if err != nil {
		vr.Errors = append(vr.Errors, fmt.Sprintf("repo path does not exist: %s", repoPath))
		return vr
	}
	if !info.IsDir() {
		vr.Errors = append(vr.Errors, fmt.Sprintf("repo path is not a directory: %s", repoPath))
		return vr
	}

	gitDir := filepath.Join(repoPath, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		vr.Errors = append(vr.Errors, fmt.Sprintf("no .git directory in %s — not a valid git repo", repoPath))
	}

	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		vr.Errors = append(vr.Errors, fmt.Sprintf(".ralph directory not writable: %v", err))
	} else {
		tmp := filepath.Join(ralphDir, ".validate_probe")
		if err := os.WriteFile(tmp, []byte("probe"), 0o644); err != nil {
			vr.Errors = append(vr.Errors, fmt.Sprintf(".ralph directory not writable: %v", err))
		} else {
			_ = os.Remove(tmp)
		}
	}

	roadmap := filepath.Join(repoPath, "ROADMAP.md")
	if f, err := os.Open(roadmap); err != nil {
		vr.Errors = append(vr.Errors, "ROADMAP.md not found — no work items to process")
	} else {
		defer f.Close()
		hasUnchecked := false
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "- [ ]") {
				hasUnchecked = true
				break
			}
		}
		if !hasUnchecked {
			vr.Errors = append(vr.Errors, "ROADMAP.md has no unchecked items — nothing to work on")
		}
	}

	if err := validateClaudeLookPath(); err != nil {
		vr.Warnings = append(vr.Warnings, "claude CLI not found in PATH — Claude provider may not work")
	}

	costPath := filepath.Join(repoPath, ".ralph", "cost_observations.json")
	if _, err := os.Stat(costPath); err != nil {
		vr.Warnings = append(vr.Warnings, "no .ralph/cost_observations.json — no cost history available")
	}

	if out, err := validateGitStatus(repoPath); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		vr.Warnings = append(vr.Warnings, "dirty git working tree — uncommitted changes present")
	}

	if err := validateTmuxLookPath(); err == nil {
		if out, err := validateTmuxList(); err != nil || len(out) == 0 {
			vr.Warnings = append(vr.Warnings, "tmux not active — continuity features (resurrect/continuum) may be unavailable")
		}
	}

	status := validateResourceStatus(repoPath)
	if status.DiskFreeBytes > 0 && status.DiskFreeBytes < validateMinDiskFreeBytes {
		vr.Warnings = append(vr.Warnings, fmt.Sprintf("low disk space: %.1fGB available, want >5GB", float64(status.DiskFreeBytes)/(1024*1024*1024)))
	}

	return vr
}
