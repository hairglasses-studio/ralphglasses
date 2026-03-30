package marathon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- SaveCheckpoint edge cases ---

func TestSaveCheckpoint_ZeroTimestamp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")

	cp := &Checkpoint{
		// Timestamp is zero — SaveCheckpoint should set it to now.
		CyclesCompleted: 3,
		SpentUSD:        1.00,
	}

	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	if cp.Timestamp.IsZero() {
		t.Fatal("expected Timestamp to be set by SaveCheckpoint")
	}

	loaded, err := LoadLatestCheckpoint(dir)
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}
	if loaded.CyclesCompleted != 3 {
		t.Fatalf("CyclesCompleted: got %d, want 3", loaded.CyclesCompleted)
	}
}

func TestSaveCheckpoint_NestedDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c", "checkpoints")

	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: 1,
	}

	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatalf("SaveCheckpoint with nested dir: %v", err)
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(cps))
	}
}

func TestSaveCheckpoint_InvalidDir(t *testing.T) {
	// Use a file as the "directory" to cause MkdirAll to fail.
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: 1,
	}

	err := SaveCheckpoint(filePath, cp)
	if err == nil {
		t.Fatal("expected error when dir path is a file")
	}
}

// --- ListCheckpoints with corruption ---

func TestListCheckpoints_MalformedJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a valid checkpoint.
	cp := &Checkpoint{
		Timestamp:       time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		CyclesCompleted: 5,
		SpentUSD:        2.00,
	}
	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatal(err)
	}

	// Write a malformed file with the cp- prefix.
	malformedPath := filepath.Join(dir, "cp-bad-data.json")
	if err := os.WriteFile(malformedPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatal(err)
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}

	// Should skip the malformed file and return the valid one.
	if len(cps) != 1 {
		t.Fatalf("expected 1 valid checkpoint, got %d", len(cps))
	}
	if cps[0].CyclesCompleted != 5 {
		t.Fatalf("expected CyclesCompleted=5, got %d", cps[0].CyclesCompleted)
	}
}

func TestListCheckpoints_NonCPFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write files that don't match the cp-*.json pattern.
	for _, name := range []string{"readme.txt", "backup.json", "cp-orphan.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Write one valid checkpoint.
	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: 1,
	}
	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatal(err)
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}

	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint (ignoring non-cp files), got %d", len(cps))
	}
}

func TestListCheckpoints_Subdirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a subdirectory that matches the prefix.
	subdir := filepath.Join(dir, "cp-subdir.json")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}

	if len(cps) != 0 {
		t.Fatalf("expected 0 checkpoints (subdirectories skipped), got %d", len(cps))
	}
}

func TestListCheckpoints_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("expected nil error for non-existent dir, got: %v", err)
	}
	if len(cps) != 0 {
		t.Fatalf("expected 0 checkpoints, got %d", len(cps))
	}
}

// --- LoadLatestCheckpoint edge cases ---

func TestLoadLatestCheckpoint_ReturnsNewest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		cp := &Checkpoint{
			Timestamp:       base.Add(time.Duration(i) * time.Hour),
			CyclesCompleted: i + 1,
			SpentUSD:        float64(i) * 0.5,
		}
		if err := SaveCheckpoint(dir, cp); err != nil {
			t.Fatalf("SaveCheckpoint[%d]: %v", i, err)
		}
	}

	latest, err := LoadLatestCheckpoint(dir)
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}

	if latest.CyclesCompleted != 5 {
		t.Fatalf("expected latest CyclesCompleted=5, got %d", latest.CyclesCompleted)
	}
}

func TestLoadLatestCheckpoint_NonExistentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")

	// Non-existent dir returns nil,nil from ListCheckpoints, then "no checkpoints found".
	_, err := LoadLatestCheckpoint(dir)
	if err == nil {
		t.Fatal("expected error for non-existent dir with no checkpoints")
	}
}

// --- Checkpoint JSON roundtrip with all fields ---

func TestCheckpoint_FullJSONRoundtrip(t *testing.T) {
	cp := &Checkpoint{
		Timestamp:       time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC),
		CyclesCompleted: 42,
		SpentUSD:        15.75,
		SupervisorState: session.SupervisorState{
			Running:        true,
			RepoPath:       "/test/repo",
			TickCount:      100,
			BudgetSpentUSD: 12.50,
		},
	}

	data, err := json.Marshal(cp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Checkpoint
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.CyclesCompleted != 42 {
		t.Fatalf("CyclesCompleted: got %d, want 42", restored.CyclesCompleted)
	}
	if restored.SpentUSD != 15.75 {
		t.Fatalf("SpentUSD: got %f, want 15.75", restored.SpentUSD)
	}
	if restored.SupervisorState.TickCount != 100 {
		t.Fatalf("TickCount: got %d, want 100", restored.SupervisorState.TickCount)
	}
	if restored.SupervisorState.BudgetSpentUSD != 12.50 {
		t.Fatalf("BudgetSpentUSD: got %f, want 12.50", restored.SupervisorState.BudgetSpentUSD)
	}
}

// --- Multiple checkpoints save and load ---

func TestMultipleCheckpoints_SaveLoadAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")

	for i := 0; i < 10; i++ {
		cp := &Checkpoint{
			Timestamp:       time.Date(2026, 1, 1, i, 0, 0, 0, time.UTC),
			CyclesCompleted: i * 10,
			SpentUSD:        float64(i) * 1.1,
		}
		if err := SaveCheckpoint(dir, cp); err != nil {
			t.Fatalf("SaveCheckpoint[%d]: %v", i, err)
		}
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(cps) != 10 {
		t.Fatalf("expected 10 checkpoints, got %d", len(cps))
	}

	// Verify ascending order.
	for i := 1; i < len(cps); i++ {
		if !cps[i].Timestamp.After(cps[i-1].Timestamp) {
			t.Fatalf("checkpoint[%d] not after checkpoint[%d]", i, i-1)
		}
	}

	latest, err := LoadLatestCheckpoint(dir)
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}
	if latest.CyclesCompleted != 90 {
		t.Fatalf("expected latest CyclesCompleted=90, got %d", latest.CyclesCompleted)
	}
}

// --- Checkpoint corruption recovery ---

func TestListCheckpoints_AllCorrupted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write only corrupted files.
	for i := 0; i < 3; i++ {
		name := filepath.Join(dir, "cp-corrupt"+string(rune('0'+i))+".json")
		if err := os.WriteFile(name, []byte("not json "+string(rune('0'+i))), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cps, err := ListCheckpoints(dir)
	if err != nil {
		t.Fatalf("ListCheckpoints: %v", err)
	}
	if len(cps) != 0 {
		t.Fatalf("expected 0 valid checkpoints from corrupted files, got %d", len(cps))
	}
}

func TestLoadLatestCheckpoint_AllCorrupted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "cp-bad.json"), []byte("{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadLatestCheckpoint(dir)
	if err == nil {
		t.Fatal("expected error when all checkpoints are corrupted")
	}
}

// --- Checkpoint with missing directory created automatically ---

func TestSaveCheckpoint_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep", "nested", "cp")

	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: 1,
		SpentUSD:        0.50,
	}

	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatalf("SaveCheckpoint should create nested dirs: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory to be created")
	}
}

// --- Zero-value Checkpoint ---

func TestCheckpoint_ZeroValue(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "checkpoints")

	cp := &Checkpoint{} // All zero values.
	if err := SaveCheckpoint(dir, cp); err != nil {
		t.Fatalf("SaveCheckpoint with zero values: %v", err)
	}

	loaded, err := LoadLatestCheckpoint(dir)
	if err != nil {
		t.Fatalf("LoadLatestCheckpoint: %v", err)
	}

	if loaded.CyclesCompleted != 0 {
		t.Fatalf("expected 0 CyclesCompleted, got %d", loaded.CyclesCompleted)
	}
	if loaded.SpentUSD != 0 {
		t.Fatalf("expected 0 SpentUSD, got %f", loaded.SpentUSD)
	}
}
