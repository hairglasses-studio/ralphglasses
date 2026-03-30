package builtin

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"

	"github.com/hairglasses-studio/ralphglasses/internal/plugin"
)

// NotifyDesktopPlugin sends desktop notifications using platform-native
// commands (notify-send on Linux, osascript on macOS). Falls back to
// logging if neither is available.
type NotifyDesktopPlugin struct {
	logger *slog.Logger
}

// NewNotifyDesktopPlugin creates a new NotifyDesktopPlugin.
func NewNotifyDesktopPlugin() *NotifyDesktopPlugin {
	return &NotifyDesktopPlugin{}
}

// Name returns the plugin identifier.
func (n *NotifyDesktopPlugin) Name() string { return "notify-desktop" }

// Version returns the plugin version.
func (n *NotifyDesktopPlugin) Version() string { return "0.1.0" }

// Init stores the host logger for fallback output.
func (n *NotifyDesktopPlugin) Init(_ context.Context, host plugin.PluginHost) error {
	if host != nil {
		n.logger = host.Logger()
	}
	if n.logger == nil {
		n.logger = slog.Default()
	}
	return nil
}

// Shutdown is a no-op.
func (n *NotifyDesktopPlugin) Shutdown() error { return nil }

// Execute sends a desktop notification. Required params: "title" (string),
// "message" (string). Optional: "urgency" (string: low/normal/critical,
// Linux only).
func (n *NotifyDesktopPlugin) Execute(ctx context.Context, params map[string]any) (map[string]any, error) {
	title, ok := params["title"].(string)
	if !ok || title == "" {
		return nil, fmt.Errorf("missing required param %q", "title")
	}
	message, ok := params["message"].(string)
	if !ok || message == "" {
		return nil, fmt.Errorf("missing required param %q", "message")
	}

	urgency := "normal"
	if u, ok := params["urgency"].(string); ok && u != "" {
		switch u {
		case "low", "normal", "critical":
			urgency = u
		default:
			return nil, fmt.Errorf("invalid urgency %q: must be low, normal, or critical", u)
		}
	}

	method, err := n.send(ctx, title, message, urgency)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"sent":   true,
		"method": method,
	}, nil
}

// send dispatches the notification using a platform-appropriate command.
// Returns the method used ("notify-send", "osascript", or "log").
func (n *NotifyDesktopPlugin) send(ctx context.Context, title, message, urgency string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		if path, err := exec.LookPath("notify-send"); err == nil && path != "" {
			cmd := exec.CommandContext(ctx, "notify-send", "-u", urgency, title, message)
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("notify-send: %w", err)
			}
			return "notify-send", nil
		}
	case "darwin":
		if path, err := exec.LookPath("osascript"); err == nil && path != "" {
			script := fmt.Sprintf(`display notification %q with title %q`, message, title)
			cmd := exec.CommandContext(ctx, "osascript", "-e", script)
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("osascript: %w", err)
			}
			return "osascript", nil
		}
	}

	// Fallback: log the notification.
	n.log(title, message)
	return "log", nil
}

// log writes the notification to the structured logger as a fallback.
func (n *NotifyDesktopPlugin) log(title, message string) {
	logger := n.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("desktop notification (fallback)",
		"title", title,
		"message", message,
	)
}
