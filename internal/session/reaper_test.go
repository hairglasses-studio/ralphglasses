package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSweepOrphans_NoSessionsDir(t *testing.T) {
	// Point at a directory that does not exist.
	orphans := SweepOrphans(t.TempDir(), nil)
	if orphans != nil {
		t.Errorf("expected nil, got %v", orphans)
	}
}

func TestSweepOrphans_DeadPID(t *testing.T) {
	// Create a sessions dir with a session file whose PID is definitely not running.
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess := map[string]any{
		"id":     "test-dead",
		"pid":    999999999, // extremely unlikely to be a real PID
		"status": "running",
	}
	data, _ := json.Marshal(sess)
	if err := os.WriteFile(filepath.Join(sessDir, "test-dead.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	orphans := SweepOrphans(dir, nil)
	if len(orphans) != 0 {
		t.Errorf("expected no orphans for dead PID, got %v", orphans)
	}
}

func TestSweepOrphans_ActivePIDSkipped(t *testing.T) {
	// Even if a PID were running, it should be skipped when in activePIDs.
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Use our own PID (which is definitely running).
	myPID := os.Getpid()
	sess := map[string]any{
		"id":     "test-active",
		"pid":    myPID,
		"status": "running",
	}
	data, _ := json.Marshal(sess)
	if err := os.WriteFile(filepath.Join(sessDir, "test-active.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	active := map[int]bool{myPID: true}
	orphans := SweepOrphans(dir, active)
	if len(orphans) != 0 {
		t.Errorf("expected active PID to be skipped, got %v", orphans)
	}
}

func TestSweepOrphans_DetectsOrphan(t *testing.T) {
	// Our own PID is running but not in activePIDs — should be detected.
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	myPID := os.Getpid()
	sess := map[string]any{
		"id":     "test-orphan",
		"pid":    myPID,
		"status": "running",
	}
	data, _ := json.Marshal(sess)
	if err := os.WriteFile(filepath.Join(sessDir, "test-orphan.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	orphans := SweepOrphans(dir, nil)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	if orphans[0].PID != myPID {
		t.Errorf("expected PID %d, got %d", myPID, orphans[0].PID)
	}
	if orphans[0].SessionFile != "test-orphan.json" {
		t.Errorf("expected session file test-orphan.json, got %s", orphans[0].SessionFile)
	}
}

func TestSweepOrphans_SkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a non-JSON file and a subdirectory — both should be skipped.
	if err := os.WriteFile(filepath.Join(sessDir, "notes.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sessDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	orphans := SweepOrphans(dir, nil)
	if len(orphans) != 0 {
		t.Errorf("expected no orphans, got %v", orphans)
	}
}

func TestSweepOrphans_NoPIDField(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Session file without a pid field.
	sess := map[string]any{
		"id":     "no-pid",
		"status": "completed",
	}
	data, _ := json.Marshal(sess)
	if err := os.WriteFile(filepath.Join(sessDir, "no-pid.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	orphans := SweepOrphans(dir, nil)
	if len(orphans) != 0 {
		t.Errorf("expected no orphans for session without PID, got %v", orphans)
	}
}

func TestOrphanInfo_Fields(t *testing.T) {
	o := OrphanInfo{PID: 42, SessionFile: "abc.json"}
	if o.PID != 42 {
		t.Errorf("expected PID 42, got %d", o.PID)
	}
	if o.SessionFile != "abc.json" {
		t.Errorf("expected SessionFile abc.json, got %s", o.SessionFile)
	}
}

func TestCleanupOrphans_NoOrphans(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	cleaned, errs := CleanupOrphans(dir, nil, false)
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned, got %d", cleaned)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestCleanupOrphans_DryRun(t *testing.T) {
	// Use our own PID (running, not in activePIDs) to create an orphan.
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	myPID := os.Getpid()
	sess := map[string]any{
		"id":     "dry-run-test",
		"pid":    myPID,
		"status": "running",
	}
	data, _ := json.Marshal(sess)
	if err := os.WriteFile(filepath.Join(sessDir, "dry-run-test.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// kill=false: should count orphan but not send any signal
	cleaned, errs := CleanupOrphans(dir, nil, false)
	if len(errs) != 0 {
		t.Errorf("expected no errors in dry run, got %v", errs)
	}
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}
}

func TestExtractPID(t *testing.T) {
	cases := []struct {
		name string
		json string
		want int
	}{
		{"valid", `{"pid": 1234}`, 1234},
		{"zero", `{"pid": 0}`, 0},
		{"missing", `{"id": "x"}`, 0},
		{"invalid json", `not json`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPID([]byte(tc.json))
			if got != tc.want {
				t.Errorf("extractPID(%s) = %d, want %d", tc.json, got, tc.want)
			}
		})
	}
}
