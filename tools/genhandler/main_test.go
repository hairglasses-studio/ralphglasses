package main

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	valid := []string{"scan", "merge_verify", "a1", "loop_start", "x"}
	for _, name := range valid {
		if err := validateName(name); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{
		"",            // empty
		"Scan",        // uppercase
		"merge-verify", // hyphen
		"1abc",        // starts with digit
		"_foo",        // starts with underscore
		"foo bar",     // space
		"foo.bar",     // dot
		"FOO",         // all caps
	}
	for _, name := range invalid {
		if err := validateName(name); err == nil {
			t.Errorf("validateName(%q) = nil, want error", name)
		}
	}
}

func TestTitleCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"scan", "Scan"},
		{"merge_verify", "MergeVerify"},
		{"loop_start_all", "LoopStartAll"},
		{"a", "A"},
	}
	for _, tt := range tests {
		got := titleCase(tt.in)
		if got != tt.want {
			t.Errorf("titleCase(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderHandler(t *testing.T) {
	t.Parallel()

	data := templateData{
		Name:      "test_handler",
		TitleName: titleCase("test_handler"),
		ToolName:  "ralphglasses_test_handler",
	}

	content, err := renderTemplate(handlerTmpl, data)
	if err != nil {
		t.Fatalf("renderTemplate(handler): %v", err)
	}

	// Verify the generated code is syntactically valid Go.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "handler_test_handler.go", content, parser.AllErrors); err != nil {
		t.Fatalf("generated handler is not valid Go:\n%s\nerror: %v", content, err)
	}
}

func TestRenderTest(t *testing.T) {
	t.Parallel()

	data := templateData{
		Name:      "test_handler",
		TitleName: titleCase("test_handler"),
		ToolName:  "ralphglasses_test_handler",
	}

	content, err := renderTemplate(testTmpl, data)
	if err != nil {
		t.Fatalf("renderTemplate(test): %v", err)
	}

	// Verify the generated test code is syntactically valid Go.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "handler_test_handler_test.go", content, parser.AllErrors); err != nil {
		t.Fatalf("generated test is not valid Go:\n%s\nerror: %v", content, err)
	}
}

func TestRenderTemplateContainsExpectedStrings(t *testing.T) {
	t.Parallel()

	data := templateData{
		Name:      "my_widget",
		TitleName: titleCase("my_widget"),
		ToolName:  "ralphglasses_my_widget",
	}

	handler, err := renderTemplate(handlerTmpl, data)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"package mcpserver",
		"handleMyWidget",
		"ralphglasses_my_widget",
		"buildMyWidgetTool",
		"mcp.CallToolRequest",
	} {
		if !contains(handler, want) {
			t.Errorf("handler output missing %q", want)
		}
	}

	test, err := renderTemplate(testTmpl, data)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"package mcpserver",
		"TestHandleMyWidget",
		"handleMyWidget",
	} {
		if !contains(test, want) {
			t.Errorf("test output missing %q", want)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
