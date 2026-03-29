package session

import (
	"errors"
	"testing"
)

func TestValidateTeamCreate_BlocksAtTeamLimit(t *testing.T) {
	config := DefaultTeamSafety // MaxTotalTeams = 20

	err := ValidateTeamCreate("new-team", 3, 20, config)
	if err == nil {
		t.Fatal("expected error at team limit, got nil")
	}
	var safetyErr *TeamSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected TeamSafetyError, got %T", err)
	}
	if safetyErr.Check != "max_total_teams" {
		t.Errorf("expected check=max_total_teams, got %s", safetyErr.Check)
	}
}

func TestValidateTeamCreate_BlocksWhenTeamTooLarge(t *testing.T) {
	config := DefaultTeamSafety // MaxTeamSize = 10

	err := ValidateTeamCreate("big-team", 15, 0, config)
	if err == nil {
		t.Fatal("expected error for oversized team, got nil")
	}
	var safetyErr *TeamSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected TeamSafetyError, got %T", err)
	}
	if safetyErr.Check != "max_team_size" {
		t.Errorf("expected check=max_team_size, got %s", safetyErr.Check)
	}
}

func TestValidateTeamNesting_BlocksAtMaxDepth(t *testing.T) {
	config := DefaultTeamSafety // MaxNestingDepth = 3

	err := ValidateTeamNesting(3, config)
	if err == nil {
		t.Fatal("expected error at max nesting depth, got nil")
	}
	var safetyErr *TeamSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected TeamSafetyError, got %T", err)
	}
	if safetyErr.Check != "max_nesting_depth" {
		t.Errorf("expected check=max_nesting_depth, got %s", safetyErr.Check)
	}
}

func TestValidateTeamCreate_PassesWithNormalValues(t *testing.T) {
	config := DefaultTeamSafety

	if err := ValidateTeamCreate("my-team", 5, 10, config); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateTeamNesting_PassesUnderLimit(t *testing.T) {
	config := DefaultTeamSafety

	if err := ValidateTeamNesting(0, config); err != nil {
		t.Fatalf("expected nil error for depth 0, got %v", err)
	}
	if err := ValidateTeamNesting(2, config); err != nil {
		t.Fatalf("expected nil error for depth 2, got %v", err)
	}
}

func TestValidateTeamNesting_ExactlyAtLimit(t *testing.T) {
	config := TeamSafetyConfig{MaxNestingDepth: 3}

	// parentDepth=2, new team at depth 3 — should pass (3 <= 3)
	if err := ValidateTeamNesting(2, config); err != nil {
		t.Fatalf("expected nil error at exactly max depth, got %v", err)
	}

	// parentDepth=3, new team at depth 4 — should fail (4 > 3)
	if err := ValidateTeamNesting(3, config); err == nil {
		t.Fatal("expected error exceeding max depth, got nil")
	}
}
