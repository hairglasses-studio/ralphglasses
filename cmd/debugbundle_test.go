package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDebugBundleCmd_Registration(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "debug-bundle" {
			found = true
			break
		}
	}
	if !found {
		t.Error("debug-bundle command not registered on rootCmd")
	}
}

func TestDebugBundleCmd_ShortDescription(t *testing.T) {
	if debugBundleCmd.Short == "" {
		t.Error("debug-bundle command missing Short description")
	}
}

func TestDebugBundleCmd_LongDescription(t *testing.T) {
	if debugBundleCmd.Long == "" {
		t.Error("debug-bundle command missing Long description")
	}
}

func TestDebugBundleCmd_Example(t *testing.T) {
	if debugBundleCmd.Example == "" {
		t.Error("debug-bundle command missing Example")
	}
}

func TestDebugBundleCmd_OutputFlag(t *testing.T) {
	f := debugBundleCmd.Flags().Lookup("output")
	if f == nil {
		t.Fatal("--output flag not registered")
	}
	if f.Shorthand != "o" {
		t.Errorf("output shorthand = %q, want %q", f.Shorthand, "o")
	}
}

func TestSanitizeSecrets(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // substring that should be present
		avoid string // substring that should NOT be present
	}{
		{
			name:  "anthropic key env",
			input: "ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnop",
			want:  "...",
			avoid: "abcdefghijklmnop",
		},
		{
			name:  "sk-ant prefix",
			input: "my key is sk-ant-api03-abcdefghijklmnopqrstuv",
			want:  "...",
			avoid: "abcdefghijklmnopqrstuv",
		},
		{
			name:  "openai key",
			input: "OPENAI_API_KEY=sk-proj-abcdefghijklmnop",
			want:  "...",
			avoid: "abcdefghijklmnop",
		},
		{
			name:  "gemini key",
			input: "GEMINI_API_KEY=AIzaSyABCDEFGHIJKLMNOP",
			want:  "...",
			avoid: "ABCDEFGHIJKLMNOP",
		},
		{
			name:  "no secrets",
			input: "PROJECT_NAME=myproject",
			want:  "PROJECT_NAME=myproject",
			avoid: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeSecrets(tt.input)
			if !strings.Contains(got, tt.want) {
				t.Errorf("sanitizeSecrets(%q) = %q, want to contain %q", tt.input, got, tt.want)
			}
			if tt.avoid != "" && strings.Contains(got, tt.avoid) {
				t.Errorf("sanitizeSecrets(%q) = %q, should NOT contain %q", tt.input, got, tt.avoid)
			}
		})
	}
}

func TestDebugBundle_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "test-bundle.txt")

	// Set output path and run
	debugBundleOutput = outPath
	defer func() { debugBundleOutput = "" }()

	// Temporarily set scanPath to something that exists
	oldScanPath := scanPath
	scanPath = dir
	defer func() { scanPath = oldScanPath }()

	err := runDebugBundle(debugBundleCmd, nil)
	if err != nil {
		t.Fatalf("runDebugBundle: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	content := string(data)

	// Verify key sections exist
	for _, section := range []string{
		"RALPHGLASSES DEBUG BUNDLE",
		"SYSTEM INFO",
		"GO VERSION",
		"GIT VERSION",
		"SCAN PATH",
	} {
		if !strings.Contains(content, section) {
			t.Errorf("bundle missing section %q", section)
		}
	}

	// Verify version info
	if !strings.Contains(content, version) {
		t.Errorf("bundle should contain version %q", version)
	}
}

func TestCollectSystemInfo(t *testing.T) {
	info := collectSystemInfo()
	if !strings.Contains(info, "OS:") {
		t.Error("collectSystemInfo missing OS field")
	}
	if !strings.Contains(info, "Arch:") {
		t.Error("collectSystemInfo missing Arch field")
	}
	if !strings.Contains(info, "NumCPU:") {
		t.Error("collectSystemInfo missing NumCPU field")
	}
}

func TestWriteSection(t *testing.T) {
	var b strings.Builder
	writeSection(&b, "TEST", "content here")
	got := b.String()
	if !strings.Contains(got, "=== TEST ===") {
		t.Error("writeSection missing title header")
	}
	if !strings.Contains(got, "content here") {
		t.Error("writeSection missing content")
	}
}
