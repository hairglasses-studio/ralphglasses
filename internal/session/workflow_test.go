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
