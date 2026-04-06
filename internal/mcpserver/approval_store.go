package mcpserver

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ApprovalStatus represents the lifecycle state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// ApprovalRecord tracks a human-in-the-loop approval request through its lifecycle.
type ApprovalRecord struct {
	ID         string         `json:"id"`
	Action     string         `json:"action"`
	Context    string         `json:"context"`
	Urgency    string         `json:"urgency"`
	SessionID  string         `json:"session_id,omitempty"`
	Status     ApprovalStatus `json:"status"`
	Decision   string         `json:"decision,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	ResolvedAt *time.Time     `json:"resolved_at,omitempty"`
}

// ApprovalStore is a thread-safe in-memory store for approval records.
type ApprovalStore struct {
	mu      sync.RWMutex
	records map[string]*ApprovalRecord
}

// NewApprovalStore creates an empty ApprovalStore.
func NewApprovalStore() *ApprovalStore {
	return &ApprovalStore{
		records: make(map[string]*ApprovalRecord),
	}
}

// Create stores a new pending approval record and returns it.
func (s *ApprovalStore) Create(action, ctx, urgency, sessionID string) *ApprovalRecord {
	rec := &ApprovalRecord{
		ID:        uuid.New().String(),
		Action:    action,
		Context:   ctx,
		Urgency:   urgency,
		SessionID: sessionID,
		Status:    ApprovalPending,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.records[rec.ID] = rec
	s.mu.Unlock()
	return rec
}

// Get returns the approval record with the given ID, or nil if not found.
func (s *ApprovalStore) Get(id string) *ApprovalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return nil
	}
	// Return a copy to avoid data races on the caller side.
	cp := *rec
	return &cp
}

// Resolve transitions a pending approval to approved or rejected.
// Returns the updated record or an error if the record is not found or
// already resolved.
func (s *ApprovalStore) Resolve(id, decision, reason string) (*ApprovalRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return nil, fmt.Errorf("approval %s not found", id)
	}
	if rec.Status != ApprovalPending {
		return nil, fmt.Errorf("approval %s already resolved (%s)", id, rec.Status)
	}
	now := time.Now()
	rec.Status = ApprovalStatus(decision)
	rec.Decision = decision
	rec.Reason = reason
	rec.ResolvedAt = &now

	cp := *rec
	return &cp, nil
}

// List returns copies of all pending approval records.
func (s *ApprovalStore) List() []*ApprovalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*ApprovalRecord
	for _, rec := range s.records {
		if rec.Status == ApprovalPending {
			cp := *rec
			out = append(out, &cp)
		}
	}
	return out
}

// ListAll returns copies of every approval record regardless of status.
func (s *ApprovalStore) ListAll() []*ApprovalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ApprovalRecord, 0, len(s.records))
	for _, rec := range s.records {
		cp := *rec
		out = append(out, &cp)
	}
	return out
}
