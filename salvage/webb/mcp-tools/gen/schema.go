// Package gen provides YAML-based tool definition generation for webb MCP tools.
// Tools are defined in YAML files and Go code is generated from them.
package gen

// ModuleDefinition represents a complete module in YAML format
type ModuleDefinition struct {
	Name        string           `yaml:"name"`
	Package     string           `yaml:"package"`
	Description string           `yaml:"description"`
	Imports     []string         `yaml:"imports,omitempty"`
	Tools       []ToolDefinition `yaml:"tools"`
}

// ToolDefinition represents a single tool in YAML format
type ToolDefinition struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Handler     string            `yaml:"handler"`
	Category    string            `yaml:"category"`
	Subcategory string            `yaml:"subcategory,omitempty"`
	Tags        []string          `yaml:"tags,omitempty"`
	UseCases    []string          `yaml:"use_cases,omitempty"`
	Complexity  string            `yaml:"complexity,omitempty"` // simple, moderate, complex
	IsWrite     bool              `yaml:"is_write,omitempty"`
	Parameters  []ParamDefinition `yaml:"parameters,omitempty"`
}

// ParamDefinition represents a tool parameter in YAML format
type ParamDefinition struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"` // string, number, boolean, array, object
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description"`
	Default     string `yaml:"default,omitempty"`
}

// ComplexityFromString converts a string to a Complexity constant name
func ComplexityFromString(s string) string {
	switch s {
	case "simple":
		return "tools.ComplexitySimple"
	case "moderate":
		return "tools.ComplexityModerate"
	case "complex":
		return "tools.ComplexityComplex"
	default:
		return "tools.ComplexitySimple"
	}
}

// TypeToMCPMethod returns the mcp-go method for creating a parameter of the given type
func TypeToMCPMethod(t string) string {
	switch t {
	case "string":
		return "mcp.WithString"
	case "number":
		return "mcp.WithNumber"
	case "boolean":
		return "mcp.WithBoolean"
	case "array":
		return "mcp.WithArray"
	case "object":
		return "mcp.WithObject"
	default:
		return "mcp.WithString"
	}
}
