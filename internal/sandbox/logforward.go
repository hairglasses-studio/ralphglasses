// Package sandbox provides container isolation for LLM sessions.
// Log forwarding support for streaming Docker container output.
package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// LogEntry represents a parsed Docker log line.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"` // "stdout" or "stderr"
	Message   string    `json:"message"`
}

// LogForwarder tails container logs and writes them to an output writer.
type LogForwarder struct {
	containerID string
	output      io.Writer
	mu          sync.Mutex
	done        chan struct{}

	// reader is the source of log lines. In production this comes from
	// `docker logs --follow`; tests can inject a pipe.
	reader io.Reader
}

// NewLogForwarder creates a LogForwarder that writes container logs to output.
func NewLogForwarder(containerID string, output io.Writer) *LogForwarder {
	return &LogForwarder{
		containerID: containerID,
		output:      output,
		done:        make(chan struct{}),
	}
}

// Start begins tailing container logs. It blocks until the context is cancelled,
// Stop is called, or the log stream ends. When reader is nil it shells out to
// `docker logs --follow`; otherwise it reads from the injected reader (for tests).
func (lf *LogForwarder) Start(ctx context.Context) error {
	var r io.Reader

	if lf.reader != nil {
		r = lf.reader
	} else {
		cmd := execCommandContext(ctx, "docker", "logs", "--follow", "--timestamps", lf.containerID)
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("logforward: stdout pipe: %w", err)
		}
		cmd.Stderr = cmd.Stdout // merge stderr into the same pipe
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("logforward: start docker logs: %w", err)
		}
		r = pipe
		defer cmd.Wait() //nolint:errcheck
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case <-lf.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		entry, err := ParseDockerLogLine(line)
		if err != nil {
			// Write raw line if parsing fails.
			lf.writeLine(line)
			continue
		}
		lf.writeLine(fmt.Sprintf("[%s] %s: %s",
			entry.Timestamp.Format(time.RFC3339), entry.Stream, entry.Message))
	}

	return scanner.Err()
}

// Stop signals the log forwarder to stop tailing.
func (lf *LogForwarder) Stop() error {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	select {
	case <-lf.done:
		// already stopped
	default:
		close(lf.done)
	}
	return nil
}

// writeLine safely writes a line to the output writer.
func (lf *LogForwarder) writeLine(s string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	fmt.Fprintln(lf.output, s)
}

// ParseDockerLogLine parses a Docker log line in the format:
//
//	TIMESTAMP STREAM MESSAGE
//
// where TIMESTAMP is RFC3339Nano and STREAM is "stdout" or "stderr".
// Also accepts the simpler "STREAM: MESSAGE" format.
func ParseDockerLogLine(line string) (*LogEntry, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty log line")
	}

	// Try RFC3339 timestamp prefix: "2024-01-01T00:00:00.000Z stdout some message"
	parts := strings.SplitN(line, " ", 3)
	if len(parts) >= 3 {
		ts, err := time.Parse(time.RFC3339Nano, parts[0])
		if err == nil {
			stream := normalizeStream(parts[1])
			return &LogEntry{
				Timestamp: ts,
				Stream:    stream,
				Message:   parts[2],
			}, nil
		}
	}

	// Fallback: "stdout: message" or "stderr: message"
	if len(parts) >= 2 {
		streamCandidate := strings.TrimSuffix(parts[0], ":")
		if streamCandidate == "stdout" || streamCandidate == "stderr" {
			return &LogEntry{
				Timestamp: time.Now(),
				Stream:    streamCandidate,
				Message:   strings.Join(parts[1:], " "),
			}, nil
		}
	}

	return nil, fmt.Errorf("unrecognized log format: %q", line)
}

// normalizeStream ensures stream is "stdout" or "stderr".
func normalizeStream(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "stderr" {
		return "stderr"
	}
	return "stdout"
}
