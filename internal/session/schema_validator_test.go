package session

import (
	"testing"
)

func TestSchemaValidator_ClaudeValidEvent(t *testing.T) {
	v := NewSchemaValidator()
	data := []byte(`{"type": "tool_use", "name": "Write", "input": {"file_path": "main.go"}}`)
	result := v.Validate(ProviderClaude, data)

	if !result.Valid {
		t.Errorf("expected valid Claude event, got errors: %s", result.Error())
	}
}

func TestSchemaValidator_ClaudeMissingType(t *testing.T) {
	v := NewSchemaValidator()
	data := []byte(`{"name": "Write", "input": {"file_path": "main.go"}}`)
	result := v.Validate(ProviderClaude, data)

	if result.Valid {
		t.Error("expected validation failure for Claude event missing 'type'")
	}
	if len(result.Errors) == 0 {
		t.Fatal("expected at least one error")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "type" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error on field 'type', got: %v", result.Errors)
	}
}

func TestSchemaValidator_InvalidJSON(t *testing.T) {
	v := NewSchemaValidator()
	data := []byte(`{not valid json}`)
	result := v.Validate(ProviderClaude, data)

	if result.Valid {
		t.Error("expected validation failure for invalid JSON")
	}
	if len(result.Errors) != 1 || result.Errors[0].Field != "<root>" {
		t.Errorf("expected root-level JSON error, got: %v", result.Errors)
	}
}

func TestSchemaValidator_CodexValidEvent(t *testing.T) {
	v := NewSchemaValidator()
	data := []byte(`{"type": "message", "output": "done"}`)
	result := v.Validate(ProviderCodex, data)

	if !result.Valid {
		t.Errorf("expected valid Codex event, got errors: %s", result.Error())
	}
}

func TestSchemaValidator_GeminiNoSchemaMatch(t *testing.T) {
	v := NewSchemaValidator()
	// Gemini schemas have no required fields — any object is valid.
	data := []byte(`{"totally_unknown_field": 42}`)
	result := v.Validate(ProviderGemini, data)

	if !result.Valid {
		t.Errorf("expected valid Gemini event (no required fields), got errors: %s", result.Error())
	}
}

func TestSchemaValidator_UnknownProvider(t *testing.T) {
	v := NewSchemaValidator()
	data := []byte(`{"anything": "goes"}`)
	result := v.Validate(Provider("unknown"), data)

	if !result.Valid {
		t.Error("expected valid result for unknown provider (no schemas)")
	}
}

func TestSchemaValidator_ValidateMap(t *testing.T) {
	v := NewSchemaValidator()
	raw := map[string]any{"type": "result", "cost_usd": 0.05}
	result := v.ValidateMap(ProviderClaude, raw)

	if !result.Valid {
		t.Errorf("expected valid Claude map, got errors: %s", result.Error())
	}
}

func TestSchemaValidator_WrongType(t *testing.T) {
	v := NewSchemaValidator()
	// "type" should be a string, not a number.
	data := []byte(`{"type": 42}`)
	result := v.Validate(ProviderClaude, data)

	if result.Valid {
		t.Error("expected validation failure for wrong type")
	}
	found := false
	for _, e := range result.Errors {
		if e.Field == "type" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error on field 'type', got: %v", result.Errors)
	}
}

func TestSchemaValidator_RegisterCustomSchema(t *testing.T) {
	v := NewSchemaValidator()
	v.RegisterSchema(ResponseSchema{
		Provider: ProviderClaude,
		Name:     "custom",
		Fields: []SchemaField{
			{Name: "custom_field", Type: FieldTypeString, Required: true},
		},
	})

	data := []byte(`{"custom_field": "hello"}`)
	result := v.Validate(ProviderClaude, data)

	if !result.Valid {
		t.Errorf("expected valid with custom schema, got: %s", result.Error())
	}
}

func TestSchemaValidationResult_ErrorString(t *testing.T) {
	r := SchemaValidationResult{Valid: true}
	if r.Error() != "" {
		t.Errorf("expected empty error for valid result, got: %q", r.Error())
	}

	r = SchemaValidationResult{
		Valid: false,
		Errors: []ValidationError{
			{Field: "type", Message: "required field missing"},
			{Field: "data", Message: "expected type object, got string"},
		},
	}
	errStr := r.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}
}
