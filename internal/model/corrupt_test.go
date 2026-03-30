package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCorruptFileHandling(t *testing.T) {
	t.Run("TruncatedJSON", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".ralph", "status.json"), []byte(`{"status":"run`), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadStatus(dir)
		if err == nil {
			t.Fatal("expected error for truncated JSON, got nil")
		}
	})

	t.Run("InvalidUTF8", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0755); err != nil {
			t.Fatal(err)
		}
		data := []byte{0xff, 0xfe}
		if err := os.WriteFile(filepath.Join(dir, ".ralph", "status.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadStatus(dir)
		if err == nil {
			t.Fatal("expected error for invalid UTF-8, got nil")
		}
	})

	t.Run("ZeroByteFile", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".ralph", "status.json"), []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadStatus(dir)
		if err == nil {
			t.Fatal("expected error for zero-byte file, got nil")
		}
	})
}
