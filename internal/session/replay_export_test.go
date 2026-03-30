package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestPlayer creates a Player with the given events written to a temp JSONL file.
func newTestPlayer(t *testing.T, events []ReplayEvent) *Player {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("test-sess", path)
	for _, ev := range events {
		if err := rec.Record(ev); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	p, err := NewPlayer(path)
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}
	return p
}

func TestExportMarkdownMixedEventTypes(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "implement feature X"},
		{Timestamp: t0.Add(1 * time.Second), Type: ReplayOutput, Data: "I'll start by reading the code"},
		{Timestamp: t0.Add(2 * time.Second), Type: ReplayTool, Data: "read_file main.go"},
		{Timestamp: t0.Add(5 * time.Second), Type: ReplayOutput, Data: "Here is the implementation"},
		{Timestamp: t0.Add(7 * time.Second), Type: ReplayTool, Data: "write_file main.go"},
		{Timestamp: t0.Add(10 * time.Second), Type: ReplayStatus, Data: "completed"},
	}
	p := newTestPlayer(t, events)

	var buf bytes.Buffer
	if err := ExportMarkdown(p, &buf, nil); err != nil {
		t.Fatalf("ExportMarkdown: %v", err)
	}

	md := buf.String()

	// Check header
	if !strings.Contains(md, "# Session Replay Export") {
		t.Error("missing main header")
	}

	// Check metadata
	if !strings.Contains(md, "**Total events:** 6") {
		t.Error("missing or wrong total events count")
	}
	if !strings.Contains(md, "**Session ID:** test-sess") {
		t.Error("missing session ID")
	}
	if !strings.Contains(md, "**Duration:**") {
		t.Error("missing duration")
	}

	// Check event type counts
	if !strings.Contains(md, "input=1") {
		t.Error("missing input count")
	}
	if !strings.Contains(md, "output=2") {
		t.Error("missing output count")
	}
	if !strings.Contains(md, "tool=2") {
		t.Error("missing tool count")
	}
	if !strings.Contains(md, "status=1") {
		t.Error("missing status count")
	}

	// Check timeline section
	if !strings.Contains(md, "## Timeline") {
		t.Error("missing timeline header")
	}

	// Check event markers
	if !strings.Contains(md, "[INPUT]") {
		t.Error("missing input marker")
	}
	if !strings.Contains(md, "[OUTPUT]") {
		t.Error("missing output marker")
	}
	if !strings.Contains(md, "[TOOL]") {
		t.Error("missing tool marker")
	}
	if !strings.Contains(md, "[STATUS]") {
		t.Error("missing status marker")
	}

	// Check data content
	if !strings.Contains(md, "implement feature X") {
		t.Error("missing input data")
	}
	if !strings.Contains(md, "read_file main.go") {
		t.Error("missing tool data")
	}
}

func TestExportJSONRoundTrip(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "hello world"},
		{Timestamp: t0.Add(2 * time.Second), Type: ReplayOutput, Data: "response here"},
		{Timestamp: t0.Add(5 * time.Second), Type: ReplayTool, Data: "run_tests"},
	}
	p := newTestPlayer(t, events)

	var buf bytes.Buffer
	if err := ExportJSON(p, &buf, nil); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	var doc ExportJSONDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}

	// Check metadata
	if doc.Version != "1.0" {
		t.Errorf("version = %q, want 1.0", doc.Version)
	}
	if doc.Metadata.TotalEvents != 3 {
		t.Errorf("total_events = %d, want 3", doc.Metadata.TotalEvents)
	}
	if doc.Metadata.SessionID != "test-sess" {
		t.Errorf("session_id = %q, want test-sess", doc.Metadata.SessionID)
	}
	if doc.Metadata.DurationMS != 5000 {
		t.Errorf("duration_ms = %d, want 5000", doc.Metadata.DurationMS)
	}
	if doc.Metadata.EventCounts["input"] != 1 {
		t.Errorf("input count = %d, want 1", doc.Metadata.EventCounts["input"])
	}
	if doc.Metadata.EventCounts["output"] != 1 {
		t.Errorf("output count = %d, want 1", doc.Metadata.EventCounts["output"])
	}
	if doc.Metadata.EventCounts["tool"] != 1 {
		t.Errorf("tool count = %d, want 1", doc.Metadata.EventCounts["tool"])
	}

	// Check round-trip event data
	if len(doc.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(doc.Events))
	}
	if doc.Events[0].Data != "hello world" {
		t.Errorf("event[0].data = %q, want 'hello world'", doc.Events[0].Data)
	}
	if doc.Events[0].Type != ReplayInput {
		t.Errorf("event[0].type = %q, want input", doc.Events[0].Type)
	}
	if doc.Events[0].OffsetMS != 0 {
		t.Errorf("event[0].offset_ms = %d, want 0", doc.Events[0].OffsetMS)
	}
	if doc.Events[2].OffsetMS != 5000 {
		t.Errorf("event[2].offset_ms = %d, want 5000", doc.Events[2].OffsetMS)
	}

	// Verify timestamps parse back correctly
	ts, err := time.Parse(time.RFC3339Nano, doc.Events[0].Timestamp)
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if !ts.Equal(t0) {
		t.Errorf("parsed timestamp = %v, want %v", ts, t0)
	}
}

func TestExportEmptyPlayer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	_ = os.WriteFile(path, nil, 0o644)

	p, err := NewPlayer(path)
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}

	// Markdown export with empty player
	var mdBuf bytes.Buffer
	if err := ExportMarkdown(p, &mdBuf, nil); err != nil {
		t.Fatalf("ExportMarkdown empty: %v", err)
	}
	md := mdBuf.String()
	if !strings.Contains(md, "**Total events:** 0") {
		t.Error("empty markdown should show 0 events")
	}
	if !strings.Contains(md, "_No events recorded._") {
		t.Error("empty markdown should show no events message")
	}

	// JSON export with empty player
	var jsonBuf bytes.Buffer
	if err := ExportJSON(p, &jsonBuf, nil); err != nil {
		t.Fatalf("ExportJSON empty: %v", err)
	}
	var doc ExportJSONDocument
	if err := json.Unmarshal(jsonBuf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if doc.Metadata.TotalEvents != 0 {
		t.Errorf("total_events = %d, want 0", doc.Metadata.TotalEvents)
	}
	if len(doc.Events) != 0 {
		t.Errorf("events = %d, want 0", len(doc.Events))
	}
	if doc.Metadata.DurationMS != 0 {
		t.Errorf("duration_ms = %d, want 0", doc.Metadata.DurationMS)
	}
}

func TestExportTimeRangeFilter(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "early"},
		{Timestamp: t0.Add(10 * time.Second), Type: ReplayOutput, Data: "middle"},
		{Timestamp: t0.Add(20 * time.Second), Type: ReplayTool, Data: "late-tool"},
		{Timestamp: t0.Add(30 * time.Second), Type: ReplayStatus, Data: "end"},
	}
	p := newTestPlayer(t, events)

	// Filter: only events between t0+5s and t0+25s
	filter := &ExportFilter{
		After:  t0.Add(5 * time.Second),
		Before: t0.Add(25 * time.Second),
	}

	// Test with Markdown
	var mdBuf bytes.Buffer
	if err := ExportMarkdown(p, &mdBuf, filter); err != nil {
		t.Fatalf("ExportMarkdown filtered: %v", err)
	}
	md := mdBuf.String()
	if strings.Contains(md, "early") {
		t.Error("time filter should exclude 'early' event")
	}
	if !strings.Contains(md, "middle") {
		t.Error("time filter should include 'middle' event")
	}
	if !strings.Contains(md, "late-tool") {
		t.Error("time filter should include 'late-tool' event")
	}
	if strings.Contains(md, `"end"`) || strings.Contains(md, "[STATUS]") {
		t.Error("time filter should exclude 'end' event")
	}

	// Test with JSON
	var jsonBuf bytes.Buffer
	if err := ExportJSON(p, &jsonBuf, filter); err != nil {
		t.Fatalf("ExportJSON filtered: %v", err)
	}
	var doc ExportJSONDocument
	if err := json.Unmarshal(jsonBuf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if doc.Metadata.TotalEvents != 2 {
		t.Errorf("filtered total_events = %d, want 2", doc.Metadata.TotalEvents)
	}
	if len(doc.Events) != 2 {
		t.Fatalf("filtered events = %d, want 2", len(doc.Events))
	}
	if doc.Events[0].Data != "middle" {
		t.Errorf("filtered event[0].data = %q, want 'middle'", doc.Events[0].Data)
	}
	if doc.Events[1].Data != "late-tool" {
		t.Errorf("filtered event[1].data = %q, want 'late-tool'", doc.Events[1].Data)
	}
}

func TestExportEventTypeFilter(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "prompt"},
		{Timestamp: t0.Add(1 * time.Second), Type: ReplayOutput, Data: "response"},
		{Timestamp: t0.Add(2 * time.Second), Type: ReplayTool, Data: "edit_file"},
		{Timestamp: t0.Add(3 * time.Second), Type: ReplayStatus, Data: "done"},
	}
	p := newTestPlayer(t, events)

	// Filter: only tool events
	filter := &ExportFilter{
		EventTypes: []ReplayEventType{ReplayTool},
	}

	var buf bytes.Buffer
	if err := ExportJSON(p, &buf, filter); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	var doc ExportJSONDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if doc.Metadata.TotalEvents != 1 {
		t.Errorf("total_events = %d, want 1", doc.Metadata.TotalEvents)
	}
	if len(doc.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(doc.Events))
	}
	if doc.Events[0].Type != ReplayTool {
		t.Errorf("event type = %q, want tool", doc.Events[0].Type)
	}
	if doc.Events[0].Data != "edit_file" {
		t.Errorf("event data = %q, want 'edit_file'", doc.Events[0].Data)
	}
}

func TestExportCombinedFilter(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "early input"},
		{Timestamp: t0.Add(5 * time.Second), Type: ReplayTool, Data: "mid tool"},
		{Timestamp: t0.Add(10 * time.Second), Type: ReplayInput, Data: "late input"},
		{Timestamp: t0.Add(15 * time.Second), Type: ReplayTool, Data: "late tool"},
	}
	p := newTestPlayer(t, events)

	// Filter: only input events after t0+3s
	filter := &ExportFilter{
		EventTypes: []ReplayEventType{ReplayInput},
		After:      t0.Add(3 * time.Second),
	}

	var buf bytes.Buffer
	if err := ExportJSON(p, &buf, filter); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	var doc ExportJSONDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if doc.Metadata.TotalEvents != 1 {
		t.Errorf("total_events = %d, want 1", doc.Metadata.TotalEvents)
	}
	if doc.Events[0].Data != "late input" {
		t.Errorf("event data = %q, want 'late input'", doc.Events[0].Data)
	}
}

func TestExportFilterNilPassesAll(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "a"},
		{Timestamp: t0.Add(1 * time.Second), Type: ReplayOutput, Data: "b"},
	}
	p := newTestPlayer(t, events)

	var buf bytes.Buffer
	if err := ExportJSON(p, &buf, nil); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	var doc ExportJSONDocument
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("JSON unmarshal: %v", err)
	}
	if doc.Metadata.TotalEvents != 2 {
		t.Errorf("nil filter should pass all events, got %d want 2", doc.Metadata.TotalEvents)
	}
}
