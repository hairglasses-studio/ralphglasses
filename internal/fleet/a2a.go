package fleet

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Sentinel errors for A2A task offer operations.
var (
	ErrOfferNotFound = errors.New("a2a: offer not found")
	ErrOfferNotOpen  = errors.New("a2a: offer is not open")
	ErrOfferExpired  = errors.New("a2a: offer has expired")
)

// OfferStatus represents the lifecycle state of a task offer.
type OfferStatus string

const (
	OfferOpen      OfferStatus = "open"
	OfferAccepted  OfferStatus = "accepted"
	OfferCompleted OfferStatus = "completed"
	OfferExpired   OfferStatus = "expired"
)

// DelegationConstraints specifies requirements and preferences for task delegation.
type DelegationConstraints struct {
	RequireProvider string  `json:"require_provider,omitempty"` // session.Provider as string
	MaxBudgetUSD    float64 `json:"max_budget_usd,omitempty"`
	RequireRepo     string  `json:"require_repo,omitempty"`
	PreferLocal     bool    `json:"prefer_local,omitempty"`
}

// TaskOffer represents a task that one agent node offers to the fleet for delegation.
// Other agents can accept, negotiate constraints, or let it expire.
type TaskOffer struct {
	ID           string                `json:"id"`
	OfferingNode string                `json:"offering_node"`
	TaskType     string                `json:"task_type"`
	Prompt       string                `json:"prompt"`
	Constraints  DelegationConstraints `json:"constraints"`
	Deadline     time.Time             `json:"deadline"`
	Status       string                `json:"status"` // "open", "accepted", "completed", "expired"
	AcceptedBy   string                `json:"accepted_by,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

// A2AAdapter implements the Agent-to-Agent protocol for task delegation.
// It manages a set of task offers that agents can publish, accept, negotiate,
// and complete. The adapter is thread-safe and can optionally reference a
// Coordinator for future integration with the fleet work queue.
type A2AAdapter struct {
	mu          sync.Mutex
	offers      map[string]*TaskOffer
	coordinator *Coordinator
}

// NewA2AAdapter creates a new Agent-to-Agent protocol adapter.
func NewA2AAdapter() *A2AAdapter {
	return &A2AAdapter{
		offers: make(map[string]*TaskOffer),
	}
}

// NewA2AAdapterWithCoordinator creates an adapter linked to a fleet coordinator.
func NewA2AAdapterWithCoordinator(c *Coordinator) *A2AAdapter {
	return &A2AAdapter{
		offers:      make(map[string]*TaskOffer),
		coordinator: c,
	}
}

// Offer publishes a task offer for other agents to accept. If the offer ID is
// empty, one is generated using "offer-" + unix nanosecond timestamp. The offer
// status is set to "open" and timestamps are initialized.
func (a *A2AAdapter) Offer(offer TaskOffer) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	if offer.ID == "" {
		offer.ID = fmt.Sprintf("offer-%d", now.UnixNano())
	}

	offer.Status = string(OfferOpen)
	offer.CreatedAt = now
	offer.UpdatedAt = now

	a.offers[offer.ID] = &offer
	return nil
}

// Accept claims an open offer for the given worker. The offer must exist,
// be in "open" status, and not past its deadline.
func (a *A2AAdapter) Accept(offerID, workerID string) (*TaskOffer, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return nil, ErrOfferNotFound
	}

	// Check expiry before status so we can give a more specific error.
	if !offer.Deadline.IsZero() && time.Now().After(offer.Deadline) {
		offer.Status = string(OfferExpired)
		offer.UpdatedAt = time.Now()
		return nil, ErrOfferExpired
	}

	if offer.Status != string(OfferOpen) {
		return nil, ErrOfferNotOpen
	}

	now := time.Now()
	offer.Status = string(OfferAccepted)
	offer.AcceptedBy = workerID
	offer.UpdatedAt = now

	// Return a copy so the caller cannot mutate internal state.
	cp := *offer
	return &cp, nil
}

// Negotiate updates the delegation constraints on an open offer. This allows
// counter-proposals (e.g., a worker requesting a higher budget or different
// provider) before acceptance.
func (a *A2AAdapter) Negotiate(offerID string, counter DelegationConstraints) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	if !offer.Deadline.IsZero() && time.Now().After(offer.Deadline) {
		offer.Status = string(OfferExpired)
		offer.UpdatedAt = time.Now()
		return ErrOfferExpired
	}

	if offer.Status != string(OfferOpen) {
		return ErrOfferNotOpen
	}

	offer.Constraints = counter
	offer.UpdatedAt = time.Now()
	return nil
}

// ListOpenOffers returns all offers that are in "open" status and have not
// passed their deadline. Expired offers are marked as such as a side effect.
func (a *A2AAdapter) ListOpenOffers() []TaskOffer {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	var result []TaskOffer

	for _, offer := range a.offers {
		// Lazily expire offers that are past deadline.
		if !offer.Deadline.IsZero() && now.After(offer.Deadline) {
			if offer.Status == string(OfferOpen) {
				offer.Status = string(OfferExpired)
				offer.UpdatedAt = now
			}
			continue
		}

		if offer.Status == string(OfferOpen) {
			cp := *offer
			result = append(result, cp)
		}
	}

	return result
}

// GetOffer retrieves a specific offer by ID. Returns the offer and true if
// found, or a zero-value TaskOffer and false if not.
func (a *A2AAdapter) GetOffer(offerID string) (*TaskOffer, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return nil, false
	}

	cp := *offer
	return &cp, true
}

// CompleteOffer marks an accepted offer as completed. The offer must exist
// and be in "accepted" status.
func (a *A2AAdapter) CompleteOffer(offerID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	if offer.Status != string(OfferAccepted) {
		return ErrOfferNotOpen
	}

	offer.Status = string(OfferCompleted)
	offer.UpdatedAt = time.Now()
	return nil
}

// ExpireStale scans all offers and marks any open offers past their deadline
// as expired. This is intended to be called periodically (e.g., from a
// maintenance loop).
func (a *A2AAdapter) ExpireStale() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for _, offer := range a.offers {
		if offer.Status == string(OfferOpen) && !offer.Deadline.IsZero() && now.After(offer.Deadline) {
			offer.Status = string(OfferExpired)
			offer.UpdatedAt = now
		}
	}
}

// OfferCount returns the total number of offers tracked by the adapter.
func (a *A2AAdapter) OfferCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.offers)
}

// CountByStatus returns the number of offers in each status.
func (a *A2AAdapter) CountByStatus() map[string]int {
	a.mu.Lock()
	defer a.mu.Unlock()

	counts := make(map[string]int)
	for _, offer := range a.offers {
		counts[offer.Status]++
	}
	return counts
}
