package session

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LeaderOption configures a LeaderFailover instance.
type LeaderOption func(*LeaderFailover)

// WithHeartbeatInterval sets the heartbeat write interval.
// Stale lock detection uses 3x this value.
func WithHeartbeatInterval(d time.Duration) LeaderOption {
	return func(lf *LeaderFailover) { lf.heartbeatInterval = d }
}

// WithInstanceID overrides the auto-generated instance ID.
func WithInstanceID(id string) LeaderOption {
	return func(lf *LeaderFailover) { lf.instanceID = id }
}

// leaderLockRecord is the JSON structure written to the lock file.
type leaderLockRecord struct {
	InstanceID string `json:"instance_id"`
	PID        int    `json:"pid"`
	HeartbeatAt int64 `json:"heartbeat_at_unix_ns"`
	AcquiredAt  int64 `json:"acquired_at_unix_ns"`
}

// LeaderFailover implements file-based leader election with PID heartbeat,
// stale lock detection, callbacks, and graceful handoff for multi-instance
// coordination.
type LeaderFailover struct {
	lockDir           string
	heartbeatInterval time.Duration
	instanceID        string

	mu              sync.Mutex
	isLeader        bool
	currentLeaderID string
	onBecomeLeader  []func()
	onLoseLeader    []func()
	stopped         bool
}

// NewLeaderFailover creates a new leader election coordinator.
// lockDir is the directory where the lock file will be written.
// Default heartbeat interval is 1 second (stale threshold = 3s).
func NewLeaderFailover(lockDir string, opts ...LeaderOption) *LeaderFailover {
	lf := &LeaderFailover{
		lockDir:           lockDir,
		heartbeatInterval: time.Second,
		instanceID:        fmt.Sprintf("inst-%d-%d", os.Getpid(), time.Now().UnixNano()),
	}
	for _, o := range opts {
		o(lf)
	}
	return lf
}

// lockPath returns the path to the leader lock file.
func (lf *LeaderFailover) lockPath() string {
	return filepath.Join(lf.lockDir, "leader.lock")
}

// staleThreshold returns 3x the heartbeat interval.
func (lf *LeaderFailover) staleThreshold() time.Duration {
	return 3 * lf.heartbeatInterval
}

// IsLeader reports whether this instance currently holds leadership.
func (lf *LeaderFailover) IsLeader() bool {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	return lf.isLeader
}

// LeaderID returns the instance ID of the current leader, or "" if unknown.
func (lf *LeaderFailover) LeaderID() string {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	return lf.currentLeaderID
}

// OnBecomeLeader registers a callback that fires when this instance
// transitions from follower to leader.
func (lf *LeaderFailover) OnBecomeLeader(fn func()) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	lf.onBecomeLeader = append(lf.onBecomeLeader, fn)
}

// OnLoseLeadership registers a callback that fires when this instance
// transitions from leader to follower (or stops).
func (lf *LeaderFailover) OnLoseLeadership(fn func()) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	lf.onLoseLeader = append(lf.onLoseLeader, fn)
}

// Run starts the leader election loop. It blocks until ctx is canceled.
// On context cancellation, if this instance is leader, it performs a
// graceful handoff by removing the lock file.
func (lf *LeaderFailover) Run(ctx context.Context) error {
	if err := os.MkdirAll(lf.lockDir, 0o755); err != nil {
		return fmt.Errorf("leader failover: mkdir %s: %w", lf.lockDir, err)
	}

	ticker := time.NewTicker(lf.heartbeatInterval)
	defer ticker.Stop()

	// Run one tick immediately.
	lf.tick()

	for {
		select {
		case <-ctx.Done():
			lf.shutdown()
			return ctx.Err()
		case <-ticker.C:
			lf.tick()
		}
	}
}

// tick performs a single election/heartbeat cycle.
func (lf *LeaderFailover) tick() {
	lf.mu.Lock()
	if lf.stopped {
		lf.mu.Unlock()
		return
	}
	wasLeader := lf.isLeader
	lf.mu.Unlock()

	if wasLeader {
		// Renew heartbeat.
		if err := lf.writeHeartbeat(); err != nil {
			// Lost ability to write — lose leadership.
			lf.transitionToFollower()
			return
		}
		// Verify we still own the lock (another instance may have taken over
		// if our heartbeat was late).
		rec, err := lf.readLock()
		if err != nil || rec.InstanceID != lf.instanceID {
			lf.transitionToFollower()
		}
	} else {
		lf.tryAcquireLock()
	}
}

// tryAcquireLock attempts to claim leadership if no valid lock exists.
// Uses write-to-temp + rename + verify to handle concurrent contention.
func (lf *LeaderFailover) tryAcquireLock() {
	rec, err := lf.readLock()
	if err == nil {
		// Lock file exists. Check if stale.
		heartbeat := time.Unix(0, rec.HeartbeatAt)
		if time.Since(heartbeat) <= lf.staleThreshold() {
			// Lock is fresh — record who the leader is.
			lf.mu.Lock()
			lf.currentLeaderID = rec.InstanceID
			lf.mu.Unlock()
			return
		}
		// Stale lock — fall through to attempt takeover.
	}

	// Random jitter to reduce simultaneous write collisions.
	//nolint:gosec // not security-sensitive
	time.Sleep(time.Duration(rand.Intn(int(lf.heartbeatInterval / 4))))

	// Attempt to acquire by writing via temp+rename (atomic on POSIX).
	if err := lf.writeHeartbeat(); err != nil {
		return
	}

	// Delay proportional to heartbeat interval then verify we won the race.
	time.Sleep(lf.heartbeatInterval / 4)
	verify, err := lf.readLock()
	if err != nil || verify.InstanceID != lf.instanceID {
		if err == nil {
			lf.mu.Lock()
			lf.currentLeaderID = verify.InstanceID
			lf.mu.Unlock()
		}
		return
	}
	lf.transitionToLeader()
}

// transitionToLeader sets this instance as leader and fires callbacks.
func (lf *LeaderFailover) transitionToLeader() {
	lf.mu.Lock()
	if lf.isLeader {
		lf.mu.Unlock()
		return
	}
	lf.isLeader = true
	lf.currentLeaderID = lf.instanceID
	callbacks := make([]func(), len(lf.onBecomeLeader))
	copy(callbacks, lf.onBecomeLeader)
	lf.mu.Unlock()

	for _, fn := range callbacks {
		fn()
	}
}

// transitionToFollower clears leadership and fires callbacks.
func (lf *LeaderFailover) transitionToFollower() {
	lf.mu.Lock()
	if !lf.isLeader {
		lf.mu.Unlock()
		return
	}
	lf.isLeader = false
	callbacks := make([]func(), len(lf.onLoseLeader))
	copy(callbacks, lf.onLoseLeader)
	lf.mu.Unlock()

	for _, fn := range callbacks {
		fn()
	}
}

// shutdown performs graceful handoff: removes the lock file if we are leader,
// fires lose-leadership callbacks.
func (lf *LeaderFailover) shutdown() {
	lf.mu.Lock()
	wasLeader := lf.isLeader
	lf.isLeader = false
	lf.stopped = true
	lf.currentLeaderID = ""
	callbacks := make([]func(), len(lf.onLoseLeader))
	copy(callbacks, lf.onLoseLeader)
	lf.mu.Unlock()

	if wasLeader {
		_ = os.Remove(lf.lockPath())
		for _, fn := range callbacks {
			fn()
		}
	}
}

// writeHeartbeat atomically writes (or overwrites) the lock file with this
// instance's PID and current timestamp using temp-file + rename.
func (lf *LeaderFailover) writeHeartbeat() error {
	now := time.Now().UnixNano()
	rec := leaderLockRecord{
		InstanceID:  lf.instanceID,
		PID:         os.Getpid(),
		HeartbeatAt: now,
		AcquiredAt:  now,
	}

	// Preserve original acquired_at if we already have a lock.
	existing, err := lf.readLock()
	if err == nil && existing.InstanceID == lf.instanceID {
		rec.AcquiredAt = existing.AcquiredAt
	}

	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	// Write to a temp file first, then atomically rename.
	tmp := filepath.Join(lf.lockDir, fmt.Sprintf(".leader.lock.%s.tmp", lf.instanceID))
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, lf.lockPath())
}

// readLock reads and parses the lock file. Returns an error if the file
// does not exist or cannot be parsed.
func (lf *LeaderFailover) readLock() (leaderLockRecord, error) {
	data, err := os.ReadFile(lf.lockPath())
	if err != nil {
		return leaderLockRecord{}, err
	}
	var rec leaderLockRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return leaderLockRecord{}, err
	}
	return rec, nil
}
