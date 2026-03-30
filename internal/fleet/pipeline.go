package fleet

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// StageFunc processes a task in a pipeline stage. It receives the task context
// and returns an error if the stage fails. The task's Data map is used to pass
// information between stages.
type StageFunc func(ctx context.Context, task *PipelineTask) error

// PipelineStage defines a named processing stage.
type PipelineStage struct {
	Name    string
	Execute StageFunc
}

// PipelineTask is a unit of work flowing through the pipeline.
type PipelineTask struct {
	ID        string
	Data      map[string]any
	CreatedAt time.Time

	// mu protects Data for concurrent stage access.
	mu sync.Mutex

	// Populated as the task moves through stages.
	StageResults []StageResult
}

// StageResult records the outcome of a single stage execution.
type StageResult struct {
	Stage    string
	Started  time.Time
	Finished time.Time
	Err      error
}

// Set stores a value in the task's data map (concurrency-safe).
func (t *PipelineTask) Set(key string, value any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.Data == nil {
		t.Data = make(map[string]any)
	}
	t.Data[key] = value
}

// Get retrieves a value from the task's data map (concurrency-safe).
func (t *PipelineTask) Get(key string) (any, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.Data[key]
	return v, ok
}

// NewPipelineTask creates a task with the given ID and optional initial data.
func NewPipelineTask(id string, data map[string]any) *PipelineTask {
	if data == nil {
		data = make(map[string]any)
	}
	return &PipelineTask{
		ID:        id,
		Data:      data,
		CreatedAt: time.Now(),
	}
}

// Pipeline orchestrates multi-stage processing for fleet tasks.
// The default stages are: validate -> plan -> execute -> verify.
// Stages run sequentially unless parallel groups are configured.
type Pipeline struct {
	stages []PipelineStage

	// parallelGroups maps a group name to stage indices that run concurrently.
	parallelGroups map[string][]int

	// OnStageComplete is called after each stage finishes (success or failure).
	OnStageComplete func(task *PipelineTask, stage string, err error)
}

// PipelineOption configures a Pipeline.
type PipelineOption func(*Pipeline)

// WithStages replaces the default pipeline stages.
func WithStages(stages ...PipelineStage) PipelineOption {
	return func(p *Pipeline) {
		p.stages = stages
	}
}

// WithParallelGroup marks a set of stage names to execute concurrently.
// Stages in a parallel group must be contiguous in the pipeline.
func WithParallelGroup(name string, stageNames ...string) PipelineOption {
	return func(p *Pipeline) {
		if p.parallelGroups == nil {
			p.parallelGroups = make(map[string][]int)
		}
		nameSet := make(map[string]bool, len(stageNames))
		for _, n := range stageNames {
			nameSet[n] = true
		}
		var indices []int
		for i, s := range p.stages {
			if nameSet[s.Name] {
				indices = append(indices, i)
			}
		}
		if len(indices) > 0 {
			p.parallelGroups[name] = indices
		}
	}
}

// WithStageCallback sets a callback invoked after each stage completes.
func WithStageCallback(cb func(task *PipelineTask, stage string, err error)) PipelineOption {
	return func(p *Pipeline) {
		p.OnStageComplete = cb
	}
}

// NewPipeline creates a pipeline with default stages (validate, plan, execute, verify).
// Use PipelineOption to customize stages or add parallel groups.
func NewPipeline(opts ...PipelineOption) *Pipeline {
	p := &Pipeline{
		stages: []PipelineStage{
			{Name: "validate", Execute: defaultValidate},
			{Name: "plan", Execute: defaultPlan},
			{Name: "execute", Execute: defaultExecute},
			{Name: "verify", Execute: defaultVerify},
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Stages returns a copy of the pipeline's stage names in order.
func (p *Pipeline) Stages() []string {
	names := make([]string, len(p.stages))
	for i, s := range p.stages {
		names[i] = s.Name
	}
	return names
}

// Process runs a task through all pipeline stages. Stages execute sequentially
// unless they belong to a parallel group. Returns the first error encountered.
// On error, remaining stages are skipped.
func (p *Pipeline) Process(ctx context.Context, task *PipelineTask) error {
	executed := make(map[int]bool, len(p.stages))

	for i := 0; i < len(p.stages); i++ {
		if executed[i] {
			continue
		}

		// Check if this stage is part of a parallel group.
		group := p.findParallelGroup(i)
		if group != nil {
			if err := p.executeParallel(ctx, task, group); err != nil {
				return err
			}
			for _, idx := range group {
				executed[idx] = true
			}
		} else {
			if err := p.executeStage(ctx, task, p.stages[i]); err != nil {
				return err
			}
			executed[i] = true
		}
	}
	return nil
}

// ProcessBatch runs multiple tasks through the pipeline concurrently.
// Returns a map of task ID -> error (nil error for success). The maxConcurrent
// parameter limits how many tasks run in parallel (0 means no limit).
func (p *Pipeline) ProcessBatch(ctx context.Context, tasks []*PipelineTask, maxConcurrent int) map[string]error {
	results := make(map[string]error, len(tasks))
	var mu sync.Mutex

	if maxConcurrent <= 0 {
		maxConcurrent = len(tasks)
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(t *PipelineTask) {
			defer wg.Done()
			defer func() { <-sem }() // release

			err := p.Process(ctx, t)
			mu.Lock()
			results[t.ID] = err
			mu.Unlock()
		}(task)
	}

	wg.Wait()
	return results
}

// findParallelGroup returns the stage indices if stageIdx is the first index
// in a parallel group. Returns nil if the stage is not part of any group,
// or if it's not the first member.
func (p *Pipeline) findParallelGroup(stageIdx int) []int {
	for _, indices := range p.parallelGroups {
		if len(indices) > 0 && indices[0] == stageIdx {
			return indices
		}
	}
	return nil
}

// executeStage runs a single stage and records the result.
func (p *Pipeline) executeStage(ctx context.Context, task *PipelineTask, stage PipelineStage) error {
	started := time.Now()
	err := stage.Execute(ctx, task)
	finished := time.Now()

	task.mu.Lock()
	task.StageResults = append(task.StageResults, StageResult{
		Stage:    stage.Name,
		Started:  started,
		Finished: finished,
		Err:      err,
	})
	task.mu.Unlock()

	if p.OnStageComplete != nil {
		p.OnStageComplete(task, stage.Name, err)
	}
	return err
}

// executeParallel runs a group of stages concurrently. If any stage fails,
// the context is cancelled and the first error is returned.
func (p *Pipeline) executeParallel(ctx context.Context, task *PipelineTask, indices []int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		firstErr error
	)

	for _, idx := range indices {
		stage := p.stages[idx]
		wg.Add(1)
		go func(s PipelineStage) {
			defer wg.Done()
			started := time.Now()
			err := s.Execute(ctx, task)
			finished := time.Now()

			task.mu.Lock()
			task.StageResults = append(task.StageResults, StageResult{
				Stage:    s.Name,
				Started:  started,
				Finished: finished,
				Err:      err,
			})
			task.mu.Unlock()

			if p.OnStageComplete != nil {
				p.OnStageComplete(task, s.Name, err)
			}

			if err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}(stage)
	}

	wg.Wait()
	return firstErr
}

// --- Default stage implementations ---

// defaultValidate checks that required task fields are present.
func defaultValidate(_ context.Context, task *PipelineTask) error {
	if task.ID == "" {
		return errors.New("pipeline: task ID is required")
	}
	task.Set("validated", true)
	return nil
}

// defaultPlan marks the task as planned. Override for real planning logic.
func defaultPlan(_ context.Context, task *PipelineTask) error {
	task.Set("planned", true)
	return nil
}

// defaultExecute is a no-op execute stage. Override with real work dispatch.
func defaultExecute(_ context.Context, task *PipelineTask) error {
	task.Set("executed", true)
	return nil
}

// defaultVerify checks that the execute stage ran. Override for real verification.
func defaultVerify(_ context.Context, task *PipelineTask) error {
	v, ok := task.Get("executed")
	if !ok || v != true {
		return fmt.Errorf("pipeline: verify failed — execute stage did not run")
	}
	task.Set("verified", true)
	return nil
}
