package session

import (
	"strings"
	"testing"
)

func TestBuildLoopPlannerPromptWithPrevIterations(t *testing.T) {
	dir := t.TempDir()

	prev := []LoopIteration{
		{Number: 1, Status: "success", Task: LoopTask{Title: "Add widget tests"}},
		{Number: 2, Status: "success", Task: LoopTask{Title: "Fix parser bug"}},
		{Number: 3, Status: "failed", Task: LoopTask{Title: "Refactor cache layer"}},
	}

	prompt, err := buildLoopPlannerPrompt(dir, prev)
	if err != nil {
		t.Fatalf("buildLoopPlannerPrompt: %v", err)
	}

	// Completed (non-failed) tasks should appear in the dedup section.
	if !strings.Contains(prompt, "Completed tasks (DO NOT repeat these)") {
		t.Error("expected dedup header in prompt")
	}
	if !strings.Contains(prompt, "Add widget tests") {
		t.Error("expected completed task 'Add widget tests' in dedup list")
	}
	if !strings.Contains(prompt, "Fix parser bug") {
		t.Error("expected completed task 'Fix parser bug' in dedup list")
	}
	// Failed task should NOT appear in the completed list (but may appear in recent types).
	lines := strings.Split(prompt, "\n")
	inDedup := false
	for _, line := range lines {
		if strings.Contains(line, "Completed tasks (DO NOT repeat these)") {
			inDedup = true
			continue
		}
		if inDedup && strings.HasPrefix(line, "\n") || (inDedup && line == "") {
			inDedup = false
		}
		if inDedup && strings.Contains(line, "Refactor cache layer") {
			// Failed task should not be in completed list
			t.Error("failed task 'Refactor cache layer' should not appear in completed tasks dedup")
		}
	}
}

func TestBuildLoopPlannerPromptNilPrev(t *testing.T) {
	dir := t.TempDir()

	// nil prev should not crash and should not contain dedup section.
	prompt, err := buildLoopPlannerPrompt(dir, nil)
	if err != nil {
		t.Fatalf("buildLoopPlannerPrompt with nil: %v", err)
	}
	if strings.Contains(prompt, "Completed tasks (DO NOT repeat these)") {
		t.Error("nil prev should not produce dedup section")
	}

	// Also test buildLoopPlannerPromptN with nil.
	prompt2, err := buildLoopPlannerPromptN(dir, 1, nil)
	if err != nil {
		t.Fatalf("buildLoopPlannerPromptN with nil: %v", err)
	}
	if prompt2 == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestBuildLoopPlannerPromptNPassesPrev(t *testing.T) {
	dir := t.TempDir()

	prev := []LoopIteration{
		{Number: 1, Status: "success", Task: LoopTask{Title: "Implement feature X"}},
	}

	// Multi-task variant should also include dedup.
	prompt, err := buildLoopPlannerPromptN(dir, 3, prev)
	if err != nil {
		t.Fatalf("buildLoopPlannerPromptN: %v", err)
	}
	if !strings.Contains(prompt, "Completed tasks (DO NOT repeat these)") {
		t.Error("multi-task prompt should include dedup section when prev is provided")
	}
	if !strings.Contains(prompt, "Implement feature X") {
		t.Error("expected completed task in dedup list")
	}
}
