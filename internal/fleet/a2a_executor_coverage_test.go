package fleet

import (
	"testing"
)

func TestNewA2AExecutor_Card(t *testing.T) {
	cfg := A2AExecutorConfig{
		Name:        "test-agent",
		Description: "A test agent",
		URL:         "http://localhost:8080",
		Version:     "1.0",
	}
	e := NewA2AExecutor(cfg)
	if e == nil {
		t.Fatal("NewA2AExecutor returned nil")
	}
	card := e.Card()
	if card.Name != "test-agent" {
		t.Errorf("Card().Name = %q, want test-agent", card.Name)
	}
	if card.URL != "http://localhost:8080" {
		t.Errorf("Card().URL = %q, want http://localhost:8080", card.URL)
	}
}

func TestA2AExecutor_TaskCount_Empty(t *testing.T) {
	e := NewA2AExecutor(A2AExecutorConfig{Name: "test"})
	if e.TaskCount() != 0 {
		t.Errorf("TaskCount() = %d, want 0", e.TaskCount())
	}
}

func TestA2AExecutor_TaskCountByState_Empty(t *testing.T) {
	e := NewA2AExecutor(A2AExecutorConfig{Name: "test"})
	counts := e.TaskCountByState()
	if len(counts) != 0 {
		t.Errorf("TaskCountByState() = %v, want empty", counts)
	}
}

func TestValidateTransition_ValidTransitions(t *testing.T) {
	cases := []struct {
		from TaskState
		to   TaskState
	}{
		{TaskStateQueued, TaskStateWorking},
		{TaskStateQueued, TaskStateCanceled},
		{TaskStateQueued, TaskStateFailed},
		{TaskStateWorking, TaskStateCompleted},
		{TaskStateWorking, TaskStateFailed},
		{TaskStateWorking, TaskStateCanceled},
		{TaskStateWorking, TaskStateInputRequired},
		{TaskStateInputRequired, TaskStateWorking},
		{TaskStateInputRequired, TaskStateCanceled},
		{TaskStateInputRequired, TaskStateFailed},
	}
	for _, tc := range cases {
		err := validateTransition(tc.from, tc.to)
		if err != nil {
			t.Errorf("validateTransition(%s, %s) = %v, want nil", tc.from, tc.to, err)
		}
	}
}

func TestValidateTransition_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from TaskState
		to   TaskState
	}{
		{TaskStateQueued, TaskStateCompleted},
		{TaskStateQueued, TaskStateInputRequired},
		{TaskStateWorking, TaskStateQueued},
		{TaskStateCompleted, TaskStateWorking},
		{TaskStateFailed, TaskStateWorking},
		{TaskStateCanceled, TaskStateWorking},
	}
	for _, tc := range cases {
		err := validateTransition(tc.from, tc.to)
		if err == nil {
			t.Errorf("validateTransition(%s, %s) = nil, want error", tc.from, tc.to)
		}
	}
}
