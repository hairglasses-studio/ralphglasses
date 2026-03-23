package util

import (
	"fmt"
	"os"
)

// Debug is the package-level debug logger.
var Debug = &debugLogger{}

type debugLogger struct {
	Enabled bool
}

// Debugf writes a formatted debug message to stderr if debug mode is enabled.
func (d *debugLogger) Debugf(format string, args ...any) {
	if !d.Enabled {
		return
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
}
