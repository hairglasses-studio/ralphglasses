package main

import (
	"go/ast"
	"go/token"
	"strconv"
)

// identName returns the identifier name for expr when expr is an *ast.Ident.
func identName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// selectorName renders identifiers and selector expressions into a stable
// dotted form such as "mcp.WithDescription".
func selectorName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		left := selectorName(e.X)
		if left == "" || e.Sel == nil {
			return ""
		}
		return left + "." + e.Sel.Name
	default:
		return ""
	}
}

// unquote evaluates simple string literals and string-literal concatenation.
func unquote(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return ""
		}
		s, err := strconv.Unquote(e.Value)
		if err != nil {
			return ""
		}
		return s
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return ""
		}
		left := unquote(e.X)
		right := unquote(e.Y)
		if left == "" && right == "" {
			return ""
		}
		return left + right
	default:
		return ""
	}
}
