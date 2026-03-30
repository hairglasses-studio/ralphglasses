package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCoordinator_ClaimAndRelease(t *testing.T) {
	t.Parallel()
	c := NewCoordinator()

	// First claim succeeds.
	ok, err := c.ClaimTask("s1", "task-A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim to succeed")
	}

	// Same session reclaiming is idempotent.
	ok, err = c.ClaimTask("s1", "task-A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected idempotent reclaim to succeed")
	}

	// Different session cannot claim same task.
	ok, err = c.ClaimTask("s2", "task-A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected claim by different session to fail")
	}

	// Release and then re-claim by other session.
	c.ReleaseTask("s1", "task-A")
	ok, err = c.ClaimTask("s2", "task-A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected claim after release to succeed")
	}
}

func TestCoordinator_ActiveClaims(t *testing.T) {
	t.Parallel()
	c := NewCoordinator()

	_, _ = c.ClaimTask("s1", "task-1")
	_, _ = c.ClaimTask("s1", "task-2")
	_, _ = c.ClaimTask("s2", "task-3")

	claims := c.ActiveClaims("s1")
	if len(claims) != 2 {
		t.Fatalf("expected 2 active claims for s1, got %d", len(claims))
	}

	claims2 := c.ActiveClaims("s2")
	if len(claims2) != 1 {
		t.Fatalf("expected 1 active claim for s2, got %d", len(claims2))
	}
}

func TestCoordinator_AllClaims(t *testing.T) {
	t.Parallel()
	c := NewCoordinator()

	_, _ = c.ClaimTask("s1", "a")
	_, _ = c.ClaimTask("s2", "b")

	all := c.AllClaims()
	if len(all) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(all))
	}
	if all["a"] != "s1" || all["b"] != "s2" {
		t.Fatalf("unexpected claims map: %v", all)
	}

	// Verify snapshot isolation -- mutating returned map does not affect coordinator.
	all["c"] = "s3"
	if len(c.AllClaims()) != 2 {
		t.Fatal("AllClaims should return a snapshot, not a reference")
	}
}

func TestCoordinator_ReleaseAll(t *testing.T) {
	t.Parallel()
	c := NewCoordinator()

	_, _ = c.ClaimTask("s1", "a")
	_, _ = c.ClaimTask("s1", "b")
	_, _ = c.ClaimTask("s2", "c")

	c.ReleaseAll("s1")

	if len(c.ActiveClaims("s1")) != 0 {
		t.Fatal("expected all s1 claims released")
	}
	if len(c.ActiveClaims("s2")) != 1 {
		t.Fatal("expected s2 claims unaffected")
	}
}

func TestCoordinator_ReleaseTask_WrongSession(t *testing.T) {
	t.Parallel()
	c := NewCoordinator()

	_, _ = c.ClaimTask("s1", "task-X")

	// Releasing from wrong session is a no-op.
	c.ReleaseTask("s2", "task-X")

	all := c.AllClaims()
	if all["task-X"] != "s1" {
		t.Fatal("release from wrong session should not remove claim")
	}
}

func TestCoordinator_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	c := NewCoordinator()

	const goroutines = 50
	var wg sync.WaitGroup
	wins := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		sid := "session-" + string(rune('A'+i%26)) + string(rune('0'+i/26))
		go func(sid string) {
			defer wg.Done()
			ok, err := c.ClaimTask(sid, "contested-task")
			if err != nil {
				t.Errorf("unexpected error from %s: %v", sid, err)
				return
			}
			if ok {
				wins <- sid
			}
		}(sid)
	}
	wg.Wait()
	close(wins)

	// Exactly one goroutine should have won the claim.
	var winners []string
	for w := range wins {
		winners = append(winners, w)
	}
	if len(winners) != 1 {
		t.Fatalf("expected exactly 1 winner, got %d: %v", len(winners), winners)
	}
}

func TestCoordinator_Persistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fp := filepath.Join(dir, "claims.json")

	// Create coordinator with persistence and add claims.
	c1, err := NewCoordinatorWithPersistence(fp)
	if err != nil {
		t.Fatalf("failed to create coordinator: %v", err)
	}
	_, _ = c1.ClaimTask("s1", "task-1")
	_, _ = c1.ClaimTask("s2", "task-2")

	// Verify file was written.
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("persistence file not created: %v", err)
	}

	// Create second coordinator from same file -- claims should load.
	c2, err := NewCoordinatorWithPersistence(fp)
	if err != nil {
		t.Fatalf("failed to load coordinator: %v", err)
	}
	all := c2.AllClaims()
	if all["task-1"] != "s1" || all["task-2"] != "s2" {
		t.Fatalf("persisted claims not loaded correctly: %v", all)
	}

	// New session should not be able to claim an already persisted task.
	ok, _ := c2.ClaimTask("s3", "task-1")
	if ok {
		t.Fatal("should not be able to claim task already owned by s1")
	}
}
