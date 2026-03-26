package fleet

import (
	"errors"
	"testing"
	"time"
)

func TestA2AStatusToWorkItemStatus(t *testing.T) {
	tests := []struct {
		a2a  OfferStatus
		want WorkItemStatus
	}{
		{OfferOpen, WorkPending},
		{OfferSubmitted, WorkPending},
		{OfferAccepted, WorkAssigned},
		{OfferWorking, WorkRunning},
		{OfferInputRequired, WorkRunning},
		{OfferCompleted, WorkCompleted},
		{OfferFailed, WorkFailed},
		{OfferCanceled, WorkFailed},
		{OfferExpired, WorkFailed},
		{OfferStatus("unknown"), WorkPending},
	}

	for _, tt := range tests {
		got := A2AStatusToWorkItemStatus(tt.a2a)
		if got != tt.want {
			t.Errorf("A2AStatusToWorkItemStatus(%q) = %q, want %q", tt.a2a, got, tt.want)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []OfferStatus{OfferCompleted, OfferFailed, OfferCanceled, OfferExpired}
	nonTerminal := []OfferStatus{OfferOpen, OfferSubmitted, OfferAccepted, OfferWorking, OfferInputRequired}

	for _, s := range terminal {
		if !s.isTerminal() {
			t.Errorf("expected %q to be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.isTerminal() {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}

func TestFullA2ALifecycle_SubmittedToCompleted(t *testing.T) {
	a := NewA2AAdapter()

	// Create offer (starts as "open" for backward compat).
	err := a.Offer(TaskOffer{
		ID:       "lc-1",
		TaskType: "code_review",
		Prompt:   "review module",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Offer() error: %v", err)
	}

	// Accept.
	_, err = a.Accept("lc-1", "worker-1")
	if err != nil {
		t.Fatalf("Accept() error: %v", err)
	}

	// Start working.
	err = a.StartWorking("lc-1")
	if err != nil {
		t.Fatalf("StartWorking() error: %v", err)
	}
	offer, _ := a.GetOffer("lc-1")
	if offer.Status != string(OfferWorking) {
		t.Errorf("expected working, got %q", offer.Status)
	}

	// Add streaming artifact.
	err = a.AddArtifact("lc-1", Artifact{
		Name:    "review.md",
		Type:    "text/markdown",
		Content: "## Review\nLooks good so far...",
	})
	if err != nil {
		t.Fatalf("AddArtifact() error: %v", err)
	}

	// Add final artifact.
	err = a.AddArtifact("lc-1", Artifact{
		Name:    "review.md",
		Type:    "text/markdown",
		Content: "\n\n## Conclusion\nApproved.",
		Final:   true,
	})
	if err != nil {
		t.Fatalf("AddArtifact() final error: %v", err)
	}

	// Complete.
	err = a.CompleteOffer("lc-1")
	if err != nil {
		t.Fatalf("CompleteOffer() error: %v", err)
	}
	offer, _ = a.GetOffer("lc-1")
	if offer.Status != string(OfferCompleted) {
		t.Errorf("expected completed, got %q", offer.Status)
	}
	if len(offer.Artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(offer.Artifacts))
	}
}

func TestFullA2ALifecycle_WorkingToFailed(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "fail-1",
		TaskType: "build",
		Prompt:   "build project",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("fail-1", "worker-1")
	_ = a.StartWorking("fail-1")

	err := a.FailOffer("fail-1", "compilation error: missing import")
	if err != nil {
		t.Fatalf("FailOffer() error: %v", err)
	}

	offer, _ := a.GetOffer("fail-1")
	if offer.Status != string(OfferFailed) {
		t.Errorf("expected failed, got %q", offer.Status)
	}
	if offer.StatusMessage != "compilation error: missing import" {
		t.Errorf("expected status message, got %q", offer.StatusMessage)
	}
}

func TestFullA2ALifecycle_WorkingToCanceled(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "cancel-1",
		TaskType: "refactor",
		Prompt:   "refactor module",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("cancel-1", "worker-1")
	_ = a.StartWorking("cancel-1")

	err := a.CancelOffer("cancel-1")
	if err != nil {
		t.Fatalf("CancelOffer() error: %v", err)
	}

	offer, _ := a.GetOffer("cancel-1")
	if offer.Status != string(OfferCanceled) {
		t.Errorf("expected canceled, got %q", offer.Status)
	}
}

func TestInputRequired(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "input-1",
		TaskType: "deploy",
		Prompt:   "deploy to staging",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("input-1", "worker-1")
	_ = a.StartWorking("input-1")

	// Request input.
	err := a.RequestInput("input-1", "Which environment: staging or production?")
	if err != nil {
		t.Fatalf("RequestInput() error: %v", err)
	}

	offer, _ := a.GetOffer("input-1")
	if offer.Status != string(OfferInputRequired) {
		t.Errorf("expected input-required, got %q", offer.Status)
	}
	if offer.StatusMessage != "Which environment: staging or production?" {
		t.Errorf("unexpected status message: %q", offer.StatusMessage)
	}

	// Can still add artifacts while in input-required.
	err = a.AddArtifact("input-1", Artifact{
		Name:    "partial.log",
		Type:    "text/plain",
		Content: "Step 1 done, waiting for input...",
	})
	if err != nil {
		t.Fatalf("AddArtifact() in input-required error: %v", err)
	}

	// Resume to working.
	err = a.StartWorking("input-1")
	if err != nil {
		t.Fatalf("StartWorking() from input-required error: %v", err)
	}

	offer, _ = a.GetOffer("input-1")
	if offer.Status != string(OfferWorking) {
		t.Errorf("expected working after resume, got %q", offer.Status)
	}
}

func TestTerminalStateTransitionsBlocked(t *testing.T) {
	a := NewA2AAdapter()

	// Complete a task.
	_ = a.Offer(TaskOffer{
		ID:       "term-1",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("term-1", "w1")
	_ = a.CompleteOffer("term-1")

	// All mutations should fail on terminal state.
	if err := a.StartWorking("term-1"); !errors.Is(err, ErrOfferAlreadyTerminal) {
		t.Errorf("StartWorking on completed: expected ErrOfferAlreadyTerminal, got %v", err)
	}
	if err := a.RequestInput("term-1", "msg"); !errors.Is(err, ErrOfferAlreadyTerminal) {
		t.Errorf("RequestInput on completed: expected ErrOfferAlreadyTerminal, got %v", err)
	}
	if err := a.FailOffer("term-1", "msg"); !errors.Is(err, ErrOfferAlreadyTerminal) {
		t.Errorf("FailOffer on completed: expected ErrOfferAlreadyTerminal, got %v", err)
	}
	if err := a.CancelOffer("term-1"); !errors.Is(err, ErrOfferAlreadyTerminal) {
		t.Errorf("CancelOffer on completed: expected ErrOfferAlreadyTerminal, got %v", err)
	}
	if err := a.AddArtifact("term-1", Artifact{Name: "x", Type: "text/plain", Content: "y"}); !errors.Is(err, ErrOfferAlreadyTerminal) {
		t.Errorf("AddArtifact on completed: expected ErrOfferAlreadyTerminal, got %v", err)
	}
}

func TestArtifactIndexAutoIncrement(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "art-idx",
		TaskType: "gen",
		Prompt:   "generate",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("art-idx", "w1")
	_ = a.StartWorking("art-idx")

	// First artifact with explicit index 0 stays 0.
	_ = a.AddArtifact("art-idx", Artifact{Name: "a", Type: "text/plain", Content: "first"})

	// Second artifact with index 0 gets auto-incremented.
	_ = a.AddArtifact("art-idx", Artifact{Name: "b", Type: "text/plain", Content: "second"})

	// Third with explicit index keeps it.
	_ = a.AddArtifact("art-idx", Artifact{Name: "c", Type: "text/plain", Content: "third", Index: 99})

	arts, err := a.GetArtifacts("art-idx")
	if err != nil {
		t.Fatalf("GetArtifacts() error: %v", err)
	}
	if len(arts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(arts))
	}
	if arts[0].Index != 0 {
		t.Errorf("first artifact index: expected 0, got %d", arts[0].Index)
	}
	if arts[1].Index != 1 {
		t.Errorf("second artifact index: expected 1, got %d", arts[1].Index)
	}
	if arts[2].Index != 99 {
		t.Errorf("third artifact index: expected 99, got %d", arts[2].Index)
	}
}

func TestGetArtifactsEmpty(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "no-arts",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	arts, err := a.GetArtifacts("no-arts")
	if err != nil {
		t.Fatalf("GetArtifacts() error: %v", err)
	}
	if arts != nil {
		t.Errorf("expected nil artifacts, got %v", arts)
	}
}

func TestGetArtifactsNotFound(t *testing.T) {
	a := NewA2AAdapter()

	_, err := a.GetArtifacts("nonexistent")
	if !errors.Is(err, ErrOfferNotFound) {
		t.Errorf("expected ErrOfferNotFound, got %v", err)
	}
}

func TestAddArtifactOnOpenOffer(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "art-open",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	// Cannot add artifact to an open offer (not working yet).
	err := a.AddArtifact("art-open", Artifact{Name: "x", Type: "text/plain", Content: "y"})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition for artifact on open offer, got %v", err)
	}
}

func TestRequestInputInvalidState(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "ri-bad",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("ri-bad", "w1")

	// Cannot request input from accepted (must be working first).
	err := a.RequestInput("ri-bad", "need info")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestStartWorkingFromSubmitted(t *testing.T) {
	a := NewA2AAdapter()

	// Manually create a submitted offer.
	_ = a.Offer(TaskOffer{
		ID:       "sub-1",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	// Internally change status to submitted to test that path.
	a.mu.Lock()
	a.offers["sub-1"].Status = string(OfferSubmitted)
	a.mu.Unlock()

	err := a.StartWorking("sub-1")
	if err != nil {
		t.Fatalf("StartWorking() from submitted error: %v", err)
	}

	offer, _ := a.GetOffer("sub-1")
	if offer.Status != string(OfferWorking) {
		t.Errorf("expected working, got %q", offer.Status)
	}
}

func TestCancelFromOpenState(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "cancel-open",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	err := a.CancelOffer("cancel-open")
	if err != nil {
		t.Fatalf("CancelOffer() error: %v", err)
	}

	offer, _ := a.GetOffer("cancel-open")
	if offer.Status != string(OfferCanceled) {
		t.Errorf("expected canceled, got %q", offer.Status)
	}
}

func TestFailFromOpenState(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "fail-open",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	err := a.FailOffer("fail-open", "bad request")
	if err != nil {
		t.Fatalf("FailOffer() error: %v", err)
	}

	offer, _ := a.GetOffer("fail-open")
	if offer.Status != string(OfferFailed) {
		t.Errorf("expected failed, got %q", offer.Status)
	}
	if offer.StatusMessage != "bad request" {
		t.Errorf("expected status message 'bad request', got %q", offer.StatusMessage)
	}
}

func TestExpireStaleSubmitted(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "sub-expire",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(-1 * time.Second),
	})

	// Change to submitted status.
	a.mu.Lock()
	a.offers["sub-expire"].Status = string(OfferSubmitted)
	a.mu.Unlock()

	a.ExpireStale()

	offer, _ := a.GetOffer("sub-expire")
	if offer.Status != string(OfferExpired) {
		t.Errorf("expected expired, got %q", offer.Status)
	}
}

func TestArtifactsCopiedOnGet(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "copy-test",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("copy-test", "w1")
	_ = a.StartWorking("copy-test")
	_ = a.AddArtifact("copy-test", Artifact{Name: "a", Type: "text/plain", Content: "original"})

	arts, _ := a.GetArtifacts("copy-test")
	// Mutate the returned slice.
	arts[0].Content = "mutated"

	// Internal state should be unchanged.
	arts2, _ := a.GetArtifacts("copy-test")
	if arts2[0].Content != "original" {
		t.Errorf("GetArtifacts returned mutable slice; internal content was changed to %q", arts2[0].Content)
	}
}

func TestStartWorkingAlreadyWorking(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "dbl-work",
		TaskType: "test",
		Prompt:   "test",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("dbl-work", "w1")
	_ = a.StartWorking("dbl-work")

	// Working -> Working should be invalid (not in the allowed transitions).
	err := a.StartWorking("dbl-work")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition for working->working, got %v", err)
	}
}
