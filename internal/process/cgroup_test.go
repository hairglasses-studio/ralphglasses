package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// setupMockCgroup creates a mock cgroup v2 filesystem in a temp directory and
// configures the package to use it. It returns the root path and a cleanup
// function that restores defaults.
func setupMockCgroup(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Create cgroup.controllers at root to indicate cgroup v2 support.
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory io\n"), 0644); err != nil {
		t.Fatalf("setup: write cgroup.controllers: %v", err)
	}

	// Point the package at our mock filesystem.
	cgroupRootOverride = root
	supported := true
	cgroupSupportedOverride = &supported
	t.Cleanup(func() {
		cgroupRootOverride = ""
		cgroupSupportedOverride = nil
	})

	return root
}

// setupMockSession creates a mock cgroup directory for a session with the
// standard control files pre-populated with sensible defaults.
func setupMockSession(t *testing.T, root, sessionID string) string {
	t.Helper()
	cgPath := filepath.Join(root, cgroupBasePath, sessionID)
	if err := os.MkdirAll(cgPath, 0755); err != nil {
		t.Fatalf("setup: mkdir session cgroup: %v", err)
	}

	// Pre-populate control files with defaults.
	files := map[string]string{
		"memory.max":     "max",
		"memory.current": "4194304",
		"cpu.max":        "max 100000",
		"cpu.stat":       "usage_usec 500000\nuser_usec 300000\nsystem_usec 200000\n",
		"cgroup.procs":   "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(cgPath, name), []byte(content), 0644); err != nil {
			t.Fatalf("setup: write %s: %v", name, err)
		}
	}

	// Also create parent-level files for controller enablement.
	parentPath := filepath.Join(root, cgroupBasePath)
	parentFiles := map[string]string{
		"cgroup.controllers":    "cpu memory io",
		"cgroup.subtree_control": "",
	}
	for name, content := range parentFiles {
		if err := os.WriteFile(filepath.Join(parentPath, name), []byte(content), 0644); err != nil {
			t.Fatalf("setup: write parent %s: %v", name, err)
		}
	}

	return cgPath
}

func TestCgroupSupported_UnsupportedPlatform(t *testing.T) {
	// Force unsupported.
	supported := false
	old := cgroupSupportedOverride
	cgroupSupportedOverride = &supported
	t.Cleanup(func() { cgroupSupportedOverride = old })

	if CgroupSupported() {
		t.Error("expected CgroupSupported() = false when override is false")
	}
}

func TestCgroupSupported_SupportedPlatform(t *testing.T) {
	supported := true
	old := cgroupSupportedOverride
	cgroupSupportedOverride = &supported
	t.Cleanup(func() { cgroupSupportedOverride = old })

	if !CgroupSupported() {
		t.Error("expected CgroupSupported() = true when override is true")
	}
}

func TestCgroupCreate_Unsupported(t *testing.T) {
	supported := false
	old := cgroupSupportedOverride
	cgroupSupportedOverride = &supported
	t.Cleanup(func() { cgroupSupportedOverride = old })

	path, err := CgroupCreate("test-session", CgroupLimits{MemoryMax: 1 << 30})
	if err != nil {
		t.Fatalf("CgroupCreate on unsupported: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path on unsupported, got %q", path)
	}
}

func TestCgroupNoopOnUnsupported(t *testing.T) {
	supported := false
	old := cgroupSupportedOverride
	cgroupSupportedOverride = &supported
	t.Cleanup(func() { cgroupSupportedOverride = old })

	// All operations should succeed silently.
	if err := CgroupSetMemory("s", 1<<20); err != nil {
		t.Errorf("CgroupSetMemory: %v", err)
	}
	if err := CgroupSetCPU("s", 50000, 100000); err != nil {
		t.Errorf("CgroupSetCPU: %v", err)
	}
	if err := CgroupAddPID("s", 12345); err != nil {
		t.Errorf("CgroupAddPID: %v", err)
	}
	usage, err := CgroupUsageRead("s")
	if err != nil {
		t.Errorf("CgroupUsageRead: %v", err)
	}
	if usage.MemoryCurrent != 0 || usage.CPUUsage != 0 {
		t.Errorf("expected zero usage on unsupported, got %+v", usage)
	}
	if err := CgroupCleanup("s"); err != nil {
		t.Errorf("CgroupCleanup: %v", err)
	}
}

func TestCgroupCreate_WithMockFS(t *testing.T) {
	root := setupMockCgroup(t)

	limits := CgroupLimits{
		MemoryMax: 512 * 1024 * 1024, // 512 MiB
		CPUQuota:  200000,             // 2 CPUs
		CPUPeriod: 100000,
	}

	cgPath, err := CgroupCreate("session-1", limits)
	if err != nil {
		t.Fatalf("CgroupCreate: %v", err)
	}

	expectedPath := filepath.Join(root, cgroupBasePath, "session-1")
	if cgPath != expectedPath {
		t.Errorf("CgroupCreate path = %q, want %q", cgPath, expectedPath)
	}

	// Verify the directory was created.
	info, err := os.Stat(cgPath)
	if err != nil {
		t.Fatalf("session cgroup dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("session cgroup path is not a directory")
	}

	// Verify memory.max was written.
	memData, err := os.ReadFile(filepath.Join(cgPath, "memory.max"))
	if err != nil {
		t.Fatalf("read memory.max: %v", err)
	}
	wantMem := strconv.FormatInt(limits.MemoryMax, 10)
	if got := strings.TrimSpace(string(memData)); got != wantMem {
		t.Errorf("memory.max = %q, want %q", got, wantMem)
	}

	// Verify cpu.max was written.
	cpuData, err := os.ReadFile(filepath.Join(cgPath, "cpu.max"))
	if err != nil {
		t.Fatalf("read cpu.max: %v", err)
	}
	wantCPU := "200000 100000"
	if got := strings.TrimSpace(string(cpuData)); got != wantCPU {
		t.Errorf("cpu.max = %q, want %q", got, wantCPU)
	}
}

func TestCgroupCreate_NoLimits(t *testing.T) {
	root := setupMockCgroup(t)

	cgPath, err := CgroupCreate("no-limits", CgroupLimits{})
	if err != nil {
		t.Fatalf("CgroupCreate: %v", err)
	}

	expectedPath := filepath.Join(root, cgroupBasePath, "no-limits")
	if cgPath != expectedPath {
		t.Errorf("path = %q, want %q", cgPath, expectedPath)
	}

	// memory.max and cpu.max should not exist since no limits were set.
	if _, err := os.Stat(filepath.Join(cgPath, "memory.max")); err == nil {
		t.Error("memory.max should not exist when MemoryMax is 0")
	}
	if _, err := os.Stat(filepath.Join(cgPath, "cpu.max")); err == nil {
		t.Error("cpu.max should not exist when CPUQuota is 0")
	}
}

func TestCgroupCreate_DefaultPeriod(t *testing.T) {
	root := setupMockCgroup(t)
	_ = root

	limits := CgroupLimits{
		CPUQuota: 50000,
		// CPUPeriod left at 0 — should default to 100000.
	}

	cgPath, err := CgroupCreate("default-period", limits)
	if err != nil {
		t.Fatalf("CgroupCreate: %v", err)
	}

	cpuData, err := os.ReadFile(filepath.Join(cgPath, "cpu.max"))
	if err != nil {
		t.Fatalf("read cpu.max: %v", err)
	}
	if got := strings.TrimSpace(string(cpuData)); got != "50000 100000" {
		t.Errorf("cpu.max = %q, want %q", got, "50000 100000")
	}
}

func TestCgroupSetMemory_WithMockFS(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "mem-test")

	// Set 1 GiB limit.
	limit := int64(1 << 30)
	if err := CgroupSetMemory("mem-test", limit); err != nil {
		t.Fatalf("CgroupSetMemory: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, cgroupBasePath, "mem-test", "memory.max"))
	if err != nil {
		t.Fatalf("read memory.max: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != strconv.FormatInt(limit, 10) {
		t.Errorf("memory.max = %q, want %q", got, strconv.FormatInt(limit, 10))
	}
}

func TestCgroupSetMemory_Unlimited(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "mem-unlim")

	// Zero means "max" (no limit).
	if err := CgroupSetMemory("mem-unlim", 0); err != nil {
		t.Fatalf("CgroupSetMemory(0): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, cgroupBasePath, "mem-unlim", "memory.max"))
	if err != nil {
		t.Fatalf("read memory.max: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "max" {
		t.Errorf("memory.max = %q, want %q", got, "max")
	}
}

func TestCgroupSetCPU_WithMockFS(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "cpu-test")

	if err := CgroupSetCPU("cpu-test", 150000, 100000); err != nil {
		t.Fatalf("CgroupSetCPU: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, cgroupBasePath, "cpu-test", "cpu.max"))
	if err != nil {
		t.Fatalf("read cpu.max: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "150000 100000" {
		t.Errorf("cpu.max = %q, want %q", got, "150000 100000")
	}
}

func TestCgroupSetCPU_DefaultPeriod(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "cpu-defperiod")

	// Period of 0 should default to 100000.
	if err := CgroupSetCPU("cpu-defperiod", 50000, 0); err != nil {
		t.Fatalf("CgroupSetCPU: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, cgroupBasePath, "cpu-defperiod", "cpu.max"))
	if err != nil {
		t.Fatalf("read cpu.max: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "50000 100000" {
		t.Errorf("cpu.max = %q, want %q", got, "50000 100000")
	}
}

func TestCgroupUsageRead_WithMockFS(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "usage-test")

	usage, err := CgroupUsageRead("usage-test")
	if err != nil {
		t.Fatalf("CgroupUsageRead: %v", err)
	}

	// memory.current in mock is 4194304 (4 MiB).
	if usage.MemoryCurrent != 4194304 {
		t.Errorf("MemoryCurrent = %d, want 4194304", usage.MemoryCurrent)
	}

	// cpu.stat in mock has usage_usec 500000.
	if usage.CPUUsage != 500000 {
		t.Errorf("CPUUsage = %d, want 500000", usage.CPUUsage)
	}
}

func TestCgroupUsageRead_NoUsageUsec(t *testing.T) {
	root := setupMockCgroup(t)
	cgPath := setupMockSession(t, root, "no-cpu-usage")

	// Write cpu.stat without usage_usec.
	if err := os.WriteFile(filepath.Join(cgPath, "cpu.stat"), []byte("user_usec 100\nsystem_usec 200\n"), 0644); err != nil {
		t.Fatalf("write cpu.stat: %v", err)
	}

	usage, err := CgroupUsageRead("no-cpu-usage")
	if err != nil {
		t.Fatalf("CgroupUsageRead: %v", err)
	}

	if usage.CPUUsage != 0 {
		t.Errorf("CPUUsage = %d, want 0 when usage_usec is absent", usage.CPUUsage)
	}
}

func TestCgroupAddPID_WithMockFS(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "pid-test")

	if err := CgroupAddPID("pid-test", 42); err != nil {
		t.Fatalf("CgroupAddPID: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, cgroupBasePath, "pid-test", "cgroup.procs"))
	if err != nil {
		t.Fatalf("read cgroup.procs: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "42" {
		t.Errorf("cgroup.procs = %q, want %q", got, "42")
	}
}

func TestCgroupCleanup_WithMockFS(t *testing.T) {
	root := setupMockCgroup(t)
	sessionID := "cleanup-test"
	cgPath := filepath.Join(root, cgroupBasePath, sessionID)

	// Create a session directory (empty, no control files).
	if err := os.MkdirAll(cgPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := CgroupCleanup(sessionID); err != nil {
		t.Fatalf("CgroupCleanup: %v", err)
	}

	if _, err := os.Stat(cgPath); !os.IsNotExist(err) {
		t.Errorf("cgroup directory should be removed, got stat error: %v", err)
	}
}

func TestCgroupCleanup_AlreadyGone(t *testing.T) {
	_ = setupMockCgroup(t)

	// Cleaning up a session that doesn't exist should not error.
	if err := CgroupCleanup("nonexistent-session"); err != nil {
		t.Errorf("CgroupCleanup for nonexistent session: %v", err)
	}
}

func TestCgroupCleanup_NonEmptyDir(t *testing.T) {
	root := setupMockCgroup(t)
	setupMockSession(t, root, "nonempty")

	// Cleanup should fail because os.Remove only removes empty directories,
	// and the mock session has control files in it.
	err := CgroupCleanup("nonempty")
	if err == nil {
		t.Error("CgroupCleanup should fail on non-empty directory")
	}
}

func TestCgroupCreate_FullLifecycle(t *testing.T) {
	root := setupMockCgroup(t)
	_ = root

	sessionID := "lifecycle-test"
	limits := CgroupLimits{
		MemoryMax: 256 * 1024 * 1024,
		CPUQuota:  100000,
		CPUPeriod: 100000,
	}

	// Create.
	cgPath, err := CgroupCreate(sessionID, limits)
	if err != nil {
		t.Fatalf("CgroupCreate: %v", err)
	}

	// Pre-populate usage files for reading (CgroupCreate only writes limits).
	if err := os.WriteFile(filepath.Join(cgPath, "memory.current"), []byte("8388608\n"), 0644); err != nil {
		t.Fatalf("write memory.current: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cgPath, "cpu.stat"), []byte("usage_usec 1000000\n"), 0644); err != nil {
		t.Fatalf("write cpu.stat: %v", err)
	}

	// Update limits.
	if err := CgroupSetMemory(sessionID, 128*1024*1024); err != nil {
		t.Fatalf("CgroupSetMemory: %v", err)
	}
	if err := CgroupSetCPU(sessionID, 50000, 100000); err != nil {
		t.Fatalf("CgroupSetCPU: %v", err)
	}

	// Add a PID.
	if err := os.WriteFile(filepath.Join(cgPath, "cgroup.procs"), []byte(""), 0644); err != nil {
		t.Fatalf("write cgroup.procs: %v", err)
	}
	if err := CgroupAddPID(sessionID, 9999); err != nil {
		t.Fatalf("CgroupAddPID: %v", err)
	}

	// Read usage.
	usage, err := CgroupUsageRead(sessionID)
	if err != nil {
		t.Fatalf("CgroupUsageRead: %v", err)
	}
	if usage.MemoryCurrent != 8388608 {
		t.Errorf("MemoryCurrent = %d, want 8388608", usage.MemoryCurrent)
	}
	if usage.CPUUsage != 1000000 {
		t.Errorf("CPUUsage = %d, want 1000000", usage.CPUUsage)
	}

	// Remove control files so cleanup can remove the directory.
	entries, _ := os.ReadDir(cgPath)
	for _, e := range entries {
		os.Remove(filepath.Join(cgPath, e.Name()))
	}

	// Cleanup.
	if err := CgroupCleanup(sessionID); err != nil {
		t.Fatalf("CgroupCleanup: %v", err)
	}

	if _, err := os.Stat(cgPath); !os.IsNotExist(err) {
		t.Error("cgroup directory should be removed after cleanup")
	}
}

func TestSessionCgroupPath(t *testing.T) {
	old := cgroupRootOverride
	cgroupRootOverride = "/tmp/fake-cgroup"
	t.Cleanup(func() { cgroupRootOverride = old })

	got := sessionCgroupPath("my-session")
	want := "/tmp/fake-cgroup/ralphglasses.sessions/my-session"
	if got != want {
		t.Errorf("sessionCgroupPath = %q, want %q", got, want)
	}
}

func TestCgroupReadFileInt_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-val")
	if err := os.WriteFile(path, []byte("12345\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	val, err := cgroupReadFileInt(path)
	if err != nil {
		t.Fatalf("cgroupReadFileInt: %v", err)
	}
	if val != 12345 {
		t.Errorf("val = %d, want 12345", val)
	}
}

func TestCgroupReadFileInt_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-val")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := cgroupReadFileInt(path)
	if err == nil {
		t.Error("expected error for non-numeric content")
	}
}

func TestCgroupReadFileInt_Missing(t *testing.T) {
	_, err := cgroupReadFileInt("/nonexistent/file")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCgroupRoot_Default(t *testing.T) {
	old := cgroupRootOverride
	cgroupRootOverride = ""
	t.Cleanup(func() { cgroupRootOverride = old })

	if got := cgroupRoot(); got != defaultCgroupRoot {
		t.Errorf("cgroupRoot() = %q, want %q", got, defaultCgroupRoot)
	}
}

func TestCgroupRoot_Override(t *testing.T) {
	old := cgroupRootOverride
	cgroupRootOverride = "/tmp/test-cgroup-root"
	t.Cleanup(func() { cgroupRootOverride = old })

	if got := cgroupRoot(); got != "/tmp/test-cgroup-root" {
		t.Errorf("cgroupRoot() = %q, want %q", got, "/tmp/test-cgroup-root")
	}
}

func TestCgroupLimits_Struct(t *testing.T) {
	limits := CgroupLimits{
		MemoryMax: 1 << 30,
		CPUQuota:  200000,
		CPUPeriod: 100000,
	}

	if limits.MemoryMax != 1073741824 {
		t.Errorf("MemoryMax = %d, want 1073741824", limits.MemoryMax)
	}
	if limits.CPUQuota != 200000 {
		t.Errorf("CPUQuota = %d, want 200000", limits.CPUQuota)
	}
	if limits.CPUPeriod != 100000 {
		t.Errorf("CPUPeriod = %d, want 100000", limits.CPUPeriod)
	}
}

func TestCgroupUsage_Struct(t *testing.T) {
	usage := CgroupUsage{
		MemoryCurrent: 4194304,
		CPUUsage:      500000,
	}

	if usage.MemoryCurrent != 4194304 {
		t.Errorf("MemoryCurrent = %d, want 4194304", usage.MemoryCurrent)
	}
	if usage.CPUUsage != 500000 {
		t.Errorf("CPUUsage = %d, want 500000", usage.CPUUsage)
	}
}
