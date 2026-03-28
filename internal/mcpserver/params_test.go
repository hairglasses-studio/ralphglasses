package mcpserver

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRequireStringPresent(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"name": "hello"}))
	val, errResult := p.RequireString("name")
	if errResult != nil {
		t.Fatalf("expected no error, got: %v", errResult)
	}
	if val != "hello" {
		t.Fatalf("expected %q, got %q", "hello", val)
	}
}

func TestRequireStringMissing(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{}))
	val, errResult := p.RequireString("name")
	if errResult == nil {
		t.Fatal("expected error for missing required string")
	}
	if val != "" {
		t.Fatalf("expected empty string, got %q", val)
	}
	if !errResult.IsError {
		t.Fatal("expected IsError to be true")
	}
}

func TestRequireStringEmpty(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"name": ""}))
	_, errResult := p.RequireString("name")
	if errResult == nil {
		t.Fatal("expected error for empty required string")
	}
}

func TestRequireStringNilArgs(t *testing.T) {
	t.Parallel()
	req := mcp.CallToolRequest{}
	p := NewParams(req)
	_, errResult := p.RequireString("name")
	if errResult == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestOptionalStringPresent(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"key": "val"}))
	got := p.OptionalString("key", "default")
	if got != "val" {
		t.Fatalf("expected %q, got %q", "val", got)
	}
}

func TestOptionalStringMissing(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{}))
	got := p.OptionalString("key", "default")
	if got != "default" {
		t.Fatalf("expected %q, got %q", "default", got)
	}
}

func TestOptionalStringEmpty(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"key": ""}))
	got := p.OptionalString("key", "fallback")
	if got != "fallback" {
		t.Fatalf("expected %q, got %q", "fallback", got)
	}
}

func TestRequireNumberPresent(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"count": float64(42)}))
	val, errResult := p.RequireNumber("count")
	if errResult != nil {
		t.Fatalf("expected no error, got: %v", errResult)
	}
	if val != 42 {
		t.Fatalf("expected 42, got %f", val)
	}
}

func TestRequireNumberMissing(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{}))
	_, errResult := p.RequireNumber("count")
	if errResult == nil {
		t.Fatal("expected error for missing required number")
	}
}

func TestRequireNumberWrongType(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"count": "not-a-number"}))
	_, errResult := p.RequireNumber("count")
	if errResult == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestRequireNumberNilArgs(t *testing.T) {
	t.Parallel()
	req := mcp.CallToolRequest{}
	p := NewParams(req)
	_, errResult := p.RequireNumber("count")
	if errResult == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestOptionalNumberPresent(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"n": float64(7)}))
	got := p.OptionalNumber("n", 99)
	if got != 7 {
		t.Fatalf("expected 7, got %f", got)
	}
}

func TestOptionalNumberMissing(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{}))
	got := p.OptionalNumber("n", 99)
	if got != 99 {
		t.Fatalf("expected 99, got %f", got)
	}
}

func TestRequireBoolPresent(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"flag": true}))
	val, errResult := p.RequireBool("flag")
	if errResult != nil {
		t.Fatalf("expected no error, got: %v", errResult)
	}
	if !val {
		t.Fatal("expected true")
	}
}

func TestRequireBoolFalse(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"flag": false}))
	val, errResult := p.RequireBool("flag")
	if errResult != nil {
		t.Fatalf("expected no error, got: %v", errResult)
	}
	if val {
		t.Fatal("expected false")
	}
}

func TestRequireBoolMissing(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{}))
	_, errResult := p.RequireBool("flag")
	if errResult == nil {
		t.Fatal("expected error for missing required bool")
	}
}

func TestRequireBoolWrongType(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"flag": "yes"}))
	_, errResult := p.RequireBool("flag")
	if errResult == nil {
		t.Fatal("expected error for wrong type")
	}
}

func TestRequireBoolNilArgs(t *testing.T) {
	t.Parallel()
	req := mcp.CallToolRequest{}
	p := NewParams(req)
	_, errResult := p.RequireBool("flag")
	if errResult == nil {
		t.Fatal("expected error for nil args")
	}
}

func TestOptionalBoolPresent(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"flag": true}))
	got := p.OptionalBool("flag", false)
	if !got {
		t.Fatal("expected true")
	}
}

func TestOptionalBoolMissing(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{}))
	got := p.OptionalBool("flag", true)
	if !got {
		t.Fatal("expected default true")
	}
}

func TestOptionalBoolWrongType(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"flag": "yes"}))
	got := p.OptionalBool("flag", true)
	if !got {
		t.Fatal("expected default true when wrong type")
	}
}

func TestParamsReqAccessor(t *testing.T) {
	t.Parallel()
	req := makeReq("test", map[string]any{"x": "y"})
	p := NewParams(req)
	got := getStringArg(p.Req(), "x")
	if got != "y" {
		t.Fatalf("expected %q, got %q", "y", got)
	}
}

func TestOptionalNumberZeroValue(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"n": float64(0)}))
	got := p.OptionalNumber("n", 99)
	if got != 0 {
		t.Fatalf("expected 0, got %f", got)
	}
}

func TestRequireNumberZeroValue(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"n": float64(0)}))
	val, errResult := p.RequireNumber("n")
	if errResult != nil {
		t.Fatalf("expected no error for zero value, got: %v", errResult)
	}
	if val != 0 {
		t.Fatalf("expected 0, got %f", val)
	}
}

func TestOptionalBoolFalseExplicit(t *testing.T) {
	t.Parallel()
	p := NewParams(makeReq("test", map[string]any{"flag": false}))
	got := p.OptionalBool("flag", true)
	if got {
		t.Fatal("expected explicit false to override default true")
	}
}
