package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppendBenchmarkEntry_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	entry := &BenchmarkEntry{TaskID: "test", Result: "pass"}
	err := AppendBenchmarkEntry(dir, entry)
	if err == nil {
		t.Error("expected error when appending to read-only directory")
	}
}

func TestLoadBenchmarkEntries_MalformedJSONL(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a JSONL file with mixed valid and invalid lines
	content := `{"task_id":"valid","result":"pass"}
{bad json line}
{"task_id":"also-valid","result":"fail"}
`
	if err := os.WriteFile(filepath.Join(ralphDir, "benchmarks.jsonl"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadBenchmarkEntries(dir)
	if err != nil {
		t.Fatalf("LoadBenchmarkEntries: %v", err)
	}
	// Should skip the bad line and return the 2 valid entries
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (skipping bad JSON), got %d", len(entries))
	}
}

func TestLoadBenchmarkEntries_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(ralphDir, "benchmarks.jsonl")
	if err := os.WriteFile(fpath, []byte(`{"task_id":"x"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(fpath, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(fpath, 0644) //nolint:errcheck

	_, err := LoadBenchmarkEntries(dir)
	if err == nil {
		t.Error("expected error for unreadable benchmarks.jsonl")
	}
}

func TestWriteBenchmarkMarkdown_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	summary := &BenchmarkSummary{SessionID: "test"}
	err := WriteBenchmarkMarkdown(dir, summary)
	if err == nil {
		t.Error("expected error when writing to read-only directory")
	}
}

func TestSaveBreakglass_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0755) //nolint:errcheck

	bg := DefaultBreakglass()
	err := SaveBreakglass(dir, bg)
	if err == nil {
		t.Error("expected error when saving breakglass to read-only directory")
	}
}

func TestLoadBreakglass_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "breakglass.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	loaded := LoadBreakglass(dir)
	// Should return defaults on malformed JSON
	if loaded.LoopTokenBudget != 200000 {
		t.Errorf("expected defaults on malformed JSON, got LoopTokenBudget=%d", loaded.LoopTokenBudget)
	}
}
