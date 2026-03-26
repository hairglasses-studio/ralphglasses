package mcpserver

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParamSync verifies that tool builder param declarations (mcp.WithString, mcp.WithNumber,
// mcp.WithBoolean in tools_builders.go) are in sync with handler param reads (getStringArg,
// getNumberArg, getBoolArg in handler_*.go files). Mismatches indicate a param declared but
// never read, or read in a handler but never declared in the tool builder.
func TestParamSync(t *testing.T) {
	t.Parallel()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	// Parse all Go files in this package.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse dir: %v", err)
	}

	// Extract builder params: tool name → set of param names.
	builderParams := make(map[string]map[string]bool)
	// Extract handler params: handler func name → set of param names.
	handlerParams := make(map[string]map[string]bool)

	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			base := filepath.Base(filename)
			if base == "tools_builders.go" {
				extractBuilderParams(file, builderParams)
			}
			if strings.HasPrefix(base, "handler_") || base == "tools.go" {
				extractHandlerParams(file, handlerParams)
			}
		}
	}

	// Build mapping: tool name → handler function name.
	// We extract this from builder code where tools are paired with handlers like:
	//   {mcp.NewTool("ralphglasses_foo", ...), s.handleFoo}
	toolToHandler := make(map[string]string)
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			if filepath.Base(filename) == "tools_builders.go" {
				extractToolHandlerMapping(file, toolToHandler)
			}
		}
	}

	// Sanity check: we should find a meaningful number of tools and handlers.
	if len(builderParams) < 10 {
		t.Fatalf("only found %d tools in builders — expected at least 10; AST extraction may be broken", len(builderParams))
	}
	if len(handlerParams) < 10 {
		t.Fatalf("only found %d handler funcs with params — expected at least 10; AST extraction may be broken", len(handlerParams))
	}
	if len(toolToHandler) < 10 {
		t.Fatalf("only found %d tool→handler mappings — expected at least 10; AST extraction may be broken", len(toolToHandler))
	}

	t.Logf("found %d tools in builders, %d handler funcs with params, %d tool→handler mappings",
		len(builderParams), len(handlerParams), len(toolToHandler))

	// Cross-check: for each tool, ensure handler params match builder params.
	var (
		errs     []string
		warnings []string
		checked  int
	)
	for toolName, bParams := range builderParams {
		handlerFunc, ok := toolToHandler[toolName]
		if !ok {
			continue // No mapping found; skip.
		}

		hParams, ok := handlerParams[handlerFunc]
		if !ok {
			// Handler exists but reads no params — fine if builder also declares none.
			if len(bParams) > 0 {
				for param := range bParams {
					warnings = append(warnings, fmt.Sprintf(
						"tool %s declares param %q in builder but handler %s reads no params at all",
						toolName, param, handlerFunc))
				}
			}
			continue
		}

		checked++

		// ERROR: handler reads a param not declared in builder (undeclared consumed param).
		for param := range hParams {
			if !bParams[param] {
				errs = append(errs, fmt.Sprintf(
					"UNDECLARED: handler %s reads param %q but tool %s does not declare it in builder",
					handlerFunc, param, toolName))
			}
		}

		// WARNING: builder declares a param not read by handler (unused declared param).
		for param := range bParams {
			if !hParams[param] {
				warnings = append(warnings, fmt.Sprintf(
					"UNUSED: tool %s declares param %q in builder but handler %s never reads it",
					toolName, param, handlerFunc))
			}
		}
	}

	t.Logf("cross-checked %d tools with both builder params and handler params", checked)

	// Log warnings (unused declared params) — informational, not failures.
	for _, w := range warnings {
		t.Log(w)
	}

	// Fail on errors (undeclared consumed params) — these are real bugs.
	for _, e := range errs {
		t.Error(e)
	}

	// Also fail on unused declared params — they indicate stale builder declarations.
	for _, w := range warnings {
		t.Error(w)
	}
}

// extractBuilderParams finds mcp.NewTool("name", ...) calls and collects With{String,Number,Boolean}
// param names associated with each tool.
func extractBuilderParams(file *ast.File, out map[string]map[string]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		// Look for composite literals like {mcp.NewTool(...), s.handleXxx}
		comp, ok := n.(*ast.CompositeLit)
		if !ok || len(comp.Elts) < 2 {
			return true
		}

		// First element should be mcp.NewTool(...)
		call, ok := comp.Elts[0].(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewTool" {
			return true
		}

		if len(call.Args) < 1 {
			return true
		}

		// Extract tool name from first arg (string literal).
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		toolName := strings.Trim(lit.Value, "\"")

		params := make(map[string]bool)
		// Remaining args are mcp.With{String,Number,Boolean}("paramName", ...)
		for _, arg := range call.Args[1:] {
			paramCall, ok := arg.(*ast.CallExpr)
			if !ok {
				continue
			}
			paramSel, ok := paramCall.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			name := paramSel.Sel.Name
			if name != "WithString" && name != "WithNumber" && name != "WithBoolean" {
				continue
			}
			if len(paramCall.Args) < 1 {
				continue
			}
			paramLit, ok := paramCall.Args[0].(*ast.BasicLit)
			if !ok || paramLit.Kind != token.STRING {
				continue
			}
			paramName := strings.Trim(paramLit.Value, "\"")
			params[paramName] = true
		}

		out[toolName] = params
		return true
	})
}

// extractHandlerParams finds getStringArg/getNumberArg/getBoolArg calls and direct m["key"]
// index expressions inside handler functions and records which param names each handler reads.
func extractHandlerParams(file *ast.File, out map[string]map[string]bool) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		funcName := fn.Name.Name

		params := make(map[string]bool)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			// Match getStringArg(req, "key"), getNumberArg(req, "key", default), getBoolArg(req, "key")
			if call, ok := n.(*ast.CallExpr); ok {
				ident, ok := call.Fun.(*ast.Ident)
				if ok {
					name := ident.Name
					if name == "getStringArg" || name == "getNumberArg" || name == "getBoolArg" {
						if len(call.Args) >= 2 {
							if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
								paramName := strings.Trim(lit.Value, "\"")
								params[paramName] = true
							}
						}
					}
				}
			}

			// Match m["key"] index expressions (direct argsMap access).
			if idx, ok := n.(*ast.IndexExpr); ok {
				if lit, ok := idx.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					paramName := strings.Trim(lit.Value, "\"")
					// Only count if it looks like a param name (not arbitrary map access).
					// We check that the indexed variable is named "m" or "args" (common patterns).
					if ident, ok := idx.X.(*ast.Ident); ok {
						if ident.Name == "m" || ident.Name == "args" {
							params[paramName] = true
						}
					}
				}
			}

			return true
		})

		if len(params) > 0 {
			out[funcName] = params
		}
	}
}

// extractToolHandlerMapping finds the handler function name associated with each tool
// in composite literals like {mcp.NewTool("name", ...), s.handleXxx}.
func extractToolHandlerMapping(file *ast.File, out map[string]string) {
	ast.Inspect(file, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok || len(comp.Elts) < 2 {
			return true
		}

		// First element: mcp.NewTool("name", ...)
		call, ok := comp.Elts[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewTool" {
			return true
		}
		if len(call.Args) < 1 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		toolName := strings.Trim(lit.Value, "\"")

		// Second element: s.handleXxx
		handlerSel, ok := comp.Elts[1].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		out[toolName] = handlerSel.Sel.Name
		return true
	})
}
