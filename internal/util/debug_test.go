package util

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

func TestDebugf_Enabled(t *testing.T) {
	t.Parallel()

	// Capture stderr
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	Debug.Enabled = true
	defer func() { Debug.Enabled = false }()

	os.Stderr = w
	Debug.Debugf("hello %s", "world")
	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read: %v", err)
	}

	got := buf.String()
	want := "[DEBUG] hello world\n"
	if got != want {
		t.Errorf("Debugf output = %q, want %q", got, want)
	}
}

func TestDebugf_Disabled(t *testing.T) {
	t.Parallel()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	Debug.Enabled = false
	os.Stderr = w
	Debug.Debugf("should not appear")
	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("expected no output when disabled, got %q", buf.String())
	}
}

func TestDebugf_FormatArgs(t *testing.T) {
	tests := []struct {
		name   string
		format string
		args   []any
		want   string
	}{
		{"no args", "plain message", nil, "[DEBUG] plain message\n"},
		{"int arg", "count=%d", []any{42}, "[DEBUG] count=42\n"},
		{"multi args", "%s=%d", []any{"x", 7}, "[DEBUG] x=7\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stderr
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}

			Debug.Enabled = true
			os.Stderr = w
			Debug.Debugf(tt.format, tt.args...)
			w.Close()
			os.Stderr = old
			Debug.Enabled = false

			var buf bytes.Buffer
			if _, err := buf.ReadFrom(r); err != nil {
				t.Fatalf("read: %v", err)
			}

			if got := buf.String(); got != tt.want {
				t.Errorf("Debugf(%q, %v) = %q, want %q", tt.format, tt.args, got, tt.want)
			}
		})
	}
}

func TestDebugLogger_DefaultDisabled(t *testing.T) {
	t.Parallel()
	d := &debugLogger{}
	if d.Enabled {
		t.Error("new debugLogger should be disabled by default")
	}
}

func TestDebugLogger_EnableToggle(t *testing.T) {
	t.Parallel()
	d := &debugLogger{}
	d.Enabled = true
	if !d.Enabled {
		t.Error("expected Enabled=true after setting")
	}
	d.Enabled = false
	if d.Enabled {
		t.Error("expected Enabled=false after clearing")
	}
}

func TestDebugGlobalInstance(t *testing.T) {
	t.Parallel()
	if Debug == nil {
		t.Fatal("Debug global should not be nil")
	}
	_ = fmt.Sprintf("%T", Debug) // ensure type is accessible
}
