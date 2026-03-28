package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateRunID(t *testing.T) {
	id := GenerateRunID()
	if id == "" {
		t.Error("GenerateRunID should return a non-empty string")
	}
	// Format should be "2006-01-02-150405" => 17 chars
	if len(id) != 17 {
		t.Errorf("GenerateRunID length = %d, want 17 (format 2006-01-02-150405)", len(id))
	}
}

func TestLoadIdentity_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write malformed JSON
	if err := os.WriteFile(filepath.Join(ralphDir, "agent_identity.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadIdentity(dir)
	if err == nil {
		t.Error("expected error for malformed JSON identity file")
	}
}

func TestSaveIdentity_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	// Make dir read-only so MkdirAll for .ralph/ fails
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	id := GenerateIdentity("test", 0, 1, "hash")
	err := SaveIdentity(dir, id)
	if err == nil {
		t.Error("expected error when saving to read-only directory")
	}
}

func TestClaimTask_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	err := ClaimTask(dir, "task-1", 0)
	if err == nil {
		t.Error("expected error when claiming task in read-only directory")
	}
}

func TestListClaims_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	claimsDir := filepath.Join(dir, ".ralph", "claimed_tasks")
	if err := os.MkdirAll(claimsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a valid lock file
	if err := os.WriteFile(filepath.Join(claimsDir, "valid.lock"), []byte(`{"task_id":"valid","agent_index":0,"status":"in_progress"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Write a malformed lock file (should be skipped)
	if err := os.WriteFile(filepath.Join(claimsDir, "bad.lock"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write a non-.lock file (should be skipped)
	if err := os.WriteFile(filepath.Join(claimsDir, "readme.txt"), []byte("not a lock"), 0644); err != nil {
		t.Fatal(err)
	}

	claims, err := ListClaims(dir)
	if err != nil {
		t.Fatalf("ListClaims: %v", err)
	}
	// Only the valid lock file should be returned
	if len(claims) != 1 {
		t.Errorf("expected 1 claim (skipping bad JSON and non-.lock), got %d", len(claims))
	}
	if len(claims) == 1 && claims[0].TaskID != "valid" {
		t.Errorf("expected task_id 'valid', got %q", claims[0].TaskID)
	}
}

func TestListClaims_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	claimsDir := filepath.Join(dir, ".ralph", "claimed_tasks")
	if err := os.MkdirAll(claimsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Make claims dir unreadable
	if err := os.Chmod(claimsDir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(claimsDir, 0755) //nolint:errcheck

	_, err := ListClaims(dir)
	if err == nil {
		t.Error("expected error when claims dir is unreadable")
	}
}
