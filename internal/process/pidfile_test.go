package process

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// PIDInfo / WritePIDFile / ReadPIDFile / RemovePIDFile / ListPIDFiles
// ---------------------------------------------------------------------------

func TestWriteAndReadPIDFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	info := PIDInfo{
		PID:       12345,
		StartTime: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		RepoPath:  "/test/repo",
		Provider:  "claude",
	}

	if err := WritePIDFile(dir, info); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	path := filepath.Join(dir, "12345.json")
	got, err := ReadPIDFile(path)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}

	if got.PID != info.PID {
		t.Errorf("PID: got %d, want %d", got.PID, info.PID)
	}
	if !got.StartTime.Equal(info.StartTime) {
		t.Errorf("StartTime: got %v, want %v", got.StartTime, info.StartTime)
	}
	if got.RepoPath != info.RepoPath {
		t.Errorf("RepoPath: got %q, want %q", got.RepoPath, info.RepoPath)
	}
	if got.Provider != info.Provider {
		t.Errorf("Provider: got %q, want %q", got.Provider, info.Provider)
	}
}

func TestWritePIDFile_CreatesDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "nested", "sessions")
	info := PIDInfo{PID: 99, RepoPath: "/test"}

	if err := WritePIDFile(dir, info); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "99.json")); err != nil {
		t.Errorf("expected PID file to exist: %v", err)
	}
}

func TestReadPIDFile_NotFound(t *testing.T) {
	t.Parallel()

	_, err := ReadPIDFile("/nonexistent/path/123.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadPIDFile_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)

	_, err := ReadPIDFile(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadPIDFile_JSON_ZeroPID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "0.json")
	data, _ := json.Marshal(PIDInfo{PID: 0, RepoPath: "/test"})
	_ = os.WriteFile(path, data, 0644)

	_, err := ReadPIDFile(path)
	if err == nil {
		t.Fatal("expected error for zero PID")
	}
}

func TestReadPIDFile_NegativePID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "neg.json")
	data, _ := json.Marshal(PIDInfo{PID: -1, RepoPath: "/test"})
	_ = os.WriteFile(path, data, 0644)

	_, err := ReadPIDFile(path)
	if err == nil {
		t.Fatal("expected error for negative PID")
	}
}

func TestRemovePIDFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	info := PIDInfo{PID: 555, RepoPath: "/test"}
	if err := WritePIDFile(dir, info); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	if err := RemovePIDFile(dir, 555); err != nil {
		t.Fatalf("RemovePIDFile: %v", err)
	}

	path := filepath.Join(dir, "555.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed")
	}
}

func TestRemovePIDFile_NotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Should not error for missing file.
	if err := RemovePIDFile(dir, 99999); err != nil {
		t.Fatalf("RemovePIDFile for missing file should not error: %v", err)
	}
}

func TestListPIDFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Write several PID files.
	for i := 1; i <= 3; i++ {
		info := PIDInfo{
			PID:       i * 100,
			StartTime: time.Now(),
			RepoPath:  fmt.Sprintf("/repo/%d", i),
			Provider:  "claude",
		}
		if err := WritePIDFile(dir, info); err != nil {
			t.Fatalf("WritePIDFile %d: %v", i, err)
		}
	}

	// Write a non-JSON file that should be skipped.
	_ = os.WriteFile(filepath.Join(dir, "README.txt"), []byte("ignore me"), 0644)

	// Write an invalid JSON file that should be skipped.
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0644)

	infos, err := ListPIDFiles(dir)
	if err != nil {
		t.Fatalf("ListPIDFiles: %v", err)
	}

	if len(infos) != 3 {
		t.Errorf("expected 3 PID files, got %d", len(infos))
	}
}

func TestListPIDFiles_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	infos, err := ListPIDFiles(dir)
	if err != nil {
		t.Fatalf("ListPIDFiles: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0, got %d", len(infos))
	}
}

func TestListPIDFiles_NonexistentDir(t *testing.T) {
	t.Parallel()

	infos, err := ListPIDFiles("/nonexistent/dir/sessions")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if infos != nil {
		t.Errorf("expected nil result, got %v", infos)
	}
}

func TestWritePIDFile_ProviderOptional(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	info := PIDInfo{PID: 42, RepoPath: "/test"}

	if err := WritePIDFile(dir, info); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	got, err := ReadPIDFile(filepath.Join(dir, "42.json"))
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if got.Provider != "" {
		t.Errorf("expected empty provider, got %q", got.Provider)
	}
}

// ---------------------------------------------------------------------------
// ScanOrphans / CleanupOrphans
// ---------------------------------------------------------------------------

func TestScanOrphans_AllAlive(t *testing.T) {
	// Stub aliveFn to report all alive.
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)
	setAliveFn(func(pid int) bool { return true })

	dir := t.TempDir()
	for _, pid := range []int{100, 200, 300} {
		_ = WritePIDFile(dir, PIDInfo{PID: pid, RepoPath: "/test"})
	}

	orphans, err := ScanOrphans(dir)
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphans, got %d", len(orphans))
	}
}

func TestScanOrphans_SomeDead(t *testing.T) {
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)

	alive := map[int]bool{100: true, 200: false, 300: true}
	setAliveFn(func(pid int) bool { return alive[pid] })

	dir := t.TempDir()
	for _, pid := range []int{100, 200, 300} {
		_ = WritePIDFile(dir, PIDInfo{PID: pid, RepoPath: "/test"})
	}

	orphans, err := ScanOrphans(dir)
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Errorf("expected 1 orphan, got %d", len(orphans))
	}
	if len(orphans) > 0 && orphans[0].PID != 200 {
		t.Errorf("expected orphan PID 200, got %d", orphans[0].PID)
	}
}

func TestScanOrphans_AllDead(t *testing.T) {
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)
	setAliveFn(func(pid int) bool { return false })

	dir := t.TempDir()
	for _, pid := range []int{100, 200} {
		_ = WritePIDFile(dir, PIDInfo{PID: pid, RepoPath: "/test"})
	}

	orphans, err := ScanOrphans(dir)
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 2 {
		t.Errorf("expected 2 orphans, got %d", len(orphans))
	}
}

func TestScanOrphans_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	orphans, err := ScanOrphans(dir)
	if err != nil {
		t.Fatalf("ScanOrphans: %v", err)
	}
	if len(orphans) != 0 {
		t.Errorf("expected 0 orphans, got %d", len(orphans))
	}
}

func TestCleanupOrphans(t *testing.T) {
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)

	alive := map[int]bool{100: true, 200: false, 300: false}
	setAliveFn(func(pid int) bool { return alive[pid] })

	dir := t.TempDir()
	for _, pid := range []int{100, 200, 300} {
		_ = WritePIDFile(dir, PIDInfo{PID: pid, RepoPath: "/test"})
	}

	cleaned, err := CleanupOrphans(dir)
	if err != nil {
		t.Fatalf("CleanupOrphans: %v", err)
	}
	if cleaned != 2 {
		t.Errorf("expected 2 cleaned, got %d", cleaned)
	}

	// Verify dead PID files are removed but live one remains.
	remaining, _ := ListPIDFiles(dir)
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining PID file, got %d", len(remaining))
	}
	if len(remaining) > 0 && remaining[0].PID != 100 {
		t.Errorf("expected remaining PID 100, got %d", remaining[0].PID)
	}
}

func TestCleanupOrphans_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cleaned, err := CleanupOrphans(dir)
	if err != nil {
		t.Fatalf("CleanupOrphans: %v", err)
	}
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned, got %d", cleaned)
	}
}

func TestCleanupOrphans_NonexistentDir(t *testing.T) {
	t.Parallel()

	cleaned, err := CleanupOrphans("/nonexistent/sessions")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned, got %d", cleaned)
	}
}

// ---------------------------------------------------------------------------
// RestartPolicy
// ---------------------------------------------------------------------------

func TestDefaultRestartPolicy(t *testing.T) {
	t.Parallel()

	rp := DefaultRestartPolicy()
	if rp.MaxRestarts != 5 {
		t.Errorf("MaxRestarts: got %d, want 5", rp.MaxRestarts)
	}
	if rp.BackoffSecs != 30 {
		t.Errorf("BackoffSecs: got %d, want 30", rp.BackoffSecs)
	}
	if rp.CooldownSecs != 300 {
		t.Errorf("CooldownSecs: got %d, want 300", rp.CooldownSecs)
	}
}

func TestRestartPolicy_ShouldRestart(t *testing.T) {
	t.Parallel()

	rp := RestartPolicy{MaxRestarts: 3}

	tests := []struct {
		count int
		want  bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false},
		{4, false},
	}

	for _, tc := range tests {
		if got := rp.ShouldRestart(tc.count); got != tc.want {
			t.Errorf("ShouldRestart(%d): got %v, want %v", tc.count, got, tc.want)
		}
	}
}

func TestRestartPolicy_BackoffDuration(t *testing.T) {
	t.Parallel()

	rp := RestartPolicy{
		MaxRestarts:  5,
		BackoffSecs:  2,
		CooldownSecs: 20,
	}

	tests := []struct {
		count int
		want  time.Duration
	}{
		{0, 2 * time.Second},  // 2 * 2^0 = 2s
		{1, 4 * time.Second},  // 2 * 2^1 = 4s
		{2, 8 * time.Second},  // 2 * 2^2 = 8s
		{3, 16 * time.Second}, // 2 * 2^3 = 16s
		{4, 20 * time.Second}, // capped at cooldown
	}

	for _, tc := range tests {
		got := rp.BackoffDuration(tc.count)
		if got != tc.want {
			t.Errorf("BackoffDuration(%d): got %v, want %v", tc.count, got, tc.want)
		}
	}
}

func TestRestartPolicy_BackoffDuration_ZeroBackoff(t *testing.T) {
	t.Parallel()

	rp := RestartPolicy{BackoffSecs: 0, CooldownSecs: 10}

	// Should default to 1s base.
	got := rp.BackoffDuration(0)
	if got != 1*time.Second {
		t.Errorf("expected 1s, got %v", got)
	}
}

func TestRestartPolicy_BackoffDuration_NoCooldownCap(t *testing.T) {
	t.Parallel()

	rp := RestartPolicy{BackoffSecs: 1, CooldownSecs: 0}

	// With no cooldown cap, backoff grows without limit.
	got := rp.BackoffDuration(10)
	if got != 1024*time.Second {
		t.Errorf("expected 1024s, got %v", got)
	}
}

func TestRestartPolicy_CooldownDuration(t *testing.T) {
	t.Parallel()

	rp := RestartPolicy{CooldownSecs: 300}
	if got := rp.CooldownDuration(); got != 300*time.Second {
		t.Errorf("expected 300s, got %v", got)
	}
}

func TestRestartPolicy_JSON(t *testing.T) {
	t.Parallel()

	rp := DefaultRestartPolicy()
	data, err := json.Marshal(rp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got RestartPolicy
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got != rp {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, rp)
	}
}

// ---------------------------------------------------------------------------
// StartHealthCheck / StartHealthCheckWithStats
// ---------------------------------------------------------------------------

func TestStartHealthCheck_StopsCleanly(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	check := func() error {
		count.Add(1)
		return nil
	}

	stop := StartHealthCheck(10*time.Millisecond, check, nil)
	time.Sleep(50 * time.Millisecond)
	stop()

	got := count.Load()
	if got < 1 {
		t.Errorf("expected at least 1 check, got %d", got)
	}
}

func TestStartHealthCheck_DefaultInterval(t *testing.T) {
	t.Parallel()

	// With interval=0, should use default 5s. We just verify it starts and stops.
	stop := StartHealthCheck(0, func() error { return nil }, nil)
	stop()
}

func TestStartHealthCheckWithStats_SuccessfulChecks(t *testing.T) {
	t.Parallel()

	stop, stats := StartHealthCheckWithStats(10*time.Millisecond, func() error {
		return nil
	}, nil)
	defer stop()

	time.Sleep(50 * time.Millisecond)
	s := stats()

	if s.TotalChecks < 1 {
		t.Errorf("expected at least 1 check, got %d", s.TotalChecks)
	}
	if s.TotalFailures != 0 {
		t.Errorf("expected 0 failures, got %d", s.TotalFailures)
	}
	if s.ConsecutiveFails != 0 {
		t.Errorf("expected 0 consecutive fails, got %d", s.ConsecutiveFails)
	}
}

func TestStartHealthCheckWithStats_FailuresTriggersCallback(t *testing.T) {
	t.Parallel()

	var failureCalled atomic.Int32
	checkErr := errors.New("unhealthy")

	stop, stats := StartHealthCheckWithStats(10*time.Millisecond, func() error {
		return checkErr
	}, func() {
		failureCalled.Add(1)
	})
	defer stop()

	// Wait for at least 3 failures to trigger the callback.
	time.Sleep(80 * time.Millisecond)

	s := stats()
	if s.TotalFailures < 3 {
		t.Errorf("expected at least 3 failures, got %d", s.TotalFailures)
	}

	if failureCalled.Load() < 1 {
		t.Error("expected onFailure to be called after 3 consecutive failures")
	}
}

func TestStartHealthCheckWithStats_ConsecutiveResetOnSuccess(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	check := func() error {
		n := callCount.Add(1)
		// Fail for first 2 calls, then succeed.
		if n <= 2 {
			return errors.New("fail")
		}
		return nil
	}

	stop, stats := StartHealthCheckWithStats(10*time.Millisecond, check, nil)
	defer stop()

	time.Sleep(80 * time.Millisecond)

	s := stats()
	// After successes, consecutive fails should be 0.
	if s.ConsecutiveFails != 0 {
		t.Errorf("expected consecutive fails to be reset to 0, got %d", s.ConsecutiveFails)
	}
	if s.TotalFailures != 2 {
		t.Errorf("expected 2 total failures, got %d", s.TotalFailures)
	}
}

func TestStartHealthCheckWithStats_NoCallbackBeforeThreshold(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	var failureCalled atomic.Int32

	// Fail twice then succeed forever — should never trigger callback.
	check := func() error {
		n := callCount.Add(1)
		if n <= 2 {
			return errors.New("fail")
		}
		return nil
	}

	stop, _ := StartHealthCheckWithStats(10*time.Millisecond, check, func() {
		failureCalled.Add(1)
	})
	defer stop()

	time.Sleep(80 * time.Millisecond)

	if failureCalled.Load() != 0 {
		t.Error("onFailure should not be called with fewer than 3 consecutive failures")
	}
}

func TestStartHealthCheckWithStats_NilCallback(t *testing.T) {
	t.Parallel()

	// Ensure no panic when onFailure is nil.
	stop, stats := StartHealthCheckWithStats(10*time.Millisecond, func() error {
		return errors.New("fail")
	}, nil)
	defer stop()

	time.Sleep(60 * time.Millisecond)

	s := stats()
	if s.TotalFailures < 3 {
		t.Errorf("expected at least 3 failures, got %d", s.TotalFailures)
	}
}

// ---------------------------------------------------------------------------
// Session-level convenience functions
// ---------------------------------------------------------------------------

func TestWriteAndListSessionPIDFiles(t *testing.T) {
	repoPath := t.TempDir()

	if err := WriteSessionPIDFile(repoPath, 1000, "claude"); err != nil {
		t.Fatalf("WriteSessionPIDFile: %v", err)
	}
	if err := WriteSessionPIDFile(repoPath, 2000, "gemini"); err != nil {
		t.Fatalf("WriteSessionPIDFile: %v", err)
	}

	infos, err := ListSessionPIDFiles(repoPath)
	if err != nil {
		t.Fatalf("ListSessionPIDFiles: %v", err)
	}
	if len(infos) != 2 {
		t.Errorf("expected 2, got %d", len(infos))
	}
}

func TestRemoveSessionPIDFile(t *testing.T) {
	repoPath := t.TempDir()

	if err := WriteSessionPIDFile(repoPath, 1234, "claude"); err != nil {
		t.Fatalf("WriteSessionPIDFile: %v", err)
	}

	if err := RemoveSessionPIDFile(repoPath, 1234); err != nil {
		t.Fatalf("RemoveSessionPIDFile: %v", err)
	}

	infos, _ := ListSessionPIDFiles(repoPath)
	if len(infos) != 0 {
		t.Errorf("expected 0 after remove, got %d", len(infos))
	}
}

func TestScanSessionOrphans(t *testing.T) {
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)

	alive := map[int]bool{1000: true, 2000: false}
	setAliveFn(func(pid int) bool { return alive[pid] })

	repoPath := t.TempDir()
	_ = WriteSessionPIDFile(repoPath, 1000, "claude")
	_ = WriteSessionPIDFile(repoPath, 2000, "gemini")

	orphans, err := ScanSessionOrphans(repoPath)
	if err != nil {
		t.Fatalf("ScanSessionOrphans: %v", err)
	}
	if len(orphans) != 1 {
		t.Errorf("expected 1 orphan, got %d", len(orphans))
	}
}

func TestCleanupSessionOrphans(t *testing.T) {
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)
	setAliveFn(func(pid int) bool { return false })

	repoPath := t.TempDir()
	_ = WriteSessionPIDFile(repoPath, 1000, "claude")
	_ = WriteSessionPIDFile(repoPath, 2000, "gemini")

	cleaned, err := CleanupSessionOrphans(repoPath)
	if err != nil {
		t.Fatalf("CleanupSessionOrphans: %v", err)
	}
	if cleaned != 2 {
		t.Errorf("expected 2 cleaned, got %d", cleaned)
	}
}

// ---------------------------------------------------------------------------
// Migration helpers
// ---------------------------------------------------------------------------

func TestPidInfoFromLegacy(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)
	_ = os.WriteFile(filepath.Join(ralphDir, pidFileName), []byte("42\n"), 0644)

	info := pidInfoFromLegacy(repoPath)
	if info == nil {
		t.Fatal("expected non-nil PIDInfo")
	}
	if info.PID != 42 {
		t.Errorf("expected PID 42, got %d", info.PID)
	}
	if info.RepoPath != repoPath {
		t.Errorf("expected RepoPath %q, got %q", repoPath, info.RepoPath)
	}
}

func TestPidInfoFromLegacy_NoPIDFile(t *testing.T) {
	t.Parallel()

	info := pidInfoFromLegacy(t.TempDir())
	if info != nil {
		t.Errorf("expected nil, got %+v", info)
	}
}

func TestFormatPIDFileName(t *testing.T) {
	t.Parallel()

	if got := formatPIDFileName(12345); got != "12345.json" {
		t.Errorf("expected 12345.json, got %q", got)
	}
}
