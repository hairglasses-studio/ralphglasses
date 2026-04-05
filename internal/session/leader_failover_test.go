package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLeaderFailover_SingleInstanceBecomesLeader(t *testing.T) {
	dir := t.TempDir()
	lf := NewLeaderFailover(dir,
		WithHeartbeatInterval(50*time.Millisecond),
		WithInstanceID("inst-A"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- lf.Run(ctx) }()

	// Wait for leadership.
	deadline := time.After(2 * time.Second)
	for !lf.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for leadership")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if lf.LeaderID() != "inst-A" {
		t.Fatalf("expected leader ID inst-A, got %s", lf.LeaderID())
	}

	cancel()
	<-done
}

func TestLeaderFailover_SecondInstanceIsFollower(t *testing.T) {
	dir := t.TempDir()
	lf1 := NewLeaderFailover(dir,
		WithHeartbeatInterval(50*time.Millisecond),
		WithInstanceID("inst-A"),
	)
	lf2 := NewLeaderFailover(dir,
		WithHeartbeatInterval(50*time.Millisecond),
		WithInstanceID("inst-B"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done1 := make(chan error, 1)
	go func() { done1 <- lf1.Run(ctx) }()

	// Wait for inst-A to become leader.
	deadline := time.After(2 * time.Second)
	for !lf1.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for inst-A leadership")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Start inst-B — it should remain a follower.
	done2 := make(chan error, 1)
	go func() { done2 <- lf2.Run(ctx) }()

	// Give it a few heartbeat cycles.
	time.Sleep(200 * time.Millisecond)

	if lf2.IsLeader() {
		t.Fatal("inst-B should not be leader while inst-A is active")
	}
	if lf2.LeaderID() != "inst-A" {
		t.Fatalf("inst-B should see inst-A as leader, got %s", lf2.LeaderID())
	}

	cancel()
	<-done1
	<-done2
}

func TestLeaderFailover_FailoverOnStaleLock(t *testing.T) {
	dir := t.TempDir()
	interval := 50 * time.Millisecond

	// Write a stale lock file from a "dead" instance.
	staleTime := time.Now().Add(-time.Second).UnixNano()
	rec := leaderLockRecord{
		InstanceID:  "dead-inst",
		PID:         99999,
		HeartbeatAt: staleTime,
		AcquiredAt:  staleTime,
	}
	data, _ := json.Marshal(rec)
	if err := os.WriteFile(filepath.Join(dir, "leader.lock"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	lf := NewLeaderFailover(dir,
		WithHeartbeatInterval(interval),
		WithInstanceID("new-inst"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- lf.Run(ctx) }()

	// Should detect stale lock and take over quickly.
	deadline := time.After(2 * time.Second)
	for !lf.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for failover")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if lf.LeaderID() != "new-inst" {
		t.Fatalf("expected new-inst as leader, got %s", lf.LeaderID())
	}

	cancel()
	<-done
}

func TestLeaderFailover_GracefulHandoff(t *testing.T) {
	dir := t.TempDir()
	interval := 50 * time.Millisecond

	lf := NewLeaderFailover(dir,
		WithHeartbeatInterval(interval),
		WithInstanceID("inst-A"),
	)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- lf.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for !lf.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for leadership")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Graceful shutdown — should remove lock file.
	cancel()
	<-done

	if _, err := os.Stat(filepath.Join(dir, "leader.lock")); !os.IsNotExist(err) {
		t.Fatal("lock file should be removed after graceful shutdown")
	}

	if lf.IsLeader() {
		t.Fatal("should not be leader after shutdown")
	}

	// A new instance should acquire immediately.
	lf2 := NewLeaderFailover(dir,
		WithHeartbeatInterval(interval),
		WithInstanceID("inst-B"),
	)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	done2 := make(chan error, 1)
	go func() { done2 <- lf2.Run(ctx2) }()

	deadline = time.After(2 * time.Second)
	for !lf2.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for inst-B after handoff")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel2()
	<-done2
}

func TestLeaderFailover_CallbacksFire(t *testing.T) {
	dir := t.TempDir()
	interval := 50 * time.Millisecond

	lf := NewLeaderFailover(dir,
		WithHeartbeatInterval(interval),
		WithInstanceID("inst-cb"),
	)

	var becameLeader atomic.Int32
	var lostLeader atomic.Int32

	lf.OnBecomeLeader(func() { becameLeader.Add(1) })
	lf.OnLoseLeadership(func() { lostLeader.Add(1) })

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- lf.Run(ctx) }()

	deadline := time.After(2 * time.Second)
	for !lf.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for leadership")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if becameLeader.Load() != 1 {
		t.Fatalf("OnBecomeLeader should fire once, fired %d", becameLeader.Load())
	}
	if lostLeader.Load() != 0 {
		t.Fatalf("OnLoseLeadership should not fire yet, fired %d", lostLeader.Load())
	}

	// Shutdown triggers lose-leadership callback.
	cancel()
	<-done

	if lostLeader.Load() != 1 {
		t.Fatalf("OnLoseLeadership should fire once after shutdown, fired %d", lostLeader.Load())
	}
}

func TestLeaderFailover_ConcurrentElectionSafety(t *testing.T) {
	dir := t.TempDir()
	interval := 30 * time.Millisecond
	const n = 5

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	instances := make([]*LeaderFailover, n)
	dones := make([]chan error, n)
	var wg sync.WaitGroup

	for i := range n {
		instances[i] = NewLeaderFailover(dir,
			WithHeartbeatInterval(interval),
			WithInstanceID(fmt.Sprintf("inst-%d", i)),
		)
		dones[i] = make(chan error, 1)
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			dones[idx] <- instances[idx].Run(ctx)
		}()
	}

	// Let them run for a while.
	time.Sleep(500 * time.Millisecond)

	// Exactly one instance should be leader.
	leaderCount := 0
	var leaderID string
	for i := range n {
		if instances[i].IsLeader() {
			leaderCount++
			leaderID = instances[i].LeaderID()
		}
	}

	if leaderCount != 1 {
		t.Fatalf("expected exactly 1 leader, got %d", leaderCount)
	}

	// All followers should agree on who the leader is.
	for i := range n {
		id := instances[i].LeaderID()
		if id != leaderID {
			t.Fatalf("instance %d sees leader %s, expected %s", i, id, leaderID)
		}
	}

	cancel()
	wg.Wait()
}

func TestLeaderFailover_FollowerTakesOverAfterLeaderStops(t *testing.T) {
	dir := t.TempDir()
	interval := 50 * time.Millisecond

	lf1 := NewLeaderFailover(dir,
		WithHeartbeatInterval(interval),
		WithInstanceID("inst-1"),
	)
	lf2 := NewLeaderFailover(dir,
		WithHeartbeatInterval(interval),
		WithInstanceID("inst-2"),
	)

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	done1 := make(chan error, 1)
	go func() { done1 <- lf1.Run(ctx1) }()

	// Wait for inst-1 leadership.
	deadline := time.After(2 * time.Second)
	for !lf1.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for inst-1")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Start follower.
	done2 := make(chan error, 1)
	go func() { done2 <- lf2.Run(ctx2) }()

	time.Sleep(200 * time.Millisecond)
	if lf2.IsLeader() {
		t.Fatal("inst-2 should be follower")
	}

	// Stop leader gracefully (removes lock file).
	cancel1()
	<-done1

	// inst-2 should take over.
	deadline = time.After(2 * time.Second)
	for !lf2.IsLeader() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for inst-2 failover")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if lf2.LeaderID() != "inst-2" {
		t.Fatalf("expected inst-2 as leader, got %s", lf2.LeaderID())
	}

	cancel2()
	<-done2
}
