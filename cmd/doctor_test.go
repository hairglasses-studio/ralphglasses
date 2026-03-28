package cmd

import (
	"testing"
)

func TestDoctorCmd_ShortDescription(t *testing.T) {
	if doctorCmd.Short == "" {
		t.Error("doctor command missing Short description")
	}
}

func TestDoctorCmd_LongDescription(t *testing.T) {
	if doctorCmd.Long == "" {
		t.Error("doctor command missing Long description")
	}
}

func TestDoctorCmd_Example(t *testing.T) {
	if doctorCmd.Example == "" {
		t.Error("doctor command missing Example")
	}
}

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		input       string
		wantMajor   int
		wantMinor   int
		wantOK      bool
	}{
		{"1.22.1", 1, 22, true},
		{"1.21", 1, 21, true},
		{"1.26.1", 1, 26, true},
		{"2.0.0", 2, 0, true},
		{"bad", 0, 0, false},
		{"1", 0, 0, false},
		{"a.b", 0, 0, false},
	}
	for _, tt := range tests {
		major, minor, ok := parseGoVersion(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseGoVersion(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok && (major != tt.wantMajor || minor != tt.wantMinor) {
			t.Errorf("parseGoVersion(%q) = (%d, %d), want (%d, %d)",
				tt.input, major, minor, tt.wantMajor, tt.wantMinor)
		}
	}
}

func TestCheckGoVersion(t *testing.T) {
	status, msg := checkGoVersion()
	if status != "OK" && status != "WARN" {
		t.Errorf("checkGoVersion: unexpected status %q", status)
	}
	if msg == "" {
		t.Error("checkGoVersion: empty message")
	}
}

func TestCheckGitVersion(t *testing.T) {
	status, msg := checkGitVersion()
	if status != "OK" && status != "WARN" {
		t.Errorf("checkGitVersion: unexpected status %q", status)
	}
	if msg == "" {
		t.Error("checkGitVersion: empty message")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	status, msg := checkDiskSpace(t.TempDir())
	if status != "OK" && status != "WARN" {
		t.Errorf("checkDiskSpace: unexpected status %q", status)
	}
	if msg == "" {
		t.Error("checkDiskSpace: empty message")
	}
}

func TestCheckDiskSpace_LowSpace(t *testing.T) {
	// Override diskFreeBytes to simulate low disk space.
	orig := diskFreeBytes
	diskFreeBytes = func(_ string) (uint64, error) {
		return 2 * 1024 * 1024 * 1024, nil // 2 GB
	}
	defer func() { diskFreeBytes = orig }()

	status, msg := checkDiskSpace("/tmp")
	if status != "WARN" {
		t.Errorf("checkDiskSpace with 2GB: status = %q, want WARN", status)
	}
	if msg == "" {
		t.Error("checkDiskSpace with 2GB: empty message")
	}
}

func TestCheckDiskSpace_HighSpace(t *testing.T) {
	orig := diskFreeBytes
	diskFreeBytes = func(_ string) (uint64, error) {
		return 100 * 1024 * 1024 * 1024, nil // 100 GB
	}
	defer func() { diskFreeBytes = orig }()

	status, msg := checkDiskSpace("/tmp")
	if status != "OK" {
		t.Errorf("checkDiskSpace with 100GB: status = %q, want OK", status)
	}
	if msg == "" {
		t.Error("checkDiskSpace with 100GB: empty message")
	}
}
