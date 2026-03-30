package session

import (
	"strings"
	"testing"
)

func TestPlannerTasksFromSession_MultiTask(t *testing.T) {
	sess := &Session{
		Status: StatusCompleted,
		OutputHistory: []string{
			`[{"title":"add tests","prompt":"write unit tests for parser"},{"title":"fix lint","prompt":"fix all lint warnings"}]`,
		},
	}
	tasks, _, err := plannerTasksFromSession(sess, 3)
	if err != nil {
		t.Fatalf("plannerTasksFromSession: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "add tests" {
		t.Errorf("task[0].Title = %q, want 'add tests'", tasks[0].Title)
	}
	if tasks[1].Title != "fix lint" {
		t.Errorf("task[1].Title = %q, want 'fix lint'", tasks[1].Title)
	}
}

func TestPlannerTasksFromSession_MultiTaskTruncated(t *testing.T) {
	// More tasks than maxTasks should truncate
	sess := &Session{
		Status: StatusCompleted,
		OutputHistory: []string{
			`[{"title":"t1","prompt":"p1"},{"title":"t2","prompt":"p2"},{"title":"t3","prompt":"p3"}]`,
		},
	}
	tasks, _, err := plannerTasksFromSession(sess, 2)
	if err != nil {
		t.Fatalf("plannerTasksFromSession: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks (maxTasks), got %d", len(tasks))
	}
}

func TestPlannerTasksFromSession_MultiTaskFromLastOutput(t *testing.T) {
	sess := &Session{
		Status:     StatusCompleted,
		LastOutput: `[{"title":"a1","prompt":"do A"},{"title":"a2","prompt":"do B"}]`,
	}
	tasks, _, err := plannerTasksFromSession(sess, 3)
	if err != nil {
		t.Fatalf("plannerTasksFromSession: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestPlannerTasksFromSession_FallbackSingleTask(t *testing.T) {
	sess := &Session{
		Status:     StatusCompleted,
		LastOutput: `{"title":"single task","prompt":"do this"}`,
	}
	tasks, _, err := plannerTasksFromSession(sess, 3)
	if err != nil {
		t.Fatalf("plannerTasksFromSession: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task (fallback to single), got %d", len(tasks))
	}
	if tasks[0].Title != "single task" {
		t.Errorf("task title = %q, want 'single task'", tasks[0].Title)
	}
}

func TestPlannerTasksFromSession_MaxTasksOne(t *testing.T) {
	// maxTasks=1 should skip the array parse path entirely
	sess := &Session{
		Status:     StatusCompleted,
		LastOutput: `{"title":"only one","prompt":"single worker task"}`,
	}
	tasks, _, err := plannerTasksFromSession(sess, 1)
	if err != nil {
		t.Fatalf("plannerTasksFromSession: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestParsePlannerTasks_Empty(t *testing.T) {
	_, err := parsePlannerTasks("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestParsePlannerTasks_ValidArray(t *testing.T) {
	input := `[{"title":"task one","prompt":"do something"},{"title":"task two","prompt":"do another"}]`
	tasks, err := parsePlannerTasks(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestParsePlannerTasks_FencedJSON(t *testing.T) {
	input := "```json\n[{\"title\":\"fenced task\",\"prompt\":\"with fences\"}]\n```"
	tasks, err := parsePlannerTasks(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Title != "fenced task" {
		t.Errorf("title = %q, want 'fenced task'", tasks[0].Title)
	}
}

func TestParsePlannerTasks_InvalidJSON(t *testing.T) {
	_, err := parsePlannerTasks("not json at all")
	if err == nil {
		t.Error("expected error for non-JSON input")
	}
}

func TestParsePlannerTasks_EmptyTitleFiltered(t *testing.T) {
	input := `[{"title":"","prompt":"no title"},{"title":"valid","prompt":"has title"}]`
	tasks, err := parsePlannerTasks(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 valid task (empty title filtered), got %d", len(tasks))
	}
}

func TestParsePlannerTasks_EmbeddedInProse(t *testing.T) {
	input := `Here are the tasks: [{"title":"embedded","prompt":"in prose"}] end of output`
	tasks, err := parsePlannerTasks(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task extracted from prose, got %d", len(tasks))
	}
}

func TestPlannerJSONArrayCandidates(t *testing.T) {
	// Plain array
	candidates := plannerJSONArrayCandidates(`[{"title":"t"}]`)
	if len(candidates) < 1 {
		t.Fatal("expected at least 1 candidate")
	}

	// With fence
	fenced := "```json\n[{\"title\":\"t\"}]\n```"
	candidates = plannerJSONArrayCandidates(fenced)
	// Should have the original, fenced content, and substring extraction
	if len(candidates) < 2 {
		t.Errorf("expected at least 2 candidates for fenced input, got %d", len(candidates))
	}
}

func TestParsePlannerTask_JSONInFence(t *testing.T) {
	input := "```json\n{\"title\":\"fenced single\",\"prompt\":\"do it\"}\n```"
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Title != "fenced single" {
		t.Errorf("title = %q, want 'fenced single'", task.Title)
	}
}

func TestParsePlannerTask_OnlyTitle(t *testing.T) {
	// Has title but no prompt — should set prompt = title
	input := `{"title":"just title"}`
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Title != "just title" {
		t.Errorf("title = %q", task.Title)
	}
	if task.Prompt != "just title" {
		t.Errorf("prompt should equal title when empty, got %q", task.Prompt)
	}
}

func TestParsePlannerTask_OnlyPrompt(t *testing.T) {
	input := `{"prompt":"only the prompt text here"}`
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Prompt != "only the prompt text here" {
		t.Errorf("prompt = %q", task.Prompt)
	}
	// Title should be set from first line of prompt
	if task.Title == "" {
		t.Error("title should be derived from prompt")
	}
}

func TestParsePlannerTask_FallbackToPlainText(t *testing.T) {
	input := "This is a plain text task description\nwith multiple lines"
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Source != "fallback" {
		t.Errorf("source = %q, want 'fallback'", task.Source)
	}
	if !strings.Contains(task.Prompt, "plain text task") {
		t.Errorf("prompt should contain input text, got %q", task.Prompt)
	}
}

func TestParsePlannerTask_JSONAfterPromptInstructions(t *testing.T) {
	// Simulates planner output where the full prompt (including JSON
	// examples in instructions) precedes the actual response JSON.
	input := `Here is the analysis of the codebase...

Respond with ONLY a JSON object: {"title":"...","prompt":"..."}

{"title":"Fix health monitor thresholds","prompt":"Add coverage and HITL rate evaluation to HealthMonitor"}`
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Title != "Fix health monitor thresholds" {
		t.Errorf("title = %q, want 'Fix health monitor thresholds'", task.Title)
	}
	if !strings.Contains(task.Prompt, "coverage and HITL") {
		t.Errorf("prompt = %q, want prompt about coverage", task.Prompt)
	}
}

func TestParsePlannerTask_Empty(t *testing.T) {
	_, err := parsePlannerTask("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestBuildLoopPlannerPromptN_SingleTask(t *testing.T) {
	dir := t.TempDir()
	prompt, err := buildLoopPlannerPromptN(dir, 1, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "single JSON object") {
		t.Error("single task prompt should mention JSON object")
	}
	if strings.Contains(prompt, "JSON array") {
		t.Error("single task prompt should not mention JSON array")
	}
}

func TestBuildLoopPlannerPromptN_MultiTask(t *testing.T) {
	dir := t.TempDir()
	prompt, err := buildLoopPlannerPromptN(dir, 3, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "JSON array") {
		t.Error("multi task prompt should mention JSON array")
	}
	if !strings.Contains(prompt, "up to 3") {
		t.Error("multi task prompt should mention task count")
	}
}

func TestBuildLoopPlannerPrompt_WithPrevIterations(t *testing.T) {
	dir := t.TempDir()
	prev := []LoopIteration{
		{
			Number: 1,
			Status: "idle",
			Task:   LoopTask{Title: "add tests"},
		},
		{
			Number:       2,
			Status:       "failed",
			Task:         LoopTask{Title: "fix compilation"},
			HasQuestions: true,
		},
	}
	prompt, err := buildLoopPlannerPrompt(dir, prev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "add tests") {
		t.Error("prompt should reference previous task titles")
	}
	if !strings.Contains(prompt, "autonomous decisions") {
		t.Error("prompt should include headless mode guidance when HasQuestions")
	}
	if !strings.Contains(prompt, "DO NOT repeat") {
		t.Error("prompt should include completed task dedup")
	}
}
