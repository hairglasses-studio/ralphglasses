// genmcpdocs parses MCP tool registrations from internal/mcpserver/ and
// generates markdown API reference documentation grouped by namespace.
//
// Usage:
//
//	go run ./tools/genmcpdocs                  # stdout
//	go run ./tools/genmcpdocs --output docs/MCP-API.md
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

// Param describes a single tool parameter.
type Param struct {
	Name        string
	Type        string // "string", "number", "boolean"
	Required    bool
	Description string
}

// Tool describes a registered MCP tool.
type Tool struct {
	Name        string
	Description string
	Params      []Param
}

// Group collects tools under a namespace.
type Group struct {
	Name        string
	Description string
	Tools       []Tool
}

func main() {
	output := flag.String("output", "", "Output file path (default: stdout)")
	dir := flag.String("dir", "", "Directory containing handler/builder Go files (default: internal/mcpserver)")
	flag.Parse()

	srcDir := *dir
	if srcDir == "" {
		srcDir = "internal/mcpserver"
	}

	groups, err := ParseDir(srcDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "genmcpdocs: %v\n", err)
		os.Exit(1)
	}

	var w io.Writer = os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "genmcpdocs: create %s: %v\n", *output, err)
			os.Exit(1)
		}
		defer f.Close()
		w = f
	}

	if err := Render(w, groups); err != nil {
		fmt.Fprintf(os.Stderr, "genmcpdocs: render: %v\n", err)
		os.Exit(1)
	}
}

// ParseDir scans all Go files in dir for build*Group methods and extracts
// tool registrations.
func ParseDir(dir string) ([]Group, error) {
	pattern := filepath.Join(dir, "tools_builders*.go")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob %s: %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no tools_builders*.go files found in %s", dir)
	}

	var groups []Group
	for _, path := range matches {
		gs, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		groups = append(groups, gs...)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups, nil
}

// ParseFile extracts tool groups from a single Go source file.
func ParseFile(path string) ([]Group, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var groups []Group
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil {
			return true
		}
		name := fn.Name.Name
		if !strings.HasPrefix(name, "build") || !strings.HasSuffix(name, "Group") {
			return true
		}

		g := extractGroup(fn.Body)
		if g.Name != "" {
			groups = append(groups, g)
		}
		return false
	})

	return groups, nil
}

// extractGroup pulls the group name, description, and tools from a
// buildXxxGroup function body.
func extractGroup(body *ast.BlockStmt) Group {
	var g Group

	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return true
		}
		cl, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok {
			return true
		}

		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key := identName(kv.Key)
			switch key {
			case "Name":
				g.Name = unquote(kv.Value)
			case "Description":
				g.Description = unquote(kv.Value)
			case "Tools":
				g.Tools = extractTools(kv.Value)
			}
		}
		return false
	})

	return g
}

// extractTools pulls individual tool entries from a []ToolEntry composite lit.
func extractTools(expr ast.Expr) []Tool {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return nil
	}

	var tools []Tool
	for _, elt := range cl.Elts {
		t := extractTool(elt)
		if t.Name != "" {
			tools = append(tools, t)
		}
	}
	return tools
}

// extractTool parses a single ToolEntry{mcp.NewTool(...), handler} element.
func extractTool(expr ast.Expr) Tool {
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return Tool{}
	}
	if len(cl.Elts) == 0 {
		return Tool{}
	}

	// The first element is the mcp.NewTool(...) call.
	call, ok := cl.Elts[0].(*ast.CallExpr)
	if !ok {
		return Tool{}
	}

	return parseNewToolCall(call)
}

// parseNewToolCall extracts the tool name, description, and parameters from
// a mcp.NewTool("name", opts...) call expression.
func parseNewToolCall(call *ast.CallExpr) Tool {
	if len(call.Args) == 0 {
		return Tool{}
	}

	var t Tool
	t.Name = unquote(call.Args[0])

	for _, arg := range call.Args[1:] {
		ac, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}
		fnName := selectorName(ac.Fun)
		switch fnName {
		case "mcp.WithDescription":
			if len(ac.Args) > 0 {
				t.Description = unquote(ac.Args[0])
			}
		case "mcp.WithString":
			t.Params = append(t.Params, parseParam(ac, "string"))
		case "mcp.WithNumber":
			t.Params = append(t.Params, parseParam(ac, "number"))
		case "mcp.WithBoolean":
			t.Params = append(t.Params, parseParam(ac, "boolean"))
		}
	}

	return t
}

// parseParam extracts a parameter definition from a mcp.WithString/Number/Boolean call.
func parseParam(call *ast.CallExpr, paramType string) Param {
	p := Param{Type: paramType}
	if len(call.Args) == 0 {
		return p
	}

	p.Name = unquote(call.Args[0])

	for _, arg := range call.Args[1:] {
		ac, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}
		fn := selectorName(ac.Fun)
		switch fn {
		case "mcp.Required":
			p.Required = true
		case "mcp.Description":
			if len(ac.Args) > 0 {
				p.Description = unquote(ac.Args[0])
			}
		}
	}

	return p
}

// identName returns the name of an identifier expression.
func identName(expr ast.Expr) string {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

// selectorName returns "pkg.Name" for a selector expression or "Name" for an ident.
func selectorName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		return identName(e.X) + "." + e.Sel.Name
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// unquote extracts a string literal value, handling basic concatenation.
func unquote(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			s := e.Value
			if len(s) >= 2 {
				if s[0] == '"' {
					s = s[1 : len(s)-1]
				} else if s[0] == '`' {
					s = s[1 : len(s)-1]
				}
			}
			return s
		}
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			return unquote(e.X) + unquote(e.Y)
		}
	}
	return ""
}

// mdTemplate is the markdown output template.
var mdTemplate = template.Must(template.New("mcp-docs").Funcs(template.FuncMap{
	"repeat": strings.Repeat,
	"requiredBadge": func(required bool) string {
		if required {
			return "**required**"
		}
		return "optional"
	},
}).Parse(`# MCP Tool API Reference

> Auto-generated by ` + "`" + `go run ./tools/genmcpdocs` + "`" + `. Do not edit manually.

**{{len .AllTools}} tools** across **{{len .Groups}} namespaces**.

## Table of Contents

{{range .Groups}}- [{{.Name}}](#{{.Name}}) - {{.Description}}
{{end}}
---
{{range .Groups}}
## {{.Name}}

{{.Description}}

{{range .Tools}}### ` + "`" + `{{.Name}}` + "`" + `

{{.Description}}
{{if .Params}}
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
{{- range .Params}}
| ` + "`" + `{{.Name}}` + "`" + ` | {{.Type}} | {{requiredBadge .Required}} | {{.Description}} |
{{- end}}
{{else}}
*No parameters.*
{{end}}
{{end}}---
{{end}}`))

// templateData wraps groups for the template.
type templateData struct {
	Groups   []Group
	AllTools []Tool
}

// Render writes the markdown documentation to w.
func Render(w io.Writer, groups []Group) error {
	var all []Tool
	for _, g := range groups {
		all = append(all, g.Tools...)
	}
	return mdTemplate.Execute(w, templateData{Groups: groups, AllTools: all})
}
