package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FieldType describes the expected JSON type for a field.
type FieldType int

const (
	FieldTypeString FieldType = iota
	FieldTypeNumber
	FieldTypeBool
	FieldTypeObject
	FieldTypeArray
	FieldTypeAny // any non-nil value
)

// String returns the human-readable name of the field type.
func (ft FieldType) String() string {
	switch ft {
	case FieldTypeString:
		return "string"
	case FieldTypeNumber:
		return "number"
	case FieldTypeBool:
		return "bool"
	case FieldTypeObject:
		return "object"
	case FieldTypeArray:
		return "array"
	case FieldTypeAny:
		return "any"
	default:
		return "unknown"
	}
}

// SchemaField describes a single expected field in a response schema.
type SchemaField struct {
	// Name is the JSON field name.
	Name string
	// Type is the expected JSON type.
	Type FieldType
	// Required indicates the field must be present.
	Required bool
}

// ResponseSchema defines the expected top-level structure of an LLM response.
type ResponseSchema struct {
	// Provider identifies which provider this schema applies to.
	Provider Provider
	// Name is a human-readable name for the schema (e.g., "claude_stream_event").
	Name string
	// Fields lists the expected top-level fields.
	Fields []SchemaField
}

// ValidationError represents a single field-level validation failure.
type ValidationError struct {
	// Field is the JSON path to the problematic field (e.g., "type", "usage.input_tokens").
	Field string
	// Message describes the validation failure.
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("field %q: %s", e.Field, e.Message)
}

// SchemaValidationResult holds the outcome of schema validation.
type SchemaValidationResult struct {
	// Valid is true when no errors were found.
	Valid bool
	// Errors lists all validation failures.
	Errors []ValidationError
}

// Error returns a combined error string, or empty string if valid.
func (r SchemaValidationResult) Error() string {
	if r.Valid {
		return ""
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// SchemaValidator validates parsed JSON responses against provider-specific schemas.
type SchemaValidator struct {
	schemas map[Provider][]ResponseSchema
}

// NewSchemaValidator creates a validator with the built-in provider schemas.
func NewSchemaValidator() *SchemaValidator {
	v := &SchemaValidator{
		schemas: make(map[Provider][]ResponseSchema),
	}
	v.registerDefaults()
	return v
}

// RegisterSchema adds a custom schema for the given provider.
func (v *SchemaValidator) RegisterSchema(schema ResponseSchema) {
	v.schemas[schema.Provider] = append(v.schemas[schema.Provider], schema)
}

// Validate checks a parsed JSON object against the schemas for the given provider.
// It tries each registered schema and returns the result for the best match
// (fewest errors). If no schemas are registered for the provider, the result is
// always valid.
func (v *SchemaValidator) Validate(provider Provider, data []byte) SchemaValidationResult {
	schemas, ok := v.schemas[provider]
	if !ok || len(schemas) == 0 {
		return SchemaValidationResult{Valid: true}
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return SchemaValidationResult{
			Valid: false,
			Errors: []ValidationError{{
				Field:   "<root>",
				Message: fmt.Sprintf("invalid JSON: %v", err),
			}},
		}
	}

	return v.validateAgainstSchemas(schemas, raw)
}

// ValidateMap is like Validate but accepts an already-parsed map.
func (v *SchemaValidator) ValidateMap(provider Provider, raw map[string]any) SchemaValidationResult {
	schemas, ok := v.schemas[provider]
	if !ok || len(schemas) == 0 {
		return SchemaValidationResult{Valid: true}
	}
	return v.validateAgainstSchemas(schemas, raw)
}

func (v *SchemaValidator) validateAgainstSchemas(schemas []ResponseSchema, raw map[string]any) SchemaValidationResult {
	var bestResult SchemaValidationResult
	bestResult.Valid = false
	bestResult.Errors = make([]ValidationError, 999) // sentinel: many errors

	for _, schema := range schemas {
		result := validateFields(schema.Fields, raw)
		if result.Valid {
			return result
		}
		if len(result.Errors) < len(bestResult.Errors) {
			bestResult = result
		}
	}

	return bestResult
}

func validateFields(fields []SchemaField, raw map[string]any) SchemaValidationResult {
	var errs []ValidationError

	for _, f := range fields {
		val, exists := raw[f.Name]
		if !exists {
			if f.Required {
				errs = append(errs, ValidationError{
					Field:   f.Name,
					Message: "required field missing",
				})
			}
			continue
		}

		if f.Type == FieldTypeAny {
			continue
		}

		if !checkType(val, f.Type) {
			errs = append(errs, ValidationError{
				Field:   f.Name,
				Message: fmt.Sprintf("expected type %s, got %T", f.Type, val),
			})
		}
	}

	if len(errs) == 0 {
		return SchemaValidationResult{Valid: true}
	}
	return SchemaValidationResult{Valid: false, Errors: errs}
}

func checkType(val any, expected FieldType) bool {
	switch expected {
	case FieldTypeString:
		_, ok := val.(string)
		return ok
	case FieldTypeNumber:
		switch val.(type) {
		case float64, json.Number:
			return true
		}
		return false
	case FieldTypeBool:
		_, ok := val.(bool)
		return ok
	case FieldTypeObject:
		_, ok := val.(map[string]any)
		return ok
	case FieldTypeArray:
		_, ok := val.([]any)
		return ok
	case FieldTypeAny:
		return true
	default:
		return false
	}
}

// registerDefaults adds the built-in schemas for Claude, Gemini, and Codex.
func (v *SchemaValidator) registerDefaults() {
	// Claude Code stream-json events always have a "type" field.
	// Common event types: tool_use, tool_result, content_block_start, result, system.
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderClaude,
		Name:     "claude_stream_event",
		Fields: []SchemaField{
			{Name: "type", Type: FieldTypeString, Required: true},
		},
	})

	// Claude result event with usage/cost.
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderClaude,
		Name:     "claude_result",
		Fields: []SchemaField{
			{Name: "type", Type: FieldTypeString, Required: true},
			{Name: "result", Type: FieldTypeString, Required: false},
			{Name: "cost_usd", Type: FieldTypeNumber, Required: false},
		},
	})

	// Gemini CLI JSON output has "candidates" or "text" fields.
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderGemini,
		Name:     "gemini_response",
		Fields: []SchemaField{
			{Name: "candidates", Type: FieldTypeArray, Required: false},
			{Name: "text", Type: FieldTypeString, Required: false},
		},
	})

	// Gemini with a role-based message structure.
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderGemini,
		Name:     "gemini_message",
		Fields: []SchemaField{
			{Name: "role", Type: FieldTypeString, Required: false},
			{Name: "content", Type: FieldTypeString, Required: false},
		},
	})

	// Codex CLI output includes a "type" and optional "output"/"message".
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderCodex,
		Name:     "codex_event",
		Fields: []SchemaField{
			{Name: "type", Type: FieldTypeString, Required: true},
		},
	})

	// Codex result with output field.
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderCodex,
		Name:     "codex_result",
		Fields: []SchemaField{
			{Name: "type", Type: FieldTypeString, Required: true},
			{Name: "output", Type: FieldTypeString, Required: false},
		},
	})
}
