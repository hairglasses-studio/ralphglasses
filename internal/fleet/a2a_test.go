package fleet

import (
	"errors"
	"testing"
	"time"
)

func TestOfferAndAccept(t *testing.T) {
	a := NewA2AAdapter()

	err := a.Offer(TaskOffer{
		ID:           "offer-1",
		OfferingNode: "node-alpha",
		TaskType:     "code_review",
		Prompt:       "Review the auth module",
		Constraints:  DelegationConstraints{RequireProvider: "claude", MaxBudgetUSD: 1.50},
		Deadline:     time.Now().Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Offer() error: %v", err)
	}

	accepted, err := a.Accept("offer-1", "worker-beta")
	if err != nil {
		t.Fatalf("Accept() error: %v", err)
	}

	if accepted.Status != "accepted" {
		t.Errorf("expected status 'accepted', got %q", accepted.Status)
	}
	if accepted.AcceptedBy != "worker-beta" {
		t.Errorf("expected AcceptedBy 'worker-beta', got %q", accepted.AcceptedBy)
	}
	if accepted.OfferingNode != "node-alpha" {
		t.Errorf("expected OfferingNode 'node-alpha', got %q", accepted.OfferingNode)
	}
}

func TestAcceptNonExistent(t *testing.T) {
	a := NewA2AAdapter()

	_, err := a.Accept("no-such-offer", "worker-1")
	if !errors.Is(err, ErrOfferNotFound) {
		t.Errorf("expected ErrOfferNotFound, got %v", err)
	}
}

func TestAcceptAlreadyAccepted(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "offer-dup",
		TaskType: "test",
		Prompt:   "do something",
		Deadline: time.Now().Add(5 * time.Minute),
	})

	_, err := a.Accept("offer-dup", "worker-1")
	if err != nil {
		t.Fatalf("first Accept() error: %v", err)
	}

	_, err = a.Accept("offer-dup", "worker-2")
	if !errors.Is(err, ErrOfferNotOpen) {
		t.Errorf("expected ErrOfferNotOpen on second accept, got %v", err)
	}
}

func TestNegotiate(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:          "offer-neg",
		TaskType:    "refactor",
		Prompt:      "refactor the fleet package",
		Constraints: DelegationConstraints{MaxBudgetUSD: 1.00},
		Deadline:    time.Now().Add(10 * time.Minute),
	})

	newConstraints := DelegationConstraints{
		MaxBudgetUSD:    2.50,
		RequireProvider: "gemini",
		RequireRepo:     "ralphglasses",
		PreferLocal:     true,
	}

	err := a.Negotiate("offer-neg", newConstraints)
	if err != nil {
		t.Fatalf("Negotiate() error: %v", err)
	}

	offer, ok := a.GetOffer("offer-neg")
	if !ok {
		t.Fatal("GetOffer returned false after negotiate")
	}
	if offer.Constraints.MaxBudgetUSD != 2.50 {
		t.Errorf("expected MaxBudgetUSD 2.50, got %.2f", offer.Constraints.MaxBudgetUSD)
	}
	if offer.Constraints.RequireProvider != "gemini" {
		t.Errorf("expected RequireProvider 'gemini', got %q", offer.Constraints.RequireProvider)
	}
	if offer.Constraints.RequireRepo != "ralphglasses" {
		t.Errorf("expected RequireRepo 'ralphglasses', got %q", offer.Constraints.RequireRepo)
	}
	if !offer.Constraints.PreferLocal {
		t.Error("expected PreferLocal true")
	}
}

func TestListOpenOffers(t *testing.T) {
	a := NewA2AAdapter()

	// Open offer with future deadline.
	_ = a.Offer(TaskOffer{
		ID:       "open-1",
		TaskType: "test",
		Prompt:   "open task",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	// Offer with past deadline (should be filtered out).
	_ = a.Offer(TaskOffer{
		ID:       "expired-1",
		TaskType: "test",
		Prompt:   "expired task",
		Deadline: time.Now().Add(-1 * time.Minute),
	})

	// Accepted offer (should be filtered out).
	_ = a.Offer(TaskOffer{
		ID:       "accepted-1",
		TaskType: "test",
		Prompt:   "accepted task",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("accepted-1", "worker-x")

	open := a.ListOpenOffers()
	if len(open) != 1 {
		t.Fatalf("expected 1 open offer, got %d", len(open))
	}
	if open[0].ID != "open-1" {
		t.Errorf("expected offer 'open-1', got %q", open[0].ID)
	}

	// Verify the expired offer was marked.
	exp, ok := a.GetOffer("expired-1")
	if !ok {
		t.Fatal("GetOffer returned false for expired offer")
	}
	if exp.Status != "expired" {
		t.Errorf("expected status 'expired', got %q", exp.Status)
	}
}

func TestExpireStale(t *testing.T) {
	a := NewA2AAdapter()

	_ = a.Offer(TaskOffer{
		ID:       "stale-1",
		TaskType: "test",
		Prompt:   "will expire",
		Deadline: time.Now().Add(-1 * time.Second),
	})

	_ = a.Offer(TaskOffer{
		ID:       "fresh-1",
		TaskType: "test",
		Prompt:   "still good",
		Deadline: time.Now().Add(10 * time.Minute),
	})

	a.ExpireStale()

	stale, _ := a.GetOffer("stale-1")
	if stale.Status != "expired" {
		t.Errorf("expected stale offer to be 'expired', got %q", stale.Status)
	}

	fresh, _ := a.GetOffer("fresh-1")
	if fresh.Status != "open" {
		t.Errorf("expected fresh offer to be 'open', got %q", fresh.Status)
	}
}

func TestCompleteOffer(t *testing.T) {
	a := NewA2AAdapter()

	// Full lifecycle: offer -> accept -> complete.
	_ = a.Offer(TaskOffer{
		ID:           "lifecycle-1",
		OfferingNode: "node-a",
		TaskType:     "build",
		Prompt:       "build the project",
		Deadline:     time.Now().Add(30 * time.Minute),
	})

	_, err := a.Accept("lifecycle-1", "worker-b")
	if err != nil {
		t.Fatalf("Accept() error: %v", err)
	}

	err = a.CompleteOffer("lifecycle-1")
	if err != nil {
		t.Fatalf("CompleteOffer() error: %v", err)
	}

	offer, ok := a.GetOffer("lifecycle-1")
	if !ok {
		t.Fatal("GetOffer returned false after complete")
	}
	if offer.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", offer.Status)
	}

	// Completing again should fail (terminal state).
	err = a.CompleteOffer("lifecycle-1")
	if !errors.Is(err, ErrOfferAlreadyTerminal) {
		t.Errorf("expected ErrOfferAlreadyTerminal on double complete, got %v", err)
	}

	// Completing a non-existent offer should fail.
	err = a.CompleteOffer("no-such-id")
	if !errors.Is(err, ErrOfferNotFound) {
		t.Errorf("expected ErrOfferNotFound, got %v", err)
	}
}

func TestAcceptPastDeadline(t *testing.T) {
	a := NewA2AAdapter()

	// Create an offer with a deadline in the past.
	_ = a.Offer(TaskOffer{
		ID:       "past-deadline",
		TaskType: "test",
		Prompt:   "expired before accept",
		Deadline: time.Now().Add(-1 * time.Minute),
	})

	_, err := a.Accept("past-deadline", "worker-1")
	if !errors.Is(err, ErrOfferExpired) {
		t.Errorf("expected ErrOfferExpired for past-deadline accept, got %v", err)
	}

	// Verify the offer was marked as expired.
	offer, ok := a.GetOffer("past-deadline")
	if !ok {
		t.Fatal("GetOffer returned false")
	}
	if offer.Status != "expired" {
		t.Errorf("expected status 'expired', got %q", offer.Status)
	}
}

func TestNegotiateCompleted(t *testing.T) {
	a := NewA2AAdapter()

	// Full lifecycle to completed state.
	_ = a.Offer(TaskOffer{
		ID:       "completed-neg",
		TaskType: "test",
		Prompt:   "will complete",
		Deadline: time.Now().Add(10 * time.Minute),
	})
	_, _ = a.Accept("completed-neg", "worker-1")
	_ = a.CompleteOffer("completed-neg")

	// Negotiate on completed offer should fail.
	err := a.Negotiate("completed-neg", DelegationConstraints{MaxBudgetUSD: 5.0})
	if !errors.Is(err, ErrOfferNotOpen) {
		t.Errorf("expected ErrOfferNotOpen for negotiate on completed offer, got %v", err)
	}
}
