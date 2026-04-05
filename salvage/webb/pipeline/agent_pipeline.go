package clients

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// v8.80 - Agent Pipelines
// =============================================================================

// PipelineStepType defines the type of pipeline step
type PipelineStepType string

const (
	StepTypeAgent      PipelineStepType = "agent"      // Run a specialized agent
	StepTypeTransform  PipelineStepType = "transform"  // Transform data between steps
	StepTypeCondition  PipelineStepType = "condition"  // Conditional branching
	StepTypeParallel   PipelineStepType = "parallel"   // Run steps in parallel
	StepTypeAggregate  PipelineStepType = "aggregate"  // Combine results from parallel steps
)

// PipelineStep represents a single step in a pipeline
type PipelineStep struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        PipelineStepType       `json:"type"`
	AgentType   string                 `json:"agent_type,omitempty"`   // For agent steps
	Config      map[string]interface{} `json:"config,omitempty"`
	NextStep    string                 `json:"next_step,omitempty"`    // Default next step
	OnSuccess   string                 `json:"on_success,omitempty"`   // Step on success (for conditions)
	OnFailure   string                 `json:"on_failure,omitempty"`   // Step on failure (for conditions)
	Condition   string                 `json:"condition,omitempty"`    // Condition expression
	ParallelIDs []string               `json:"parallel_ids,omitempty"` // Steps to run in parallel
	Timeout     time.Duration          `json:"timeout,omitempty"`
}

// Pipeline represents a chain of agent steps
type Pipeline struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Steps       map[string]*PipelineStep `json:"steps"`
	StartStep   string                  `json:"start_step"`
	CreatedAt   time.Time               `json:"created_at"`
	Version     int                     `json:"version"`
}

// PipelineExecution tracks a pipeline run
type PipelineExecution struct {
	ID           string                      `json:"id"`
	PipelineID   string                      `json:"pipeline_id"`
	PipelineName string                      `json:"pipeline_name"`
	Status       string                      `json:"status"` // running, completed, failed, cancelled
	StartedAt    time.Time                   `json:"started_at"`
	CompletedAt  *time.Time                  `json:"completed_at,omitempty"`
	CurrentStep  string                      `json:"current_step"`
	StepResults  map[string]*StepResult      `json:"step_results"`
	Input        map[string]interface{}      `json:"input"`
	Output       map[string]interface{}      `json:"output,omitempty"`
	Error        string                      `json:"error,omitempty"`
}

// StepResult contains the result of a single step
type StepResult struct {
	StepID      string                 `json:"step_id"`
	StepName    string                 `json:"step_name"`
	Status      string                 `json:"status"` // pending, running, completed, failed, skipped
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// PipelineRunner executes pipelines
type PipelineRunner struct {
	mu         sync.RWMutex
	pipelines  map[string]*Pipeline
	executions map[string]*PipelineExecution
	router     *ModelRouter
}

// NewPipelineRunner creates a new pipeline runner
func NewPipelineRunner() (*PipelineRunner, error) {
	router, err := NewModelRouter()
	if err != nil {
		return nil, err
	}

	runner := &PipelineRunner{
		pipelines:  make(map[string]*Pipeline),
		executions: make(map[string]*PipelineExecution),
		router:     router,
	}

	// Register built-in pipeline templates
	runner.registerBuiltInPipelines()

	return runner, nil
}

// registerBuiltInPipelines registers pre-built pipeline templates
func (r *PipelineRunner) registerBuiltInPipelines() {
	// Code Review Pipeline: Extract -> Review -> Summarize
	r.pipelines["code-review-full"] = &Pipeline{
		ID:          "code-review-full",
		Name:        "Full Code Review Pipeline",
		Description: "Extract code structure, perform deep review, and generate summary",
		StartStep:   "extract",
		Version:     1,
		CreatedAt:   time.Now(),
		Steps: map[string]*PipelineStep{
			"extract": {
				ID:        "extract",
				Name:      "Extract Code Structure",
				Type:      StepTypeAgent,
				AgentType: "extractor",
				Config: map[string]interface{}{
					"schema": "Extract: functions, classes, imports, dependencies, complexity metrics",
				},
				NextStep: "review",
			},
			"review": {
				ID:        "review",
				Name:      "Deep Code Review",
				Type:      StepTypeAgent,
				AgentType: "code-reviewer",
				Config: map[string]interface{}{
					"include_structure": true,
				},
				NextStep: "summarize",
			},
			"summarize": {
				ID:        "summarize",
				Name:      "Generate Summary",
				Type:      StepTypeAgent,
				AgentType: "consensus",
				Config: map[string]interface{}{
					"system_prompt": "Summarize the code review findings into actionable items",
				},
			},
		},
	}

	// Consensus Analysis Pipeline: Parallel analysis -> Aggregate
	r.pipelines["consensus-analysis"] = &Pipeline{
		ID:          "consensus-analysis",
		Name:        "Multi-Model Consensus Analysis",
		Description: "Run analysis on multiple models in parallel and synthesize results",
		StartStep:   "parallel-analyze",
		Version:     1,
		CreatedAt:   time.Now(),
		Steps: map[string]*PipelineStep{
			"parallel-analyze": {
				ID:          "parallel-analyze",
				Name:        "Parallel Model Analysis",
				Type:        StepTypeParallel,
				ParallelIDs: []string{"claude-analysis", "openai-analysis", "gemini-analysis"},
				NextStep:    "aggregate",
			},
			"claude-analysis": {
				ID:        "claude-analysis",
				Name:      "Claude Analysis",
				Type:      StepTypeAgent,
				AgentType: "model-analyze",
				Config: map[string]interface{}{
					"provider": "claude",
				},
			},
			"openai-analysis": {
				ID:        "openai-analysis",
				Name:      "OpenAI Analysis",
				Type:      StepTypeAgent,
				AgentType: "model-analyze",
				Config: map[string]interface{}{
					"provider": "openai",
				},
			},
			"gemini-analysis": {
				ID:        "gemini-analysis",
				Name:      "Gemini Analysis",
				Type:      StepTypeAgent,
				AgentType: "model-analyze",
				Config: map[string]interface{}{
					"provider": "gemini",
				},
			},
			"aggregate": {
				ID:        "aggregate",
				Name:      "Synthesize Results",
				Type:      StepTypeAggregate,
				AgentType: "consensus",
				Config: map[string]interface{}{
					"system_prompt": "Synthesize the analyses from multiple models into a unified conclusion",
				},
			},
		},
	}

	// Data Processing Pipeline: Extract -> Validate -> Transform
	r.pipelines["data-processing"] = &Pipeline{
		ID:          "data-processing",
		Name:        "Data Processing Pipeline",
		Description: "Extract, validate, and transform structured data",
		StartStep:   "extract",
		Version:     1,
		CreatedAt:   time.Now(),
		Steps: map[string]*PipelineStep{
			"extract": {
				ID:        "extract",
				Name:      "Extract Data",
				Type:      StepTypeAgent,
				AgentType: "extractor",
				NextStep:  "validate",
			},
			"validate": {
				ID:        "validate",
				Name:      "Validate Data",
				Type:      StepTypeCondition,
				Condition: "output.valid == true",
				OnSuccess: "transform",
				OnFailure: "fix-errors",
			},
			"fix-errors": {
				ID:        "fix-errors",
				Name:      "Fix Extraction Errors",
				Type:      StepTypeAgent,
				AgentType: "extractor",
				Config: map[string]interface{}{
					"mode": "fix",
				},
				NextStep: "validate",
			},
			"transform": {
				ID:        "transform",
				Name:      "Transform Data",
				Type:      StepTypeTransform,
				Config: map[string]interface{}{
					"format": "json",
				},
			},
		},
	}

	// Investigation Pipeline: Extract context -> Analyze -> Recommend
	r.pipelines["investigation"] = &Pipeline{
		ID:          "investigation",
		Name:        "Investigation Pipeline",
		Description: "Investigate an issue by gathering context, analyzing, and recommending actions",
		StartStep:   "gather-context",
		Version:     1,
		CreatedAt:   time.Now(),
		Steps: map[string]*PipelineStep{
			"gather-context": {
				ID:        "gather-context",
				Name:      "Gather Context",
				Type:      StepTypeAgent,
				AgentType: "extractor",
				Config: map[string]interface{}{
					"schema": "Extract: error messages, stack traces, timestamps, affected systems, recent changes",
				},
				NextStep: "analyze",
			},
			"analyze": {
				ID:        "analyze",
				Name:      "Analyze Issue",
				Type:      StepTypeAgent,
				AgentType: "code-reviewer",
				Config: map[string]interface{}{
					"focus": "root cause analysis",
				},
				NextStep: "recommend",
			},
			"recommend": {
				ID:        "recommend",
				Name:      "Generate Recommendations",
				Type:      StepTypeAgent,
				AgentType: "consensus",
				Config: map[string]interface{}{
					"system_prompt": "Based on the analysis, provide actionable recommendations to resolve the issue",
				},
			},
		},
	}
}

// CreatePipeline creates a new custom pipeline
func (r *PipelineRunner) CreatePipeline(name, description string, steps map[string]*PipelineStep, startStep string) *Pipeline {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := fmt.Sprintf("pipeline-%d", time.Now().UnixNano())
	pipeline := &Pipeline{
		ID:          id,
		Name:        name,
		Description: description,
		Steps:       steps,
		StartStep:   startStep,
		CreatedAt:   time.Now(),
		Version:     1,
	}

	r.pipelines[id] = pipeline
	return pipeline
}

// GetPipeline returns a pipeline by ID
func (r *PipelineRunner) GetPipeline(id string) (*Pipeline, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pipeline, ok := r.pipelines[id]
	if !ok {
		return nil, fmt.Errorf("pipeline not found: %s", id)
	}
	return pipeline, nil
}

// ListPipelines returns all available pipelines
func (r *PipelineRunner) ListPipelines() []*Pipeline {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pipelines := make([]*Pipeline, 0, len(r.pipelines))
	for _, p := range r.pipelines {
		pipelines = append(pipelines, p)
	}
	return pipelines
}

// Execute runs a pipeline with the given input
func (r *PipelineRunner) Execute(ctx context.Context, pipelineID string, input map[string]interface{}) (*PipelineExecution, error) {
	r.mu.RLock()
	pipeline, ok := r.pipelines[pipelineID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("pipeline not found: %s", pipelineID)
	}

	// Create execution record
	execID := fmt.Sprintf("exec-%d", time.Now().UnixNano())
	execution := &PipelineExecution{
		ID:           execID,
		PipelineID:   pipelineID,
		PipelineName: pipeline.Name,
		Status:       "running",
		StartedAt:    time.Now(),
		CurrentStep:  pipeline.StartStep,
		StepResults:  make(map[string]*StepResult),
		Input:        input,
	}

	r.mu.Lock()
	r.executions[execID] = execution
	r.mu.Unlock()

	// Execute pipeline
	go r.runPipeline(ctx, pipeline, execution)

	return execution, nil
}

// runPipeline executes the pipeline steps
func (r *PipelineRunner) runPipeline(ctx context.Context, pipeline *Pipeline, execution *PipelineExecution) {
	currentStepID := pipeline.StartStep
	currentData := execution.Input

	for currentStepID != "" {
		step, ok := pipeline.Steps[currentStepID]
		if !ok {
			r.failExecution(execution, fmt.Sprintf("step not found: %s", currentStepID))
			return
		}

		// Update current step
		r.mu.Lock()
		execution.CurrentStep = currentStepID
		r.mu.Unlock()

		// Execute step
		result, nextStep, err := r.executeStep(ctx, step, currentData, pipeline)

		// Record result
		r.mu.Lock()
		execution.StepResults[currentStepID] = result
		r.mu.Unlock()

		if err != nil {
			r.failExecution(execution, fmt.Sprintf("step %s failed: %v", currentStepID, err))
			return
		}

		// Update data for next step
		if result.Output != nil {
			currentData = result.Output
		}

		currentStepID = nextStep
	}

	// Complete execution
	r.mu.Lock()
	now := time.Now()
	execution.CompletedAt = &now
	execution.Status = "completed"
	execution.Output = currentData
	r.mu.Unlock()
}

// executeStep runs a single pipeline step
func (r *PipelineRunner) executeStep(ctx context.Context, step *PipelineStep, input map[string]interface{}, pipeline *Pipeline) (*StepResult, string, error) {
	now := time.Now()
	result := &StepResult{
		StepID:    step.ID,
		StepName:  step.Name,
		Status:    "running",
		StartedAt: &now,
		Input:     input,
	}

	var output map[string]interface{}
	var nextStep string
	var err error

	switch step.Type {
	case StepTypeAgent:
		output, err = r.runAgentStep(ctx, step, input)
		nextStep = step.NextStep

	case StepTypeParallel:
		output, err = r.runParallelSteps(ctx, step, input, pipeline)
		nextStep = step.NextStep

	case StepTypeCondition:
		nextStep, err = r.evaluateCondition(step, input)
		output = input // Pass through

	case StepTypeAggregate:
		output, err = r.aggregateResults(ctx, step, input)
		nextStep = step.NextStep

	case StepTypeTransform:
		output, err = r.transformData(step, input)
		nextStep = step.NextStep

	default:
		err = fmt.Errorf("unknown step type: %s", step.Type)
	}

	endTime := time.Now()
	result.CompletedAt = &endTime
	result.Duration = endTime.Sub(now)

	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result, "", err
	}

	result.Status = "completed"
	result.Output = output
	return result, nextStep, nil
}

// runAgentStep executes an agent step
func (r *PipelineRunner) runAgentStep(ctx context.Context, step *PipelineStep, input map[string]interface{}) (map[string]interface{}, error) {
	// Get content from input
	content := ""
	if c, ok := input["content"].(string); ok {
		content = c
	} else if c, ok := input["code"].(string); ok {
		content = c
	} else if c, ok := input["text"].(string); ok {
		content = c
	}

	output := make(map[string]interface{})

	switch step.AgentType {
	case "extractor":
		agent, err := NewExtractionAgent()
		if err != nil {
			return nil, err
		}
		schema := "Extract key information"
		if s, ok := step.Config["schema"].(string); ok {
			schema = s
		}
		result, err := agent.Extract(ctx, content, schema)
		if err != nil {
			return nil, err
		}
		output["extracted"] = result.Data
		output["content"] = result.Data
		output["provider"] = string(result.Provider)
		output["tokens"] = result.InputTokens + result.OutputTokens

	case "code-reviewer":
		agent, err := NewCodeReviewerAgent()
		if err != nil {
			return nil, err
		}
		language := "go"
		if l, ok := step.Config["language"].(string); ok {
			language = l
		}
		reviewContext := ""
		if c, ok := step.Config["context"].(string); ok {
			reviewContext = c
		}
		result, err := agent.Review(ctx, content, language, reviewContext)
		if err != nil {
			return nil, err
		}
		output["review"] = result.Review
		output["content"] = result.Review
		output["thinking"] = result.Thinking
		output["provider"] = string(result.Provider)
		output["tokens"] = result.InputTokens + result.OutputTokens

	case "consensus":
		agent, err := NewConsensusAgent(nil)
		if err != nil {
			return nil, err
		}
		systemPrompt := "Analyze the following:"
		if s, ok := step.Config["system_prompt"].(string); ok {
			systemPrompt = s
		}
		result, err := agent.Analyze(ctx, content, systemPrompt)
		if err != nil {
			return nil, err
		}
		output["summary"] = result.Summary
		output["content"] = result.Summary
		output["consensus_level"] = result.ConsensusLevel
		output["recommendation"] = result.Recommendation
		output["responses"] = len(result.Responses)

	case "model-analyze":
		provider := ProviderClaude
		if p, ok := step.Config["provider"].(string); ok {
			provider = ProviderType(p)
		}
		config := &RoutingConfig{PreferProvider: provider}
		req := AnalysisRequest{
			Prompt:    content,
			MaxTokens: 4000,
		}
		result, decision, err := r.router.AnalyzeWithRouting(ctx, req, TaskAnalysis, config)
		if err != nil {
			return nil, err
		}
		output["analysis"] = result.Content
		output["content"] = result.Content
		output["provider"] = string(decision.SelectedProvider)
		output["tokens"] = result.InputTokens + result.OutputTokens

	default:
		return nil, fmt.Errorf("unknown agent type: %s", step.AgentType)
	}

	return output, nil
}

// runParallelSteps executes steps in parallel
func (r *PipelineRunner) runParallelSteps(ctx context.Context, step *PipelineStep, input map[string]interface{}, pipeline *Pipeline) (map[string]interface{}, error) {
	results := make(map[string]map[string]interface{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	errCh := make(chan error, len(step.ParallelIDs))

	for _, stepID := range step.ParallelIDs {
		parallelStep, ok := pipeline.Steps[stepID]
		if !ok {
			continue
		}

		wg.Add(1)
		go func(s *PipelineStep) {
			defer wg.Done()
			output, err := r.runAgentStep(ctx, s, input)
			if err != nil {
				errCh <- fmt.Errorf("parallel step %s failed: %v", s.ID, err)
				return
			}
			mu.Lock()
			results[s.ID] = output
			mu.Unlock()
		}(parallelStep)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("parallel execution errors: %s", strings.Join(errs, "; "))
	}

	output := map[string]interface{}{
		"parallel_results": results,
		"content":          results, // For next step
	}
	return output, nil
}

// evaluateCondition evaluates a condition and returns the next step
func (r *PipelineRunner) evaluateCondition(step *PipelineStep, input map[string]interface{}) (string, error) {
	// Simple condition evaluation
	// In production, you'd use a proper expression evaluator

	condition := step.Condition
	if condition == "" {
		return step.OnSuccess, nil
	}

	// Check for simple "output.valid == true" pattern
	if strings.Contains(condition, "valid") {
		if valid, ok := input["valid"].(bool); ok && valid {
			return step.OnSuccess, nil
		}
		return step.OnFailure, nil
	}

	// Check for error presence
	if strings.Contains(condition, "error") {
		if _, hasError := input["error"]; hasError {
			return step.OnFailure, nil
		}
		return step.OnSuccess, nil
	}

	// Default to success path
	return step.OnSuccess, nil
}

// aggregateResults combines results from parallel steps
func (r *PipelineRunner) aggregateResults(ctx context.Context, step *PipelineStep, input map[string]interface{}) (map[string]interface{}, error) {
	parallelResults, ok := input["parallel_results"].(map[string]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no parallel results to aggregate")
	}

	// Build combined content
	var combined strings.Builder
	for stepID, result := range parallelResults {
		if content, ok := result["content"].(string); ok {
			combined.WriteString(fmt.Sprintf("=== %s ===\n%s\n\n", stepID, content))
		}
	}

	// If there's an agent to synthesize, use it
	if step.AgentType != "" {
		synthesizedInput := map[string]interface{}{
			"content": combined.String(),
		}
		return r.runAgentStep(ctx, step, synthesizedInput)
	}

	return map[string]interface{}{
		"aggregated": combined.String(),
		"content":    combined.String(),
		"sources":    len(parallelResults),
	}, nil
}

// transformData transforms data between formats
func (r *PipelineRunner) transformData(step *PipelineStep, input map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})
	for k, v := range input {
		output[k] = v
	}

	// Apply format transformation if specified
	if format, ok := step.Config["format"].(string); ok {
		output["format"] = format
	}

	return output, nil
}

// failExecution marks an execution as failed
func (r *PipelineRunner) failExecution(execution *PipelineExecution, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	execution.CompletedAt = &now
	execution.Status = "failed"
	execution.Error = errMsg
}

// GetExecution returns an execution by ID
func (r *PipelineRunner) GetExecution(execID string) (*PipelineExecution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exec, ok := r.executions[execID]
	if !ok {
		return nil, fmt.Errorf("execution not found: %s", execID)
	}
	return exec, nil
}

// ListExecutions returns recent executions
func (r *PipelineRunner) ListExecutions(limit int) []*PipelineExecution {
	r.mu.RLock()
	defer r.mu.RUnlock()

	execs := make([]*PipelineExecution, 0, len(r.executions))
	for _, e := range r.executions {
		execs = append(execs, e)
	}

	// Sort by start time descending
	for i := 0; i < len(execs)-1; i++ {
		for j := i + 1; j < len(execs); j++ {
			if execs[j].StartedAt.After(execs[i].StartedAt) {
				execs[i], execs[j] = execs[j], execs[i]
			}
		}
	}

	if limit > 0 && len(execs) > limit {
		execs = execs[:limit]
	}

	return execs
}

// FormatPipelineList formats pipelines as markdown
func FormatPipelineList(pipelines []*Pipeline) string {
	var sb strings.Builder
	sb.WriteString("# Available Pipelines\n\n")

	for _, p := range pipelines {
		sb.WriteString(fmt.Sprintf("## %s\n\n", p.Name))
		sb.WriteString(fmt.Sprintf("- **ID:** `%s`\n", p.ID))
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", p.Description))
		sb.WriteString(fmt.Sprintf("- **Steps:** %d\n", len(p.Steps)))
		sb.WriteString(fmt.Sprintf("- **Start Step:** %s\n", p.StartStep))
		sb.WriteString(fmt.Sprintf("- **Version:** %d\n\n", p.Version))

		sb.WriteString("### Steps\n\n")
		sb.WriteString("| Step | Type | Agent | Next |\n")
		sb.WriteString("|------|------|-------|------|\n")
		for _, step := range p.Steps {
			next := step.NextStep
			if step.Type == StepTypeCondition {
				next = fmt.Sprintf("✓→%s / ✗→%s", step.OnSuccess, step.OnFailure)
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
				step.Name, step.Type, step.AgentType, next))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatExecutionStatus formats execution status as markdown
func FormatExecutionStatus(exec *PipelineExecution) string {
	var sb strings.Builder
	sb.WriteString("# Pipeline Execution Status\n\n")

	sb.WriteString(fmt.Sprintf("- **Execution ID:** `%s`\n", exec.ID))
	sb.WriteString(fmt.Sprintf("- **Pipeline:** %s\n", exec.PipelineName))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n", exec.Status))
	sb.WriteString(fmt.Sprintf("- **Current Step:** %s\n", exec.CurrentStep))
	sb.WriteString(fmt.Sprintf("- **Started:** %s\n", exec.StartedAt.Format("2006-01-02 15:04:05")))

	if exec.CompletedAt != nil {
		sb.WriteString(fmt.Sprintf("- **Completed:** %s\n", exec.CompletedAt.Format("2006-01-02 15:04:05")))
		sb.WriteString(fmt.Sprintf("- **Duration:** %s\n", exec.CompletedAt.Sub(exec.StartedAt)))
	}

	if exec.Error != "" {
		sb.WriteString(fmt.Sprintf("\n**Error:** %s\n", exec.Error))
	}

	sb.WriteString("\n## Step Results\n\n")
	sb.WriteString("| Step | Status | Duration | Output |\n")
	sb.WriteString("|------|--------|----------|--------|\n")
	for _, result := range exec.StepResults {
		outputPreview := "-"
		if result.Output != nil {
			if content, ok := result.Output["content"].(string); ok {
				if len(content) > 50 {
					outputPreview = content[:50] + "..."
				} else {
					outputPreview = content
				}
			}
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			result.StepName, result.Status, result.Duration, outputPreview))
	}

	if exec.Status == "completed" && exec.Output != nil {
		sb.WriteString("\n## Final Output\n\n")
		if content, ok := exec.Output["content"].(string); ok {
			sb.WriteString(content)
		}
	}

	return sb.String()
}

// =============================================================================
// v8.85 - Pipeline Persistence & Scheduling
// =============================================================================

// ScheduleType defines the type of schedule
type ScheduleType string

const (
	ScheduleCron     ScheduleType = "cron"     // Cron expression
	ScheduleInterval ScheduleType = "interval" // Fixed interval
	ScheduleWebhook  ScheduleType = "webhook"  // Webhook trigger
	ScheduleManual   ScheduleType = "manual"   // Manual only
)

// PipelineSchedule defines when a pipeline should run
type PipelineSchedule struct {
	ID           string                 `json:"id"`
	PipelineID   string                 `json:"pipeline_id"`
	PipelineName string                 `json:"pipeline_name"`
	Type         ScheduleType           `json:"type"`
	CronExpr     string                 `json:"cron_expr,omitempty"`     // For cron schedules
	Interval     time.Duration          `json:"interval,omitempty"`      // For interval schedules
	WebhookToken string                 `json:"webhook_token,omitempty"` // For webhook triggers
	DefaultInput map[string]interface{} `json:"default_input,omitempty"`
	Enabled      bool                   `json:"enabled"`
	LastRun      *time.Time             `json:"last_run,omitempty"`
	NextRun      *time.Time             `json:"next_run,omitempty"`
	RunCount     int                    `json:"run_count"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// PipelineVersion tracks pipeline changes
type PipelineVersion struct {
	Version     int       `json:"version"`
	Pipeline    *Pipeline `json:"pipeline"`
	ChangedAt   time.Time `json:"changed_at"`
	ChangedBy   string    `json:"changed_by"`
	ChangeNotes string    `json:"change_notes"`
}

// PipelineStore handles pipeline persistence
type PipelineStore struct {
	mu        sync.RWMutex
	pipelines map[string]*Pipeline
	versions  map[string][]*PipelineVersion // pipelineID -> versions
	schedules map[string]*PipelineSchedule
	storePath string
}

// NewPipelineStore creates a new pipeline store
func NewPipelineStore(storePath string) *PipelineStore {
	store := &PipelineStore{
		pipelines: make(map[string]*Pipeline),
		versions:  make(map[string][]*PipelineVersion),
		schedules: make(map[string]*PipelineSchedule),
		storePath: storePath,
	}
	return store
}

// SavePipeline saves a pipeline and creates a new version
func (s *PipelineStore) SavePipeline(pipeline *Pipeline, changedBy, changeNotes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get current version
	currentVersion := 0
	if existing, ok := s.pipelines[pipeline.ID]; ok {
		currentVersion = existing.Version
	}

	// Increment version
	pipeline.Version = currentVersion + 1

	// Create version record
	version := &PipelineVersion{
		Version:     pipeline.Version,
		Pipeline:    pipeline,
		ChangedAt:   time.Now(),
		ChangedBy:   changedBy,
		ChangeNotes: changeNotes,
	}

	// Store pipeline and version
	s.pipelines[pipeline.ID] = pipeline
	s.versions[pipeline.ID] = append(s.versions[pipeline.ID], version)

	return nil
}

// GetPipeline returns a pipeline by ID
func (s *PipelineStore) GetPipeline(id string) (*Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pipeline, ok := s.pipelines[id]
	if !ok {
		return nil, fmt.Errorf("pipeline not found: %s", id)
	}
	return pipeline, nil
}

// GetPipelineVersion returns a specific version of a pipeline
func (s *PipelineStore) GetPipelineVersion(id string, version int) (*Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, ok := s.versions[id]
	if !ok {
		return nil, fmt.Errorf("pipeline not found: %s", id)
	}

	for _, v := range versions {
		if v.Version == version {
			return v.Pipeline, nil
		}
	}

	return nil, fmt.Errorf("version %d not found for pipeline %s", version, id)
}

// ListVersions returns all versions of a pipeline
func (s *PipelineStore) ListVersions(id string) ([]*PipelineVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versions, ok := s.versions[id]
	if !ok {
		return nil, fmt.Errorf("pipeline not found: %s", id)
	}
	return versions, nil
}

// DeletePipeline removes a pipeline
func (s *PipelineStore) DeletePipeline(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.pipelines[id]; !ok {
		return fmt.Errorf("pipeline not found: %s", id)
	}

	delete(s.pipelines, id)
	delete(s.versions, id)
	return nil
}

// ListPipelines returns all pipelines
func (s *PipelineStore) ListPipelines() []*Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pipelines := make([]*Pipeline, 0, len(s.pipelines))
	for _, p := range s.pipelines {
		pipelines = append(pipelines, p)
	}
	return pipelines
}

// PipelineScheduler manages scheduled pipeline executions
type PipelineScheduler struct {
	mu        sync.RWMutex
	store     *PipelineStore
	runner    *PipelineRunner
	schedules map[string]*PipelineSchedule
	stopCh    chan struct{}
	running   bool
}

// NewPipelineScheduler creates a new pipeline scheduler
func NewPipelineScheduler(store *PipelineStore, runner *PipelineRunner) *PipelineScheduler {
	return &PipelineScheduler{
		store:     store,
		runner:    runner,
		schedules: make(map[string]*PipelineSchedule),
		stopCh:    make(chan struct{}),
	}
}

// CreateSchedule creates a new schedule for a pipeline
func (s *PipelineScheduler) CreateSchedule(pipelineID string, schedType ScheduleType, config map[string]interface{}) (*PipelineSchedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pipeline, err := s.store.GetPipeline(pipelineID)
	if err != nil {
		// Try runner's built-in pipelines
		if s.runner != nil {
			pipeline, err = s.runner.GetPipeline(pipelineID)
			if err != nil {
				return nil, fmt.Errorf("pipeline not found: %s", pipelineID)
			}
		} else {
			return nil, err
		}
	}

	schedID := fmt.Sprintf("sched-%d", time.Now().UnixNano())
	now := time.Now()

	schedule := &PipelineSchedule{
		ID:           schedID,
		PipelineID:   pipelineID,
		PipelineName: pipeline.Name,
		Type:         schedType,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Parse config based on type
	switch schedType {
	case ScheduleCron:
		if expr, ok := config["cron"].(string); ok {
			schedule.CronExpr = expr
			nextRun := s.calculateNextCronRun(expr, now)
			schedule.NextRun = &nextRun
		}
	case ScheduleInterval:
		if interval, ok := config["interval"].(string); ok {
			d, err := time.ParseDuration(interval)
			if err != nil {
				return nil, fmt.Errorf("invalid interval: %v", err)
			}
			schedule.Interval = d
			nextRun := now.Add(d)
			schedule.NextRun = &nextRun
		}
	case ScheduleWebhook:
		schedule.WebhookToken = fmt.Sprintf("wh-%d", time.Now().UnixNano())
	}

	if input, ok := config["default_input"].(map[string]interface{}); ok {
		schedule.DefaultInput = input
	}

	s.schedules[schedID] = schedule
	return schedule, nil
}

// calculateNextCronRun calculates the next run time for a cron expression
func (s *PipelineScheduler) calculateNextCronRun(expr string, from time.Time) time.Time {
	// Simple cron parsing for common patterns
	parts := strings.Fields(expr)
	if len(parts) < 5 {
		return from.Add(time.Hour) // Default to 1 hour
	}

	// Handle common patterns
	switch {
	case expr == "0 * * * *": // Every hour
		return from.Truncate(time.Hour).Add(time.Hour)
	case expr == "0 0 * * *": // Daily at midnight
		next := time.Date(from.Year(), from.Month(), from.Day()+1, 0, 0, 0, 0, from.Location())
		return next
	case expr == "0 0 * * 0": // Weekly on Sunday
		daysUntilSunday := (7 - int(from.Weekday())) % 7
		if daysUntilSunday == 0 {
			daysUntilSunday = 7
		}
		next := time.Date(from.Year(), from.Month(), from.Day()+daysUntilSunday, 0, 0, 0, 0, from.Location())
		return next
	case strings.HasPrefix(expr, "*/"):
		// Every N minutes pattern: */5 * * * *
		if minutes := strings.TrimPrefix(parts[0], "*/"); minutes != "" {
			var n int
			fmt.Sscanf(minutes, "%d", &n)
			if n > 0 {
				return from.Truncate(time.Duration(n) * time.Minute).Add(time.Duration(n) * time.Minute)
			}
		}
	}

	// Default: next hour
	return from.Add(time.Hour)
}

// GetSchedule returns a schedule by ID
func (s *PipelineScheduler) GetSchedule(id string) (*PipelineSchedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedule, ok := s.schedules[id]
	if !ok {
		return nil, fmt.Errorf("schedule not found: %s", id)
	}
	return schedule, nil
}

// ListSchedules returns all schedules
func (s *PipelineScheduler) ListSchedules() []*PipelineSchedule {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schedules := make([]*PipelineSchedule, 0, len(s.schedules))
	for _, sched := range s.schedules {
		schedules = append(schedules, sched)
	}
	return schedules
}

// EnableSchedule enables a schedule
func (s *PipelineScheduler) EnableSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[id]
	if !ok {
		return fmt.Errorf("schedule not found: %s", id)
	}

	schedule.Enabled = true
	schedule.UpdatedAt = time.Now()

	// Recalculate next run
	if schedule.Type == ScheduleCron {
		nextRun := s.calculateNextCronRun(schedule.CronExpr, time.Now())
		schedule.NextRun = &nextRun
	} else if schedule.Type == ScheduleInterval {
		nextRun := time.Now().Add(schedule.Interval)
		schedule.NextRun = &nextRun
	}

	return nil
}

// DisableSchedule disables a schedule
func (s *PipelineScheduler) DisableSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedule, ok := s.schedules[id]
	if !ok {
		return fmt.Errorf("schedule not found: %s", id)
	}

	schedule.Enabled = false
	schedule.UpdatedAt = time.Now()
	schedule.NextRun = nil
	return nil
}

// DeleteSchedule removes a schedule
func (s *PipelineScheduler) DeleteSchedule(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.schedules[id]; !ok {
		return fmt.Errorf("schedule not found: %s", id)
	}

	delete(s.schedules, id)
	return nil
}

// TriggerWebhook triggers a pipeline via webhook token
func (s *PipelineScheduler) TriggerWebhook(ctx context.Context, token string, input map[string]interface{}) (*PipelineExecution, error) {
	s.mu.RLock()
	var schedule *PipelineSchedule
	for _, sched := range s.schedules {
		if sched.WebhookToken == token && sched.Enabled {
			schedule = sched
			break
		}
	}
	s.mu.RUnlock()

	if schedule == nil {
		return nil, fmt.Errorf("invalid or disabled webhook token")
	}

	// Merge default input with provided input
	mergedInput := make(map[string]interface{})
	for k, v := range schedule.DefaultInput {
		mergedInput[k] = v
	}
	for k, v := range input {
		mergedInput[k] = v
	}

	// Execute pipeline
	execution, err := s.runner.Execute(ctx, schedule.PipelineID, mergedInput)
	if err != nil {
		return nil, err
	}

	// Update schedule stats
	s.mu.Lock()
	now := time.Now()
	schedule.LastRun = &now
	schedule.RunCount++
	schedule.UpdatedAt = now
	s.mu.Unlock()

	return execution, nil
}

// Start starts the scheduler loop
func (s *PipelineScheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go s.runSchedulerLoop(ctx)
}

// Stop stops the scheduler loop
func (s *PipelineScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	close(s.stopCh)
	s.running = false
}

// runSchedulerLoop runs the main scheduler loop
func (s *PipelineScheduler) runSchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.checkAndRunSchedules(ctx)
		}
	}
}

// checkAndRunSchedules checks for due schedules and runs them
func (s *PipelineScheduler) checkAndRunSchedules(ctx context.Context) {
	s.mu.RLock()
	var dueSchedules []*PipelineSchedule
	now := time.Now()

	for _, sched := range s.schedules {
		if !sched.Enabled {
			continue
		}
		if sched.Type == ScheduleWebhook || sched.Type == ScheduleManual {
			continue
		}
		if sched.NextRun != nil && now.After(*sched.NextRun) {
			dueSchedules = append(dueSchedules, sched)
		}
	}
	s.mu.RUnlock()

	// Execute due schedules
	for _, sched := range dueSchedules {
		go s.executeScheduledPipeline(ctx, sched)
	}
}

// executeScheduledPipeline executes a scheduled pipeline
func (s *PipelineScheduler) executeScheduledPipeline(ctx context.Context, schedule *PipelineSchedule) {
	input := schedule.DefaultInput
	if input == nil {
		input = make(map[string]interface{})
	}

	_, err := s.runner.Execute(ctx, schedule.PipelineID, input)

	// Update schedule
	s.mu.Lock()
	now := time.Now()
	schedule.LastRun = &now
	schedule.RunCount++
	schedule.UpdatedAt = now

	// Calculate next run
	if schedule.Type == ScheduleCron {
		nextRun := s.calculateNextCronRun(schedule.CronExpr, now)
		schedule.NextRun = &nextRun
	} else if schedule.Type == ScheduleInterval {
		nextRun := now.Add(schedule.Interval)
		schedule.NextRun = &nextRun
	}

	if err != nil {
		schedule.UpdatedAt = now
	}
	s.mu.Unlock()
}

// FormatScheduleList formats schedules as markdown
func FormatScheduleList(schedules []*PipelineSchedule) string {
	var sb strings.Builder
	sb.WriteString("# Pipeline Schedules\n\n")

	if len(schedules) == 0 {
		sb.WriteString("_No schedules configured._\n")
		return sb.String()
	}

	sb.WriteString("| ID | Pipeline | Type | Schedule | Status | Last Run | Next Run | Runs |\n")
	sb.WriteString("|----|----------|------|----------|--------|----------|----------|------|\n")

	for _, sched := range schedules {
		status := "Enabled"
		if !sched.Enabled {
			status = "Disabled"
		}

		schedStr := ""
		switch sched.Type {
		case ScheduleCron:
			schedStr = sched.CronExpr
		case ScheduleInterval:
			schedStr = sched.Interval.String()
		case ScheduleWebhook:
			schedStr = "webhook:" + sched.WebhookToken[:8] + "..."
		case ScheduleManual:
			schedStr = "manual"
		}

		lastRun := "-"
		if sched.LastRun != nil {
			lastRun = sched.LastRun.Format("01-02 15:04")
		}

		nextRun := "-"
		if sched.NextRun != nil {
			nextRun = sched.NextRun.Format("01-02 15:04")
		}

		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s | %s | %d |\n",
			sched.ID[:12], sched.PipelineName, sched.Type, schedStr, status, lastRun, nextRun, sched.RunCount))
	}

	return sb.String()
}

// FormatPipelineVersionList formats pipeline versions as markdown
func FormatPipelineVersionList(versions []*PipelineVersion) string {
	var sb strings.Builder
	sb.WriteString("# Pipeline Versions\n\n")

	if len(versions) == 0 {
		sb.WriteString("_No versions found._\n")
		return sb.String()
	}

	sb.WriteString("| Version | Changed At | Changed By | Notes |\n")
	sb.WriteString("|---------|------------|------------|-------|\n")

	for _, v := range versions {
		sb.WriteString(fmt.Sprintf("| v%d | %s | %s | %s |\n",
			v.Version, v.ChangedAt.Format("2006-01-02 15:04"), v.ChangedBy, v.ChangeNotes))
	}

	return sb.String()
}

// =============================================================================
// v8.90 - Pipeline Analytics
// =============================================================================

// ExecutionMetrics tracks metrics for a single pipeline execution
type ExecutionMetrics struct {
	ExecutionID   string                   `json:"execution_id"`
	PipelineID    string                   `json:"pipeline_id"`
	PipelineName  string                   `json:"pipeline_name"`
	Status        string                   `json:"status"`
	StartedAt     time.Time                `json:"started_at"`
	CompletedAt   *time.Time               `json:"completed_at,omitempty"`
	Duration      time.Duration            `json:"duration"`
	StepMetrics   map[string]*StepMetrics  `json:"step_metrics"`
	TotalTokens   int                      `json:"total_tokens"`
	EstimatedCost float64                  `json:"estimated_cost"`
	ErrorCount    int                      `json:"error_count"`
	RetryCount    int                      `json:"retry_count"`
}

// StepMetrics tracks metrics for a single pipeline step
type StepMetrics struct {
	StepID      string        `json:"step_id"`
	StepName    string        `json:"step_name"`
	StepType    string        `json:"step_type"`
	Status      string        `json:"status"`
	StartedAt   time.Time     `json:"started_at"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	Duration    time.Duration `json:"duration"`
	Tokens      int           `json:"tokens"`
	Cost        float64       `json:"cost"`
	Error       string        `json:"error,omitempty"`
	RetryCount  int           `json:"retry_count"`
}

// PipelineAnalytics aggregates metrics across executions
type PipelineAnalytics struct {
	PipelineID        string    `json:"pipeline_id"`
	PipelineName      string    `json:"pipeline_name"`
	TotalExecutions   int       `json:"total_executions"`
	SuccessCount      int       `json:"success_count"`
	FailureCount      int       `json:"failure_count"`
	SuccessRate       float64   `json:"success_rate"`
	AvgDuration       time.Duration `json:"avg_duration"`
	P50Duration       time.Duration `json:"p50_duration"`
	P95Duration       time.Duration `json:"p95_duration"`
	P99Duration       time.Duration `json:"p99_duration"`
	TotalTokens       int       `json:"total_tokens"`
	TotalCost         float64   `json:"total_cost"`
	AvgTokens         int       `json:"avg_tokens"`
	AvgCost           float64   `json:"avg_cost"`
	LastExecution     *time.Time `json:"last_execution,omitempty"`
	LastSuccess       *time.Time `json:"last_success,omitempty"`
	LastFailure       *time.Time `json:"last_failure,omitempty"`
	StepAnalytics     map[string]*StepAnalytics `json:"step_analytics"`
}

// StepAnalytics aggregates metrics for a specific step across executions
type StepAnalytics struct {
	StepID          string        `json:"step_id"`
	StepName        string        `json:"step_name"`
	ExecutionCount  int           `json:"execution_count"`
	SuccessCount    int           `json:"success_count"`
	FailureCount    int           `json:"failure_count"`
	SuccessRate     float64       `json:"success_rate"`
	AvgDuration     time.Duration `json:"avg_duration"`
	P95Duration     time.Duration `json:"p95_duration"`
	TotalTokens     int           `json:"total_tokens"`
	TotalCost       float64       `json:"total_cost"`
	CommonErrors    map[string]int `json:"common_errors"`
}

// AnalyticsPeriod defines the time range for analytics
type AnalyticsPeriod string

const (
	PeriodHour  AnalyticsPeriod = "hour"
	PeriodDay   AnalyticsPeriod = "day"
	PeriodWeek  AnalyticsPeriod = "week"
	PeriodMonth AnalyticsPeriod = "month"
)

// TimeSeriesPoint represents a single data point in a time series
type TimeSeriesPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	Executions   int       `json:"executions"`
	SuccessRate  float64   `json:"success_rate"`
	AvgDuration  float64   `json:"avg_duration_ms"`
	Tokens       int       `json:"tokens"`
	Cost         float64   `json:"cost"`
}

// PipelineAnalyticsEngine provides analytics for pipeline executions
type PipelineAnalyticsEngine struct {
	store    *PipelineStore
	runner   *PipelineRunner
	metrics  map[string][]*ExecutionMetrics // pipelineID -> metrics
	mu       sync.RWMutex
}

// NewPipelineAnalyticsEngine creates a new analytics engine
func NewPipelineAnalyticsEngine(store *PipelineStore, runner *PipelineRunner) *PipelineAnalyticsEngine {
	return &PipelineAnalyticsEngine{
		store:   store,
		runner:  runner,
		metrics: make(map[string][]*ExecutionMetrics),
	}
}

// RecordExecution records metrics for a completed execution
func (e *PipelineAnalyticsEngine) RecordExecution(exec *PipelineExecution) *ExecutionMetrics {
	e.mu.Lock()
	defer e.mu.Unlock()

	metrics := &ExecutionMetrics{
		ExecutionID:  exec.ID,
		PipelineID:   exec.PipelineID,
		PipelineName: exec.PipelineName,
		Status:       exec.Status,
		StartedAt:    exec.StartedAt,
		CompletedAt:  exec.CompletedAt,
		StepMetrics:  make(map[string]*StepMetrics),
	}

	if exec.CompletedAt != nil {
		metrics.Duration = exec.CompletedAt.Sub(exec.StartedAt)
	}

	// Collect step metrics
	for stepID, result := range exec.StepResults {
		sm := &StepMetrics{
			StepID: stepID,
			Status: result.Status,
		}
		if result.StartedAt != nil {
			sm.StartedAt = *result.StartedAt
		}
		if result.CompletedAt != nil {
			sm.CompletedAt = result.CompletedAt
			if result.StartedAt != nil {
				sm.Duration = result.CompletedAt.Sub(*result.StartedAt)
			}
		}
		if result.Error != "" {
			sm.Error = result.Error
			metrics.ErrorCount++
		}
		// Estimate tokens from output size (rough heuristic)
		if result.Output != nil {
			outputStr := fmt.Sprintf("%v", result.Output)
			sm.Tokens = len(outputStr) / 4 // rough token estimate
			sm.Cost = float64(sm.Tokens) * 0.00001 // rough cost estimate
			metrics.TotalTokens += sm.Tokens
			metrics.EstimatedCost += sm.Cost
		}
		metrics.StepMetrics[stepID] = sm
	}

	// Store metrics
	e.metrics[exec.PipelineID] = append(e.metrics[exec.PipelineID], metrics)

	return metrics
}

// GetPipelineAnalytics returns aggregated analytics for a pipeline
func (e *PipelineAnalyticsEngine) GetPipelineAnalytics(pipelineID string, period AnalyticsPeriod) *PipelineAnalytics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	allMetrics := e.metrics[pipelineID]
	if len(allMetrics) == 0 {
		return nil
	}

	// Filter by period
	cutoff := e.getPeriodCutoff(period)
	var metrics []*ExecutionMetrics
	for _, m := range allMetrics {
		if m.StartedAt.After(cutoff) {
			metrics = append(metrics, m)
		}
	}

	if len(metrics) == 0 {
		return nil
	}

	analytics := &PipelineAnalytics{
		PipelineID:    pipelineID,
		PipelineName:  metrics[0].PipelineName,
		StepAnalytics: make(map[string]*StepAnalytics),
	}

	var durations []time.Duration
	stepDurations := make(map[string][]time.Duration)

	for _, m := range metrics {
		analytics.TotalExecutions++
		if m.Status == "completed" {
			analytics.SuccessCount++
			if m.CompletedAt != nil && (analytics.LastSuccess == nil || m.CompletedAt.After(*analytics.LastSuccess)) {
				analytics.LastSuccess = m.CompletedAt
			}
		} else if m.Status == "failed" {
			analytics.FailureCount++
			if m.CompletedAt != nil && (analytics.LastFailure == nil || m.CompletedAt.After(*analytics.LastFailure)) {
				analytics.LastFailure = m.CompletedAt
			}
		}

		if m.CompletedAt != nil && (analytics.LastExecution == nil || m.CompletedAt.After(*analytics.LastExecution)) {
			analytics.LastExecution = m.CompletedAt
		}

		durations = append(durations, m.Duration)
		analytics.TotalTokens += m.TotalTokens
		analytics.TotalCost += m.EstimatedCost

		// Aggregate step metrics
		for stepID, sm := range m.StepMetrics {
			if _, exists := analytics.StepAnalytics[stepID]; !exists {
				analytics.StepAnalytics[stepID] = &StepAnalytics{
					StepID:       stepID,
					StepName:     sm.StepName,
					CommonErrors: make(map[string]int),
				}
			}
			sa := analytics.StepAnalytics[stepID]
			sa.ExecutionCount++
			if sm.Status == "completed" {
				sa.SuccessCount++
			} else if sm.Status == "failed" {
				sa.FailureCount++
				if sm.Error != "" {
					sa.CommonErrors[sm.Error]++
				}
			}
			sa.TotalTokens += sm.Tokens
			sa.TotalCost += sm.Cost
			stepDurations[stepID] = append(stepDurations[stepID], sm.Duration)
		}
	}

	// Calculate rates and averages
	analytics.SuccessRate = float64(analytics.SuccessCount) / float64(analytics.TotalExecutions) * 100
	analytics.AvgTokens = analytics.TotalTokens / analytics.TotalExecutions
	analytics.AvgCost = analytics.TotalCost / float64(analytics.TotalExecutions)

	// Calculate duration percentiles
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		analytics.AvgDuration = e.avgDuration(durations)
		analytics.P50Duration = e.percentile(durations, 50)
		analytics.P95Duration = e.percentile(durations, 95)
		analytics.P99Duration = e.percentile(durations, 99)
	}

	// Calculate step analytics
	for stepID, sa := range analytics.StepAnalytics {
		if sa.ExecutionCount > 0 {
			sa.SuccessRate = float64(sa.SuccessCount) / float64(sa.ExecutionCount) * 100
			if durs := stepDurations[stepID]; len(durs) > 0 {
				sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
				sa.AvgDuration = e.avgDuration(durs)
				sa.P95Duration = e.percentile(durs, 95)
			}
		}
	}

	return analytics
}

// GetTimeSeries returns time series data for a pipeline
func (e *PipelineAnalyticsEngine) GetTimeSeries(pipelineID string, period AnalyticsPeriod, points int) []*TimeSeriesPoint {
	e.mu.RLock()
	defer e.mu.RUnlock()

	allMetrics := e.metrics[pipelineID]
	if len(allMetrics) == 0 {
		return nil
	}

	cutoff := e.getPeriodCutoff(period)
	now := time.Now()

	// Calculate bucket size
	bucketSize := now.Sub(cutoff) / time.Duration(points)

	// Initialize buckets
	buckets := make(map[int]*TimeSeriesPoint)
	for i := 0; i < points; i++ {
		t := cutoff.Add(bucketSize * time.Duration(i))
		buckets[i] = &TimeSeriesPoint{
			Timestamp: t,
		}
	}

	// Aggregate metrics into buckets
	for _, m := range allMetrics {
		if m.StartedAt.Before(cutoff) {
			continue
		}
		bucketIdx := int(m.StartedAt.Sub(cutoff) / bucketSize)
		if bucketIdx >= points {
			bucketIdx = points - 1
		}
		bucket := buckets[bucketIdx]
		bucket.Executions++
		if m.Status == "completed" {
			bucket.SuccessRate += 1
		}
		bucket.AvgDuration += float64(m.Duration.Milliseconds())
		bucket.Tokens += m.TotalTokens
		bucket.Cost += m.EstimatedCost
	}

	// Calculate averages
	result := make([]*TimeSeriesPoint, points)
	for i := 0; i < points; i++ {
		bucket := buckets[i]
		if bucket.Executions > 0 {
			bucket.SuccessRate = bucket.SuccessRate / float64(bucket.Executions) * 100
			bucket.AvgDuration = bucket.AvgDuration / float64(bucket.Executions)
		}
		result[i] = bucket
	}

	return result
}

// GetTopPipelines returns the most executed pipelines
func (e *PipelineAnalyticsEngine) GetTopPipelines(limit int, period AnalyticsPeriod) []*PipelineAnalytics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var allAnalytics []*PipelineAnalytics
	for pipelineID := range e.metrics {
		if analytics := e.GetPipelineAnalytics(pipelineID, period); analytics != nil {
			allAnalytics = append(allAnalytics, analytics)
		}
	}

	// Sort by execution count
	sort.Slice(allAnalytics, func(i, j int) bool {
		return allAnalytics[i].TotalExecutions > allAnalytics[j].TotalExecutions
	})

	if len(allAnalytics) > limit {
		allAnalytics = allAnalytics[:limit]
	}

	return allAnalytics
}

// GetFailingPipelines returns pipelines with low success rates
func (e *PipelineAnalyticsEngine) GetFailingPipelines(threshold float64, period AnalyticsPeriod) []*PipelineAnalytics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var failing []*PipelineAnalytics
	for pipelineID := range e.metrics {
		if analytics := e.GetPipelineAnalytics(pipelineID, period); analytics != nil {
			if analytics.SuccessRate < threshold && analytics.TotalExecutions >= 5 {
				failing = append(failing, analytics)
			}
		}
	}

	// Sort by success rate (lowest first)
	sort.Slice(failing, func(i, j int) bool {
		return failing[i].SuccessRate < failing[j].SuccessRate
	})

	return failing
}

// GetSlowestPipelines returns pipelines with highest average duration
func (e *PipelineAnalyticsEngine) GetSlowestPipelines(limit int, period AnalyticsPeriod) []*PipelineAnalytics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var allAnalytics []*PipelineAnalytics
	for pipelineID := range e.metrics {
		if analytics := e.GetPipelineAnalytics(pipelineID, period); analytics != nil {
			allAnalytics = append(allAnalytics, analytics)
		}
	}

	// Sort by average duration (highest first)
	sort.Slice(allAnalytics, func(i, j int) bool {
		return allAnalytics[i].AvgDuration > allAnalytics[j].AvgDuration
	})

	if len(allAnalytics) > limit {
		allAnalytics = allAnalytics[:limit]
	}

	return allAnalytics
}

// GetCostSummary returns cost summary across all pipelines
func (e *PipelineAnalyticsEngine) GetCostSummary(period AnalyticsPeriod) map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var totalCost float64
	var totalTokens int
	var totalExecutions int
	pipelineCosts := make(map[string]float64)

	cutoff := e.getPeriodCutoff(period)

	for pipelineID, metrics := range e.metrics {
		for _, m := range metrics {
			if m.StartedAt.After(cutoff) {
				totalCost += m.EstimatedCost
				totalTokens += m.TotalTokens
				totalExecutions++
				pipelineCosts[pipelineID] += m.EstimatedCost
			}
		}
	}

	// Find top cost pipelines
	type pipelineCostEntry struct {
		ID   string
		Cost float64
	}
	var entries []pipelineCostEntry
	for id, cost := range pipelineCosts {
		entries = append(entries, pipelineCostEntry{id, cost})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Cost > entries[j].Cost
	})

	topPipelines := make([]map[string]interface{}, 0)
	for i := 0; i < len(entries) && i < 5; i++ {
		topPipelines = append(topPipelines, map[string]interface{}{
			"pipeline_id": entries[i].ID,
			"cost":        entries[i].Cost,
		})
	}

	return map[string]interface{}{
		"period":           string(period),
		"total_cost":       totalCost,
		"total_tokens":     totalTokens,
		"total_executions": totalExecutions,
		"avg_cost":         totalCost / max(1.0, float64(totalExecutions)),
		"avg_tokens":       totalTokens / int(max(1.0, float64(totalExecutions))),
		"top_pipelines":    topPipelines,
	}
}

func (e *PipelineAnalyticsEngine) getPeriodCutoff(period AnalyticsPeriod) time.Time {
	now := time.Now()
	switch period {
	case PeriodHour:
		return now.Add(-time.Hour)
	case PeriodDay:
		return now.Add(-24 * time.Hour)
	case PeriodWeek:
		return now.Add(-7 * 24 * time.Hour)
	case PeriodMonth:
		return now.Add(-30 * 24 * time.Hour)
	default:
		return now.Add(-24 * time.Hour)
	}
}

func (e *PipelineAnalyticsEngine) avgDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return total / time.Duration(len(durations))
}

func (e *PipelineAnalyticsEngine) percentile(durations []time.Duration, p int) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	idx := (p * len(durations)) / 100
	if idx >= len(durations) {
		idx = len(durations) - 1
	}
	return durations[idx]
}

// FormatPipelineAnalytics formats analytics as markdown
func FormatPipelineAnalytics(analytics *PipelineAnalytics) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Pipeline Analytics: %s\n\n", analytics.PipelineName))

	sb.WriteString("## Overview\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Executions | %d |\n", analytics.TotalExecutions))
	sb.WriteString(fmt.Sprintf("| Success Rate | %.1f%% |\n", analytics.SuccessRate))
	sb.WriteString(fmt.Sprintf("| Average Duration | %s |\n", analytics.AvgDuration))
	sb.WriteString(fmt.Sprintf("| P95 Duration | %s |\n", analytics.P95Duration))
	sb.WriteString(fmt.Sprintf("| Total Tokens | %d |\n", analytics.TotalTokens))
	sb.WriteString(fmt.Sprintf("| Total Cost | $%.4f |\n", analytics.TotalCost))

	if analytics.LastExecution != nil {
		sb.WriteString(fmt.Sprintf("| Last Execution | %s |\n", analytics.LastExecution.Format("2006-01-02 15:04")))
	}

	if len(analytics.StepAnalytics) > 0 {
		sb.WriteString("\n## Step Analytics\n\n")
		sb.WriteString("| Step | Executions | Success Rate | Avg Duration | Tokens |\n")
		sb.WriteString("|------|------------|--------------|--------------|--------|\n")
		for _, sa := range analytics.StepAnalytics {
			sb.WriteString(fmt.Sprintf("| %s | %d | %.1f%% | %s | %d |\n",
				sa.StepID, sa.ExecutionCount, sa.SuccessRate, sa.AvgDuration, sa.TotalTokens))
		}
	}

	return sb.String()
}

// FormatCostSummary formats cost summary as markdown
func FormatCostSummary(summary map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("# Pipeline Cost Summary\n\n")

	sb.WriteString(fmt.Sprintf("**Period:** %s\n\n", summary["period"]))

	sb.WriteString("## Totals\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Total Executions | %d |\n", summary["total_executions"]))
	sb.WriteString(fmt.Sprintf("| Total Tokens | %d |\n", summary["total_tokens"]))
	sb.WriteString(fmt.Sprintf("| Total Cost | $%.4f |\n", summary["total_cost"]))
	sb.WriteString(fmt.Sprintf("| Avg Cost/Execution | $%.6f |\n", summary["avg_cost"]))
	sb.WriteString(fmt.Sprintf("| Avg Tokens/Execution | %d |\n", summary["avg_tokens"]))

	if topPipelines, ok := summary["top_pipelines"].([]map[string]interface{}); ok && len(topPipelines) > 0 {
		sb.WriteString("\n## Top Cost Pipelines\n\n")
		sb.WriteString("| Pipeline | Cost |\n")
		sb.WriteString("|----------|------|\n")
		for _, p := range topPipelines {
			sb.WriteString(fmt.Sprintf("| %s | $%.4f |\n", p["pipeline_id"], p["cost"]))
		}
	}

	return sb.String()
}

// FormatTimeSeriesChart formats time series as ASCII chart
func FormatTimeSeriesChart(series []*TimeSeriesPoint, metric string) string {
	if len(series) == 0 {
		return "_No data available._"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Time Series: %s\n\n", metric))

	// Find max value for scaling
	var maxVal float64
	values := make([]float64, len(series))
	for i, p := range series {
		switch metric {
		case "executions":
			values[i] = float64(p.Executions)
		case "success_rate":
			values[i] = p.SuccessRate
		case "duration":
			values[i] = p.AvgDuration
		case "cost":
			values[i] = p.Cost
		}
		if values[i] > maxVal {
			maxVal = values[i]
		}
	}

	// Draw simple bar chart
	chartHeight := 10
	for i, p := range series {
		barLen := 0
		if maxVal > 0 {
			barLen = int((values[i] / maxVal) * float64(chartHeight))
		}
		bar := strings.Repeat("█", barLen) + strings.Repeat("░", chartHeight-barLen)
		sb.WriteString(fmt.Sprintf("%s | %s %.2f\n", p.Timestamp.Format("15:04"), bar, values[i]))
	}

	return sb.String()
}

// =============================================================================
// v8.95 - External Pipeline Integrations
// =============================================================================

// IntegrationType defines the type of external integration
type IntegrationType string

const (
	IntegrationSlack    IntegrationType = "slack"
	IntegrationGitHub   IntegrationType = "github"
	IntegrationWebhook  IntegrationType = "webhook"
	IntegrationEmail    IntegrationType = "email"
)

// PipelineNotifyType defines when pipeline notifications are sent
type PipelineNotifyType string

const (
	PipelineNotifyOnStart    PipelineNotifyType = "on_start"
	PipelineNotifyOnComplete PipelineNotifyType = "on_complete"
	PipelineNotifyOnFailure  PipelineNotifyType = "on_failure"
	PipelineNotifyOnAll      PipelineNotifyType = "on_all"
)

// PipelineIntegration defines an external integration for a pipeline
type PipelineIntegration struct {
	ID            string                 `json:"id"`
	PipelineID    string                 `json:"pipeline_id"`
	Type          IntegrationType        `json:"type"`
	Name          string                 `json:"name"`
	Config        map[string]interface{} `json:"config"`
	NotifyOn      []PipelineNotifyType   `json:"notify_on"`
	Enabled       bool                   `json:"enabled"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// SlackTriggerConfig defines Slack-specific trigger configuration
type SlackTriggerConfig struct {
	ChannelID     string   `json:"channel_id"`
	TriggerPhrase string   `json:"trigger_phrase"` // e.g., "/run-pipeline"
	AllowedUsers  []string `json:"allowed_users,omitempty"`
	ResponseType  string   `json:"response_type"` // "in_channel" or "ephemeral"
}

// GitHubTriggerConfig defines GitHub-specific trigger configuration
type GitHubTriggerConfig struct {
	Repository    string   `json:"repository"`
	Events        []string `json:"events"` // "push", "pull_request", "workflow_dispatch"
	Branches      []string `json:"branches,omitempty"`
	Secret        string   `json:"secret,omitempty"`
}

// WebhookNotifyConfig defines webhook notification configuration
type WebhookNotifyConfig struct {
	URL           string            `json:"url"`
	Method        string            `json:"method"` // POST, PUT
	Headers       map[string]string `json:"headers,omitempty"`
	IncludeOutput bool              `json:"include_output"`
}

// PipelineNotification represents a notification to be sent
type PipelineNotification struct {
	ID           string                 `json:"id"`
	PipelineID   string                 `json:"pipeline_id"`
	ExecutionID  string                 `json:"execution_id"`
	Type         PipelineNotifyType     `json:"type"`
	Integration  *PipelineIntegration   `json:"integration"`
	Payload      map[string]interface{} `json:"payload"`
	Status       string                 `json:"status"` // pending, sent, failed
	SentAt       *time.Time             `json:"sent_at,omitempty"`
	Error        string                 `json:"error,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// IntegrationManager manages pipeline integrations
type IntegrationManager struct {
	integrations map[string][]*PipelineIntegration // pipelineID -> integrations
	triggers     map[string]*PipelineIntegration   // triggerKey -> integration
	runner       *PipelineRunner
	mu           sync.RWMutex
}

// NewIntegrationManager creates a new integration manager
func NewIntegrationManager(runner *PipelineRunner) *IntegrationManager {
	return &IntegrationManager{
		integrations: make(map[string][]*PipelineIntegration),
		triggers:     make(map[string]*PipelineIntegration),
		runner:       runner,
	}
}

// AddIntegration adds an integration to a pipeline
func (m *IntegrationManager) AddIntegration(pipelineID string, intType IntegrationType, name string, config map[string]interface{}, notifyOn []PipelineNotifyType) (*PipelineIntegration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	integration := &PipelineIntegration{
		ID:         fmt.Sprintf("int-%d", time.Now().UnixNano()),
		PipelineID: pipelineID,
		Type:       intType,
		Name:       name,
		Config:     config,
		NotifyOn:   notifyOn,
		Enabled:    true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	m.integrations[pipelineID] = append(m.integrations[pipelineID], integration)

	// Register trigger if applicable
	if intType == IntegrationSlack {
		if trigger, ok := config["trigger_phrase"].(string); ok && trigger != "" {
			m.triggers[trigger] = integration
		}
	}

	return integration, nil
}

// RemoveIntegration removes an integration
func (m *IntegrationManager) RemoveIntegration(integrationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for pipelineID, integrations := range m.integrations {
		for i, int := range integrations {
			if int.ID == integrationID {
				m.integrations[pipelineID] = append(integrations[:i], integrations[i+1:]...)
				// Remove trigger if exists
				for key, trigger := range m.triggers {
					if trigger.ID == integrationID {
						delete(m.triggers, key)
					}
				}
				return nil
			}
		}
	}
	return fmt.Errorf("integration not found: %s", integrationID)
}

// GetIntegrations returns all integrations for a pipeline
func (m *IntegrationManager) GetIntegrations(pipelineID string) []*PipelineIntegration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.integrations[pipelineID]
}

// ListAllIntegrations returns all integrations
func (m *IntegrationManager) ListAllIntegrations() []*PipelineIntegration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []*PipelineIntegration
	for _, integrations := range m.integrations {
		all = append(all, integrations...)
	}
	return all
}

// HandleSlackCommand handles a Slack slash command trigger
func (m *IntegrationManager) HandleSlackCommand(ctx context.Context, command string, userID string, channelID string, args map[string]interface{}) (*PipelineExecution, error) {
	m.mu.RLock()
	integration, exists := m.triggers[command]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no pipeline configured for command: %s", command)
	}

	if !integration.Enabled {
		return nil, fmt.Errorf("integration is disabled")
	}

	// Check allowed users if configured
	if config, ok := integration.Config["allowed_users"].([]string); ok && len(config) > 0 {
		allowed := false
		for _, u := range config {
			if u == userID {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("user not authorized to trigger this pipeline")
		}
	}

	// Execute the pipeline
	input := map[string]interface{}{
		"trigger_source": "slack",
		"user_id":        userID,
		"channel_id":     channelID,
	}
	for k, v := range args {
		input[k] = v
	}

	return m.runner.Execute(ctx, integration.PipelineID, input)
}

// HandleGitHubEvent handles a GitHub webhook event
func (m *IntegrationManager) HandleGitHubEvent(ctx context.Context, event string, payload map[string]interface{}) ([]*PipelineExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var executions []*PipelineExecution

	for _, integrations := range m.integrations {
		for _, integration := range integrations {
			if integration.Type != IntegrationGitHub || !integration.Enabled {
				continue
			}

			// Check if this integration handles this event
			events, _ := integration.Config["events"].([]string)
			eventMatches := false
			for _, e := range events {
				if e == event {
					eventMatches = true
					break
				}
			}
			if !eventMatches {
				continue
			}

			// Check branch filter if applicable
			if branches, ok := integration.Config["branches"].([]string); ok && len(branches) > 0 {
				branch := extractBranchFromPayload(payload)
				branchMatches := false
				for _, b := range branches {
					if b == branch || b == "*" {
						branchMatches = true
						break
					}
				}
				if !branchMatches {
					continue
				}
			}

			// Execute the pipeline
			input := map[string]interface{}{
				"trigger_source": "github",
				"event":          event,
				"payload":        payload,
			}

			exec, err := m.runner.Execute(ctx, integration.PipelineID, input)
			if err == nil {
				executions = append(executions, exec)
			}
		}
	}

	return executions, nil
}

func extractBranchFromPayload(payload map[string]interface{}) string {
	if ref, ok := payload["ref"].(string); ok {
		// Extract branch from ref like "refs/heads/main"
		if len(ref) > 11 && ref[:11] == "refs/heads/" {
			return ref[11:]
		}
	}
	return ""
}

// NotifyExecution sends notifications for a pipeline execution
func (m *IntegrationManager) NotifyExecution(ctx context.Context, exec *PipelineExecution, notifyType PipelineNotifyType) error {
	m.mu.RLock()
	integrations := m.integrations[exec.PipelineID]
	m.mu.RUnlock()

	for _, integration := range integrations {
		if !integration.Enabled {
			continue
		}

		// Check if this notification type is enabled
		shouldNotify := false
		for _, nt := range integration.NotifyOn {
			if nt == notifyType || nt == PipelineNotifyOnAll {
				shouldNotify = true
				break
			}
		}
		if !shouldNotify {
			continue
		}

		// Send notification based on type
		switch integration.Type {
		case IntegrationSlack:
			m.sendSlackNotification(ctx, integration, exec, notifyType)
		case IntegrationWebhook:
			m.sendWebhookNotification(ctx, integration, exec, notifyType)
		}
	}

	return nil
}

func (m *IntegrationManager) sendSlackNotification(ctx context.Context, integration *PipelineIntegration, exec *PipelineExecution, notifyType PipelineNotifyType) error {
	channelID, _ := integration.Config["channel_id"].(string)
	if channelID == "" {
		return fmt.Errorf("no channel_id configured")
	}

	// Build message based on notification type
	var emoji, status string
	switch notifyType {
	case PipelineNotifyOnStart:
		emoji = ":rocket:"
		status = "started"
	case PipelineNotifyOnComplete:
		emoji = ":white_check_mark:"
		status = "completed"
	case PipelineNotifyOnFailure:
		emoji = ":x:"
		status = "failed"
	}

	message := fmt.Sprintf("%s Pipeline *%s* %s\nExecution ID: `%s`", emoji, exec.PipelineName, status, exec.ID[:12])
	if exec.Error != "" {
		message += fmt.Sprintf("\nError: %s", exec.Error)
	}

	// Note: Actual Slack API call would go here
	// This is a placeholder that could be integrated with the existing Slack client
	_ = message
	return nil
}

func (m *IntegrationManager) sendWebhookNotification(ctx context.Context, integration *PipelineIntegration, exec *PipelineExecution, notifyType PipelineNotifyType) error {
	url, _ := integration.Config["url"].(string)
	if url == "" {
		return fmt.Errorf("no url configured")
	}

	// Build payload
	payload := map[string]interface{}{
		"pipeline_id":   exec.PipelineID,
		"pipeline_name": exec.PipelineName,
		"execution_id":  exec.ID,
		"status":        exec.Status,
		"started_at":    exec.StartedAt,
		"notify_type":   string(notifyType),
	}

	if exec.CompletedAt != nil {
		payload["completed_at"] = exec.CompletedAt
		payload["duration_ms"] = exec.CompletedAt.Sub(exec.StartedAt).Milliseconds()
	}

	if includeOutput, _ := integration.Config["include_output"].(bool); includeOutput && exec.Output != nil {
		payload["output"] = exec.Output
	}

	if exec.Error != "" {
		payload["error"] = exec.Error
	}

	// Note: Actual HTTP call would go here
	_ = payload
	return nil
}

// ToggleIntegration enables or disables an integration
func (m *IntegrationManager) ToggleIntegration(integrationID string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, integrations := range m.integrations {
		for _, int := range integrations {
			if int.ID == integrationID {
				int.Enabled = enabled
				int.UpdatedAt = time.Now()
				return nil
			}
		}
	}
	return fmt.Errorf("integration not found: %s", integrationID)
}

// FormatIntegrationList formats integrations as markdown
func FormatIntegrationList(integrations []*PipelineIntegration) string {
	var sb strings.Builder
	sb.WriteString("# Pipeline Integrations\n\n")

	if len(integrations) == 0 {
		sb.WriteString("_No integrations configured._\n")
		return sb.String()
	}

	sb.WriteString("| ID | Pipeline | Type | Name | Status | Notify On |\n")
	sb.WriteString("|----|----------|------|------|--------|----------|\n")

	for _, int := range integrations {
		status := "Enabled"
		if !int.Enabled {
			status = "Disabled"
		}
		notifyStr := ""
		for i, n := range int.NotifyOn {
			if i > 0 {
				notifyStr += ", "
			}
			notifyStr += string(n)
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %s |\n",
			int.ID[:12], int.PipelineID[:8], int.Type, int.Name, status, notifyStr))
	}

	return sb.String()
}

// SlackCommandPayload represents an incoming Slack slash command
type SlackCommandPayload struct {
	Command     string `json:"command"`
	Text        string `json:"text"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	TeamID      string `json:"team_id"`
	ResponseURL string `json:"response_url"`
}

// GitHubWebhookPayload represents common GitHub webhook fields
type GitHubWebhookPayload struct {
	Action     string                 `json:"action,omitempty"`
	Ref        string                 `json:"ref,omitempty"`
	Repository map[string]interface{} `json:"repository,omitempty"`
	Sender     map[string]interface{} `json:"sender,omitempty"`
	Commits    []interface{}          `json:"commits,omitempty"`
}

// ParseSlackCommand parses a Slack command into arguments
func ParseSlackCommand(text string) map[string]interface{} {
	args := make(map[string]interface{})
	parts := strings.Fields(text)

	for i := 0; i < len(parts); i++ {
		if strings.HasPrefix(parts[i], "--") && i+1 < len(parts) {
			key := strings.TrimPrefix(parts[i], "--")
			args[key] = parts[i+1]
			i++
		} else {
			// Positional argument
			args[fmt.Sprintf("arg%d", i)] = parts[i]
		}
	}

	return args
}

// =============================================================================
// v9.0 - Intelligent Routing & Auto-Pipelines
// =============================================================================

// IntentType represents the detected intent from a user query
type IntentType string

const (
	IntentInvestigate   IntentType = "investigate"
	IntentMonitor       IntentType = "monitor"
	IntentDeploy        IntentType = "deploy"
	IntentDebug         IntentType = "debug"
	IntentAnalyze       IntentType = "analyze"
	IntentReport        IntentType = "report"
	IntentAutomate      IntentType = "automate"
	IntentUnknown       IntentType = "unknown"
)

// Intent represents a detected user intent with confidence
type Intent struct {
	Type       IntentType             `json:"type"`
	Confidence float64                `json:"confidence"`
	Entities   map[string]string      `json:"entities"`
	Context    map[string]interface{} `json:"context"`
}

// ToolRecommendation represents a recommended tool for an intent
type ToolRecommendation struct {
	ToolName    string  `json:"tool_name"`
	Relevance   float64 `json:"relevance"`
	Reason      string  `json:"reason"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// AutoPipelineRequest represents a request to generate a pipeline
type AutoPipelineRequest struct {
	Query       string                 `json:"query"`
	Intent      *Intent                `json:"intent,omitempty"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
	MaxSteps    int                    `json:"max_steps,omitempty"`
}

// AutoPipelineResult represents the result of auto-pipeline generation
type AutoPipelineResult struct {
	Pipeline     *Pipeline              `json:"pipeline"`
	Intent       *Intent                `json:"intent"`
	Tools        []*ToolRecommendation  `json:"tools"`
	Confidence   float64                `json:"confidence"`
	Explanation  string                 `json:"explanation"`
}

// IntentPatterns defines patterns for intent detection
var IntentPatterns = map[IntentType][]string{
	IntentInvestigate: {
		"investigate", "look into", "check", "what's wrong", "why is",
		"troubleshoot", "diagnose", "find out", "examine", "inspect",
	},
	IntentMonitor: {
		"monitor", "watch", "track", "observe", "keep an eye",
		"alert me", "notify", "status", "health", "how is",
	},
	IntentDeploy: {
		"deploy", "release", "ship", "push", "rollout",
		"update", "upgrade", "migrate", "launch",
	},
	IntentDebug: {
		"debug", "fix", "resolve", "error", "exception",
		"crash", "failing", "broken", "not working",
	},
	IntentAnalyze: {
		"analyze", "review", "evaluate", "assess", "compare",
		"benchmark", "performance", "metrics", "statistics",
	},
	IntentReport: {
		"report", "summary", "overview", "digest", "briefing",
		"standup", "weekly", "daily", "status report",
	},
	IntentAutomate: {
		"automate", "schedule", "recurring", "every day", "cron",
		"trigger", "webhook", "on push", "on merge",
	},
}

// EntityPatterns defines patterns for entity extraction
var EntityPatterns = map[string][]string{
	"cluster": {
		"cluster", "context", "k8s", "kubernetes", "eks", "gke",
	},
	"customer": {
		"customer", "client", "tenant", "account",
	},
	"service": {
		"service", "deployment", "pod", "container", "app",
	},
	"timeframe": {
		"today", "yesterday", "this week", "last hour", "past",
	},
}

// IntentDetector detects intents from natural language queries
type IntentDetector struct {
	patterns     map[IntentType][]string
	entityPatterns map[string][]string
}

// NewIntentDetector creates a new intent detector
func NewIntentDetector() *IntentDetector {
	return &IntentDetector{
		patterns:       IntentPatterns,
		entityPatterns: EntityPatterns,
	}
}

// DetectIntent analyzes a query and returns the detected intent
func (d *IntentDetector) DetectIntent(query string) *Intent {
	query = strings.ToLower(query)

	intent := &Intent{
		Type:       IntentUnknown,
		Confidence: 0.0,
		Entities:   make(map[string]string),
		Context:    make(map[string]interface{}),
	}

	// Score each intent type
	scores := make(map[IntentType]float64)
	for intentType, patterns := range d.patterns {
		score := 0.0
		matchCount := 0
		for _, pattern := range patterns {
			if strings.Contains(query, pattern) {
				score += 1.0
				matchCount++
			}
		}
		if matchCount > 0 {
			scores[intentType] = score / float64(len(patterns))
		}
	}

	// Find best match
	var bestType IntentType
	var bestScore float64
	for t, s := range scores {
		if s > bestScore {
			bestType = t
			bestScore = s
		}
	}

	if bestScore > 0 {
		intent.Type = bestType
		intent.Confidence = bestScore
	}

	// Extract entities
	for entityType, patterns := range d.entityPatterns {
		for _, pattern := range patterns {
			if idx := strings.Index(query, pattern); idx != -1 {
				// Try to extract the value after the pattern
				remaining := query[idx+len(pattern):]
				words := strings.Fields(remaining)
				if len(words) > 0 {
					intent.Entities[entityType] = words[0]
				}
				break
			}
		}
	}

	return intent
}

// ToolSelector selects appropriate tools for an intent
type ToolSelector struct {
	toolMappings map[IntentType][]string
}

// NewToolSelector creates a new tool selector
func NewToolSelector() *ToolSelector {
	return &ToolSelector{
		toolMappings: map[IntentType][]string{
			IntentInvestigate: {
				"webb_cluster_health_full",
				"webb_k8s_pods",
				"webb_k8s_logs",
				"webb_k8s_events",
				"webb_grafana_alerts",
				"webb_investigate_summary",
			},
			IntentMonitor: {
				"webb_cluster_health_full",
				"webb_grafana_alerts",
				"webb_k8s_pods",
				"webb_queue_health_full",
				"webb_database_health_full",
			},
			IntentDeploy: {
				"webb_k8s_deployments",
				"webb_k8s_rollout_status",
				"webb_helm_release_health",
				"webb_github_actions",
				"webb_preflight_full",
			},
			IntentDebug: {
				"webb_k8s_logs",
				"webb_k8s_events",
				"webb_k8s_pod_diagnostic",
				"webb_db_long_queries",
				"webb_rca_suggest",
			},
			IntentAnalyze: {
				"webb_pipeline_analytics",
				"webb_metrics_summary",
				"webb_benchmark_summary",
				"webb_model_costs",
				"webb_sentiment_analyze",
			},
			IntentReport: {
				"webb_ticket_summary",
				"webb_oncall_dashboard",
				"webb_standup_briefing",
				"webb_month_analysis",
				"webb_my_assignments",
			},
			IntentAutomate: {
				"webb_pipeline_schedule_create",
				"webb_pipeline_integration_add",
				"webb_pipeline_run",
				"webb_pipeline_list",
			},
		},
	}
}

// SelectTools returns recommended tools for an intent
func (s *ToolSelector) SelectTools(intent *Intent) []*ToolRecommendation {
	var recommendations []*ToolRecommendation

	tools, exists := s.toolMappings[intent.Type]
	if !exists {
		return recommendations
	}

	for i, tool := range tools {
		relevance := 1.0 - (float64(i) * 0.1) // Decreasing relevance
		if relevance < 0.5 {
			relevance = 0.5
		}

		rec := &ToolRecommendation{
			ToolName:   tool,
			Relevance:  relevance * intent.Confidence,
			Reason:     fmt.Sprintf("Recommended for %s intent", intent.Type),
			Parameters: make(map[string]interface{}),
		}

		// Add entity-based parameters
		if cluster, ok := intent.Entities["cluster"]; ok {
			rec.Parameters["context"] = cluster
		}
		if customer, ok := intent.Entities["customer"]; ok {
			rec.Parameters["customer"] = customer
		}

		recommendations = append(recommendations, rec)
	}

	return recommendations
}

// AutoPipelineGenerator generates pipelines from natural language
type AutoPipelineGenerator struct {
	detector     *IntentDetector
	selector     *ToolSelector
	runner       *PipelineRunner
}

// NewAutoPipelineGenerator creates a new auto-pipeline generator
func NewAutoPipelineGenerator(runner *PipelineRunner) *AutoPipelineGenerator {
	return &AutoPipelineGenerator{
		detector: NewIntentDetector(),
		selector: NewToolSelector(),
		runner:   runner,
	}
}

// GeneratePipeline creates a pipeline from a natural language query
func (g *AutoPipelineGenerator) GeneratePipeline(req *AutoPipelineRequest) (*AutoPipelineResult, error) {
	// Detect intent
	intent := req.Intent
	if intent == nil {
		intent = g.detector.DetectIntent(req.Query)
	}

	if intent.Type == IntentUnknown {
		return nil, fmt.Errorf("could not determine intent from query: %s", req.Query)
	}

	// Select tools
	tools := g.selector.SelectTools(intent)
	if len(tools) == 0 {
		return nil, fmt.Errorf("no tools available for intent: %s", intent.Type)
	}

	// Limit steps if specified
	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 5
	}
	if len(tools) > maxSteps {
		tools = tools[:maxSteps]
	}

	// Generate pipeline
	pipeline := &Pipeline{
		ID:          fmt.Sprintf("auto-%d", time.Now().UnixNano()),
		Name:        fmt.Sprintf("Auto: %s", truncateQuery(req.Query, 50)),
		Description: fmt.Sprintf("Auto-generated pipeline for: %s", req.Query),
		Steps:       make(map[string]*PipelineStep),
		CreatedAt:   time.Now(),
	}

	// Create steps from recommended tools
	for i, tool := range tools {
		stepID := fmt.Sprintf("step-%d", i+1)
		step := &PipelineStep{
			ID:     stepID,
			Name:   tool.ToolName,
			Type:   StepTypeAgent,
			Config: map[string]interface{}{
				"tool": tool.ToolName,
			},
		}

		// Add parameters
		for k, v := range tool.Parameters {
			step.Config[k] = v
		}

		// Link to next step
		if i < len(tools)-1 {
			step.NextStep = fmt.Sprintf("step-%d", i+2)
		}

		pipeline.Steps[stepID] = step
	}

	// Set start step
	if len(tools) > 0 {
		pipeline.StartStep = "step-1"
	}

	// Calculate overall confidence
	var totalRelevance float64
	for _, tool := range tools {
		totalRelevance += tool.Relevance
	}
	confidence := totalRelevance / float64(len(tools))

	// Generate explanation
	explanation := fmt.Sprintf(
		"Detected %s intent (%.0f%% confidence). Generated %d-step pipeline using: %s",
		intent.Type,
		intent.Confidence*100,
		len(tools),
		joinToolNames(tools),
	)

	return &AutoPipelineResult{
		Pipeline:    pipeline,
		Intent:      intent,
		Tools:       tools,
		Confidence:  confidence,
		Explanation: explanation,
	}, nil
}

// ExecuteQuery generates and executes a pipeline from a query
func (g *AutoPipelineGenerator) ExecuteQuery(ctx context.Context, query string) (*PipelineExecution, error) {
	result, err := g.GeneratePipeline(&AutoPipelineRequest{Query: query})
	if err != nil {
		return nil, err
	}

	// Add the pipeline to runner
	g.runner.AddPipeline(result.Pipeline)

	// Execute the pipeline
	return g.runner.Execute(ctx, result.Pipeline.ID, map[string]interface{}{
		"query":       query,
		"auto_intent": result.Intent.Type,
	})
}

// AddPipeline adds a pipeline to the runner
func (r *PipelineRunner) AddPipeline(pipeline *Pipeline) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pipelines[pipeline.ID] = pipeline
}

func truncateQuery(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func joinToolNames(tools []*ToolRecommendation) string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.ToolName
	}
	return strings.Join(names, ", ")
}

// FormatAutoPipelineResult formats the result as markdown
func FormatAutoPipelineResult(result *AutoPipelineResult) string {
	var sb strings.Builder

	sb.WriteString("# Auto-Generated Pipeline\n\n")
	sb.WriteString(fmt.Sprintf("**Name:** %s\n", result.Pipeline.Name))
	sb.WriteString(fmt.Sprintf("**Confidence:** %.1f%%\n\n", result.Confidence*100))

	sb.WriteString("## Detected Intent\n\n")
	sb.WriteString(fmt.Sprintf("- **Type:** %s\n", result.Intent.Type))
	sb.WriteString(fmt.Sprintf("- **Confidence:** %.1f%%\n", result.Intent.Confidence*100))
	if len(result.Intent.Entities) > 0 {
		sb.WriteString("- **Entities:**\n")
		for k, v := range result.Intent.Entities {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", k, v))
		}
	}

	sb.WriteString("\n## Pipeline Steps\n\n")
	sb.WriteString("| # | Tool | Relevance | Parameters |\n")
	sb.WriteString("|---|------|-----------|------------|\n")
	for i, tool := range result.Tools {
		params := ""
		for k, v := range tool.Parameters {
			if params != "" {
				params += ", "
			}
			params += fmt.Sprintf("%s=%v", k, v)
		}
		if params == "" {
			params = "-"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %.0f%% | %s |\n",
			i+1, tool.ToolName, tool.Relevance*100, params))
	}

	sb.WriteString(fmt.Sprintf("\n## Explanation\n\n%s\n", result.Explanation))

	return sb.String()
}
