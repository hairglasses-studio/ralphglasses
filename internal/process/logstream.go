package process

import (
	"bufio"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// LogFilePath returns the canonical path to the ralph log file for a given
// base directory (either a repo path or the scan root). All code that reads
// or writes ralph.log MUST use this function to avoid path mismatches.
func LogFilePath(basePath string) string {
	return filepath.Join(basePath, ".ralph", "logs", "ralph.log")
}

// LogDirPath returns the directory containing the ralph log file.
func LogDirPath(basePath string) string {
	return filepath.Join(basePath, ".ralph", "logs")
}

// LogLinesMsg carries new log lines read from a file.
type LogLinesMsg struct {
	Lines []string
}

// TailLog reads the ralph log from the last known offset and returns new lines.
// Call this as a tea.Cmd to feed lines into the TUI.
func TailLog(repoPath string, offset *int64) tea.Cmd {
	return func() tea.Msg {
		logPath := LogFilePath(repoPath)
		f, err := os.Open(logPath)
		if err != nil {
			return LogLinesMsg{Lines: []string{"[error] Cannot read log: " + err.Error()}}
		}
		defer f.Close()

		if *offset > 0 {
			if _, err := f.Seek(*offset, io.SeekStart); err != nil {
				return LogLinesMsg{Lines: []string{"[error] Cannot read log: " + err.Error()}}
			}
		}

		var lines []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		pos, _ := f.Seek(0, io.SeekCurrent)
		*offset = pos

		return LogLinesMsg{Lines: lines}
	}
}

// ReadFullLog reads the entire ralph log file.
func ReadFullLog(repoPath string) ([]string, error) {
	logPath := LogFilePath(repoPath)
	f, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 256*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
