package session

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ValidationResult holds pre-flight check results.
type ValidationResult struct {
	Errors   []string // fatal — cannot proceed
	Warnings []string // non-fatal — can proceed with caution
}

// OK returns true when there are no fatal errors.
func (vr ValidationResult) OK() bool { return len(vr.Errors) == 0 }

// ValidateConfig performs pre-flight checks for autonomous operation.
// Errors are fatal (supervisor cannot start); warnings are advisory.
func ValidateConfig(repoPath string) ValidationResult {
	var vr ValidationResult

	// --- Fatal checks ---

	// 1. repoPath exists and is a directory.
	info, err := os.Stat(repoPath)
	if err != nil {
		vr.Errors = append(vr.Errors, fmt.Sprintf("repo path does not exist: %s", repoPath))
		return vr // remaining checks depend on the path existing
	}
	if !info.IsDir() {
		vr.Errors = append(vr.Errors, fmt.Sprintf("repo path is not a directory: %s", repoPath))
		return vr
	}

	// 2. .git directory exists (valid repo).
	gitDir := filepath.Join(repoPath, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		vr.Errors = append(vr.Errors, fmt.Sprintf("no .git directory in %s — not a valid git repo", repoPath))
	}

	// 3. .ralph/ is writable (try creating a temp file).
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

	// 4. ROADMAP.md exists and has at least one unchecked item.
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

	// --- Warning checks ---

	// 1. claude CLI available.
	if _, err := exec.LookPath("claude"); err != nil {
		vr.Warnings = append(vr.Warnings, "claude CLI not found in PATH — Claude provider may not work")
	}

	// 2. Cost observations history.
	costPath := filepath.Join(repoPath, ".ralph", "cost_observations.json")
	if _, err := os.Stat(costPath); err != nil {
		vr.Warnings = append(vr.Warnings, "no .ralph/cost_observations.json — no cost history available")
	}

	// 3. Dirty git working tree.
	cmd := exec.Command("git", "-C", repoPath, "status", "--porcelain")
	if out, err := cmd.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		vr.Warnings = append(vr.Warnings, "dirty git working tree — uncommitted changes present")
	}

	return vr
}
