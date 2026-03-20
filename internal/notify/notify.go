package notify

import (
	"os/exec"
	"runtime"
)

// Send dispatches a desktop notification.
// macOS: osascript, Linux: notify-send, other: no-op.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + escapeOSA(body) + `" with title "` + escapeOSA(title) + `"`
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return nil // no-op if not available
		}
		return exec.Command("notify-send", title, body).Run()
	default:
		return nil
	}
}

func escapeOSA(s string) string {
	// Escape backslashes and double quotes for AppleScript strings
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			out = append(out, '\\', '\\')
		case '"':
			out = append(out, '\\', '"')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
