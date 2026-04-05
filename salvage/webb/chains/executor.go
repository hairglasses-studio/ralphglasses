package chains

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ToolInvoker is the interface for invoking MCP tools
type ToolInvoker interface {
	InvokeTool(ctx context.Context, toolName string, params map[string]interface{}) (map[string]interface{}, error)
}

// Executor executes chain definitions
type Executor struct {
	registry      *Registry
	invoker       ToolInvoker
	state         StateStore
	notifications *NotificationManager
	metrics       *MetricsCollector
	mu            sync.RWMutex
	executions    map[string]*ChainExecution
	gateQueue     map[string]chan GateApproval
}

// SetNotificationManager sets the notification manager
func (e *Executor) SetNotificationManager(nm *NotificationManager) {
	e.notifications = nm
}

// SetMetricsCollector sets the metrics collector
func (e *Executor) SetMetricsCollector(mc *MetricsCollector) {
	e.metrics = mc
}

// NewExecutor creates a new chain executor
func NewExecutor(registry *Registry, invoker ToolInvoker, state StateStore) *Executor {
	return &Executor{
		registry:   registry,
		invoker:    invoker,
		state:      state,
		executions: make(map[string]*ChainExecution),
		gateQueue:  make(map[string]chan GateApproval),
	}
}

// ExecuteOptions configures chain execution
type ExecuteOptions struct {
	Input       map[string]interface{}
	TriggeredBy string
	ParentExecID string
	Async       bool
}

// Execute runs a chain and returns the execution result
func (e *Executor) Execute(ctx context.Context, chainName string, opts ExecuteOptions) (*ChainExecution, error) {
	chain, err := e.registry.Get(chainName)
	if err != nil {
		return nil, err
	}

	exec := &ChainExecution{
		ID:           uuid.New().String(),
		ChainName:    chainName,
		Status:       StatusRunning,
		StartedAt:    time.Now(),
		Input:        opts.Input,
		Variables:    make(map[string]interface{}),
		StepResults:  make(map[string]StepResult),
		TriggeredBy:  opts.TriggeredBy,
		ParentExecID: opts.ParentExecID,
	}

	// Initialize variables from chain defaults
	for k, v := range chain.Variables {
		exec.Variables[k] = e.interpolate(v, exec.Variables, opts.Input)
	}

	// Merge input into variables
	for k, v := range opts.Input {
		exec.Variables[k] = v
	}

	// Store execution
	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.mu.Unlock()

	// Save initial state
	if e.state != nil {
		if err := e.state.SaveExecution(exec); err != nil {
			// Log but continue
		}
	}

	if opts.Async {
		go e.runChain(context.Background(), chain, exec)
		return exec, nil
	}

	return e.runChain(ctx, chain, exec)
}

func (e *Executor) runChain(ctx context.Context, chain *ChainDefinition, exec *ChainExecution) (*ChainExecution, error) {
	// Notify chain started
	if e.notifications != nil {
		e.notifications.NotifyChainStart(ctx, exec)
	}

	// Parse timeout
	var cancel context.CancelFunc
	if chain.Timeout != "" {
		if timeout, err := time.ParseDuration(chain.Timeout); err == nil {
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	// Execute steps sequentially
	for _, step := range chain.Steps {
		select {
		case <-ctx.Done():
			exec.Status = StatusCancelled
			exec.Error = ctx.Err().Error()
			e.finalizeExecution(exec)
			if e.notifications != nil {
				e.notifications.NotifyChainFailed(ctx, exec, ctx.Err())
			}
			return exec, ctx.Err()
		default:
		}

		exec.CurrentStep = step.ID
		if err := e.executeStep(ctx, &step, exec); err != nil {
			// Check error handling policy
			if chain.OnError != nil && chain.OnError.Action == "continue" {
				continue
			}
			exec.Status = StatusFailed
			exec.Error = err.Error()
			e.finalizeExecution(exec)
			if e.notifications != nil {
				e.notifications.NotifyChainFailed(ctx, exec, err)
			}
			return exec, err
		}
	}

	exec.Status = StatusCompleted
	e.finalizeExecution(exec)
	if e.notifications != nil {
		e.notifications.NotifyChainComplete(ctx, exec)
	}
	return exec, nil
}

func (e *Executor) executeStep(ctx context.Context, step *ChainStep, exec *ChainExecution) error {
	result := StepResult{
		StepID:    step.ID,
		Status:    StatusRunning,
		StartedAt: time.Now(),
		Attempts:  1,
	}

	var err error
	var output map[string]interface{}

	stepType := step.Type
	if stepType == "" {
		stepType = StepTypeTool
	}

	switch stepType {
	case StepTypeTool:
		output, err = e.executeToolStep(ctx, step, exec)
	case StepTypeChain:
		output, err = e.executeChainStep(ctx, step, exec)
	case StepTypeParallel:
		output, err = e.executeParallelStep(ctx, step, exec)
	case StepTypeBranch:
		output, err = e.executeBranchStep(ctx, step, exec)
	case StepTypeGate:
		output, err = e.executeGateStep(ctx, step, exec)
	default:
		err = fmt.Errorf("unknown step type: %s", stepType)
	}

	now := time.Now()
	result.CompletedAt = &now

	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()

		// Handle retry
		if step.Retry != nil && result.Attempts < step.Retry.MaxAttempts {
			delay := time.Second * 5
			if step.Retry.Delay != "" {
				if d, parseErr := time.ParseDuration(step.Retry.Delay); parseErr == nil {
					delay = d
				}
			}
			time.Sleep(delay)
			result.Attempts++
			return e.executeStep(ctx, step, exec)
		}
	} else {
		result.Status = StatusCompleted
		result.Output = output

		// Store result in variables if requested
		if step.StoreAs != "" && output != nil {
			exec.Variables[step.StoreAs] = output
		}
	}

	exec.StepResults[step.ID] = result

	// Save checkpoint
	if e.state != nil {
		checkpoint := &Checkpoint{
			ExecutionID: exec.ID,
			ChainName:   exec.ChainName,
			StepID:      step.ID,
			Variables:   exec.Variables,
			StepResults: exec.StepResults,
			CreatedAt:   time.Now(),
		}
		e.state.SaveCheckpoint(checkpoint)
	}

	// Check continue_on policy
	if err != nil && step.ContinueOn == "failure" || step.ContinueOn == "always" {
		return nil
	}

	return err
}

func (e *Executor) executeToolStep(ctx context.Context, step *ChainStep, exec *ChainExecution) (map[string]interface{}, error) {
	if e.invoker == nil {
		return nil, fmt.Errorf("no tool invoker configured")
	}

	// Interpolate parameters
	params := make(map[string]interface{})
	for k, v := range step.Params {
		params[k] = e.interpolate(v, exec.Variables, exec.Input)
	}

	return e.invoker.InvokeTool(ctx, step.Tool, params)
}

func (e *Executor) executeChainStep(ctx context.Context, step *ChainStep, exec *ChainExecution) (map[string]interface{}, error) {
	// Execute sub-chain
	subExec, err := e.Execute(ctx, step.Chain, ExecuteOptions{
		Input:        exec.Variables,
		TriggeredBy:  "chain:" + exec.ChainName,
		ParentExecID: exec.ID,
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"execution_id": subExec.ID,
		"status":       subExec.Status,
		"variables":    subExec.Variables,
	}, nil
}

func (e *Executor) executeParallelStep(ctx context.Context, step *ChainStep, exec *ChainExecution) (map[string]interface{}, error) {
	var wg sync.WaitGroup
	results := make(map[string]interface{})
	var resultsMu sync.Mutex
	var firstErr error
	var errMu sync.Mutex

	for i := range step.Steps {
		wg.Add(1)
		go func(s *ChainStep) {
			defer wg.Done()

			if err := e.executeStep(ctx, s, exec); err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}

			// Collect results
			if result, exists := exec.StepResults[s.ID]; exists && result.Output != nil {
				resultsMu.Lock()
				results[s.ID] = result.Output
				resultsMu.Unlock()
			}
		}(&step.Steps[i])
	}

	wg.Wait()

	if firstErr != nil {
		return results, firstErr
	}

	return results, nil
}

func (e *Executor) executeBranchStep(ctx context.Context, step *ChainStep, exec *ChainExecution) (map[string]interface{}, error) {
	// Evaluate condition
	conditionResult := e.evaluateCondition(step.Condition, exec.Variables, exec.Input)

	// Find matching branch
	var branchSteps []ChainStep
	if steps, exists := step.Branches[conditionResult]; exists {
		branchSteps = steps
	} else if steps, exists := step.Branches["default"]; exists {
		branchSteps = steps
	} else {
		// No matching branch, skip
		return map[string]interface{}{"branch": "none"}, nil
	}

	// Execute branch steps
	for i := range branchSteps {
		if err := e.executeStep(ctx, &branchSteps[i], exec); err != nil {
			return nil, err
		}
	}

	return map[string]interface{}{"branch": conditionResult}, nil
}

func (e *Executor) executeGateStep(ctx context.Context, step *ChainStep, exec *ChainExecution) (map[string]interface{}, error) {
	// Create approval channel
	gateKey := fmt.Sprintf("%s:%s", exec.ID, step.ID)
	approvalChan := make(chan GateApproval, 1)

	e.mu.Lock()
	e.gateQueue[gateKey] = approvalChan
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.gateQueue, gateKey)
		e.mu.Unlock()
	}()

	// Update execution status
	exec.Status = StatusPaused

	// Notify gate awaiting approval
	if e.notifications != nil {
		e.notifications.NotifyGateAwaiting(ctx, exec, step)
	}

	// Parse timeout
	timeout := time.Hour // Default 1 hour
	if step.GateTimeout != "" {
		if t, err := time.ParseDuration(step.GateTimeout); err == nil {
			timeout = t
		}
	}

	select {
	case approval := <-approvalChan:
		exec.Status = StatusRunning
		if !approval.Approved {
			return nil, fmt.Errorf("gate rejected by %s: %s", approval.ApprovedBy, approval.Comment)
		}
		return map[string]interface{}{
			"approved":    true,
			"approved_by": approval.ApprovedBy,
			"comment":     approval.Comment,
		}, nil

	case <-time.After(timeout):
		exec.Status = StatusRunning
		switch step.OnTimeout {
		case "continue":
			return map[string]interface{}{"approved": true, "auto": true}, nil
		case "abort":
			return nil, fmt.Errorf("gate timed out after %s", timeout)
		default:
			return nil, fmt.Errorf("gate timed out after %s", timeout)
		}

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ApproveGate approves a pending gate
func (e *Executor) ApproveGate(executionID, stepID string, approved bool, approvedBy, comment string) error {
	gateKey := fmt.Sprintf("%s:%s", executionID, stepID)

	e.mu.RLock()
	approvalChan, exists := e.gateQueue[gateKey]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no pending gate found for execution %s step %s", executionID, stepID)
	}

	approvalChan <- GateApproval{
		ExecutionID: executionID,
		StepID:      stepID,
		Approved:    approved,
		ApprovedBy:  approvedBy,
		ApprovedAt:  time.Now(),
		Comment:     comment,
	}

	return nil
}

// GetExecution returns the current state of an execution
func (e *Executor) GetExecution(executionID string) (*ChainExecution, error) {
	e.mu.RLock()
	exec, exists := e.executions[executionID]
	e.mu.RUnlock()

	if exists {
		return exec, nil
	}

	// Try loading from state store
	if e.state != nil {
		return e.state.GetExecution(executionID)
	}

	return nil, fmt.Errorf("execution %s not found", executionID)
}

// Resume resumes an execution from a checkpoint
func (e *Executor) Resume(ctx context.Context, executionID string) (*ChainExecution, error) {
	if e.state == nil {
		return nil, fmt.Errorf("no state store configured")
	}

	checkpoint, err := e.state.GetLatestCheckpoint(executionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoint: %w", err)
	}

	chain, err := e.registry.Get(checkpoint.ChainName)
	if err != nil {
		return nil, err
	}

	// Find the step index to resume from
	resumeIdx := -1
	for i, step := range chain.Steps {
		if step.ID == checkpoint.StepID {
			resumeIdx = i + 1 // Resume from next step
			break
		}
	}

	if resumeIdx < 0 || resumeIdx >= len(chain.Steps) {
		return nil, fmt.Errorf("invalid checkpoint step: %s", checkpoint.StepID)
	}

	// Restore execution state
	exec := &ChainExecution{
		ID:          executionID,
		ChainName:   checkpoint.ChainName,
		Status:      StatusRunning,
		StartedAt:   time.Now(),
		Variables:   checkpoint.Variables,
		StepResults: checkpoint.StepResults,
		TriggeredBy: "resume",
	}

	e.mu.Lock()
	e.executions[exec.ID] = exec
	e.mu.Unlock()

	// Execute remaining steps
	for i := resumeIdx; i < len(chain.Steps); i++ {
		exec.CurrentStep = chain.Steps[i].ID
		if err := e.executeStep(ctx, &chain.Steps[i], exec); err != nil {
			exec.Status = StatusFailed
			exec.Error = err.Error()
			e.finalizeExecution(exec)
			return exec, err
		}
	}

	exec.Status = StatusCompleted
	e.finalizeExecution(exec)
	return exec, nil
}

// Cancel cancels a running execution
func (e *Executor) Cancel(executionID string) error {
	e.mu.Lock()
	exec, exists := e.executions[executionID]
	if exists {
		exec.Status = StatusCancelled
		now := time.Now()
		exec.CompletedAt = &now
	}
	e.mu.Unlock()

	if !exists {
		return fmt.Errorf("execution %s not found", executionID)
	}

	return nil
}

func (e *Executor) finalizeExecution(exec *ChainExecution) {
	now := time.Now()
	exec.CompletedAt = &now
	exec.CurrentStep = ""

	if e.state != nil {
		e.state.SaveExecution(exec)
	}

	// Record metrics
	if e.metrics != nil {
		e.metrics.RecordExecution(exec)
	}
}

// interpolate replaces {{ variable }} patterns in a string
func (e *Executor) interpolate(template string, variables, input map[string]interface{}) string {
	re := regexp.MustCompile(`\{\{\s*([^}]+)\s*\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable path
		path := strings.TrimSpace(match[2 : len(match)-2])

		// Check trigger.* for input
		if strings.HasPrefix(path, "trigger.") {
			key := strings.TrimPrefix(path, "trigger.")
			if val := getNestedValue(input, key); val != nil {
				return fmt.Sprintf("%v", val)
			}
		}

		// Check steps.* for step results
		if strings.HasPrefix(path, "steps.") {
			key := strings.TrimPrefix(path, "steps.")
			if val := getNestedValue(variables, key); val != nil {
				return fmt.Sprintf("%v", val)
			}
		}

		// Check input.* for explicit input reference
		if strings.HasPrefix(path, "input.") {
			key := strings.TrimPrefix(path, "input.")
			if val := getNestedValue(input, key); val != nil {
				return fmt.Sprintf("%v", val)
			}
		}

		// Check variables with nested path support
		if val := getNestedValue(variables, path); val != nil {
			return fmt.Sprintf("%v", val)
		}

		// Check input with nested path support
		if val := getNestedValue(input, path); val != nil {
			return fmt.Sprintf("%v", val)
		}

		return match // Return original if not found
	})
}

// getNestedValue retrieves a value from a nested map using dot notation
func getNestedValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := interface{}(data)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, exists := v[part]
			if !exists {
				return nil
			}
			current = val
		case map[string]string:
			val, exists := v[part]
			if !exists {
				return nil
			}
			return val
		default:
			return nil
		}
	}

	return current
}

// evaluateCondition evaluates a condition expression with operator support
func (e *Executor) evaluateCondition(condition string, variables, input map[string]interface{}) string {
	// Interpolate the condition first
	result := e.interpolate(condition, variables, input)
	result = strings.TrimSpace(result)

	// Try to evaluate as expression with operators
	if evaluated, ok := e.evaluateExpression(result); ok {
		if evaluated {
			return "true"
		}
		return "false"
	}

	// Basic boolean evaluation as fallback
	lower := strings.ToLower(result)
	if lower == "true" || lower == "1" || lower == "yes" {
		return "true"
	}
	if lower == "false" || lower == "0" || lower == "no" || lower == "" {
		return "false"
	}

	return result
}

// evaluateExpression evaluates expressions with comparison and logical operators
func (e *Executor) evaluateExpression(expr string) (bool, bool) {
	expr = strings.TrimSpace(expr)

	// Handle logical operators (lower precedence, evaluate last)
	// Check for " or " first (lower precedence)
	if idx := strings.Index(strings.ToLower(expr), " or "); idx != -1 {
		left := expr[:idx]
		right := expr[idx+4:]
		leftResult, leftOk := e.evaluateExpression(left)
		rightResult, rightOk := e.evaluateExpression(right)
		if leftOk && rightOk {
			return leftResult || rightResult, true
		}
	}

	// Check for " and " (higher precedence than or)
	if idx := strings.Index(strings.ToLower(expr), " and "); idx != -1 {
		left := expr[:idx]
		right := expr[idx+5:]
		leftResult, leftOk := e.evaluateExpression(left)
		rightResult, rightOk := e.evaluateExpression(right)
		if leftOk && rightOk {
			return leftResult && rightResult, true
		}
	}

	// Handle "not in" operator
	if idx := strings.Index(strings.ToLower(expr), " not in "); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+8:])
		return !e.evaluateIn(left, right), true
	}

	// Handle "in" operator
	if idx := strings.Index(strings.ToLower(expr), " in "); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+4:])
		return e.evaluateIn(left, right), true
	}

	// Handle comparison operators
	operators := []struct {
		op   string
		eval func(a, b string) bool
	}{
		{"<=", func(a, b string) bool { return e.compareValues(a, b) <= 0 }},
		{">=", func(a, b string) bool { return e.compareValues(a, b) >= 0 }},
		{"!=", func(a, b string) bool { return strings.TrimSpace(a) != strings.TrimSpace(b) }},
		{"==", func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }},
		{"<", func(a, b string) bool { return e.compareValues(a, b) < 0 }},
		{">", func(a, b string) bool { return e.compareValues(a, b) > 0 }},
	}

	for _, op := range operators {
		if idx := strings.Index(expr, op.op); idx != -1 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+len(op.op):])
			return op.eval(left, right), true
		}
	}

	// Simple boolean check
	lower := strings.ToLower(expr)
	if lower == "true" || lower == "1" || lower == "yes" {
		return true, true
	}
	if lower == "false" || lower == "0" || lower == "no" || lower == "" {
		return false, true
	}

	return false, false
}

// compareValues compares two values, attempting numeric comparison first
func (e *Executor) compareValues(a, b string) int {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	// Try numeric comparison
	var aNum, bNum float64
	_, aErr := fmt.Sscanf(a, "%f", &aNum)
	_, bErr := fmt.Sscanf(b, "%f", &bNum)

	if aErr == nil && bErr == nil {
		if aNum < bNum {
			return -1
		} else if aNum > bNum {
			return 1
		}
		return 0
	}

	// Fall back to string comparison
	return strings.Compare(a, b)
}

// evaluateIn checks if value is in a list (e.g., "P0" in ['P0', 'P1'])
func (e *Executor) evaluateIn(value, list string) bool {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "'\"")

	// Parse list format: ['item1', 'item2'] or [item1, item2]
	list = strings.TrimSpace(list)
	if strings.HasPrefix(list, "[") && strings.HasSuffix(list, "]") {
		list = list[1 : len(list)-1]
	}

	items := strings.Split(list, ",")
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "'\"")
		if item == value {
			return true
		}
	}
	return false
}

// ListExecutions returns recent executions
func (e *Executor) ListExecutions(limit int) []*ChainExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()

	executions := make([]*ChainExecution, 0, len(e.executions))
	for _, exec := range e.executions {
		executions = append(executions, exec)
	}

	// Sort by start time (newest first)
	for i := 0; i < len(executions)-1; i++ {
		for j := i + 1; j < len(executions); j++ {
			if executions[j].StartedAt.After(executions[i].StartedAt) {
				executions[i], executions[j] = executions[j], executions[i]
			}
		}
	}

	if limit > 0 && limit < len(executions) {
		executions = executions[:limit]
	}

	return executions
}

// PendingGates returns executions waiting for gate approval
func (e *Executor) PendingGates() []*ChainExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()

	pending := make([]*ChainExecution, 0)
	for _, exec := range e.executions {
		if exec.Status == StatusPaused {
			pending = append(pending, exec)
		}
	}
	return pending
}
