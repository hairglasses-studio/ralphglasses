package parity

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/telemetry"
)

type TelemetryOptions struct {
	Path     string
	Since    time.Time
	Until    time.Time
	Repo     string
	Provider string
	Type     string
	Limit    int
}

func DefaultTelemetryPath() string {
	return telemetry.DefaultPath()
}

func LoadTelemetry(opts TelemetryOptions) ([]telemetry.Event, error) {
	path := opts.Path
	if path == "" {
		path = DefaultTelemetryPath()
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []telemetry.Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev telemetry.Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		if !opts.Since.IsZero() && ev.Timestamp.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && ev.Timestamp.After(opts.Until) {
			continue
		}
		if opts.Repo != "" && ev.RepoName != opts.Repo {
			continue
		}
		if opts.Provider != "" && ev.Provider != opts.Provider {
			continue
		}
		if opts.Type != "" && string(ev.Type) != opts.Type {
			continue
		}
		events = append(events, ev)
		if opts.Limit > 0 && len(events) >= opts.Limit {
			break
		}
	}
	return events, scanner.Err()
}

func TelemetryJSON(events []telemetry.Event) (string, error) {
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func TelemetryCSV(events []telemetry.Event) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"timestamp", "type", "session_id", "provider", "repo_name"}); err != nil {
		return "", err
	}
	for _, ev := range events {
		if err := w.Write([]string{
			ev.Timestamp.Format(time.RFC3339),
			string(ev.Type),
			ev.SessionID,
			ev.Provider,
			ev.RepoName,
		}); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}
