package session

import (
	"encoding/json"
	"testing"
)

func TestFloatVal_FromFloat64(t *testing.T) {
	m := map[string]any{"cost": float64(1.23)}
	got, ok := floatVal(m, "cost")
	if !ok || got != 1.23 {
		t.Errorf("floatVal float64 = %v, %v, want 1.23, true", got, ok)
	}
}

func TestFloatVal_FromInt(t *testing.T) {
	m := map[string]any{"count": 42}
	got, ok := floatVal(m, "count")
	if !ok || got != 42.0 {
		t.Errorf("floatVal int = %v, %v, want 42.0, true", got, ok)
	}
}

func TestFloatVal_FromJSONNumber(t *testing.T) {
	m := map[string]any{"val": json.Number("3.14")}
	got, ok := floatVal(m, "val")
	if !ok || got < 3.13 || got > 3.15 {
		t.Errorf("floatVal json.Number = %v, %v, want ~3.14, true", got, ok)
	}
}

func TestFloatVal_MissingKey(t *testing.T) {
	m := map[string]any{"other": 1.0}
	_, ok := floatVal(m, "missing")
	if ok {
		t.Error("floatVal missing key should return false")
	}
}

func TestFloatVal_NilMap(t *testing.T) {
	_, ok := floatVal(nil, "key")
	if ok {
		t.Error("floatVal nil map should return false")
	}
}

func TestFloatVal_InvalidType(t *testing.T) {
	m := map[string]any{"key": "not-a-number"}
	_, ok := floatVal(m, "key")
	if ok {
		t.Error("floatVal invalid type should return false")
	}
}

func TestIntVal_FromFloat64(t *testing.T) {
	m := map[string]any{"count": float64(5)}
	got, ok := intVal(m, "count")
	if !ok || got != 5 {
		t.Errorf("intVal float64 = %v, %v, want 5, true", got, ok)
	}
}

func TestIntVal_FromInt(t *testing.T) {
	m := map[string]any{"n": 7}
	got, ok := intVal(m, "n")
	if !ok || got != 7 {
		t.Errorf("intVal int = %v, %v, want 7, true", got, ok)
	}
}

func TestIntVal_FromJSONNumber(t *testing.T) {
	m := map[string]any{"n": json.Number("99")}
	got, ok := intVal(m, "n")
	if !ok || got != 99 {
		t.Errorf("intVal json.Number = %v, %v, want 99, true", got, ok)
	}
}

func TestIntVal_NilMap(t *testing.T) {
	_, ok := intVal(nil, "key")
	if ok {
		t.Error("intVal nil map should return false")
	}
}

func TestIntVal_InvalidType(t *testing.T) {
	m := map[string]any{"key": true}
	_, ok := intVal(m, "key")
	if ok {
		t.Error("intVal invalid type should return false")
	}
}

func TestTokenizeShellArgs_Simple(t *testing.T) {
	got := tokenizeShellArgs("hello world")
	if len(got) != 2 || got[0] != "hello" || got[1] != "world" {
		t.Errorf("tokenizeShellArgs simple = %v, want [hello world]", got)
	}
}

func TestTokenizeShellArgs_DoubleQuoted(t *testing.T) {
	got := tokenizeShellArgs(`"hello world"`)
	if len(got) != 1 || got[0] != "hello world" {
		t.Errorf("tokenizeShellArgs double-quoted = %v, want [hello world]", got)
	}
}

func TestTokenizeShellArgs_SingleQuoted(t *testing.T) {
	got := tokenizeShellArgs("'hello world'")
	if len(got) != 1 || got[0] != "hello world" {
		t.Errorf("tokenizeShellArgs single-quoted = %v, want [hello world]", got)
	}
}

func TestTokenizeShellArgs_Escaped(t *testing.T) {
	got := tokenizeShellArgs(`hello\ world`)
	if len(got) != 1 || got[0] != "hello world" {
		t.Errorf("tokenizeShellArgs escaped = %v, want [hello world]", got)
	}
}

func TestTokenizeShellArgs_Empty(t *testing.T) {
	got := tokenizeShellArgs("")
	if len(got) != 0 {
		t.Errorf("tokenizeShellArgs empty = %v, want empty", got)
	}
}
