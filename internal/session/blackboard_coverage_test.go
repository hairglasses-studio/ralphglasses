package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlackboard_PutAndSave_FileContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bb := NewBlackboard(dir)

	bb.Put("test_key", "test_value", "test_source")

	// Verify the file was written with content.
	path := filepath.Join(dir, "blackboard.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if len(data) == 0 {
		t.Error("saved file is empty")
	}
}

func TestBlackboard_OverwriteUpdatesValue(t *testing.T) {
	t.Parallel()
	bb := NewBlackboard(t.TempDir())
	bb.Put("k1", "original", "s")
	bb.Put("k1", "updated", "s")

	val, ok := bb.Get("k1")
	if !ok {
		t.Fatal("key not found after overwrite")
	}
	if val != "updated" {
		t.Errorf("value = %v, want updated", val)
	}
	if bb.Len() != 1 {
		t.Errorf("Len after overwrite = %d, want 1", bb.Len())
	}
}
