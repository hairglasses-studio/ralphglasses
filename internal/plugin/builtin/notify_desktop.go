package builtin

import (
	"context"
	"errors"
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
	logger     *slog.Logger
	goos       string
	lookPathFn func(string) (string, error)
	runCmdFn   func(context.Context, string, ...string) error
}

// NewNotifyDesktopPlugin creates a new NotifyDesktopPlugin.
func NewNotifyDesktopPlugin() *NotifyDesktopPlugin {
	return &NotifyDesktopPlugin{
		goos:       runtime.GOOS,
		lookPathFn: exec.LookPath,
		runCmdFn: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
	}
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
		"sent":      true,
		"method":    method,
		"delivered": method != "log",
		"fallback":  method == "log",
	}, nil
}

// send dispatches the notification using a platform-appropriate command.
// Returns the method used ("notify-send", "osascript", or "log").
func (n *NotifyDesktopPlugin) send(ctx context.Context, title, message, urgency string) (string, error) {
	switch n.goos {
	case "linux":
		if path, err := n.lookPath("notify-send"); err == nil && path != "" {
			if err := n.runCommand(ctx, "notify-send", "-u", urgency, title, message); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return "", ctxErr
				}
				n.logCommandFailure("notify-send", err, title, message)
				return "log", nil
			}
			return "notify-send", nil
		}
	case "darwin":
		if path, err := n.lookPath("osascript"); err == nil && path != "" {
			script := fmt.Sprintf(`display notification %q with title %q`, message, title)
			if err := n.runCommand(ctx, "osascript", "-e", script); err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return "", ctxErr
				}
				n.logCommandFailure("osascript", err, title, message)
				return "log", nil
			}
			return "osascript", nil
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}

	// Fallback: log the notification.
	n.log(title, message)
	return "log", nil
}

func (n *NotifyDesktopPlugin) lookPath(name string) (string, error) {
	if n.lookPathFn == nil {
		n.lookPathFn = exec.LookPath
	}
	return n.lookPathFn(name)
}

func (n *NotifyDesktopPlugin) runCommand(ctx context.Context, name string, args ...string) error {
	if n.runCmdFn == nil {
		n.runCmdFn = func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		}
	}
	return n.runCmdFn(ctx, name, args...)
}

func (n *NotifyDesktopPlugin) logCommandFailure(command string, err error, title, message string) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	logger := n.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("desktop notification command failed; using log fallback",
		"command", command,
		"error", err,
		"title", title,
		"message", message,
	)
	n.log(title, message)
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
