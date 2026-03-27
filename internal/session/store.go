package session

import "context"

// ListOpts controls filtering for Store.ListSessions.
type ListOpts struct {
	RepoPath string        // filter by repo path (empty = all)
	RepoName string        // filter by repo name (empty = all)
	Status   SessionStatus // filter by status (empty = all)
	Limit    int           // max results (0 = unlimited)
}

// Store is the persistence interface for session state.
// Both in-memory and SQLite implementations satisfy this interface.
type Store interface {
	// SaveSession upserts a session (insert or update).
	SaveSession(ctx context.Context, s *Session) error

	// GetSession returns a session by ID. Returns ErrSessionNotFound if missing.
	GetSession(ctx context.Context, id string) (*Session, error)

	// ListSessions returns sessions matching the given filters.
	ListSessions(ctx context.Context, opts ListOpts) ([]*Session, error)

	// DeleteSession removes a session by ID. No error if not found.
	DeleteSession(ctx context.Context, id string) error

	// UpdateSessionStatus sets the status of a session.
	UpdateSessionStatus(ctx context.Context, id string, status SessionStatus) error

	// AggregateSpend returns total spend_usd across sessions for a repo path.
	// If repo is empty, returns total across all sessions.
	AggregateSpend(ctx context.Context, repo string) (float64, error)

	// Close releases any resources held by the store.
	Close() error
}
