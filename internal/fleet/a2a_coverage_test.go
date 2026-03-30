package fleet

import (
	"testing"
)

func TestOfferStatusToTaskState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input OfferStatus
		want  TaskState
	}{
		{OfferOpen, TaskStateQueued},
		{OfferSubmitted, TaskStateQueued},
		{OfferAccepted, TaskStateWorking},
		{OfferWorking, TaskStateWorking},
		{OfferInputRequired, TaskStateInputRequired},
		{OfferCompleted, TaskStateCompleted},
		{OfferFailed, TaskStateFailed},
		{OfferExpired, TaskStateFailed},
		{OfferCanceled, TaskStateCanceled},
		{OfferStatus("unknown"), TaskStateQueued}, // default case
	}
	for _, tt := range tests {
		got := OfferStatusToTaskState(tt.input)
		if got != tt.want {
			t.Errorf("OfferStatusToTaskState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTaskStateToOfferStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input TaskState
		want  OfferStatus
	}{
		{TaskStateQueued, OfferSubmitted},
		{TaskStateWorking, OfferWorking},
		{TaskStateInputRequired, OfferInputRequired},
		{TaskStateCompleted, OfferCompleted},
		{TaskStateFailed, OfferFailed},
		{TaskStateCanceled, OfferCanceled},
		{TaskState("unknown"), OfferSubmitted}, // default case
	}
	for _, tt := range tests {
		got := TaskStateToOfferStatus(tt.input)
		if got != tt.want {
			t.Errorf("TaskStateToOfferStatus(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOfferStatusToTaskState_Roundtrip(t *testing.T) {
	t.Parallel()
	// For non-legacy statuses, converting to TaskState and back should be stable.
	statuses := []OfferStatus{OfferSubmitted, OfferWorking, OfferInputRequired, OfferCompleted, OfferFailed, OfferCanceled}
	for _, s := range statuses {
		ts := OfferStatusToTaskState(s)
		back := TaskStateToOfferStatus(ts)
		if back != s {
			t.Errorf("roundtrip %q -> %q -> %q (want %q)", s, ts, back, s)
		}
	}
}

func TestNewTextPart(t *testing.T) {
	t.Parallel()
	p := NewTextPart("hello world")
	if p.Type != PartTypeText {
		t.Errorf("Type = %q, want %q", p.Type, PartTypeText)
	}
	if p.Text != "hello world" {
		t.Errorf("Text = %q, want %q", p.Text, "hello world")
	}
	if p.Data != nil {
		t.Error("Data should be nil for text part")
	}
}

func TestNewTextPart_Empty(t *testing.T) {
	t.Parallel()
	p := NewTextPart("")
	if p.Type != PartTypeText {
		t.Errorf("Type = %q, want %q", p.Type, PartTypeText)
	}
	if p.Text != "" {
		t.Errorf("Text = %q, want empty", p.Text)
	}
}

func TestNewDataPart(t *testing.T) {
	t.Parallel()
	data := map[string]int{"count": 42}
	p := NewDataPart(data, "application/json")
	if p.Type != PartTypeData {
		t.Errorf("Type = %q, want %q", p.Type, PartTypeData)
	}
	if p.MimeType != "application/json" {
		t.Errorf("MimeType = %q", p.MimeType)
	}
	if p.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestNewFilePart(t *testing.T) {
	t.Parallel()
	p := NewFilePart("file:///tmp/test.txt", "text/plain")
	if p.Type != PartTypeFile {
		t.Errorf("Type = %q, want %q", p.Type, PartTypeFile)
	}
	if p.FileURI != "file:///tmp/test.txt" {
		t.Errorf("FileURI = %q", p.FileURI)
	}
	if p.MimeType != "text/plain" {
		t.Errorf("MimeType = %q", p.MimeType)
	}
}

// A2AStatusToWorkItemStatus and isTerminal are tested in a2a_lifecycle_test.go.
