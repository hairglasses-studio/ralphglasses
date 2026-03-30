package main

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestIdentName_Ident(t *testing.T) {
	expr := &ast.Ident{Name: "myVar"}
	got := identName(expr)
	if got != "myVar" {
		t.Errorf("identName(Ident) = %q, want myVar", got)
	}
}

func TestIdentName_NotIdent(t *testing.T) {
	// A non-Ident expression should return "".
	expr := &ast.BasicLit{Kind: token.STRING, Value: `"hello"`}
	got := identName(expr)
	if got != "" {
		t.Errorf("identName(BasicLit) = %q, want empty", got)
	}
}

func TestSelectorName_Selector(t *testing.T) {
	expr := &ast.SelectorExpr{
		X:   &ast.Ident{Name: "mcp"},
		Sel: &ast.Ident{Name: "WithDescription"},
	}
	got := selectorName(expr)
	if got != "mcp.WithDescription" {
		t.Errorf("selectorName(SelectorExpr) = %q, want mcp.WithDescription", got)
	}
}

func TestSelectorName_Ident(t *testing.T) {
	expr := &ast.Ident{Name: "ToolGroup"}
	got := selectorName(expr)
	if got != "ToolGroup" {
		t.Errorf("selectorName(Ident) = %q, want ToolGroup", got)
	}
}

func TestSelectorName_Other(t *testing.T) {
	// Any other expression type returns "".
	expr := &ast.BasicLit{Kind: token.INT, Value: "42"}
	got := selectorName(expr)
	if got != "" {
		t.Errorf("selectorName(other) = %q, want empty", got)
	}
}

func TestUnquote_DoubleQuoted(t *testing.T) {
	expr := &ast.BasicLit{Kind: token.STRING, Value: `"hello world"`}
	got := unquote(expr)
	if got != "hello world" {
		t.Errorf("unquote(double-quoted) = %q, want hello world", got)
	}
}

func TestUnquote_Backtick(t *testing.T) {
	expr := &ast.BasicLit{Kind: token.STRING, Value: "`raw string`"}
	got := unquote(expr)
	if got != "raw string" {
		t.Errorf("unquote(backtick) = %q, want raw string", got)
	}
}

func TestUnquote_NonString(t *testing.T) {
	expr := &ast.BasicLit{Kind: token.INT, Value: "42"}
	got := unquote(expr)
	if got != "" {
		t.Errorf("unquote(int literal) = %q, want empty", got)
	}
}

func TestUnquote_Concatenation(t *testing.T) {
	// "foo" + "bar" should give "foobar".
	expr := &ast.BinaryExpr{
		Op: token.ADD,
		X:  &ast.BasicLit{Kind: token.STRING, Value: `"foo"`},
		Y:  &ast.BasicLit{Kind: token.STRING, Value: `"bar"`},
	}
	got := unquote(expr)
	if got != "foobar" {
		t.Errorf("unquote(concat) = %q, want foobar", got)
	}
}

func TestUnquote_OtherExpr(t *testing.T) {
	// A call expression should return "".
	expr := &ast.CallExpr{}
	got := unquote(expr)
	if got != "" {
		t.Errorf("unquote(CallExpr) = %q, want empty", got)
	}
}
