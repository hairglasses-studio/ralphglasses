package fleet

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestNewPipelineTask_NilData(t *testing.T) {
	task := NewPipelineTask("task-1", nil)
	if task.ID != "task-1" {
		t.Errorf("ID = %q, want task-1", task.ID)
	}
	if task.Data == nil {
		t.Error("Data should be non-nil after NewPipelineTask with nil")
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestNewPipelineTask_WithData(t *testing.T) {
	data := map[string]any{"key": "value"}
	task := NewPipelineTask("task-2", data)
	if task.Data["key"] != "value" {
		t.Errorf("Data[key] = %v, want value", task.Data["key"])
	}
}

func TestPipelineTask_SetAndGet(t *testing.T) {
	task := NewPipelineTask("t", nil)
	task.Set("foo", 42)
	v, ok := task.Get("foo")
	if !ok {
		t.Fatal("Get(foo) returned not found")
	}
	if v != 42 {
		t.Errorf("Get(foo) = %v, want 42", v)
	}
}

func TestPipelineTask_Get_Missing(t *testing.T) {
	task := NewPipelineTask("t", nil)
	v, ok := task.Get("missing")
	if ok {
		t.Error("Get(missing) should return false")
	}
	if v != nil {
		t.Errorf("Get(missing) value should be nil, got %v", v)
	}
}

func TestPipelineTask_Set_NilDataMap(t *testing.T) {
	// Task with nil Data (not via constructor) should auto-init on Set.
	task := &PipelineTask{ID: "raw"}
	task.Set("bar", "baz")
	v, ok := task.Get("bar")
	if !ok || v != "baz" {
		t.Errorf("Get(bar) = %v, %v; want baz, true", v, ok)
	}
}

func TestNewPipeline_DefaultStages(t *testing.T) {
	p := NewPipeline()
	stages := p.Stages()
	expected := []string{"validate", "plan", "execute", "verify"}
	if len(stages) != len(expected) {
		t.Fatalf("Stages() len = %d, want %d", len(stages), len(expected))
	}
	for i, name := range expected {
		if stages[i] != name {
			t.Errorf("stages[%d] = %q, want %q", i, stages[i], name)
		}
	}
}

func TestNewPipeline_WithStages(t *testing.T) {
	custom := []PipelineStage{
		{Name: "alpha", Execute: func(_ context.Context, task *PipelineTask) error {
			task.Set("alpha", true)
			return nil
		}},
		{Name: "beta", Execute: func(_ context.Context, task *PipelineTask) error {
			task.Set("beta", true)
			return nil
		}},
	}
	p := NewPipeline(WithStages(custom...))
	stages := p.Stages()
	if len(stages) != 2 || stages[0] != "alpha" || stages[1] != "beta" {
		t.Errorf("unexpected stages: %v", stages)
	}
}

func TestPipeline_Process_DefaultStages(t *testing.T) {
	p := NewPipeline()
	task := NewPipelineTask("task-ok", nil)

	err := p.Process(context.Background(), task)
	if err != nil {
		t.Fatalf("Process() unexpected error: %v", err)
	}

	for _, key := range []string{"validated", "planned", "executed", "verified"} {
		v, ok := task.Get(key)
		if !ok || v != true {
			t.Errorf("expected task.Get(%q) = true, got %v, %v", key, v, ok)
		}
	}

	if len(task.StageResults) != 4 {
		t.Errorf("StageResults len = %d, want 4", len(task.StageResults))
	}
}

func TestPipeline_Process_EmptyID_Fails(t *testing.T) {
	p := NewPipeline()
	task := NewPipelineTask("", nil)

	err := p.Process(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for empty task ID")
	}
}

func TestPipeline_Process_StageFails_StopsEarly(t *testing.T) {
	called := map[string]bool{}
	p := NewPipeline(WithStages(
		PipelineStage{Name: "s1", Execute: func(_ context.Context, t *PipelineTask) error {
			called["s1"] = true
			return nil
		}},
		PipelineStage{Name: "s2", Execute: func(_ context.Context, t *PipelineTask) error {
			called["s2"] = true
			return errors.New("stage s2 failed")
		}},
		PipelineStage{Name: "s3", Execute: func(_ context.Context, t *PipelineTask) error {
			called["s3"] = true
			return nil
		}},
	))

	task := NewPipelineTask("t", nil)
	err := p.Process(context.Background(), task)
	if err == nil || err.Error() != "stage s2 failed" {
		t.Errorf("expected s2 error, got %v", err)
	}
	if !called["s1"] {
		t.Error("s1 should have been called")
	}
	if !called["s2"] {
		t.Error("s2 should have been called")
	}
	if called["s3"] {
		t.Error("s3 should NOT have been called after s2 failure")
	}
}

func TestPipeline_WithStageCallback(t *testing.T) {
	var cbCalls []string
	p := NewPipeline(
		WithStages(
			PipelineStage{Name: "step1", Execute: func(_ context.Context, t *PipelineTask) error { return nil }},
			PipelineStage{Name: "step2", Execute: func(_ context.Context, t *PipelineTask) error { return nil }},
		),
		WithStageCallback(func(_ *PipelineTask, stage string, err error) {
			cbCalls = append(cbCalls, stage)
		}),
	)

	task := NewPipelineTask("t", nil)
	if err := p.Process(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(cbCalls) != 2 || cbCalls[0] != "step1" || cbCalls[1] != "step2" {
		t.Errorf("callback calls = %v, want [step1 step2]", cbCalls)
	}
}

func TestPipeline_ProcessBatch(t *testing.T) {
	p := NewPipeline()
	tasks := []*PipelineTask{
		NewPipelineTask("t1", nil),
		NewPipelineTask("t2", nil),
		NewPipelineTask("t3", nil),
	}

	results := p.ProcessBatch(context.Background(), tasks, 2)
	if len(results) != 3 {
		t.Fatalf("results len = %d, want 3", len(results))
	}
	for id, err := range results {
		if err != nil {
			t.Errorf("task %q: unexpected error %v", id, err)
		}
	}
}

func TestPipeline_ProcessBatch_ZeroConcurrency(t *testing.T) {
	// maxConcurrent=0 means "no limit" — should still succeed.
	p := NewPipeline()
	tasks := []*PipelineTask{
		NewPipelineTask("a", nil),
		NewPipelineTask("b", nil),
	}
	results := p.ProcessBatch(context.Background(), tasks, 0)
	for id, err := range results {
		if err != nil {
			t.Errorf("task %q: %v", id, err)
		}
	}
}

func TestPipeline_ProcessBatch_Empty(t *testing.T) {
	p := NewPipeline()
	results := p.ProcessBatch(context.Background(), nil, 1)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestPipeline_WithParallelGroup(t *testing.T) {
	var callCount int64
	makeStage := func(name string) PipelineStage {
		return PipelineStage{Name: name, Execute: func(_ context.Context, task *PipelineTask) error {
			atomic.AddInt64(&callCount, 1)
			task.Set(name+"_done", true)
			return nil
		}}
	}

	p := NewPipeline(
		WithStages(makeStage("s1"), makeStage("s2"), makeStage("s3")),
		WithParallelGroup("group1", "s1", "s2"),
	)

	task := NewPipelineTask("parallel-task", nil)
	if err := p.Process(context.Background(), task); err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if atomic.LoadInt64(&callCount) != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}

	for _, key := range []string{"s1_done", "s2_done", "s3_done"} {
		if v, ok := task.Get(key); !ok || v != true {
			t.Errorf("expected %q to be set to true", key)
		}
	}
}

func TestPipeline_ParallelGroup_ErrorPropagates(t *testing.T) {
	p := NewPipeline(
		WithStages(
			PipelineStage{Name: "pa", Execute: func(_ context.Context, _ *PipelineTask) error {
				return errors.New("pa failed")
			}},
			PipelineStage{Name: "pb", Execute: func(_ context.Context, _ *PipelineTask) error {
				return nil
			}},
		),
		WithParallelGroup("grp", "pa", "pb"),
	)

	task := NewPipelineTask("t", nil)
	err := p.Process(context.Background(), task)
	if err == nil {
		t.Fatal("expected error from parallel group")
	}
}

func TestDefaultValidate_EmptyID(t *testing.T) {
	task := &PipelineTask{}
	err := defaultValidate(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for empty task ID")
	}
}

func TestDefaultValidate_NonEmptyID(t *testing.T) {
	task := NewPipelineTask("abc", nil)
	err := defaultValidate(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := task.Get("validated")
	if !ok || v != true {
		t.Error("validated flag not set")
	}
}

func TestDefaultPlan(t *testing.T) {
	task := NewPipelineTask("t", nil)
	err := defaultPlan(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := task.Get("planned")
	if !ok || v != true {
		t.Error("planned flag not set")
	}
}

func TestDefaultExecute(t *testing.T) {
	task := NewPipelineTask("t", nil)
	err := defaultExecute(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := task.Get("executed")
	if !ok || v != true {
		t.Error("executed flag not set")
	}
}

func TestDefaultVerify_NotExecuted(t *testing.T) {
	task := NewPipelineTask("t", nil)
	err := defaultVerify(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when executed flag not set")
	}
}

func TestDefaultVerify_Executed(t *testing.T) {
	task := NewPipelineTask("t", nil)
	task.Set("executed", true)
	err := defaultVerify(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, ok := task.Get("verified")
	if !ok || v != true {
		t.Error("verified flag not set")
	}
}
