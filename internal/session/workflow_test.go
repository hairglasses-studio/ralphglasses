package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateWorkflow(t *testing.T) {
	tests := []struct {
		name string
		wf   WorkflowDef
		want string
	}{
		{
			name: "missing name",
			wf: WorkflowDef{
				Steps: []WorkflowStep{{Name: "step1", Prompt: "do it"}},
			},
			want: "workflow name required",
		},
		{
			name: "duplicate steps",
			wf: WorkflowDef{
				Name: "dup",
				Steps: []WorkflowStep{
					{Name: "step1", Prompt: "a"},
					{Name: "step1", Prompt: "b"},
				},
			},
			want: "must be unique",
		},
		{
			name: "missing dependency",
			wf: WorkflowDef{
				Name: "deps",
				Steps: []WorkflowStep{
					{Name: "step1", Prompt: "a", DependsOn: []string{"missing"}},
				},
			},
			want: "depends on unknown step",
		},
		{
			name: "cycle",
			wf: WorkflowDef{
				Name: "cycle",
				Steps: []WorkflowStep{
					{Name: "step1", Prompt: "a", DependsOn: []string{"step2"}},
					{Name: "step2", Prompt: "b", DependsOn: []string{"step1"}},
				},
			},
			want: "dependency cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkflow(tt.wf)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateWorkflow error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestManagerRunWorkflowSequentialFailureBlocksDependents(t *testing.T) {
	m := NewManager()
	m.launchSession = func(_ context.Context, opts LaunchOptions) (*Session, error) {
		status := StatusCompleted
		if opts.Prompt == "step2" {
			status = StatusErrored
		}
		return &Session{
			ID:         opts.Prompt,
			Provider:   opts.Provider,
			RepoPath:   opts.RepoPath,
			RepoName:   "repo",
			Prompt:     opts.Prompt,
			Status:     status,
			OutputCh:   make(chan string, 1),
			LaunchedAt: time.Now(),
		}, nil
	}
	m.waitSession = func(_ context.Context, s *Session) error {
		if s.Status == StatusErrored {
			return errors.New("boom")
		}
		return nil
	}

	run, err := m.RunWorkflow(context.Background(), "/tmp/repo", WorkflowDef{
		Name: "ship-it",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: "step1"},
			{Name: "step2", Prompt: "step2", DependsOn: []string{"step1"}},
			{Name: "step3", Prompt: "step3", DependsOn: []string{"step2"}},
		},
	})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}

	waitForWorkflowStatus(t, run, "failed")

	run.Lock()
	defer run.Unlock()

	if run.Steps[0].Status != "completed" {
		t.Fatalf("step1 status = %q", run.Steps[0].Status)
	}
	if run.Steps[1].Status != "failed" {
		t.Fatalf("step2 status = %q", run.Steps[1].Status)
	}
	if run.Steps[2].Status != "blocked" {
		t.Fatalf("step3 status = %q", run.Steps[2].Status)
	}
}

func TestManagerRunWorkflowParallelGroup(t *testing.T) {
	m := NewManager()
	var mu sync.Mutex
	active := 0
	maxActive := 0

	m.launchSession = func(_ context.Context, opts LaunchOptions) (*Session, error) {
		return &Session{
			ID:         opts.Prompt,
			Provider:   opts.Provider,
			RepoPath:   opts.RepoPath,
			RepoName:   "repo",
			Prompt:     opts.Prompt,
			Status:     StatusRunning,
			OutputCh:   make(chan string, 1),
			LaunchedAt: time.Now(),
		}, nil
	}
	m.waitSession = func(_ context.Context, s *Session) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(25 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()

		s.Lock()
		s.Status = StatusCompleted
		s.Unlock()
		return nil
	}

	run, err := m.RunWorkflow(context.Background(), "/tmp/repo", WorkflowDef{
		Name: "parallel",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: "step1", Parallel: true},
			{Name: "step2", Prompt: "step2", Parallel: true},
		},
	})
	if err != nil {
		t.Fatalf("RunWorkflow: %v", err)
	}

	waitForWorkflowStatus(t, run, "completed")

	if maxActive < 2 {
		t.Fatalf("maxActive = %d, want parallel execution", maxActive)
	}
}

func TestParseWorkflow_Valid(t *testing.T) {
	yaml := `
name: deploy-pipeline
steps:
  - name: lint
    prompt: "Run linting checks"
  - name: test
    prompt: "Run test suite"
    depends_on: [lint]
  - name: deploy
    prompt: "Deploy to staging"
    depends_on: [test]
`
	wf, err := ParseWorkflow("", []byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "deploy-pipeline" {
		t.Errorf("name = %q, want deploy-pipeline", wf.Name)
	}
	if len(wf.Steps) != 3 {
		t.Errorf("steps = %d, want 3", len(wf.Steps))
	}
}

func TestParseWorkflow_DefaultName(t *testing.T) {
	yaml := `
steps:
  - name: step1
    prompt: "Do something"
`
	wf, err := ParseWorkflow("fallback-name", []byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.Name != "fallback-name" {
		t.Errorf("name = %q, want fallback-name", wf.Name)
	}
}

func TestParseWorkflow_InvalidYAML(t *testing.T) {
	_, err := ParseWorkflow("bad", []byte(":::invalid"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestSaveLoadDeleteWorkflow(t *testing.T) {
	dir := t.TempDir()

	wf := WorkflowDef{
		Name: "test workflow",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: "Do thing"},
		},
	}

	if err := SaveWorkflow(dir, wf); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}

	loaded, err := LoadWorkflow(dir, "test workflow")
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	if loaded.Name != "test workflow" {
		t.Errorf("loaded name = %q", loaded.Name)
	}
	if len(loaded.Steps) != 1 {
		t.Errorf("loaded steps = %d, want 1", len(loaded.Steps))
	}

	if err := DeleteWorkflow(dir, "test workflow"); err != nil {
		t.Fatalf("DeleteWorkflow: %v", err)
	}

	_, err = LoadWorkflow(dir, "test workflow")
	if err == nil {
		t.Fatal("expected error loading deleted workflow")
	}
}

func TestListWorkflows(t *testing.T) {
	dir := t.TempDir()

	// Empty dir — no error
	wfs, err := ListWorkflows(dir)
	if err != nil {
		t.Fatalf("ListWorkflows empty: %v", err)
	}
	if wfs != nil {
		t.Errorf("expected nil for no workflows dir, got %v", wfs)
	}

	// Save two workflows
	_ = SaveWorkflow(dir, WorkflowDef{Name: "wf1", Steps: []WorkflowStep{{Name: "s1", Prompt: "p1"}}})
	_ = SaveWorkflow(dir, WorkflowDef{Name: "wf2", Steps: []WorkflowStep{{Name: "s2", Prompt: "p2"}}})

	wfs, err = ListWorkflows(dir)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(wfs) != 2 {
		t.Errorf("ListWorkflows = %d, want 2", len(wfs))
	}
}

func TestDeleteWorkflow_NotFound(t *testing.T) {
	dir := t.TempDir()
	err := DeleteWorkflow(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent workflow")
	}
}

func TestWorkflowRun_StepByName(t *testing.T) {
	run := newWorkflowRun("/tmp", WorkflowDef{
		Name: "test",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: "do 1"},
			{Name: "step2", Prompt: "do 2"},
		},
	})

	step := run.stepByName("step1")
	if step == nil {
		t.Fatal("expected step1 to be found")
	}
	if step.Name != "step1" {
		t.Errorf("name = %q, want step1", step.Name)
	}

	missing := run.stepByName("nonexistent")
	if missing != nil {
		t.Errorf("expected nil for missing step, got %+v", missing)
	}
}

func TestWorkflowRun_UpdateStep(t *testing.T) {
	run := newWorkflowRun("/tmp", WorkflowDef{
		Name: "test",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: "do 1"},
		},
	})

	// Update with mutate func
	run.updateStep("step1", "running", func(s *WorkflowStepResult) {
		s.SessionID = "sess-123"
	})

	run.Lock()
	if run.Steps[0].Status != "running" {
		t.Errorf("step status = %q, want running", run.Steps[0].Status)
	}
	if run.Steps[0].SessionID != "sess-123" {
		t.Errorf("session ID = %q, want sess-123", run.Steps[0].SessionID)
	}
	run.Unlock()

	// Update with nil mutate
	run.updateStep("step1", "completed", nil)
	run.Lock()
	if run.Steps[0].Status != "completed" {
		t.Errorf("step status = %q, want completed", run.Steps[0].Status)
	}
	run.Unlock()

	// Update nonexistent step - should not panic
	run.updateStep("nonexistent", "failed", nil)
}

func TestWorkflowRun_SetStatus(t *testing.T) {
	run := newWorkflowRun("/tmp", WorkflowDef{
		Name:  "test",
		Steps: []WorkflowStep{{Name: "s1", Prompt: "p1"}},
	})
	run.setStatus("running")
	run.Lock()
	if run.Status != "running" {
		t.Errorf("status = %q, want running", run.Status)
	}
	run.Unlock()
}

func TestNewWorkflowRun(t *testing.T) {
	wf := WorkflowDef{
		Name: "deploy",
		Steps: []WorkflowStep{
			{Name: "build", Prompt: "build it"},
			{Name: "test", Prompt: "test it"},
			{Name: "deploy", Prompt: "ship it"},
		},
	}
	run := newWorkflowRun("/tmp/repo", wf)

	if run.Name != "deploy" {
		t.Errorf("name = %q, want deploy", run.Name)
	}
	if run.RepoPath != "/tmp/repo" {
		t.Errorf("repo = %q", run.RepoPath)
	}
	if run.Status != "pending" {
		t.Errorf("status = %q, want pending", run.Status)
	}
	if len(run.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(run.Steps))
	}
	for _, step := range run.Steps {
		if step.Status != "pending" {
			t.Errorf("step %q status = %q, want pending", step.Name, step.Status)
		}
	}
	if run.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestValidateWorkflow_EmptyStepName(t *testing.T) {
	wf := WorkflowDef{
		Name: "test",
		Steps: []WorkflowStep{
			{Name: "", Prompt: "no name"},
		},
	}
	err := ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "step name required") {
		t.Errorf("expected step name required error, got: %v", err)
	}
}

func TestValidateWorkflow_EmptyStepPrompt(t *testing.T) {
	wf := WorkflowDef{
		Name: "test",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: ""},
		},
	}
	err := ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "prompt required") {
		t.Errorf("expected prompt required error, got: %v", err)
	}
}

func TestValidateWorkflow_UnknownProvider(t *testing.T) {
	wf := WorkflowDef{
		Name: "test",
		Steps: []WorkflowStep{
			{Name: "step1", Prompt: "do it", Provider: "unknown_provider"},
		},
	}
	err := ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("expected unknown provider error, got: %v", err)
	}
}

func TestValidateWorkflow_NoSteps(t *testing.T) {
	wf := WorkflowDef{
		Name:  "empty",
		Steps: nil,
	}
	err := ValidateWorkflow(wf)
	if err == nil || !strings.Contains(err.Error(), "at least one step") {
		t.Errorf("expected at least one step error, got: %v", err)
	}
}

func TestValidateWorkflow_ValidDAG(t *testing.T) {
	wf := WorkflowDef{
		Name: "valid",
		Steps: []WorkflowStep{
			{Name: "a", Prompt: "p1"},
			{Name: "b", Prompt: "p2", DependsOn: []string{"a"}},
			{Name: "c", Prompt: "p3", DependsOn: []string{"a", "b"}},
		},
	}
	if err := ValidateWorkflow(wf); err != nil {
		t.Fatalf("expected valid workflow, got: %v", err)
	}
}

func TestSaveWorkflow_SpacesInName(t *testing.T) {
	dir := t.TempDir()
	wf := WorkflowDef{
		Name: "my workflow name",
		Steps: []WorkflowStep{
			{Name: "s1", Prompt: "p1"},
		},
	}
	if err := SaveWorkflow(dir, wf); err != nil {
		t.Fatalf("SaveWorkflow: %v", err)
	}
	loaded, err := LoadWorkflow(dir, "my workflow name")
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	if loaded.Name != "my workflow name" {
		t.Errorf("loaded name = %q", loaded.Name)
	}
}

func TestLoadWorkflow_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadWorkflow(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for missing workflow")
	}
}

func waitForWorkflowStatus(t *testing.T, run *WorkflowRun, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run.Lock()
		status := run.Status
		run.Unlock()
		if status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	run.Lock()
	defer run.Unlock()
	t.Fatalf("workflow status = %q, want %q", run.Status, want)
}
