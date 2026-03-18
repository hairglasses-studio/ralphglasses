package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateIdentity(t *testing.T) {
	id := GenerateIdentity("2026-03-17-run1", 0, 3, "task123")

	if id.AgentIndex != 0 {
		t.Errorf("AgentIndex = %d, want 0", id.AgentIndex)
	}
	if id.AgentCount != 3 {
		t.Errorf("AgentCount = %d, want 3", id.AgentCount)
	}
	if id.SeedHash == "" {
		t.Error("SeedHash should not be empty")
	}
	if len(id.SeedHash) != 8 {
		t.Errorf("SeedHash length = %d, want 8", len(id.SeedHash))
	}
	if id.Persona != "implementer" {
		t.Errorf("Persona = %q, want %q (index 0)", id.Persona, "implementer")
	}
}

func TestGenerateIdentityDeterministic(t *testing.T) {
	id1 := GenerateIdentity("run1", 0, 3, "task1")
	id2 := GenerateIdentity("run1", 0, 3, "task1")

	if id1.SeedHash != id2.SeedHash {
		t.Errorf("same inputs should produce same hash: %q != %q", id1.SeedHash, id2.SeedHash)
	}
}

func TestGenerateIdentityUniqueness(t *testing.T) {
	seeds := make(map[string]bool)
	for i := 0; i < 5; i++ {
		id := GenerateIdentity("run1", i, 5, "task1")
		if seeds[id.SeedHash] {
			t.Errorf("duplicate seed hash at index %d: %s", i, id.SeedHash)
		}
		seeds[id.SeedHash] = true
	}
}

func TestPersonaRotation(t *testing.T) {
	for i := 0; i < 10; i++ {
		id := GenerateIdentity("run1", i, 10, "task1")
		expected := Personas[i%len(Personas)].Name
		if id.Persona != expected {
			t.Errorf("index %d: persona = %q, want %q", i, id.Persona, expected)
		}
	}
}

func TestSaveLoadIdentity(t *testing.T) {
	dir := t.TempDir()
	id := GenerateIdentity("test-run", 2, 5, "hash123")

	if err := SaveIdentity(dir, id); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}

	loaded, err := LoadIdentity(dir)
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}

	if loaded.SeedHash != id.SeedHash {
		t.Errorf("SeedHash mismatch: %q != %q", loaded.SeedHash, id.SeedHash)
	}
	if loaded.Persona != id.Persona {
		t.Errorf("Persona mismatch: %q != %q", loaded.Persona, id.Persona)
	}
}

func TestClaimTask(t *testing.T) {
	dir := t.TempDir()

	if err := ClaimTask(dir, "test-task-1", 0); err != nil {
		t.Fatalf("ClaimTask: %v", err)
	}

	// Double-claim should fail
	if err := ClaimTask(dir, "test-task-1", 1); err == nil {
		t.Error("expected error on double claim")
	}

	// Different task should succeed
	if err := ClaimTask(dir, "test-task-2", 1); err != nil {
		t.Fatalf("ClaimTask different task: %v", err)
	}

	claims, err := ListClaims(dir)
	if err != nil {
		t.Fatalf("ListClaims: %v", err)
	}
	if len(claims) != 2 {
		t.Errorf("expected 2 claims, got %d", len(claims))
	}
}

func TestListClaimsEmpty(t *testing.T) {
	dir := t.TempDir()
	claims, err := ListClaims(dir)
	if err != nil {
		t.Fatalf("ListClaims: %v", err)
	}
	if len(claims) != 0 {
		t.Errorf("expected 0 claims, got %d", len(claims))
	}
}

func TestLoadIdentityNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadIdentity(dir)
	if err == nil {
		t.Error("expected error for missing identity file")
	}
}

func TestSaveIdentityCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	id := GenerateIdentity("test", 0, 1, "hash")

	if err := SaveIdentity(dir, id); err != nil {
		t.Fatalf("SaveIdentity: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".ralph", "agent_identity.json")); err != nil {
		t.Fatalf("identity file not created: %v", err)
	}
}
