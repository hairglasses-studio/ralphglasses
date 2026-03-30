package worktree

import (
	"testing"
)

func TestIsConflictStatus_ConflictCodes(t *testing.T) {
	conflictCodes := []string{"UU", "AA", "DD", "AU", "UA", "DU", "UD"}
	for _, code := range conflictCodes {
		t.Run(code, func(t *testing.T) {
			if !isConflictStatus(code) {
				t.Errorf("isConflictStatus(%q) = false, want true", code)
			}
		})
	}
}

func TestIsConflictStatus_NonConflictCodes(t *testing.T) {
	nonConflict := []string{"M ", " M", "A ", " A", "D ", "??", "R ", "C ", "MM", ""}
	for _, code := range nonConflict {
		t.Run(code, func(t *testing.T) {
			if isConflictStatus(code) {
				t.Errorf("isConflictStatus(%q) = true, want false", code)
			}
		})
	}
}

func TestWithAbortOnConflict_SetsField(t *testing.T) {
	cfg := &mergeConfig{}
	opt := WithAbortOnConflict()
	opt(cfg)
	if !cfg.abortOnConflict {
		t.Error("WithAbortOnConflict should set abortOnConflict to true")
	}
}

func TestWithSquash_SetsField(t *testing.T) {
	cfg := &mergeConfig{}
	opt := WithSquash()
	opt(cfg)
	if !cfg.squash {
		t.Error("WithSquash should set squash to true")
	}
}

func TestWithMessage_SetsField(t *testing.T) {
	cfg := &mergeConfig{}
	opt := WithMessage("my commit message")
	opt(cfg)
	if cfg.message != "my commit message" {
		t.Errorf("WithMessage: message = %q, want %q", cfg.message, "my commit message")
	}
}

func TestWithDryRun_SetsField(t *testing.T) {
	cfg := &mergeConfig{}
	opt := WithDryRun()
	opt(cfg)
	if !cfg.dryRun {
		t.Error("WithDryRun should set dryRun to true")
	}
}
