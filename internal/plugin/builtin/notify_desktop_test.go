package builtin

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/plugin"
)

func TestNotifyDesktopPlugin_Name(t *testing.T) {
	t.Parallel()

	var p plugin.Plugin = NewNotifyDesktopPlugin()
	if got := p.Name(); got != "notify-desktop" {
		t.Errorf("Name() = %q, want %q", got, "notify-desktop")
	}
	if got := p.Version(); got != "0.1.0" {
		t.Errorf("Version() = %q, want %q", got, "0.1.0")
	}
}

func TestNotifyDesktopPlugin_InitShutdown(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	// Init with nil host should not panic.
	if err := p.Init(context.Background(), nil); err != nil {
		t.Fatalf("Init(nil) error: %v", err)
	}
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
}

func TestNotifyDesktopPlugin_Execute_MissingTitle(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	_, err := p.Execute(context.Background(), map[string]any{
		"message": "hello",
	})
	if err == nil {
		t.Fatal("expected error for missing title, got nil")
	}
}

func TestNotifyDesktopPlugin_Execute_EmptyTitle(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	_, err := p.Execute(context.Background(), map[string]any{
		"title":   "",
		"message": "hello",
	})
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}
}

func TestNotifyDesktopPlugin_Execute_MissingMessage(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	_, err := p.Execute(context.Background(), map[string]any{
		"title": "Test",
	})
	if err == nil {
		t.Fatal("expected error for missing message, got nil")
	}
}

func TestNotifyDesktopPlugin_Execute_EmptyMessage(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	_, err := p.Execute(context.Background(), map[string]any{
		"title":   "Test",
		"message": "",
	})
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
}

func TestNotifyDesktopPlugin_Execute_InvalidUrgency(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	_, err := p.Execute(context.Background(), map[string]any{
		"title":   "Test",
		"message": "hello",
		"urgency": "extreme",
	})
	if err == nil {
		t.Fatal("expected error for invalid urgency, got nil")
	}
}

func newTestNotifyDesktopPlugin(goos string, runErr error, argsSink *[]string) *NotifyDesktopPlugin {
	p := NewNotifyDesktopPlugin()
	p.goos = goos
	switch goos {
	case "linux":
		p.lookPathFn = func(name string) (string, error) {
			if name != "notify-send" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/notify-send", nil
		}
	case "darwin":
		p.lookPathFn = func(name string) (string, error) {
			if name != "osascript" {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/osascript", nil
		}
	default:
		p.lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
	}
	p.runCmdFn = func(_ context.Context, name string, args ...string) error {
		if argsSink != nil {
			*argsSink = append([]string{name}, args...)
		}
		return runErr
	}
	return p
}

func TestNotifyDesktopPlugin_Execute_ValidParams(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	p := newTestNotifyDesktopPlugin("linux", nil, &gotArgs)
	p.Init(context.Background(), nil)

	result, err := p.Execute(context.Background(), map[string]any{
		"title":   "ralphglasses test",
		"message": "plugin test notification",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	sent, ok := result["sent"].(bool)
	if !ok || !sent {
		t.Errorf("result[sent] = %v, want true", result["sent"])
	}
	if method := result["method"]; method != "notify-send" {
		t.Errorf("result[method] = %v, want notify-send", method)
	}
	wantArgs := []string{"notify-send", "-u", "normal", "ralphglasses test", "plugin test notification"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("got args %v, want %v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("got args %v, want %v", gotArgs, wantArgs)
		}
	}
}

func TestNotifyDesktopPlugin_Execute_ValidUrgencyLevels(t *testing.T) {
	t.Parallel()

	for _, urgency := range []string{"low", "normal", "critical"} {
		t.Run(urgency, func(t *testing.T) {
			var gotArgs []string
			p := newTestNotifyDesktopPlugin("linux", nil, &gotArgs)
			p.Init(context.Background(), nil)

			result, err := p.Execute(context.Background(), map[string]any{
				"title":   "test",
				"message": "msg",
				"urgency": urgency,
			})
			if err != nil {
				t.Fatalf("Execute with urgency=%q: %v", urgency, err)
			}
			if result["sent"] != true {
				t.Errorf("expected sent=true for urgency=%q", urgency)
			}
			if len(gotArgs) < 3 || gotArgs[2] != urgency {
				t.Fatalf("notify-send args = %v, want urgency %q", gotArgs, urgency)
			}
		})
	}
}

func TestNotifyDesktopPlugin_Execute_NilParams(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	_, err := p.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil params, got nil")
	}
}

func TestNotifyDesktopPlugin_Execute_CommandFailureFallsBackToLog(t *testing.T) {
	t.Parallel()

	p := newTestNotifyDesktopPlugin("linux", errors.New("notify-send failed"), nil)
	p.Init(context.Background(), nil)

	result, err := p.Execute(context.Background(), map[string]any{
		"title":   "Test",
		"message": "hello",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result["method"] != "log" {
		t.Fatalf("method = %v, want log", result["method"])
	}
}

func TestNotifyDesktopPlugin_Execute_MissingCommandFallsBackToLog(t *testing.T) {
	t.Parallel()

	p := NewNotifyDesktopPlugin()
	p.goos = "linux"
	p.lookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
	p.runCmdFn = func(context.Context, string, ...string) error {
		t.Fatal("runCmdFn should not be called when command is unavailable")
		return nil
	}
	p.Init(context.Background(), nil)

	result, err := p.Execute(context.Background(), map[string]any{
		"title":   "Test",
		"message": "hello",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if result["method"] != "log" {
		t.Fatalf("method = %v, want log", result["method"])
	}
}

func TestNotifyDesktopPlugin_Execute_ContextCanceled(t *testing.T) {
	t.Parallel()

	p := newTestNotifyDesktopPlugin("linux", context.Canceled, nil)
	p.Init(context.Background(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Execute(ctx, map[string]any{
		"title":   "Test",
		"message": "hello",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
}
