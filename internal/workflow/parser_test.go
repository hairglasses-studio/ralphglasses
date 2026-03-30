package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse_ValidWorkflow(t *testing.T) {
	input := `
name: test-workflow
steps:
  - name: step1
    type: shell_exec
    command: "echo hello"
  - name: step2
    type: shell_exec
    command: "echo world"
    depends_on: [step1]
    timeout: "30s"
    retry_count: 3
    retry_delay: "2s"
    params:
      key: value
`
	wf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "test-workflow" {
		t.Errorf("name = %q, want %q", wf.Name, "test-workflow")
	}
	if len(wf.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(wf.Steps))
	}

	s := wf.Steps[1]
	if s.Name != "step2" {
		t.Errorf("step name = %q, want %q", s.Name, "step2")
	}
	if s.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", s.Timeout)
	}
	if s.RetryCount != 3 {
		t.Errorf("retry_count = %d, want 3", s.RetryCount)
	}
	if s.RetryDelay != 2*time.Second {
		t.Errorf("retry_delay = %v, want 2s", s.RetryDelay)
	}
	if len(s.DependsOn) != 1 || s.DependsOn[0] != "step1" {
		t.Errorf("depends_on = %v, want [step1]", s.DependsOn)
	}
	if s.Params["key"] != "value" {
		t.Errorf("params[key] = %q, want %q", s.Params["key"], "value")
	}
}

func TestParse_EmptyName(t *testing.T) {
	input := `
name: ""
steps:
  - name: step1
    type: shell_exec
    command: "echo hello"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for empty workflow name")
	}
}

func TestParse_MissingName(t *testing.T) {
	input := `
steps:
  - name: step1
    type: shell_exec
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for missing workflow name")
	}
}

func TestParse_NoSteps(t *testing.T) {
	input := `
name: empty-workflow
steps: []
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for workflow with no steps")
	}
}

func TestParse_StepMissingName(t *testing.T) {
	input := `
name: test
steps:
  - type: shell_exec
    command: "echo hello"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for step with no name")
	}
}

func TestParse_DuplicateStepName(t *testing.T) {
	input := `
name: test
steps:
  - name: dup
    type: shell_exec
    command: "echo 1"
  - name: dup
    type: shell_exec
    command: "echo 2"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for duplicate step name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want 'duplicate' mentioned", err.Error())
	}
}

func TestParse_InvalidTimeout(t *testing.T) {
	input := `
name: test
steps:
  - name: step1
    type: shell_exec
    command: "echo"
    timeout: "not-a-duration"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error = %q, want 'timeout' mentioned", err.Error())
	}
}

func TestParse_InvalidRetryDelay(t *testing.T) {
	input := `
name: test
steps:
  - name: step1
    type: shell_exec
    command: "echo"
    retry_delay: "bad"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid retry_delay")
	}
	if !strings.Contains(err.Error(), "retry_delay") {
		t.Errorf("error = %q, want 'retry_delay' mentioned", err.Error())
	}
}

func TestParse_DefaultTypeFromCommand(t *testing.T) {
	input := `
name: test
steps:
  - name: auto-type
    command: "echo hello"
`
	wf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Steps[0].Type != "shell_exec" {
		t.Errorf("type = %q, want %q (auto-inferred from command)", wf.Steps[0].Type, "shell_exec")
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	input := `not: valid: yaml: [[[`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParse_UnknownField(t *testing.T) {
	input := `
name: test
steps:
  - name: step1
    type: shell_exec
    bogus_field: "should fail"
`
	_, err := Parse(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for unknown field (KnownFields=true)")
	}
}

func TestParseBytes_Valid(t *testing.T) {
	data := []byte(`
name: bytes-test
steps:
  - name: s1
    type: shell_exec
    command: "echo ok"
`)
	wf, err := ParseBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "bytes-test" {
		t.Errorf("name = %q, want %q", wf.Name, "bytes-test")
	}
}

func TestParseBytes_InvalidYAML(t *testing.T) {
	_, err := ParseBytes([]byte(`{{{`))
	if err == nil {
		t.Fatal("expected error for invalid YAML bytes")
	}
}

func TestParseFile_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	content := `
name: file-test
steps:
  - name: s1
    type: shell_exec
    command: "echo file"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	wf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "file-test" {
		t.Errorf("name = %q, want %q", wf.Name, "file-test")
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/to/workflow.yaml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestParse_Condition(t *testing.T) {
	input := `
name: cond-test
steps:
  - name: s1
    type: shell_exec
    command: "echo"
    condition: "env == production"
`
	wf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Steps[0].Condition != "env == production" {
		t.Errorf("condition = %q, want %q", wf.Steps[0].Condition, "env == production")
	}
}

func TestParse_MultipleStepsWithDeps(t *testing.T) {
	input := `
name: pipeline
steps:
  - name: build
    type: shell_exec
    command: "make build"
  - name: test
    type: test_run
    depends_on: [build]
    params:
      package: "./..."
      race: "true"
  - name: deploy
    type: deploy
    depends_on: [test]
    params:
      target: staging
`
	wf, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wf.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(wf.Steps))
	}
	if wf.Steps[1].Type != "test_run" {
		t.Errorf("step 1 type = %q, want %q", wf.Steps[1].Type, "test_run")
	}
	if wf.Steps[2].Params["target"] != "staging" {
		t.Errorf("deploy target = %q, want %q", wf.Steps[2].Params["target"], "staging")
	}
}
