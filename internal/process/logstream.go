package process

import (
	"bufio"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
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

// OpenLogFile creates the log directory and opens the ralph log file for
// appending. The caller is responsible for closing the returned file.
// This is the canonical way to obtain a writable log handle — both the
// process manager (ralph_loop.sh stdout/stderr) and the session runner
// (streaming JSON output) should use it.
func OpenLogFile(repoPath string) (*os.File, error) {
	dir := LogDirPath(repoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return os.OpenFile(LogFilePath(repoPath), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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

// ReadFullLogFallback attempts to read logs from legacy paths when the primary
// log file does not exist. It checks:
//  1. .ralph/ralph.log (legacy single-file location)
//  2. .ralph/logs/*.log (glob for any log files in the log directory)
//
// Returns os.ErrNotExist if no log files are found at any fallback path.
func ReadFullLogFallback(repoPath string) ([]string, error) {
	// Try legacy .ralph/ralph.log
	legacyPath := filepath.Join(repoPath, ".ralph", "ralph.log")
	if lines, err := readLogFile(legacyPath); err == nil {
		return lines, nil
	}

	// Try .ralph/logs/*.log (glob)
	pattern := filepath.Join(repoPath, ".ralph", "logs", "*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil, os.ErrNotExist
	}

	var allLines []string
	for _, m := range matches {
		if lines, err := readLogFile(m); err == nil {
			allLines = append(allLines, lines...)
		}
	}
	if len(allLines) == 0 {
		return nil, os.ErrNotExist
	}
	return allLines, nil
}

// readLogFile reads all lines from a single log file.
func readLogFile(path string) ([]string, error) {
	f, err := os.Open(path)
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
