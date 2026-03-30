package session

import (
	"testing"
)

func TestExtractReportSubject_Found(t *testing.T) {
	tokens := []string{"show", "cost", "report"}
	got := extractReportSubject(tokens)
	if got != "cost" {
		t.Errorf("extractReportSubject = %q, want cost", got)
	}
}

func TestExtractReportSubject_Status(t *testing.T) {
	tokens := []string{"check", "status"}
	got := extractReportSubject(tokens)
	if got != "status" {
		t.Errorf("extractReportSubject status = %q, want status", got)
	}
}

func TestExtractReportSubject_NotFound(t *testing.T) {
	tokens := []string{"hello", "world"}
	got := extractReportSubject(tokens)
	if got != "" {
		t.Errorf("extractReportSubject not found = %q, want empty", got)
	}
}

func TestExtractReportSubject_Empty(t *testing.T) {
	got := extractReportSubject(nil)
	if got != "" {
		t.Errorf("extractReportSubject nil = %q, want empty", got)
	}
}

func TestExtractFleetKeyword_Found(t *testing.T) {
	tokens := []string{"show", "fleet", "status"}
	if !extractFleetKeyword(tokens) {
		t.Error("extractFleetKeyword should return true for 'fleet' in tokens")
	}
}

func TestExtractFleetKeyword_NotFound(t *testing.T) {
	tokens := []string{"show", "sessions"}
	if extractFleetKeyword(tokens) {
		t.Error("extractFleetKeyword should return false when 'fleet' not in tokens")
	}
}

func TestExtractFleetKeyword_Empty(t *testing.T) {
	if extractFleetKeyword(nil) {
		t.Error("extractFleetKeyword(nil) should return false")
	}
}

func TestExtractSessionID_Found(t *testing.T) {
	tokens := []string{"stop", "session", "42"}
	got := extractSessionID(tokens)
	if got != "42" {
		t.Errorf("extractSessionID = %q, want 42", got)
	}
}

func TestExtractSessionID_NotFound(t *testing.T) {
	tokens := []string{"show", "all", "sessions"}
	got := extractSessionID(tokens)
	if got != "" {
		t.Errorf("extractSessionID not found = %q, want empty", got)
	}
}
