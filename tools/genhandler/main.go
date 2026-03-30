// genhandler scaffolds new MCP handler files for the ralphglasses mcpserver package.
//
// Usage:
//
//	go run tools/genhandler/main.go <handler-name>
//
// The handler name must be lowercase alphanumeric (with optional underscores).
// It generates two files:
//
//	internal/mcpserver/handler_<name>.go
//	internal/mcpserver/handler_<name>_test.go
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

// validName matches lowercase letters, digits, and underscores. Must start with a letter.
var validName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validateName checks that the handler name is well-formed.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("handler name must not be empty")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("handler name %q is invalid: must match [a-z][a-z0-9_]*", name)
	}
	return nil
}

// titleCase converts a snake_case name to TitleCase (e.g. "merge_verify" -> "MergeVerify").
func titleCase(name string) string {
	parts := strings.Split(name, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// templateData holds the values injected into handler templates.
type templateData struct {
	Name      string // raw name, e.g. "merge_verify"
	TitleName string // TitleCase, e.g. "MergeVerify"
	ToolName  string // prefixed tool name, e.g. "ralphglasses_merge_verify"
}

var handlerTmpl = template.Must(template.New("handler").Parse(`package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handle{{.TitleName}} implements the {{.ToolName}} tool.
func (s *Server) handle{{.TitleName}}(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: implement handler logic.
	_ = req
	return jsonResult(map[string]any{
		"status": "ok",
	}), nil
}

// build{{.TitleName}}Tool returns the tool definition and handler for {{.ToolName}}.
func (s *Server) build{{.TitleName}}Tool() ToolEntry {
	return ToolEntry{
		mcp.NewTool("{{.ToolName}}",
			mcp.WithDescription("TODO: describe {{.ToolName}}"),
		),
		s.handle{{.TitleName}},
	}
}
`))

var testTmpl = template.Must(template.New("test").Parse(`package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandle{{.TitleName}}(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handle{{.TitleName}}(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success result")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}
`))

// renderTemplate executes a template into a string, returning an error on failure.
func renderTemplate(tmpl *template.Template, data templateData) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execute: %w", err)
	}
	return buf.String(), nil
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: go run tools/genhandler/main.go <handler-name>")
	}
	name := os.Args[1]

	if err := validateName(name); err != nil {
		return err
	}

	data := templateData{
		Name:      name,
		TitleName: titleCase(name),
		ToolName:  "ralphglasses_" + name,
	}

	handlerContent, err := renderTemplate(handlerTmpl, data)
	if err != nil {
		return fmt.Errorf("render handler: %w", err)
	}

	testContent, err := renderTemplate(testTmpl, data)
	if err != nil {
		return fmt.Errorf("render test: %w", err)
	}

	dir := filepath.Join("internal", "mcpserver")
	handlerPath := filepath.Join(dir, fmt.Sprintf("handler_%s.go", name))
	testPath := filepath.Join(dir, fmt.Sprintf("handler_%s_test.go", name))

	// Check that target directory exists.
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("target directory %s does not exist: %w", dir, err)
	}

	// Refuse to overwrite existing files.
	for _, p := range []string{handlerPath, testPath} {
		if _, err := os.Stat(p); err == nil {
			return fmt.Errorf("file already exists: %s", p)
		}
	}

	if err := os.WriteFile(handlerPath, []byte(handlerContent), 0644); err != nil {
		return fmt.Errorf("write handler: %w", err)
	}
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		return fmt.Errorf("write test: %w", err)
	}

	fmt.Println(handlerPath)
	fmt.Println(testPath)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "genhandler: %s\n", err)
		os.Exit(1)
	}
}
