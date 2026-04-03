package process

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Common patterns that indicate an agent session has completed.
var completionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)session complete`),
	regexp.MustCompile(`(?i)\bcompleted\b`),
	regexp.MustCompile(`\x{2713}`), // ✓
	regexp.MustCompile(`(?m)^[$>]\s*$`), // shell prompt reappearance
	regexp.MustCompile(`(?i)exit code[:\s]+\d+`),
	regexp.MustCompile(`(?i)exited with`),
}

// WaitForSessionComplete polls a tmux pane for completion markers.
// Returns when it detects common agent completion patterns or timeout.
// This is a supplementary detection method — use alongside existing
// stderr monitoring for redundant completion detection.
func WaitForSessionComplete(sessionName string, timeout time.Duration) (exitText string, err error) {
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastLineCount int

	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", fmt.Errorf("timeout after %s waiting for session %q", timeout, sessionName)
			}

			content, captureErr := captureTmuxPane(sessionName)
			if captureErr != nil {
				// Pane gone — session likely exited.
				return "", fmt.Errorf("pane capture failed (session may have exited): %w", captureErr)
			}

			lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
			// Only scan new lines to avoid re-matching old output.
			startIdx := lastLineCount
			if startIdx > len(lines) {
				startIdx = 0 // output was cleared/reset
			}
			lastLineCount = len(lines)

			for i := startIdx; i < len(lines); i++ {
				line := lines[i]
				for _, pat := range completionPatterns {
					if pat.MatchString(line) {
						return strings.TrimSpace(line), nil
					}
				}
			}
		}
	}
}

// captureTmuxPane runs `tmux capture-pane -p -t <target>` and returns the output.
func captureTmuxPane(target string) (string, error) {
	out, err := exec.Command("tmux", "capture-pane", "-p", "-t", target).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
