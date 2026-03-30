package sandbox

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseDockerLogLine_TimestampFormat(t *testing.T) {
	line := "2024-06-15T10:30:00.123456789Z stdout hello world"
	entry, err := ParseDockerLogLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Stream != "stdout" {
		t.Errorf("stream = %q, want stdout", entry.Stream)
	}
	if entry.Message != "hello world" {
		t.Errorf("message = %q, want %q", entry.Message, "hello world")
	}
	if entry.Timestamp.Year() != 2024 {
		t.Errorf("year = %d, want 2024", entry.Timestamp.Year())
	}
}

func TestParseDockerLogLine_Stderr(t *testing.T) {
	line := "2024-01-01T00:00:00Z stderr error occurred"
	entry, err := ParseDockerLogLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Stream != "stderr" {
		t.Errorf("stream = %q, want stderr", entry.Stream)
	}
	if entry.Message != "error occurred" {
		t.Errorf("message = %q, want %q", entry.Message, "error occurred")
	}
}

func TestParseDockerLogLine_FallbackFormat(t *testing.T) {
	line := "stdout: some output here"
	entry, err := ParseDockerLogLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Stream != "stdout" {
		t.Errorf("stream = %q, want stdout", entry.Stream)
	}
	if !strings.Contains(entry.Message, "some output here") {
		t.Errorf("message = %q, want to contain %q", entry.Message, "some output here")
	}
}

func TestParseDockerLogLine_EmptyLine(t *testing.T) {
	_, err := ParseDockerLogLine("")
	if err == nil {
		t.Fatal("expected error for empty line")
	}
}

func TestParseDockerLogLine_UnrecognizedFormat(t *testing.T) {
	_, err := ParseDockerLogLine("just some random text")
	if err == nil {
		t.Fatal("expected error for unrecognized format")
	}
}

func TestLogForwarder_StartStop(t *testing.T) {
	pr, pw := io.Pipe()
	var buf bytes.Buffer

	lf := NewLogForwarder("test-container", &buf)
	lf.reader = pr

	// Write log lines in a goroutine.
	go func() {
		pw.Write([]byte("2024-01-01T00:00:00Z stdout line one\n"))
		pw.Write([]byte("2024-01-01T00:00:01Z stderr line two\n"))
		time.Sleep(50 * time.Millisecond)
		pw.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := lf.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "line one") {
		t.Errorf("output missing 'line one': %q", output)
	}
	if !strings.Contains(output, "line two") {
		t.Errorf("output missing 'line two': %q", output)
	}
}

func TestLogForwarder_StopSignal(t *testing.T) {
	pr, pw := io.Pipe()
	var buf bytes.Buffer

	lf := NewLogForwarder("test-container", &buf)
	lf.reader = pr

	done := make(chan error, 1)
	go func() {
		done <- lf.Start(context.Background())
	}()

	// Write one line, then stop.
	pw.Write([]byte("2024-01-01T00:00:00Z stdout hello\n"))
	time.Sleep(50 * time.Millisecond)
	lf.Stop()
	pw.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestLogForwarder_ConcurrentAccess(t *testing.T) {
	pr, pw := io.Pipe()
	var buf bytes.Buffer

	lf := NewLogForwarder("test-container", &buf)
	lf.reader = pr

	done := make(chan error, 1)
	go func() {
		done <- lf.Start(context.Background())
	}()

	// Concurrently write and stop.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			pw.Write([]byte("2024-01-01T00:00:00Z stdout concurrent\n"))
		}
	}()
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)
		lf.Stop()
	}()

	wg.Wait()
	pw.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
}

func TestLogForwarder_DoubleStop(t *testing.T) {
	lf := NewLogForwarder("test-container", &bytes.Buffer{})
	// Double stop should not panic.
	if err := lf.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := lf.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
