package session

import (
	"context"
	"testing"
)

// stubAgent implements MicroAgent for testing.
type stubAgent struct {
	role           AgentRole
	maxConcurrency int
	result         *TaskResult
	err            error
}

func (s *stubAgent) Role() AgentRole        { return s.role }
func (s *stubAgent) MaxConcurrency() int    { return s.maxConcurrency }
func (s *stubAgent) Execute(_ context.Context, _ AgentTaskSpec) (*TaskResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func TestAgentRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	agent := &stubAgent{role: RolePlanner, maxConcurrency: 2}
	r.Register(agent)

	got := r.Get(RolePlanner)
	if got == nil {
		t.Fatal("expected agent, got nil")
	}
	if got.Role() != RolePlanner {
		t.Errorf("expected planner, got %s", got.Role())
	}
	if got.MaxConcurrency() != 2 {
		t.Errorf("expected max concurrency 2, got %d", got.MaxConcurrency())
	}
}

func TestAgentRegistry_GetMissing(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	if r.Get(RoleTester) != nil {
		t.Error("expected nil for unregistered role")
	}
}

func TestAgentRegistry_Roles(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	r.Register(&stubAgent{role: RolePlanner})
	r.Register(&stubAgent{role: RoleTester})

	roles := r.Roles()
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestDefaultPipeline(t *testing.T) {
	t.Parallel()
	p := DefaultPipeline()
	if len(p.Steps) != 4 {
		t.Errorf("expected 4 steps, got %d", len(p.Steps))
	}
	expected := []AgentRole{RolePlanner, RoleImplementer, RoleReviewer, RoleTester}
	for i, step := range p.Steps {
		if step != expected[i] {
			t.Errorf("step %d: expected %s, got %s", i, expected[i], step)
		}
	}
}

func TestExecutePipeline_Success(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	r.Register(&stubAgent{
		role:   RolePlanner,
		result: &TaskResult{AgentRole: RolePlanner, Success: true, Output: "plan done"},
	})
	r.Register(&stubAgent{
		role:   RoleImplementer,
		result: &TaskResult{AgentRole: RoleImplementer, Success: true, Output: "code done"},
	})

	pipeline := Pipeline{Steps: []AgentRole{RolePlanner, RoleImplementer}}
	results, err := r.ExecutePipeline(context.Background(), pipeline, AgentTaskSpec{ID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Output != "plan done" {
		t.Errorf("expected 'plan done', got %q", results[0].Output)
	}
	if results[1].Output != "code done" {
		t.Errorf("expected 'code done', got %q", results[1].Output)
	}
}

func TestExecutePipeline_StopsOnFailure(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	r.Register(&stubAgent{
		role:   RolePlanner,
		result: &TaskResult{AgentRole: RolePlanner, Success: false, Error: "bad plan"},
	})
	r.Register(&stubAgent{
		role:   RoleImplementer,
		result: &TaskResult{AgentRole: RoleImplementer, Success: true},
	})

	pipeline := Pipeline{Steps: []AgentRole{RolePlanner, RoleImplementer}}
	results, err := r.ExecutePipeline(context.Background(), pipeline, AgentTaskSpec{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result (stopped on failure), got %d", len(results))
	}
}

func TestExecutePipeline_MissingAgent(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	pipeline := Pipeline{Steps: []AgentRole{RolePlanner}}
	_, err := r.ExecutePipeline(context.Background(), pipeline, AgentTaskSpec{})
	if err == nil {
		t.Error("expected error for missing agent")
	}
}

func TestExecutePipeline_PassesContext(t *testing.T) {
	t.Parallel()
	r := NewAgentRegistry()
	r.Register(&stubAgent{
		role:   RolePlanner,
		result: &TaskResult{AgentRole: RolePlanner, Success: true, Output: "plan output"},
	})

	var capturedSpec AgentTaskSpec
	impl := &contextCapturingAgent{
		role:   RoleImplementer,
		result: &TaskResult{AgentRole: RoleImplementer, Success: true},
		capture: func(spec AgentTaskSpec) { capturedSpec = spec },
	}
	r.Register(impl)

	pipeline := Pipeline{Steps: []AgentRole{RolePlanner, RoleImplementer}}
	_, err := r.ExecutePipeline(context.Background(), pipeline, AgentTaskSpec{ID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSpec.Context["previous_output"] != "plan output" {
		t.Errorf("expected previous_output='plan output', got %q", capturedSpec.Context["previous_output"])
	}
	if capturedSpec.Context["previous_role"] != "planner" {
		t.Errorf("expected previous_role='planner', got %q", capturedSpec.Context["previous_role"])
	}
}

type contextCapturingAgent struct {
	role    AgentRole
	result  *TaskResult
	capture func(AgentTaskSpec)
}

func (a *contextCapturingAgent) Role() AgentRole     { return a.role }
func (a *contextCapturingAgent) MaxConcurrency() int { return 1 }
func (a *contextCapturingAgent) Execute(_ context.Context, spec AgentTaskSpec) (*TaskResult, error) {
	if a.capture != nil {
		a.capture(spec)
	}
	return a.result, nil
}
