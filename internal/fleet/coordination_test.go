package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// cleanCoordDir removes the coordination directory for a clean test slate,
// then restores whatever existed before the test.
func cleanCoordDir(t *testing.T) {
	t.Helper()

	// Back up existing claims if any.
	claimDir := filepath.Join(CoordDir, claimsSubdir)
	entries, _ := os.ReadDir(claimDir)
	backups := make(map[string][]byte)
	for _, e := range entries {
		if data, err := os.ReadFile(filepath.Join(claimDir, e.Name())); err == nil {
			backups[e.Name()] = data
		}
	}

	t.Cleanup(func() {
		// Remove test artifacts.
		os.RemoveAll(filepath.Join(CoordDir, claimsSubdir))
		os.MkdirAll(filepath.Join(CoordDir, claimsSubdir), 0755)
		// Restore originals.
		for name, data := range backups {
			os.WriteFile(filepath.Join(claimDir, name), data, 0644)
		}
	})

	// Clear claims for the test.
	os.RemoveAll(filepath.Join(CoordDir, claimsSubdir))
}

func TestCoordEnsureCoordDir(t *testing.T) {
	cleanCoordDir(t)
	// Remove the entire coordination dir to test creation from scratch.
	os.RemoveAll(CoordDir)
	t.Cleanup(func() {
		// Ensure it exists again for other tests.
		os.MkdirAll(filepath.Join(CoordDir, claimsSubdir), 0755)
	})

	if err := EnsureCoordDir(); err != nil {
		t.Fatalf("EnsureCoordDir() error: %v", err)
	}

	// Verify both directories were created.
	for _, dir := range []string{CoordDir, filepath.Join(CoordDir, claimsSubdir)} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("directory %s not created: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}
}

func TestCoordEnsureCoordDirIdempotent(t *testing.T) {
	cleanCoordDir(t)

	// Call twice — should not error.
	if err := EnsureCoordDir(); err != nil {
		t.Fatalf("first EnsureCoordDir() error: %v", err)
	}
	if err := EnsureCoordDir(); err != nil {
		t.Fatalf("second EnsureCoordDir() error: %v", err)
	}
}

func TestCoordClaimResource(t *testing.T) {
	cleanCoordDir(t)

	tests := []struct {
		name     string
		agent    string
		resource string
	}{
		{"simple resource", "claude-code", "repo:dotfiles"},
		{"path resource", "gemini", "/home/user/project"},
		{"unicode resource", "codex", "repo:emoji-\U0001F680"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ClaimResource(tt.agent, tt.resource); err != nil {
				t.Fatalf("ClaimResource(%q, %q) error: %v", tt.agent, tt.resource, err)
			}

			// Read the claim file and verify JSON contents.
			claimPath := filepath.Join(CoordDir, claimsSubdir, claimFileName(tt.resource))
			data, err := os.ReadFile(claimPath)
			if err != nil {
				t.Fatalf("claim file not found at %s: %v", claimPath, err)
			}

			var claim Claim
			if err := json.Unmarshal(data, &claim); err != nil {
				t.Fatalf("invalid JSON in claim file: %v", err)
			}

			if claim.Agent != tt.agent {
				t.Errorf("Agent = %q, want %q", claim.Agent, tt.agent)
			}
			if claim.Resource != tt.resource {
				t.Errorf("Resource = %q, want %q", claim.Resource, tt.resource)
			}
			if claim.Timestamp.IsZero() {
				t.Error("Timestamp is zero, expected non-zero")
			}
		})
	}
}

func TestCoordReleaseClaim(t *testing.T) {
	cleanCoordDir(t)

	agent, resource := "claude-code", "repo:test-release"
	if err := ClaimResource(agent, resource); err != nil {
		t.Fatalf("ClaimResource setup error: %v", err)
	}

	// Verify the file exists first.
	claimPath := filepath.Join(CoordDir, claimsSubdir, claimFileName(resource))
	if _, err := os.Stat(claimPath); err != nil {
		t.Fatalf("claim file should exist before release: %v", err)
	}

	if err := ReleaseClaim(resource); err != nil {
		t.Fatalf("ReleaseClaim() error: %v", err)
	}

	// File should be gone.
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Fatalf("claim file should not exist after release, got err: %v", err)
	}
}

func TestCoordReleaseClaimNonExistent(t *testing.T) {
	cleanCoordDir(t)
	if err := EnsureCoordDir(); err != nil {
		t.Fatal(err)
	}

	// Releasing a non-existent claim should not error.
	if err := ReleaseClaim("never-claimed-resource"); err != nil {
		t.Fatalf("ReleaseClaim() for non-existent should not error, got: %v", err)
	}
}

func TestCoordIsResourceClaimed(t *testing.T) {
	cleanCoordDir(t)

	resource := "repo:is-claimed-test"

	// Before claiming.
	claimed, claim, err := IsResourceClaimed(resource)
	if err != nil {
		t.Fatalf("IsResourceClaimed() error: %v", err)
	}
	if claimed {
		t.Fatal("expected resource to not be claimed initially")
	}
	if claim != nil {
		t.Fatal("expected nil claim when not claimed")
	}

	// After claiming.
	if err := ClaimResource("agent-x", resource); err != nil {
		t.Fatal(err)
	}
	claimed, claim, err = IsResourceClaimed(resource)
	if err != nil {
		t.Fatalf("IsResourceClaimed() error: %v", err)
	}
	if !claimed {
		t.Fatal("expected resource to be claimed")
	}
	if claim == nil {
		t.Fatal("expected non-nil claim")
	}
	if claim.Agent != "agent-x" {
		t.Errorf("claim.Agent = %q, want %q", claim.Agent, "agent-x")
	}

	// After releasing.
	if err := ReleaseClaim(resource); err != nil {
		t.Fatal(err)
	}
	claimed, _, err = IsResourceClaimed(resource)
	if err != nil {
		t.Fatalf("IsResourceClaimed() error: %v", err)
	}
	if claimed {
		t.Fatal("expected resource to not be claimed after release")
	}
}

func TestCoordListClaims(t *testing.T) {
	cleanCoordDir(t)

	// Empty list when no claims.
	if err := EnsureCoordDir(); err != nil {
		t.Fatal(err)
	}
	claims, err := ListClaims()
	if err != nil {
		t.Fatalf("ListClaims() error: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("expected 0 claims, got %d", len(claims))
	}

	// Create multiple claims.
	resources := []struct {
		agent    string
		resource string
	}{
		{"claude", "repo:alpha"},
		{"gemini", "repo:beta"},
		{"codex", "repo:gamma"},
	}
	for _, r := range resources {
		if err := ClaimResource(r.agent, r.resource); err != nil {
			t.Fatal(err)
		}
	}

	claims, err = ListClaims()
	if err != nil {
		t.Fatalf("ListClaims() error: %v", err)
	}
	if len(claims) != 3 {
		t.Fatalf("expected 3 claims, got %d", len(claims))
	}

	// Verify all agents are present.
	agents := make(map[string]bool)
	for _, c := range claims {
		agents[c.Agent] = true
	}
	for _, r := range resources {
		if !agents[r.agent] {
			t.Errorf("agent %q not found in claims", r.agent)
		}
	}
}
