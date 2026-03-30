package views

import (
	"testing"
)

func TestConfigEditor_Filter_ReturnsCurrentFilter(t *testing.T) {
	cfg := makeTestConfig(t)
	ce := NewConfigEditor(cfg)
	// Initially empty.
	if ce.Filter() != "" {
		t.Errorf("Filter() = %q, want empty", ce.Filter())
	}
	// Set a filter and verify it's returned.
	ce.filter = "foo"
	if ce.Filter() != "foo" {
		t.Errorf("Filter() = %q, want %q", ce.Filter(), "foo")
	}
}
