package cmd

import (
	"testing"
	"time"
)

func TestScanTimeoutDuration_DefaultValue(t *testing.T) {
	old := scanTimeout
	defer func() { scanTimeout = old }()

	scanTimeout = "30s"
	d := ScanTimeoutDuration()
	if d != 30*time.Second {
		t.Errorf("ScanTimeoutDuration with default = %v, want 30s", d)
	}
}

func TestScanTimeoutDuration_CustomValues(t *testing.T) {
	old := scanTimeout
	defer func() { scanTimeout = old }()

	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"10s", 10 * time.Second},
		{"1m", 1 * time.Minute},
		{"500ms", 500 * time.Millisecond},
		{"2m30s", 2*time.Minute + 30*time.Second},
	}

	for _, tt := range tests {
		scanTimeout = tt.input
		d := ScanTimeoutDuration()
		if d != tt.expected {
			t.Errorf("ScanTimeoutDuration(%q) = %v, want %v", tt.input, d, tt.expected)
		}
	}
}

func TestScanTimeoutDuration_InvalidFallback(t *testing.T) {
	old := scanTimeout
	defer func() { scanTimeout = old }()

	scanTimeout = "not-a-duration"
	d := ScanTimeoutDuration()
	if d != 30*time.Second {
		t.Errorf("ScanTimeoutDuration with invalid input = %v, want 30s fallback", d)
	}
}

func TestScanTimeoutDuration_EmptyString(t *testing.T) {
	old := scanTimeout
	defer func() { scanTimeout = old }()

	scanTimeout = ""
	d := ScanTimeoutDuration()
	if d != 30*time.Second {
		t.Errorf("ScanTimeoutDuration with empty string = %v, want 30s fallback", d)
	}
}

func TestRootCmd_Sprint6_Exists(t *testing.T) {
	if rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}
	if rootCmd.Use != "ralphglasses" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "ralphglasses")
	}
}

func TestRootCmd_Sprint6_HasPersistentFlags(t *testing.T) {
	flags := []string{"scan-path", "debug", "log-level", "log-format", "scan-timeout"}
	for _, name := range flags {
		f := rootCmd.PersistentFlags().Lookup(name)
		if f == nil {
			t.Errorf("persistent flag %q should exist", name)
		}
	}
}

func TestRootCmd_Sprint6_HasLocalFlags(t *testing.T) {
	localFlags := []string{"theme", "notify"}
	for _, name := range localFlags {
		f := rootCmd.Flags().Lookup(name)
		if f == nil {
			t.Errorf("local flag %q should exist", name)
		}
	}
}

func TestRootCmd_Sprint6_Help(t *testing.T) {
	rootCmd.SetArgs([]string{"--help"})
	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("--help should not error, got: %v", err)
	}
}

func TestNewLogHandler_Sprint6_JSON(t *testing.T) {
	old := logFormat
	oldLevel := logLevel
	defer func() { logFormat = old; logLevel = oldLevel }()

	logFormat = "json"
	logLevel = "info"
	h := newLogHandler(nil)
	if h == nil {
		t.Error("newLogHandler should return non-nil handler for json format")
	}
}

func TestNewLogHandler_Sprint6_Text(t *testing.T) {
	old := logFormat
	oldLevel := logLevel
	defer func() { logFormat = old; logLevel = oldLevel }()

	logFormat = "text"
	logLevel = "debug"
	h := newLogHandler(nil)
	if h == nil {
		t.Error("newLogHandler should return non-nil handler for text format")
	}
}
