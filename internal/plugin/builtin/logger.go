// Package builtin provides built-in ralphglasses plugins.
package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/plugin"
)

// LoggerPlugin writes each fleet event to a log file.
type LoggerPlugin struct {
	// LogPath is the file to append events to.
	// Defaults to ~/.ralph/plugin-events.log.
	LogPath string
}

// NewLoggerPlugin creates a LoggerPlugin with the default log path.
func NewLoggerPlugin() *LoggerPlugin {
	home, _ := os.UserHomeDir()
	return &LoggerPlugin{
		LogPath: filepath.Join(home, ".ralph", "plugin-events.log"),
	}
}

// Name returns the plugin identifier "builtin.logger".
func (l *LoggerPlugin) Name() string { return "builtin.logger" }

// Version returns the plugin version.
func (l *LoggerPlugin) Version() string { return "0.1.0" }

// OnEvent appends a line to LogPath with the event timestamp, type, and repo.
func (l *LoggerPlugin) OnEvent(_ context.Context, event plugin.Event) error {
	f, err := os.OpenFile(l.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log %s: %w", l.LogPath, err)
	}
	defer f.Close()

	line := fmt.Sprintf("%s\t%s\t%s\n",
		time.Now().UTC().Format(time.RFC3339),
		event.Type,
		event.Repo,
	)
	_, err = f.WriteString(line)
	return err
}
