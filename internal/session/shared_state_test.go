package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestSharedState(t *testing.T) *SharedState {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shared_state.db")
	ss, err := NewSharedState(dbPath)
	if err != nil {
		t.Fatalf("NewSharedState: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	return ss
}

func TestSharedState_WALMode(t *testing.T) {
	ss := newTestSharedState(t)
	var mode string
	if err := ss.DB().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestSharedState_PutGet(t *testing.T) {
	ss := newTestSharedState(t)

	if err := ss.Put("key1", "val1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	v, err := ss.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "val1" {
		t.Errorf("Get = %q, want %q", v, "val1")
	}
}

func TestSharedState_PutOverwrite(t *testing.T) {
	ss := newTestSharedState(t)

	if err := ss.Put("k", "v1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := ss.Put("k", "v2"); err != nil {
		t.Fatalf("Put overwrite: %v", err)
	}
	v, err := ss.Get("k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "v2" {
		t.Errorf("Get = %q, want %q", v, "v2")
	}
}

func TestSharedState_GetNotFound(t *testing.T) {
	ss := newTestSharedState(t)
	_, err := ss.Get("nonexistent")
	if err != sql.ErrNoRows {
		t.Errorf("Get nonexistent err = %v, want sql.ErrNoRows", err)
	}
}

func TestSharedState_Delete(t *testing.T) {
	ss := newTestSharedState(t)

	if err := ss.Put("k", "v"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := ss.Delete("k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := ss.Get("k")
	if err != sql.ErrNoRows {
		t.Errorf("Get after delete err = %v, want sql.ErrNoRows", err)
	}
}

func TestSharedState_DeleteNonexistent(t *testing.T) {
	ss := newTestSharedState(t)
	// Deleting a non-existent key should not error.
	if err := ss.Delete("nope"); err != nil {
		t.Errorf("Delete nonexistent: %v", err)
	}
}

func TestSharedState_List(t *testing.T) {
	ss := newTestSharedState(t)

	for _, kv := range []struct{ k, v string }{
		{"session:a:status", "running"},
		{"session:a:prompt", "hello"},
		{"session:b:status", "done"},
		{"global:version", "1"},
	} {
		if err := ss.Put(kv.k, kv.v); err != nil {
			t.Fatalf("Put %s: %v", kv.k, err)
		}
	}

	m, err := ss.List("session:a:")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("List len = %d, want 2", len(m))
	}
	if m["session:a:status"] != "running" {
		t.Errorf("status = %q", m["session:a:status"])
	}

	m2, err := ss.List("global:")
	if err != nil {
		t.Fatalf("List global: %v", err)
	}
	if len(m2) != 1 {
		t.Errorf("List global len = %d, want 1", len(m2))
	}
}

func TestSharedState_ListEmpty(t *testing.T) {
	ss := newTestSharedState(t)
	m, err := ss.List("nope:")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("List empty = %d, want 0", len(m))
	}
}

func TestSharedState_LockAcquireRelease(t *testing.T) {
	ss := newTestSharedState(t)

	ok, err := ss.Lock("build", "worker-1", 5*time.Second)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if !ok {
		t.Fatal("Lock should have been acquired")
	}

	// Another holder should fail.
	ok2, err := ss.Lock("build", "worker-2", 5*time.Second)
	if err != nil {
		t.Fatalf("Lock worker-2: %v", err)
	}
	if ok2 {
		t.Error("Lock should have been denied for worker-2")
	}

	// Same holder should succeed (refresh).
	ok3, err := ss.Lock("build", "worker-1", 5*time.Second)
	if err != nil {
		t.Fatalf("Lock refresh: %v", err)
	}
	if !ok3 {
		t.Error("Lock refresh should succeed for same holder")
	}

	// Unlock.
	if err := ss.Unlock("build", "worker-1"); err != nil {
		t.Fatalf("Unlock: %v", err)
	}

	// Now worker-2 should acquire.
	ok4, err := ss.Lock("build", "worker-2", 5*time.Second)
	if err != nil {
		t.Fatalf("Lock worker-2 after unlock: %v", err)
	}
	if !ok4 {
		t.Error("worker-2 should acquire after unlock")
	}
}

func TestSharedState_LockTTLExpiry(t *testing.T) {
	ss := newTestSharedState(t)

	// Acquire with very short TTL.
	ok, err := ss.Lock("ephemeral", "holder-a", 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if !ok {
		t.Fatal("Lock should be acquired")
	}

	// Wait for expiry.
	time.Sleep(10 * time.Millisecond)

	// Another holder should acquire since TTL expired.
	ok2, err := ss.Lock("ephemeral", "holder-b", 5*time.Second)
	if err != nil {
		t.Fatalf("Lock after expiry: %v", err)
	}
	if !ok2 {
		t.Error("Lock should be acquired after TTL expiry")
	}
}

func TestSharedState_UnlockWrongHolder(t *testing.T) {
	ss := newTestSharedState(t)

	ok, err := ss.Lock("res", "owner", 5*time.Second)
	if err != nil || !ok {
		t.Fatalf("Lock: ok=%v err=%v", ok, err)
	}

	// Unlock by wrong holder should be no-op.
	if err := ss.Unlock("res", "intruder"); err != nil {
		t.Fatalf("Unlock wrong holder: %v", err)
	}

	// Original holder should still fail for a different holder.
	ok2, err := ss.Lock("res", "intruder", 5*time.Second)
	if err != nil {
		t.Fatalf("Lock intruder: %v", err)
	}
	if ok2 {
		t.Error("intruder should not acquire lock still held by owner")
	}
}

func TestSharedState_ConcurrentAccess(t *testing.T) {
	ss := newTestSharedState(t)
	const n = 50
	var wg sync.WaitGroup

	// Concurrent puts.
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("conc:%d", i)
			if err := ss.Put(key, fmt.Sprintf("val%d", i)); err != nil {
				t.Errorf("concurrent Put %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	// Verify all written.
	m, err := ss.List("conc:")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(m) != n {
		t.Errorf("List len = %d, want %d", len(m), n)
	}

	// Concurrent reads.
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("conc:%d", i)
			v, err := ss.Get(key)
			if err != nil {
				t.Errorf("concurrent Get %d: %v", i, err)
				return
			}
			want := fmt.Sprintf("val%d", i)
			if v != want {
				t.Errorf("concurrent Get %d = %q, want %q", i, v, want)
			}
		}(i)
	}
	wg.Wait()
}

func TestSharedState_Watch(t *testing.T) {
	ss := newTestSharedState(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var mu sync.Mutex
	seen := make(map[string]string)

	go func() {
		_ = ss.Watch(ctx, "watch:", func(key, value string) {
			mu.Lock()
			seen[key] = value
			mu.Unlock()
		})
	}()

	// Give the watcher time to start.
	time.Sleep(50 * time.Millisecond)

	// Write some keys.
	if err := ss.Put("watch:x", "1"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := ss.Put("watch:y", "2"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Write unrelated key.
	if err := ss.Put("other:z", "3"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Wait for at least one poll cycle.
	time.Sleep(700 * time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 {
		t.Errorf("Watch saw %d keys, want >= 2: %v", len(seen), seen)
	}
	if seen["watch:x"] != "1" || seen["watch:y"] != "2" {
		t.Errorf("Watch seen = %v", seen)
	}
	if _, ok := seen["other:z"]; ok {
		t.Error("Watch should not see keys outside prefix")
	}
}

func TestSharedState_NewCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	dbPath := filepath.Join(dir, "state.db")
	ss, err := NewSharedState(dbPath)
	if err != nil {
		t.Fatalf("NewSharedState nested: %v", err)
	}
	ss.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("DB file should exist after NewSharedState")
	}
}
