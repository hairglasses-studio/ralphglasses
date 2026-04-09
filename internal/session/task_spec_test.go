package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskSpecValidate_Valid(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-001",
		Name:                "fix null pointer in parser",
		Description:         "Handle nil input in ParseConfig to avoid panic",
		Type:                TaskBugfix,
		Priority:            PriorityP1,
		EstimatedComplexity: ComplexityS,
	}
	if err := ts.Validate(); err != nil {
		t.Fatalf("expected valid spec, got error: %v", err)
	}
}

func TestTaskSpecValidate_MissingID(t *testing.T) {
	ts := TaskSpec{
		Name:                "some task",
		Description:         "does something",
		Type:                TaskFeature,
		Priority:            PriorityP2,
		EstimatedComplexity: ComplexityM,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("error %q should mention missing id", err)
	}
}

func TestTaskSpecValidate_MissingName(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-002",
		Description:         "does something",
		Type:                TaskFeature,
		Priority:            PriorityP2,
		EstimatedComplexity: ComplexityM,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error %q should mention missing name", err)
	}
}

func TestTaskSpecValidate_MissingDescription(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-003",
		Name:                "add feature X",
		Type:                TaskFeature,
		Priority:            PriorityP1,
		EstimatedComplexity: ComplexityL,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for missing description")
	}
	if !strings.Contains(err.Error(), "description is required") {
		t.Errorf("error %q should mention missing description", err)
	}
}

func TestTaskSpecValidate_MissingType(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-004",
		Name:                "something",
		Description:         "details",
		Priority:            PriorityP0,
		EstimatedComplexity: ComplexityS,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	if !strings.Contains(err.Error(), "type is required") {
		t.Errorf("error %q should mention missing type", err)
	}
}

func TestTaskSpecValidate_InvalidType(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-005",
		Name:                "something",
		Description:         "details",
		Type:                TaskType("invalid"),
		Priority:            PriorityP1,
		EstimatedComplexity: ComplexityM,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), `invalid type "invalid"`) {
		t.Errorf("error %q should mention invalid type", err)
	}
}

func TestTaskSpecValidate_MissingPriority(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-006",
		Name:                "something",
		Description:         "details",
		Type:                TaskRefactor,
		EstimatedComplexity: ComplexityM,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for missing priority")
	}
	if !strings.Contains(err.Error(), "priority is required") {
		t.Errorf("error %q should mention missing priority", err)
	}
}

func TestTaskSpecValidate_InvalidPriority(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-007",
		Name:                "something",
		Description:         "details",
		Type:                TaskTest,
		Priority:            Priority("P9"),
		EstimatedComplexity: ComplexityS,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
	if !strings.Contains(err.Error(), `invalid priority "P9"`) {
		t.Errorf("error %q should mention invalid priority", err)
	}
}

func TestTaskSpecValidate_MissingComplexity(t *testing.T) {
	ts := TaskSpec{
		ID:          "task-008",
		Name:        "something",
		Description: "details",
		Type:        TaskDocs,
		Priority:    PriorityP3,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for missing complexity")
	}
	if !strings.Contains(err.Error(), "estimated_complexity is required") {
		t.Errorf("error %q should mention missing complexity", err)
	}
}

func TestTaskSpecValidate_InvalidComplexity(t *testing.T) {
	ts := TaskSpec{
		ID:                  "task-009",
		Name:                "something",
		Description:         "details",
		Type:                TaskResearch,
		Priority:            PriorityP2,
		EstimatedComplexity: Complexity("XXL"),
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for invalid complexity")
	}
	if !strings.Contains(err.Error(), `invalid estimated_complexity "XXL"`) {
		t.Errorf("error %q should mention invalid complexity", err)
	}
}

func TestTaskSpecValidate_MultipleErrors(t *testing.T) {
	ts := TaskSpec{} // all fields missing
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for empty spec")
	}
	msg := err.Error()
	for _, want := range []string{"id is required", "name is required", "description is required", "type is required", "priority is required", "estimated_complexity is required"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should contain %q", msg, want)
		}
	}
}

func TestTaskTypeConstants(t *testing.T) {
	types := []struct {
		tt   TaskType
		want string
	}{
		{TaskBugfix, "bugfix"},
		{TaskFeature, "feature"},
		{TaskRefactor, "refactor"},
		{TaskTest, "test"},
		{TaskDocs, "docs"},
		{TaskResearch, "research"},
		{TaskPublish, "publish"},
	}
	for _, tc := range types {
		if string(tc.tt) != tc.want {
			t.Errorf("TaskType %v = %q, want %q", tc.tt, string(tc.tt), tc.want)
		}
		if !validTaskTypes[tc.tt] {
			t.Errorf("TaskType %q not in validTaskTypes", tc.tt)
		}
	}
}

func TestPriorityConstants(t *testing.T) {
	priorities := []struct {
		p    Priority
		want string
	}{
		{PriorityP0, "P0"},
		{PriorityP1, "P1"},
		{PriorityP2, "P2"},
		{PriorityP3, "P3"},
	}
	for _, tc := range priorities {
		if string(tc.p) != tc.want {
			t.Errorf("Priority %v = %q, want %q", tc.p, string(tc.p), tc.want)
		}
		if !validPriorities[tc.p] {
			t.Errorf("Priority %q not in validPriorities", tc.p)
		}
	}
}

func TestComplexityConstants(t *testing.T) {
	complexities := []struct {
		c    Complexity
		want string
	}{
		{ComplexityS, "S"},
		{ComplexityM, "M"},
		{ComplexityL, "L"},
		{ComplexityXL, "XL"},
	}
	for _, tc := range complexities {
		if string(tc.c) != tc.want {
			t.Errorf("Complexity %v = %q, want %q", tc.c, string(tc.c), tc.want)
		}
		if !validComplexities[tc.c] {
			t.Errorf("Complexity %q not in validComplexities", tc.c)
		}
	}
}

func TestTaskSpecJSONRoundTrip(t *testing.T) {
	original := TaskSpec{
		ID:          "ts-round-trip",
		Name:        "implement caching layer",
		Description: "Add an LRU cache in front of the database queries",
		Type:        TaskFeature,
		Priority:    PriorityP1,
		Inputs: []TaskInput{
			{Type: "file", Value: "internal/db/query.go", Required: true},
			{Type: "context", Value: "Current p99 latency is 200ms", Required: false},
			{Type: "constraint", Value: "Must not break existing API", Required: true},
		},
		Outputs: []TaskOutput{
			{Type: "file", Path: "internal/cache/lru.go", Validation: "go build ./..."},
			{Type: "test", Path: "internal/cache/lru_test.go", Validation: "go test ./internal/cache/..."},
			{Type: "doc", Path: "docs/caching.md"},
		},
		Dependencies:        []string{"ts-setup-001", "ts-schema-002"},
		EstimatedComplexity: ComplexityL,
		Tags:                []string{"performance", "database", "sprint-7"},
		Metadata: map[string]string{
			"roadmap_item": "6.1.2",
			"assignee":     "agent-alpha",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded TaskSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Verify all fields survive the round trip.
	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, original.Description)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
	if decoded.Priority != original.Priority {
		t.Errorf("Priority = %q, want %q", decoded.Priority, original.Priority)
	}
	if decoded.EstimatedComplexity != original.EstimatedComplexity {
		t.Errorf("EstimatedComplexity = %q, want %q", decoded.EstimatedComplexity, original.EstimatedComplexity)
	}
	if len(decoded.Inputs) != len(original.Inputs) {
		t.Fatalf("Inputs len = %d, want %d", len(decoded.Inputs), len(original.Inputs))
	}
	for i, inp := range decoded.Inputs {
		if inp != original.Inputs[i] {
			t.Errorf("Inputs[%d] = %+v, want %+v", i, inp, original.Inputs[i])
		}
	}
	if len(decoded.Outputs) != len(original.Outputs) {
		t.Fatalf("Outputs len = %d, want %d", len(decoded.Outputs), len(original.Outputs))
	}
	for i, out := range decoded.Outputs {
		if out != original.Outputs[i] {
			t.Errorf("Outputs[%d] = %+v, want %+v", i, out, original.Outputs[i])
		}
	}
	if len(decoded.Dependencies) != len(original.Dependencies) {
		t.Fatalf("Dependencies len = %d, want %d", len(decoded.Dependencies), len(original.Dependencies))
	}
	for i, dep := range decoded.Dependencies {
		if dep != original.Dependencies[i] {
			t.Errorf("Dependencies[%d] = %q, want %q", i, dep, original.Dependencies[i])
		}
	}
	if len(decoded.Tags) != len(original.Tags) {
		t.Fatalf("Tags len = %d, want %d", len(decoded.Tags), len(original.Tags))
	}
	for i, tag := range decoded.Tags {
		if tag != original.Tags[i] {
			t.Errorf("Tags[%d] = %q, want %q", i, tag, original.Tags[i])
		}
	}
	if len(decoded.Metadata) != len(original.Metadata) {
		t.Fatalf("Metadata len = %d, want %d", len(decoded.Metadata), len(original.Metadata))
	}
	for k, v := range original.Metadata {
		if decoded.Metadata[k] != v {
			t.Errorf("Metadata[%q] = %q, want %q", k, decoded.Metadata[k], v)
		}
	}

	// Validate the decoded spec passes validation.
	if err := decoded.Validate(); err != nil {
		t.Errorf("decoded spec should be valid: %v", err)
	}
}

func TestTaskSpecJSONRoundTrip_Minimal(t *testing.T) {
	original := TaskSpec{
		ID:                  "ts-minimal",
		Name:                "quick fix",
		Description:         "patch a typo",
		Type:                TaskBugfix,
		Priority:            PriorityP3,
		EstimatedComplexity: ComplexityS,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded TaskSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Type != original.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, original.Type)
	}
	if decoded.Inputs != nil {
		t.Errorf("Inputs should be nil for minimal spec, got %v", decoded.Inputs)
	}
	if decoded.Outputs != nil {
		t.Errorf("Outputs should be nil for minimal spec, got %v", decoded.Outputs)
	}
	if decoded.Dependencies != nil {
		t.Errorf("Dependencies should be nil for minimal spec, got %v", decoded.Dependencies)
	}
	if decoded.Tags != nil {
		t.Errorf("Tags should be nil for minimal spec, got %v", decoded.Tags)
	}
	if decoded.Metadata != nil {
		t.Errorf("Metadata should be nil for minimal spec, got %v", decoded.Metadata)
	}
}

func TestTaskSpecValidate_WhitespaceOnlyFields(t *testing.T) {
	ts := TaskSpec{
		ID:                  "   ",
		Name:                "\t",
		Description:         " \n ",
		Type:                TaskFeature,
		Priority:            PriorityP1,
		EstimatedComplexity: ComplexityM,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for whitespace-only fields")
	}
	msg := err.Error()
	if !strings.Contains(msg, "id is required") {
		t.Errorf("error %q should mention id", msg)
	}
	if !strings.Contains(msg, "name is required") {
		t.Errorf("error %q should mention name", msg)
	}
	if !strings.Contains(msg, "description is required") {
		t.Errorf("error %q should mention description", msg)
	}
}

func TestTaskSpecValidate_ValidWithOptionalFields(t *testing.T) {
	ts := TaskSpec{
		ID:          "task-full",
		Name:        "add integration tests",
		Description: "Write integration tests for the API endpoints",
		Type:        TaskTest,
		Priority:    PriorityP0,
		Inputs: []TaskInput{
			{Type: "file", Value: "internal/api/handler.go", Required: true},
		},
		Outputs: []TaskOutput{
			{Type: "test", Path: "internal/api/handler_test.go", Validation: "go test -v ./internal/api/..."},
		},
		Dependencies:        []string{"task-api-001"},
		EstimatedComplexity: ComplexityM,
		Tags:                []string{"testing", "api"},
		Metadata:            map[string]string{"sprint": "7"},
	}
	if err := ts.Validate(); err != nil {
		t.Fatalf("expected valid spec with optional fields, got error: %v", err)
	}
}
