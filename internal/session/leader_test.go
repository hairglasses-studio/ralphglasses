package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLeaderElection_CampaignAndIsLeader(t *testing.T) {
	dir := t.TempDir()
	le := NewLeaderElection(dir, 2*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := le.Campaign(ctx, "inst-1"); err != nil {
		t.Fatalf("Campaign: %v", err)
	}

	if !le.IsLeader() {
		t.Fatal("expected to be leader after Campaign")
	}
	if le.LeaderID() != "inst-1" {
		t.Fatalf("expected leader ID inst-1, got %s", le.LeaderID())
	}

	le.Resign()
}

func TestLeaderElection_Resign(t *testing.T) {
	dir := t.TempDir()
	le := NewLeaderElection(dir, 2*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := le.Campaign(ctx, "inst-1"); err != nil {
		t.Fatalf("Campaign: %v", err)
	}

	le.Resign()

	if le.IsLeader() {
		t.Fatal("should not be leader after Resign")
	}
	if le.LeaderID() != "" {
		t.Fatalf("expected empty leader ID after Resign, got %s", le.LeaderID())
	}

	// Lease file should be removed.
	if _, err := os.Stat(filepath.Join(dir, "leader")); !os.IsNotExist(err) {
		t.Fatal("leader file should be removed after Resign")
	}
}

func TestLeaderElection_SecondInstanceBlocked(t *testing.T) {
	dir := t.TempDir()
	le1 := NewLeaderElection(dir, 2*time.Second)
	le2 := NewLeaderElection(dir, 2*time.Second)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	if err := le1.Campaign(ctx1, "inst-1"); err != nil {
		t.Fatalf("Campaign inst-1: %v", err)
	}
	defer le1.Resign()

	// Second instance should fail to acquire within a short timeout.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	err := le2.Campaign(ctx2, "inst-2")
	if err == nil {
		le2.Resign()
		t.Fatal("expected second instance to fail Campaign while first holds lease")
	}

	if !le1.IsLeader() {
		t.Fatal("first instance should still be leader")
	}
}

func TestLeaderElection_Failover(t *testing.T) {
	dir := t.TempDir()
	ttl := 200 * time.Millisecond

	le1 := NewLeaderElection(dir, ttl)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	if err := le1.Campaign(ctx1, "inst-1"); err != nil {
		t.Fatalf("Campaign inst-1: %v", err)
	}

	// Resign first instance (simulates crash — no more renewals).
	le1.Resign()

	// Write an expired lease to simulate a crash where the file remains.
	rec := leaderRecord{
		LeaderID:  "inst-1",
		AcquireAt: time.Now().Add(-2 * ttl),
		RenewAt:   time.Now().Add(-2 * ttl),
	}
	data, _ := json.Marshal(rec)
	_ = os.WriteFile(filepath.Join(dir, "leader"), data, 0o644)

	// Second instance should take over.
	le2 := NewLeaderElection(dir, ttl)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if err := le2.Campaign(ctx2, "inst-2"); err != nil {
		t.Fatalf("Campaign inst-2 after failover: %v", err)
	}
	defer le2.Resign()

	if !le2.IsLeader() {
		t.Fatal("inst-2 should be leader after failover")
	}
	if le2.LeaderID() != "inst-2" {
		t.Fatalf("expected leader ID inst-2, got %s", le2.LeaderID())
	}
}

func TestLeaderElection_CampaignCanceled(t *testing.T) {
	dir := t.TempDir()
	le1 := NewLeaderElection(dir, 2*time.Second)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	if err := le1.Campaign(ctx1, "inst-1"); err != nil {
		t.Fatalf("Campaign inst-1: %v", err)
	}
	defer le1.Resign()

	// Cancel the context before Campaign for inst-2.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2() // cancel immediately

	le2 := NewLeaderElection(dir, 2*time.Second)
	err := le2.Campaign(ctx2, "inst-2")
	if err == nil {
		le2.Resign()
		t.Fatal("expected error from Campaign with canceled context")
	}
}

func TestLeaderElection_LeaseFileCreated(t *testing.T) {
	dir := t.TempDir()
	le := NewLeaderElection(dir, 2*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := le.Campaign(ctx, "inst-1"); err != nil {
		t.Fatalf("Campaign: %v", err)
	}
	defer le.Resign()

	data, err := os.ReadFile(filepath.Join(dir, "leader"))
	if err != nil {
		t.Fatalf("leader file should exist: %v", err)
	}

	var rec leaderRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal leader record: %v", err)
	}
	if rec.LeaderID != "inst-1" {
		t.Fatalf("expected leader_id inst-1, got %s", rec.LeaderID)
	}
}
