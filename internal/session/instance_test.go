package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewInstance(t *testing.T) {
	inst := NewInstance()
	if inst.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if inst.PID != os.Getpid() {
		t.Fatalf("expected PID %d, got %d", os.Getpid(), inst.PID)
	}
	if inst.StartedAt.IsZero() {
		t.Fatal("expected non-zero StartedAt")
	}
	if inst.HeartbeatAt.IsZero() {
		t.Fatal("expected non-zero HeartbeatAt")
	}
}

func TestInstance_Heartbeat(t *testing.T) {
	inst := NewInstance()
	before := inst.HeartbeatAt

	// Ensure some time passes.
	time.Sleep(5 * time.Millisecond)
	inst.Heartbeat()

	if !inst.HeartbeatAt.After(before) {
		t.Fatal("Heartbeat did not advance HeartbeatAt")
	}
}

func TestInstance_IsStale(t *testing.T) {
	inst := NewInstance()

	if inst.IsStale(1 * time.Second) {
		t.Fatal("freshly created instance should not be stale")
	}

	// Manually set heartbeat in the past.
	inst.mu.Lock()
	inst.HeartbeatAt = time.Now().Add(-2 * time.Second)
	inst.mu.Unlock()

	if !inst.IsStale(1 * time.Second) {
		t.Fatal("instance with old heartbeat should be stale")
	}
}

func TestInstance_MarshalUnmarshal(t *testing.T) {
	inst := NewInstance()
	data, err := inst.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	inst2 := &Instance{}
	if err := inst2.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if inst2.ID != inst.ID {
		t.Fatalf("ID mismatch: %s vs %s", inst.ID, inst2.ID)
	}
	if inst2.PID != inst.PID {
		t.Fatalf("PID mismatch: %d vs %d", inst.PID, inst2.PID)
	}
}

func TestInstance_RegisterDeregister(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstance()

	if err := inst.Register(dir); err != nil {
		t.Fatalf("Register: %v", err)
	}

	path := filepath.Join(dir, "instances", inst.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("instance file should exist: %v", err)
	}

	if err := inst.Deregister(); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("instance file should be removed after Deregister")
	}
}

func TestInstance_HeartbeatPersists(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstance()
	if err := inst.Register(dir); err != nil {
		t.Fatalf("Register: %v", err)
	}

	time.Sleep(5 * time.Millisecond)
	inst.Heartbeat()

	// Read back from disk.
	path := filepath.Join(dir, "instances", inst.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	inst2 := &Instance{}
	if err := inst2.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !inst2.HeartbeatAt.After(inst2.StartedAt) {
		t.Fatal("persisted heartbeat should be after started_at")
	}
}
