package fleet

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Sentinel errors for A2A task offer operations.
var (
	ErrOfferNotFound        = errors.New("a2a: offer not found")
	ErrOfferNotOpen         = errors.New("a2a: offer is not open")
	ErrOfferExpired         = errors.New("a2a: offer has expired")
	ErrInvalidTransition    = errors.New("a2a: invalid state transition")
	ErrOfferNotAccepted     = errors.New("a2a: offer is not in an accepted/working state")
	ErrOfferAlreadyTerminal = errors.New("a2a: offer is in a terminal state")
)

// TaskState represents the A2A v1.0 task lifecycle states.
// These are the canonical wire values used in JSON payloads.
type TaskState string

const (
	TaskStateQueued        TaskState = "queued"
	TaskStateWorking       TaskState = "working"
	TaskStateInputRequired TaskState = "input-required"
	TaskStateCompleted     TaskState = "completed"
	TaskStateFailed        TaskState = "failed"
	TaskStateCanceled      TaskState = "canceled"
)

// OfferStatus represents the lifecycle state of a task offer.
// These map to the A2A protocol lifecycle states.
type OfferStatus string

const (
	// OfferOpen is the legacy alias for OfferSubmitted.
	OfferOpen OfferStatus = "open"

	// A2A lifecycle states.
	OfferSubmitted     OfferStatus = "submitted"
	OfferWorking       OfferStatus = "working"
	OfferInputRequired OfferStatus = "input-required"
	OfferCompleted     OfferStatus = "completed"
	OfferFailed        OfferStatus = "failed"
	OfferCanceled      OfferStatus = "canceled"

	// Legacy states retained for backward compatibility.
	OfferAccepted OfferStatus = "accepted"
	OfferExpired  OfferStatus = "expired"
)

// OfferStatusToTaskState maps internal OfferStatus values to A2A v1.0 TaskState constants.
func OfferStatusToTaskState(s OfferStatus) TaskState {
	switch s {
	case OfferOpen, OfferSubmitted:
		return TaskStateQueued
	case OfferAccepted, OfferWorking:
		return TaskStateWorking
	case OfferInputRequired:
		return TaskStateInputRequired
	case OfferCompleted:
		return TaskStateCompleted
	case OfferFailed, OfferExpired:
		return TaskStateFailed
	case OfferCanceled:
		return TaskStateCanceled
	default:
		return TaskStateQueued
	}
}

// TaskStateToOfferStatus maps A2A v1.0 TaskState values back to internal OfferStatus.
func TaskStateToOfferStatus(s TaskState) OfferStatus {
	switch s {
	case TaskStateQueued:
		return OfferSubmitted
	case TaskStateWorking:
		return OfferWorking
	case TaskStateInputRequired:
		return OfferInputRequired
	case TaskStateCompleted:
		return OfferCompleted
	case TaskStateFailed:
		return OfferFailed
	case TaskStateCanceled:
		return OfferCanceled
	default:
		return OfferSubmitted
	}
}

// isTerminal returns true if the status is a terminal state (completed, failed, canceled, expired).
func (s OfferStatus) isTerminal() bool {
	switch s {
	case OfferCompleted, OfferFailed, OfferCanceled, OfferExpired:
		return true
	}
	return false
}

// A2AStatusToWorkItemStatus maps A2A offer statuses to WorkItemStatus values.
func A2AStatusToWorkItemStatus(s OfferStatus) WorkItemStatus {
	switch s {
	case OfferOpen, OfferSubmitted:
		return WorkPending
	case OfferAccepted:
		return WorkAssigned
	case OfferWorking, OfferInputRequired:
		return WorkRunning
	case OfferCompleted:
		return WorkCompleted
	case OfferFailed, OfferCanceled, OfferExpired:
		return WorkFailed
	default:
		return WorkPending
	}
}

// PartType identifies the kind of content in a message part.
type PartType string

const (
	PartTypeText PartType = "text"
	PartTypeData PartType = "data"
	PartTypeFile PartType = "file"
)

// Part is a single content element within a Message.
// A2A v1.0 defines TextPart, DataPart, and FilePart.
type Part struct {
	Type     PartType `json:"type"`
	Text     string   `json:"text,omitempty"`     // for TextPart
	Data     any      `json:"data,omitempty"`     // for DataPart (arbitrary JSON)
	MimeType string   `json:"mimeType,omitempty"` // for DataPart/FilePart
	FileURI  string   `json:"fileUri,omitempty"`  // for FilePart (URI reference)
	FileData string   `json:"fileData,omitempty"` // for FilePart (base64-encoded inline data)
}

// NewTextPart creates a Part containing plain text.
func NewTextPart(text string) Part {
	return Part{Type: PartTypeText, Text: text}
}

// NewDataPart creates a Part containing structured data with a MIME type.
func NewDataPart(data any, mimeType string) Part {
	return Part{Type: PartTypeData, Data: data, MimeType: mimeType}
}

// NewFilePart creates a Part referencing a file by URI.
func NewFilePart(uri, mimeType string) Part {
	return Part{Type: PartTypeFile, FileURI: uri, MimeType: mimeType}
}

// MessageRole indicates the sender of a message.
type MessageRole string

const (
	MessageRoleUser  MessageRole = "user"
	MessageRoleAgent MessageRole = "agent"
)

// Message represents an A2A v1.0 message exchanged between agents.
// Messages consist of one or more Parts and carry a role indicating the sender.
type Message struct {
	Role  MessageRole `json:"role"`
	Parts []Part      `json:"parts"`
}

// Artifact represents a partial or final result produced during task execution.
// Aligned with A2A v1.0: artifacts carry structured Parts instead of raw content,
// though the Content and Type fields are retained for backward compatibility.
type Artifact struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`                // MIME type or semantic type (e.g., "text/plain", "application/json", "code")
	Content     string `json:"content"`             // backward-compatible raw content
	Parts       []Part `json:"parts,omitempty"`     // A2A v1.0 structured parts
	Index       int    `json:"index"`               // ordering index for streaming
	Append      bool   `json:"append,omitempty"`    // true if this extends a previous artifact at the same index
	LastChunk   bool   `json:"lastChunk,omitempty"` // true if this is the final chunk for this artifact
	Final       bool   `json:"final,omitempty"`     // legacy: true if this is the last chunk for this artifact name
}

// DelegationConstraints specifies requirements and preferences for task delegation.
type DelegationConstraints struct {
	RequireProvider string  `json:"require_provider,omitempty"` // session.Provider as string
	MaxBudgetUSD    float64 `json:"max_budget_usd,omitempty"`
	RequireRepo     string  `json:"require_repo,omitempty"`
	PreferLocal     bool    `json:"prefer_local,omitempty"`
}

// TaskOffer represents a task that one agent node offers to the fleet for delegation.
// Other agents can accept, negotiate constraints, or let it expire.
// The Status field tracks the full A2A lifecycle: submitted -> working -> completed/failed/canceled.
// The InputRequired state allows a working task to pause and request additional input.
type TaskOffer struct {
	ID            string                `json:"id"`
	OfferingNode  string                `json:"offering_node"`
	TaskType      string                `json:"task_type"`
	Prompt        string                `json:"prompt"`
	Constraints   DelegationConstraints `json:"constraints"`
	Deadline      time.Time             `json:"deadline"`
	Status        string                `json:"status"` // "submitted", "working", "input-required", "completed", "failed", "canceled", "open", "accepted", "expired"
	AcceptedBy    string                `json:"accepted_by,omitempty"`
	Artifacts     []Artifact            `json:"artifacts,omitempty"`
	StatusMessage string                `json:"status_message,omitempty"` // human-readable context for current state
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
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

// CompleteOffer marks an offer as completed. The offer must exist
// and be in "accepted" or "working" status.
func (a *A2AAdapter) CompleteOffer(offerID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	s := OfferStatus(offer.Status)
	if s.isTerminal() {
		return ErrOfferAlreadyTerminal
	}

	switch s {
	case OfferAccepted, OfferWorking, OfferInputRequired:
		// valid
	default:
		return ErrOfferNotAccepted
	}

	offer.Status = string(OfferCompleted)
	offer.UpdatedAt = time.Now()
	return nil
}

// StartWorking transitions an accepted/submitted offer to the "working" state.
func (a *A2AAdapter) StartWorking(offerID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	s := OfferStatus(offer.Status)
	if s.isTerminal() {
		return ErrOfferAlreadyTerminal
	}

	// Allow transition from open, submitted, or accepted.
	switch s {
	case OfferOpen, OfferSubmitted, OfferAccepted, OfferInputRequired:
		// valid
	default:
		return ErrInvalidTransition
	}

	offer.Status = string(OfferWorking)
	offer.UpdatedAt = time.Now()
	return nil
}

// RequestInput transitions a working offer to "input-required", pausing execution
// until additional input is provided. The message explains what input is needed.
func (a *A2AAdapter) RequestInput(offerID, message string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	s := OfferStatus(offer.Status)
	if s.isTerminal() {
		return ErrOfferAlreadyTerminal
	}

	if s != OfferWorking {
		return ErrInvalidTransition
	}

	offer.Status = string(OfferInputRequired)
	offer.StatusMessage = message
	offer.UpdatedAt = time.Now()
	return nil
}

// FailOffer marks an offer as failed with an error message.
// Can transition from any non-terminal state.
func (a *A2AAdapter) FailOffer(offerID, message string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	if OfferStatus(offer.Status).isTerminal() {
		return ErrOfferAlreadyTerminal
	}

	offer.Status = string(OfferFailed)
	offer.StatusMessage = message
	offer.UpdatedAt = time.Now()
	return nil
}

// CancelOffer marks an offer as canceled.
// Can transition from any non-terminal state.
func (a *A2AAdapter) CancelOffer(offerID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	if OfferStatus(offer.Status).isTerminal() {
		return ErrOfferAlreadyTerminal
	}

	offer.Status = string(OfferCanceled)
	offer.UpdatedAt = time.Now()
	return nil
}

// AddArtifact appends an artifact to an offer that is in a working or input-required state.
// The artifact's Index is automatically set to the next sequential value if zero.
func (a *A2AAdapter) AddArtifact(offerID string, art Artifact) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return ErrOfferNotFound
	}

	s := OfferStatus(offer.Status)
	if s.isTerminal() {
		return ErrOfferAlreadyTerminal
	}

	switch s {
	case OfferWorking, OfferInputRequired, OfferAccepted:
		// valid states for adding artifacts
	default:
		return ErrInvalidTransition
	}

	if art.Index == 0 && len(offer.Artifacts) > 0 {
		art.Index = len(offer.Artifacts)
	}

	offer.Artifacts = append(offer.Artifacts, art)
	offer.UpdatedAt = time.Now()
	return nil
}

// GetArtifacts returns a copy of the artifacts for a given offer.
func (a *A2AAdapter) GetArtifacts(offerID string) ([]Artifact, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	offer, ok := a.offers[offerID]
	if !ok {
		return nil, ErrOfferNotFound
	}

	if len(offer.Artifacts) == 0 {
		return nil, nil
	}

	result := make([]Artifact, len(offer.Artifacts))
	copy(result, offer.Artifacts)
	return result, nil
}

// ExpireStale scans all offers and marks any open offers past their deadline
// as expired. This is intended to be called periodically (e.g., from a
// maintenance loop).
func (a *A2AAdapter) ExpireStale() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for _, offer := range a.offers {
		s := OfferStatus(offer.Status)
		if (s == OfferOpen || s == OfferSubmitted) && !offer.Deadline.IsZero() && now.After(offer.Deadline) {
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
