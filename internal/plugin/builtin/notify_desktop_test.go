package builtin

import (
	"context"
	"os/exec"
	"runtime"
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

func skipIfNoDesktopNotifications(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping live notification test in short mode")
	}
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			t.Skip("notify-send not available, skipping live notification test")
		}
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			t.Skip("osascript not available, skipping live notification test")
		}
	}
}

func TestNotifyDesktopPlugin_Execute_ValidParams(t *testing.T) {
	t.Parallel()
	skipIfNoDesktopNotifications(t)

	p := NewNotifyDesktopPlugin()
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
	method, ok := result["method"].(string)
	if !ok || method == "" {
		t.Errorf("result[method] = %v, want non-empty string", result["method"])
	}
}

func TestNotifyDesktopPlugin_Execute_ValidUrgencyLevels(t *testing.T) {
	t.Parallel()
	skipIfNoDesktopNotifications(t)

	p := NewNotifyDesktopPlugin()
	p.Init(context.Background(), nil)

	for _, urgency := range []string{"low", "normal", "critical"} {
		t.Run(urgency, func(t *testing.T) {
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
