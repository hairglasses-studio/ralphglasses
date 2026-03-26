package session

import (
	"testing"
)

func TestBlackboard_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	bb := NewBlackboard(dir)

	bb.Put("key1", "value1", "test-source")
	bb.Put("key2", 42, "other-source")

	val, ok := bb.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("key1 = %v, want value1", val)
	}

	val, ok = bb.Get("key2")
	if !ok {
		t.Fatal("expected key2 to exist")
	}
	// JSON round-trip may convert int to float64
	if v, isFloat := val.(float64); isFloat {
		if v != 42 {
			t.Errorf("key2 = %v, want 42", val)
		}
	}

	_, ok = bb.Get("missing")
	if ok {
		t.Error("expected missing key to return false")
	}
}

func TestBlackboard_Len(t *testing.T) {
	dir := t.TempDir()
	bb := NewBlackboard(dir)

	if bb.Len() != 0 {
		t.Errorf("Len = %d, want 0", bb.Len())
	}

	bb.Put("a", 1, "src")
	bb.Put("b", 2, "src")
	if bb.Len() != 2 {
		t.Errorf("Len = %d, want 2", bb.Len())
	}

	// Overwrite should not increase count
	bb.Put("a", 10, "src")
	if bb.Len() != 2 {
		t.Errorf("Len = %d after overwrite, want 2", bb.Len())
	}
}

func TestBlackboard_Query(t *testing.T) {
	dir := t.TempDir()
	bb := NewBlackboard(dir)

	bb.Put("key1", "v1", "source-a")
	bb.Put("key2", "v2", "source-b")
	bb.Put("key3", "v3", "source-a")

	// Query by source
	results := bb.Query("source-a")
	if len(results) != 2 {
		t.Errorf("Query(source-a) = %d results, want 2", len(results))
	}

	// Query all
	all := bb.Query("")
	if len(all) != 3 {
		t.Errorf("Query(\"\") = %d results, want 3", len(all))
	}

	// Query non-existent source
	none := bb.Query("nonexistent")
	if len(none) != 0 {
		t.Errorf("Query(nonexistent) = %d results, want 0", len(none))
	}
}

func TestBlackboard_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Write data
	bb1 := NewBlackboard(dir)
	bb1.Put("persist-key", "persist-value", "test")

	// Load into new instance
	bb2 := NewBlackboard(dir)
	val, ok := bb2.Get("persist-key")
	if !ok {
		t.Fatal("expected persist-key to survive reload")
	}
	if val != "persist-value" {
		t.Errorf("persist-key = %v, want persist-value", val)
	}
}

func TestBlackboard_EmptyStateDir(t *testing.T) {
	bb := NewBlackboard("")

	// Should work in-memory even without persistence
	bb.Put("key", "val", "src")
	val, ok := bb.Get("key")
	if !ok || val != "val" {
		t.Errorf("expected key=val, got ok=%v val=%v", ok, val)
	}
}
