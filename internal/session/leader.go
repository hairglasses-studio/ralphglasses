package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// leaderRecord is the on-disk representation of a leader lease.
type leaderRecord struct {
	LeaderID  string    `json:"leader_id"`
	AcquireAt time.Time `json:"acquire_at"`
	RenewAt   time.Time `json:"renew_at"`
}

// LeaderElection implements file-based leader election with TTL expiry.
// At most one ralphglasses instance holds the leader lease at a time.
// Other instances can detect a stale lease (TTL exceeded) and take over.
type LeaderElection struct {
	stateDir string
	ttl      time.Duration

	mu       sync.Mutex
	leader   bool
	leaderID string
	cancel   context.CancelFunc
}

// NewLeaderElection creates a LeaderElection that stores its lease file
// under stateDir/leader. The ttl controls how long a lease is valid
// without renewal.
func NewLeaderElection(stateDir string, ttl time.Duration) *LeaderElection {
	return &LeaderElection{
		stateDir: stateDir,
		ttl:      ttl,
	}
}

// leaderPath returns the path to the leader lease file.
func (le *LeaderElection) leaderPath() string {
	return filepath.Join(le.stateDir, "leader")
}

// Campaign attempts to acquire leadership for the given instance ID.
// It blocks until leadership is acquired or the context is canceled.
// Once elected, it renews the lease at ttl/2 intervals in a background
// goroutine.
func (le *LeaderElection) Campaign(ctx context.Context, instanceID string) error {
	ticker := time.NewTicker(le.ttl / 4)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if le.tryAcquire(instanceID) {
			// Start renewal loop.
			rCtx, rCancel := context.WithCancel(ctx)
			le.mu.Lock()
			le.cancel = rCancel
			le.mu.Unlock()
			go le.renewLoop(rCtx, instanceID)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// IsLeader returns true if this election instance currently holds the
// leader lease.
func (le *LeaderElection) IsLeader() bool {
	le.mu.Lock()
	defer le.mu.Unlock()
	return le.leader
}

// LeaderID returns the ID of the current leader, or "" if unknown.
func (le *LeaderElection) LeaderID() string {
	le.mu.Lock()
	defer le.mu.Unlock()
	return le.leaderID
}

// Resign gives up leadership and removes the lease file.
func (le *LeaderElection) Resign() {
	le.mu.Lock()
	wasLeader := le.leader
	le.leader = false
	le.leaderID = ""
	cancel := le.cancel
	le.cancel = nil
	le.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if wasLeader {
		_ = os.Remove(le.leaderPath())
	}
}

// tryAcquire attempts to claim or take over the lease. Returns true on success.
func (le *LeaderElection) tryAcquire(instanceID string) bool {
	le.mu.Lock()
	defer le.mu.Unlock()

	path := le.leaderPath()

	// Read existing lease.
	data, err := os.ReadFile(path)
	if err == nil {
		var rec leaderRecord
		if json.Unmarshal(data, &rec) == nil {
			// If the lease belongs to us, just renew.
			if rec.LeaderID == instanceID {
				le.leader = true
				le.leaderID = instanceID
				le.writeLease(instanceID)
				return true
			}
			// If the lease is still valid, we cannot take over.
			if time.Since(rec.RenewAt) <= le.ttl {
				le.leaderID = rec.LeaderID
				return false
			}
			// Lease expired — fall through to claim.
		}
	}

	// No valid lease — claim leadership.
	if le.writeLease(instanceID) != nil {
		return false
	}
	le.leader = true
	le.leaderID = instanceID
	return true
}

// writeLease persists the lease file.
func (le *LeaderElection) writeLease(instanceID string) error {
	if err := os.MkdirAll(le.stateDir, 0o755); err != nil {
		return err
	}
	now := time.Now()
	rec := leaderRecord{
		LeaderID:  instanceID,
		AcquireAt: now,
		RenewAt:   now,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return os.WriteFile(le.leaderPath(), data, 0o644)
}

// renewLoop periodically renews the lease while ctx is active.
func (le *LeaderElection) renewLoop(ctx context.Context, instanceID string) {
	ticker := time.NewTicker(le.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			le.mu.Lock()
			if le.leader {
				_ = le.writeLease(instanceID)
			}
			le.mu.Unlock()
		}
	}
}
