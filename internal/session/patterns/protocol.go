package patterns

import (
	"encoding/json"
	"time"
)

// MessageType identifies the kind of inter-session message.
type MessageType string

const (
	MsgTaskAssignment MessageType = "task_assignment"
	MsgReviewRequest  MessageType = "review_request"
	MsgReviewResponse MessageType = "review_response"
	MsgMemoryUpdate   MessageType = "memory_update"
)

// Envelope wraps every inter-session message with routing metadata.
type Envelope struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	From      string          `json:"from"`       // source session ID
	To        string          `json:"to"`         // destination session ID ("" = broadcast)
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// TaskAssignment tells a session to execute a plan step or specialist task.
type TaskAssignment struct {
	TaskID      string            `json:"task_id"`
	Description string            `json:"description"`
	Priority    int               `json:"priority"`              // higher = more urgent
	SkillTags   []string          `json:"skill_tags,omitempty"`  // for specialist routing
	Metadata    map[string]string `json:"metadata,omitempty"`
	Deadline    *time.Time        `json:"deadline,omitempty"`
}

// ReviewRequest asks a session to review the attached content.
type ReviewRequest struct {
	TaskID    string `json:"task_id"`
	Content   string `json:"content"`    // the artifact to review
	ChainStep int    `json:"chain_step"` // position in the review chain
}

// Verdict classifies a review outcome.
type Verdict string

const (
	VerdictApproved     Verdict = "approved"
	VerdictNeedsChanges Verdict = "needs_changes"
	VerdictRejected     Verdict = "rejected"
)

// ReviewResponse carries review results back to the orchestrator.
type ReviewResponse struct {
	TaskID   string   `json:"task_id"`
	Verdict  Verdict  `json:"verdict"`
	Comments []string `json:"comments,omitempty"`
	Score    float64  `json:"score"` // 0.0-1.0 quality score
}

// MemoryUpdate notifies sessions about a shared memory change.
type MemoryUpdate struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Revision int64  `json:"revision"`
}

// MarshalEnvelope creates an Envelope from a typed message.
func MarshalEnvelope(id, from, to string, msg interface{}) (*Envelope, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	var mt MessageType
	switch msg.(type) {
	case TaskAssignment, *TaskAssignment:
		mt = MsgTaskAssignment
	case ReviewRequest, *ReviewRequest:
		mt = MsgReviewRequest
	case ReviewResponse, *ReviewResponse:
		mt = MsgReviewResponse
	case MemoryUpdate, *MemoryUpdate:
		mt = MsgMemoryUpdate
	default:
		return nil, ErrUnknownMessageType
	}
	return &Envelope{
		ID:        id,
		Type:      mt,
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Payload:   payload,
	}, nil
}

// DecodePayload unmarshals the envelope payload into the appropriate type.
func (e *Envelope) DecodePayload() (interface{}, error) {
	switch e.Type {
	case MsgTaskAssignment:
		var v TaskAssignment
		if err := json.Unmarshal(e.Payload, &v); err != nil {
			return nil, err
		}
		return &v, nil
	case MsgReviewRequest:
		var v ReviewRequest
		if err := json.Unmarshal(e.Payload, &v); err != nil {
			return nil, err
		}
		return &v, nil
	case MsgReviewResponse:
		var v ReviewResponse
		if err := json.Unmarshal(e.Payload, &v); err != nil {
			return nil, err
		}
		return &v, nil
	case MsgMemoryUpdate:
		var v MemoryUpdate
		if err := json.Unmarshal(e.Payload, &v); err != nil {
			return nil, err
		}
		return &v, nil
	default:
		return nil, ErrUnknownMessageType
	}
}
