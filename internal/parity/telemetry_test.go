package parity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/telemetry"
)

func TestLoadTelemetry_DefaultPathAndFilters(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("HOME", "")

	path := telemetry.DefaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(): %v", err)
	}
	content := strings.Join([]string{
		`{"type":"session_start","timestamp":"2026-04-08T10:00:00Z","provider":"claude","repo_name":"alpha"}`,
		`{"type":"crash","timestamp":"2026-04-08T11:00:00Z","provider":"codex","repo_name":"beta"}`,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}

	events, err := LoadTelemetry(TelemetryOptions{
		Repo:     "beta",
		Provider: "codex",
		Type:     "crash",
		Since:    time.Date(2026, 4, 8, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("LoadTelemetry(): %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(events))
	}
	if got := events[0].Type; got != telemetry.EventCrash {
		t.Fatalf("event type = %q, want %q", got, telemetry.EventCrash)
	}
}

func TestTelemetryJSONAndCSV(t *testing.T) {
	events := []telemetry.Event{{
		Type:      telemetry.EventBudgetHit,
		Timestamp: time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC),
		SessionID: "sess-1",
		Provider:  "claude",
		RepoName:  "alpha",
	}}

	jsonOut, err := TelemetryJSON(events)
	if err != nil {
		t.Fatalf("TelemetryJSON(): %v", err)
	}
	if !strings.Contains(jsonOut, `"budget_hit"`) {
		t.Fatalf("json output missing event type: %s", jsonOut)
	}

	csvOut, err := TelemetryCSV(events)
	if err != nil {
		t.Fatalf("TelemetryCSV(): %v", err)
	}
	if !strings.Contains(csvOut, "timestamp,type,session_id,provider,repo_name") {
		t.Fatalf("csv output missing header: %s", csvOut)
	}
	if !strings.Contains(csvOut, "budget_hit,sess-1,claude,alpha") {
		t.Fatalf("csv output missing event row: %s", csvOut)
	}
}
