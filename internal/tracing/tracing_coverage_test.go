package tracing

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
)

func TestToOTLPValue_Types(t *testing.T) {
	tests := []struct {
		name string
		val  any
	}{
		{"string", "hello"},
		{"int", 42},
		{"int64", int64(100)},
		{"float64", 3.14},
		{"bool", true},
		{"other", []byte{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify no panic.
			got := toOTLPValue(tt.val)
			_ = got
		})
	}
}

func TestToOTLPValue_StringValue(t *testing.T) {
	v := toOTLPValue("test-string")
	if v.StringValue == nil || *v.StringValue != "test-string" {
		t.Errorf("toOTLPValue(string) StringValue = %v, want test-string", v.StringValue)
	}
}

func TestToOTLPValue_IntValue(t *testing.T) {
	v := toOTLPValue(42)
	if v.IntValue == nil || *v.IntValue != 42 {
		t.Errorf("toOTLPValue(int) IntValue = %v, want 42", v.IntValue)
	}
}

func TestToOTLPValue_Int64Value(t *testing.T) {
	v := toOTLPValue(int64(999))
	if v.IntValue == nil || *v.IntValue != 999 {
		t.Errorf("toOTLPValue(int64) IntValue = %v, want 999", v.IntValue)
	}
}

func TestToOTLPValue_Float64Value(t *testing.T) {
	v := toOTLPValue(3.14)
	if v.DoubleValue == nil {
		t.Error("toOTLPValue(float64) DoubleValue should not be nil")
	}
}

func TestToOTLPValue_BoolValue(t *testing.T) {
	v := toOTLPValue(true)
	if v.BoolValue == nil || !*v.BoolValue {
		t.Errorf("toOTLPValue(bool) BoolValue = %v, want true", v.BoolValue)
	}
}

func TestToOTLPValue_FallbackToString(t *testing.T) {
	// Slice falls back to string representation.
	v := toOTLPValue([]int{1, 2, 3})
	if v.StringValue == nil || !strings.Contains(*v.StringValue, "1 2 3") {
		t.Errorf("toOTLPValue(slice) StringValue = %v, want string representation", v.StringValue)
	}
}

func TestGenerateTraceID_Format(t *testing.T) {
	id := generateTraceID()
	if len(id) != 32 {
		t.Errorf("generateTraceID() len = %d, want 32", len(id))
	}
	// Must be valid hex.
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("generateTraceID() = %q, not valid hex: %v", id, err)
	}
}

func TestGenerateSpanID_Format(t *testing.T) {
	id := generateSpanID()
	if len(id) != 16 {
		t.Errorf("generateSpanID() len = %d, want 16", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("generateSpanID() = %q, not valid hex: %v", id, err)
	}
}

func TestGenerateIDs_Unique(t *testing.T) {
	id1 := generateTraceID()
	id2 := generateTraceID()
	if id1 == id2 {
		t.Errorf("generateTraceID() produced same ID twice: %q", id1)
	}
}

func TestNewSampler_AlwaysTrue(t *testing.T) {
	s := newSampler(1.0)
	for range 10 {
		if !s() {
			t.Error("newSampler(1.0) should always return true")
		}
	}
}

func TestNewSampler_AlwaysFalse(t *testing.T) {
	s := newSampler(0.0)
	for range 10 {
		if s() {
			t.Error("newSampler(0.0) should always return false")
		}
	}
}

func TestNewSampler_GreaterThanOne(t *testing.T) {
	s := newSampler(2.0)
	for range 5 {
		if !s() {
			t.Error("newSampler(2.0) should always return true")
		}
	}
}

func TestNewSampler_Fractional(t *testing.T) {
	// With rate=0.5, we just verify no panic and it returns bool values.
	s := newSampler(0.5)
	for range 100 {
		_ = s()
	}
}

func TestNoopRecorder_Methods(t *testing.T) {
	n := &NoopRecorder{}
	_, span := n.StartSessionSpan(context.Background(), "sess-1", "claude", "sonnet", "myrepo")
	if span == nil {
		t.Fatal("StartSessionSpan should return non-nil")
	}

	// These are all no-ops — just verify they don't panic.
	n.EndSessionSpan(span, 1.0, 1000, "completed")
	n.RecordTurnMetric(context.Background(), "claude", "sonnet", "sess-1", 100, 200, 0.5, 1000)
	n.RecordError(span, "some error")
	n.RecordCostMetric(context.Background(), "claude", "sonnet", 0.01)
}
