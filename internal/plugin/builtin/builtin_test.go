package builtin

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/plugin"
)

func TestLoggerPluginInterface(t *testing.T) {
	t.Parallel()

	var p plugin.Plugin = NewLoggerPlugin()

	if got := p.Name(); got != "builtin.logger" {
		t.Errorf("Name() = %q, want %q", got, "builtin.logger")
	}
	if got := p.Version(); got != "0.1.0" {
		t.Errorf("Version() = %q, want %q", got, "0.1.0")
	}
}

func TestNewLoggerPluginDefaultPath(t *testing.T) {
	t.Parallel()
	p := NewLoggerPlugin()
	if p.LogPath == "" {
		t.Error("LogPath is empty")
	}
	if !strings.HasSuffix(p.LogPath, filepath.Join(".ralph", "plugin-events.log")) {
		t.Errorf("LogPath = %q, want suffix .ralph/plugin-events.log", p.LogPath)
	}
}

func TestLoggerPluginOnEvent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.log")

	p := &LoggerPlugin{LogPath: logPath}

	events := []plugin.Event{
		{Type: "session.start", Repo: "/tmp/repo-a"},
		{Type: "loop.done", Repo: "/tmp/repo-b"},
		{Type: "error", Repo: "/tmp/repo-c"},
	}

	for _, evt := range events {
		if err := p.OnEvent(context.Background(), evt); err != nil {
			t.Fatalf("OnEvent(%+v) error: %v", evt, err)
		}
	}

	// Read back the log file and verify lines.
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanning log file: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("got %d log lines, want 3", len(lines))
	}

	for i, evt := range events {
		parts := strings.Split(lines[i], "\t")
		if len(parts) != 3 {
			t.Errorf("line %d: got %d tab-separated parts, want 3: %q", i, len(parts), lines[i])
			continue
		}
		if parts[1] != evt.Type {
			t.Errorf("line %d: event type = %q, want %q", i, parts[1], evt.Type)
		}
		if parts[2] != evt.Repo {
			t.Errorf("line %d: repo = %q, want %q", i, parts[2], evt.Repo)
		}
	}
}

func TestLoggerPluginOnEventAppendsToExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "events.log")

	// Write an initial line.
	if err := os.WriteFile(logPath, []byte("existing line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &LoggerPlugin{LogPath: logPath}
	if err := p.OnEvent(context.Background(), plugin.Event{Type: "new", Repo: "/r"}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (existing + new)", len(lines))
	}
	if lines[0] != "existing line" {
		t.Errorf("first line = %q, want %q", lines[0], "existing line")
	}
}

func TestLoggerPluginOnEventInvalidPath(t *testing.T) {
	t.Parallel()

	p := &LoggerPlugin{LogPath: "/nonexistent/deeply/nested/dir/events.log"}
	err := p.OnEvent(context.Background(), plugin.Event{Type: "test", Repo: "/r"})
	if err == nil {
		t.Error("expected error for invalid log path, got nil")
	}
}

func TestNewOllamaProviderDefaults(t *testing.T) {
	t.Parallel()

	provider := NewOllamaProvider(OllamaConfig{})

	if provider.endpoint != "http://127.0.0.1:11434" {
		t.Fatalf("provider.endpoint = %q, want %q", provider.endpoint, "http://127.0.0.1:11434")
	}
	if provider.model != "qwen3:8b" {
		t.Fatalf("provider.model = %q, want %q", provider.model, "qwen3:8b")
	}
}
