package cmd

import (
	"encoding/json"
	"testing"
)

// ---------------------------------------------------------------------------
// doctorResult and report tests
// ---------------------------------------------------------------------------

func TestBuildDoctorReport_AllPass(t *testing.T) {
	results := []doctorResult{
		{Name: "a", Status: statusPass, Message: "ok"},
		{Name: "b", Status: statusPass, Message: "ok"},
	}
	report := buildDoctorReport(results)
	if report.Summary.Pass != 2 {
		t.Errorf("pass = %d, want 2", report.Summary.Pass)
	}
	if report.Summary.Warn != 0 || report.Summary.Fail != 0 {
		t.Errorf("unexpected warn=%d fail=%d", report.Summary.Warn, report.Summary.Fail)
	}
	if !report.Summary.OK {
		t.Error("OK should be true when no failures")
	}
}

func TestBuildDoctorReport_WithFailure(t *testing.T) {
	results := []doctorResult{
		{Name: "a", Status: statusPass, Message: "ok"},
		{Name: "b", Status: statusFail, Message: "missing"},
	}
	report := buildDoctorReport(results)
	if report.Summary.Fail != 1 {
		t.Errorf("fail = %d, want 1", report.Summary.Fail)
	}
	if report.Summary.OK {
		t.Error("OK should be false when there are failures")
	}
}

func TestBuildDoctorReport_WarningsDoNotFail(t *testing.T) {
	results := []doctorResult{
		{Name: "a", Status: statusPass, Message: "ok"},
		{Name: "b", Status: statusWarn, Message: "optional"},
	}
	report := buildDoctorReport(results)
	if report.Summary.Warn != 1 {
		t.Errorf("warn = %d, want 1", report.Summary.Warn)
	}
	if !report.Summary.OK {
		t.Error("OK should be true when only warnings (no failures)")
	}
}

// ---------------------------------------------------------------------------
// JSON output format test
// ---------------------------------------------------------------------------

func TestDoctorReport_JSONRoundTrip(t *testing.T) {
	results := []doctorResult{
		{Name: "claude", Status: statusPass, Message: "/usr/local/bin/claude"},
		{Name: "gemini", Status: statusWarn, Message: "gemini not found in PATH (optional)"},
		{Name: "git", Status: statusFail, Message: "git not found in PATH"},
	}
	report := buildDoctorReport(results)

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded doctorReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Results) != 3 {
		t.Fatalf("results len = %d, want 3", len(decoded.Results))
	}
	if decoded.Results[0].Status != statusPass {
		t.Errorf("result[0] status = %q, want %q", decoded.Results[0].Status, statusPass)
	}
	if decoded.Results[1].Status != statusWarn {
		t.Errorf("result[1] status = %q, want %q", decoded.Results[1].Status, statusWarn)
	}
	if decoded.Results[2].Status != statusFail {
		t.Errorf("result[2] status = %q, want %q", decoded.Results[2].Status, statusFail)
	}
	if decoded.Summary.Pass != 1 || decoded.Summary.Warn != 1 || decoded.Summary.Fail != 1 {
		t.Errorf("summary = pass=%d warn=%d fail=%d, want 1/1/1",
			decoded.Summary.Pass, decoded.Summary.Warn, decoded.Summary.Fail)
	}
	if decoded.Summary.OK {
		t.Error("summary.ok should be false with a failure")
	}
}

// ---------------------------------------------------------------------------
// redactKey tests
// ---------------------------------------------------------------------------

func TestRedactKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"sk-ant-1234567890abcdef", "sk-a***************cdef"},
		{"short", "****"},
		{"12345678", "****"},
		{"123456789", "1234*6789"},
	}
	for _, tt := range tests {
		got := redactKey(tt.input)
		if got != tt.want {
			t.Errorf("redactKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseGitVersion tests
// ---------------------------------------------------------------------------

func TestParseGitVersion(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"git version 2.43.0", 2, 43, true},
		{"git version 2.20.1", 2, 20, true},
		{"git version 2.39.5 (Apple Git-154)", 2, 39, true},
		{"git version 1.8.3", 1, 8, true},
		{"bad", 0, 0, false},
		{"git version", 0, 0, false},
	}
	for _, tt := range tests {
		major, minor, ok := parseGitVersion(tt.input)
		if ok != tt.wantOK {
			t.Errorf("parseGitVersion(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok && (major != tt.wantMajor || minor != tt.wantMinor) {
			t.Errorf("parseGitVersion(%q) = (%d, %d), want (%d, %d)",
				tt.input, major, minor, tt.wantMajor, tt.wantMinor)
		}
	}
}

// ---------------------------------------------------------------------------
// parseGoVersion tests (preserved from original)
// ---------------------------------------------------------------------------

func TestParseGoVersion(t *testing.T) {
	tests := []struct {
		input     string
		wantMajor int
		wantMinor int
		wantOK    bool
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

// ---------------------------------------------------------------------------
// checkGit version comparison
// ---------------------------------------------------------------------------

func TestCheckGit_VersionBoundary(t *testing.T) {
	// Just test the parse + version logic, not exec.LookPath.
	cases := []struct {
		rawVer string
		expect doctorStatus
	}{
		{"git version 2.20.0", statusPass},
		{"git version 2.43.0", statusPass},
		{"git version 2.19.9", statusFail},
		{"git version 1.9.0", statusFail},
	}
	for _, c := range cases {
		major, minor, ok := parseGitVersion(c.rawVer)
		if !ok {
			t.Errorf("parseGitVersion(%q) failed", c.rawVer)
			continue
		}
		var got doctorStatus
		if major < 2 || (major == 2 && minor < 20) {
			got = statusFail
		} else {
			got = statusPass
		}
		if got != c.expect {
			t.Errorf("git version %q: status = %q, want %q", c.rawVer, got, c.expect)
		}
	}
}

// ---------------------------------------------------------------------------
// checkDiskSpaceCheck with mocked diskFreeBytes
// ---------------------------------------------------------------------------

func TestCheckDiskSpaceCheck_LowSpace(t *testing.T) {
	orig := diskFreeBytes
	diskFreeBytes = func(_ string) (uint64, error) {
		return 500 * 1024 * 1024, nil // 500 MB
	}
	defer func() { diskFreeBytes = orig }()

	r := checkDiskSpaceCheck("/tmp")
	if r.Status != statusWarn {
		t.Errorf("status = %q, want %q", r.Status, statusWarn)
	}
}

func TestCheckDiskSpaceCheck_OK(t *testing.T) {
	orig := diskFreeBytes
	diskFreeBytes = func(_ string) (uint64, error) {
		return 50 * 1024 * 1024 * 1024, nil // 50 GB
	}
	defer func() { diskFreeBytes = orig }()

	r := checkDiskSpaceCheck("/tmp")
	if r.Status != statusPass {
		t.Errorf("status = %q, want %q", r.Status, statusPass)
	}
}

// ---------------------------------------------------------------------------
// checkAPIKeys with mocked environment
// ---------------------------------------------------------------------------

func TestCheckAPIKeys_AllSet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test1234567890xx")
	t.Setenv("GEMINI_API_KEY", "AIzaSyTest1234567890xx")
	t.Setenv("OPENAI_API_KEY", "sk-test1234567890abcdef")

	results := checkAPIKeys()
	if len(results) != 3 {
		t.Fatalf("len = %d, want 3", len(results))
	}
	for _, r := range results {
		if r.Status != statusPass {
			t.Errorf("%s: status = %q, want pass", r.Name, r.Status)
		}
		// Verify keys are redacted (should not contain the full key).
		if r.Message == "sk-ant-test1234567890xx" || r.Message == "AIzaSyTest1234567890xx" || r.Message == "sk-test1234567890abcdef" {
			t.Errorf("%s: key not redacted in message", r.Name)
		}
	}
}

func TestCheckAPIKeys_NoneSet(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	results := checkAPIKeys()
	for _, r := range results {
		if r.Status != statusWarn {
			t.Errorf("%s: status = %q, want warn", r.Name, r.Status)
		}
	}
}

// ---------------------------------------------------------------------------
// checkStateDir with temp directory
// ---------------------------------------------------------------------------

func TestCheckStateDir_WritableDir(t *testing.T) {
	// checkStateDir uses os.UserHomeDir; we cannot easily override that,
	// but we can test the function runs without panicking and returns
	// a valid status.
	r := checkStateDir()
	if r.Status != statusPass && r.Status != statusWarn {
		t.Errorf("status = %q, want pass or warn", r.Status)
	}
	if r.Message == "" {
		t.Error("empty message")
	}
}

// ---------------------------------------------------------------------------
// Doctor command metadata
// ---------------------------------------------------------------------------

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

func TestDoctorCmd_HasJSONFlag(t *testing.T) {
	f := doctorCmd.Flags().Lookup("json")
	if f == nil {
		t.Fatal("doctor command missing --json flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--json default = %q, want %q", f.DefValue, "false")
	}
}
