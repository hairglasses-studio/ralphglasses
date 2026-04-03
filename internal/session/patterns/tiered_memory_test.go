package patterns

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
)

// mockWarmStore implements WarmStore for testing without SQLite.
type mockWarmStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMockWarmStore() *mockWarmStore {
	return &mockWarmStore{data: make(map[string]string)}
}

func (m *mockWarmStore) Get(key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return v, nil
}

func (m *mockWarmStore) Put(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockWarmStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// mockColdStore implements ColdStore for testing.
type mockColdStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMockColdStore() *mockColdStore {
	return &mockColdStore{data: make(map[string]string)}
}

func (m *mockColdStore) Get(key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return "", ErrKeyNotFound
	}
	return v, nil
}

func (m *mockColdStore) Put(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockColdStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func TestTieredMemoryGetHot(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("key1", "hot-value")

	tm := NewTieredMemory(hot, nil, nil)
	v, tier, err := tm.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "hot-value" {
		t.Errorf("value = %q, want hot-value", v)
	}
	if tier != TierHot {
		t.Errorf("tier = %v, want hot", tier)
	}
	stats := tm.Stats()
	if stats.HotHits != 1 {
		t.Errorf("hot hits = %d, want 1", stats.HotHits)
	}
}

func TestTieredMemoryGetWarm(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	warm.data["key1"] = "warm-value"

	tm := NewTieredMemory(hot, warm, nil)
	v, tier, err := tm.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "warm-value" {
		t.Errorf("value = %q, want warm-value", v)
	}
	if tier != TierWarm {
		t.Errorf("tier = %v, want warm", tier)
	}
	stats := tm.Stats()
	if stats.WarmHits != 1 {
		t.Errorf("warm hits = %d, want 1", stats.WarmHits)
	}
}

func TestTieredMemoryGetCold(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	cold := newMockColdStore()
	cold.data["key1"] = "cold-value"

	tm := NewTieredMemory(hot, warm, cold)
	v, tier, err := tm.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "cold-value" {
		t.Errorf("value = %q, want cold-value", v)
	}
	if tier != TierCold {
		t.Errorf("tier = %v, want cold", tier)
	}
	stats := tm.Stats()
	if stats.ColdHits != 1 {
		t.Errorf("cold hits = %d, want 1", stats.ColdHits)
	}
}

func TestTieredMemoryGetMiss(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	cold := newMockColdStore()

	tm := NewTieredMemory(hot, warm, cold)
	_, _, err := tm.Get("nonexistent")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
	stats := tm.Stats()
	if stats.Misses != 1 {
		t.Errorf("misses = %d, want 1", stats.Misses)
	}
}

func TestTieredMemoryGetMissNilTiers(t *testing.T) {
	hot := NewSharedMemory()
	tm := NewTieredMemory(hot, nil, nil)
	_, _, err := tm.Get("nonexistent")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
}

func TestTieredMemoryGetFallthrough(t *testing.T) {
	// Hot has key1, warm has key2, cold has key3.
	hot := NewSharedMemory()
	hot.Set("key1", "hot")
	warm := newMockWarmStore()
	warm.data["key2"] = "warm"
	cold := newMockColdStore()
	cold.data["key3"] = "cold"

	tm := NewTieredMemory(hot, warm, cold)

	v, tier, _ := tm.Get("key1")
	if v != "hot" || tier != TierHot {
		t.Errorf("key1: v=%q tier=%v", v, tier)
	}
	v, tier, _ = tm.Get("key2")
	if v != "warm" || tier != TierWarm {
		t.Errorf("key2: v=%q tier=%v", v, tier)
	}
	v, tier, _ = tm.Get("key3")
	if v != "cold" || tier != TierCold {
		t.Errorf("key3: v=%q tier=%v", v, tier)
	}

	stats := tm.Stats()
	if stats.HotHits != 1 || stats.WarmHits != 1 || stats.ColdHits != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestTieredMemoryPut(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	cold := newMockColdStore()
	tm := NewTieredMemory(hot, warm, cold)

	if err := tm.Put("k", "v-hot", TierHot); err != nil {
		t.Fatalf("Put hot: %v", err)
	}
	v, _, _ := hot.Get("k")
	if v != "v-hot" {
		t.Errorf("hot value = %q", v)
	}

	if err := tm.Put("k", "v-warm", TierWarm); err != nil {
		t.Fatalf("Put warm: %v", err)
	}
	v, _ = warm.Get("k")
	if v != "v-warm" {
		t.Errorf("warm value = %q", v)
	}

	if err := tm.Put("k", "v-cold", TierCold); err != nil {
		t.Fatalf("Put cold: %v", err)
	}
	v, _ = cold.Get("k")
	if v != "v-cold" {
		t.Errorf("cold value = %q", v)
	}
}

func TestTieredMemoryPutNilTier(t *testing.T) {
	hot := NewSharedMemory()
	tm := NewTieredMemory(hot, nil, nil)

	if err := tm.Put("k", "v", TierWarm); !errors.Is(err, ErrInvalidTier) {
		t.Errorf("put warm nil: err = %v, want ErrInvalidTier", err)
	}
	if err := tm.Put("k", "v", TierCold); !errors.Is(err, ErrInvalidTier) {
		t.Errorf("put cold nil: err = %v, want ErrInvalidTier", err)
	}
}

func TestTieredMemoryPutInvalidTier(t *testing.T) {
	hot := NewSharedMemory()
	tm := NewTieredMemory(hot, nil, nil)
	if err := tm.Put("k", "v", MemoryTier(99)); !errors.Is(err, ErrInvalidTier) {
		t.Errorf("err = %v, want ErrInvalidTier", err)
	}
}

func TestTieredMemoryPromoteColdToHot(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	cold := newMockColdStore()
	cold.data["key1"] = "cold-val"

	tm := NewTieredMemory(hot, warm, cold)

	if err := tm.Promote("key1", TierHot); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	// Should be in hot now.
	v, _, err := hot.Get("key1")
	if err != nil || v != "cold-val" {
		t.Errorf("hot after promote: v=%q err=%v", v, err)
	}

	// Should be removed from cold.
	_, err = cold.Get("key1")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("cold after promote: err = %v, want ErrKeyNotFound", err)
	}
}

func TestTieredMemoryPromoteWarmToHot(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	warm.data["key1"] = "warm-val"

	tm := NewTieredMemory(hot, warm, nil)

	if err := tm.Promote("key1", TierHot); err != nil {
		t.Fatalf("Promote: %v", err)
	}

	v, _, err := hot.Get("key1")
	if err != nil || v != "warm-val" {
		t.Errorf("hot after promote: v=%q err=%v", v, err)
	}

	_, err = warm.Get("key1")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("warm after promote: err = %v, want sql.ErrNoRows", err)
	}
}

func TestTieredMemoryPromoteToSlowerFails(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("key1", "val")

	tm := NewTieredMemory(hot, newMockWarmStore(), nil)

	err := tm.Promote("key1", TierWarm)
	if !errors.Is(err, ErrPromoteToSlower) {
		t.Errorf("err = %v, want ErrPromoteToSlower", err)
	}
}

func TestTieredMemoryPromoteMissing(t *testing.T) {
	hot := NewSharedMemory()
	tm := NewTieredMemory(hot, nil, nil)
	err := tm.Promote("nope", TierHot)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
}

func TestTieredMemoryDemoteHotToWarm(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("key1", "hot-val")
	warm := newMockWarmStore()

	tm := NewTieredMemory(hot, warm, nil)

	if err := tm.Demote("key1", TierWarm); err != nil {
		t.Fatalf("Demote: %v", err)
	}

	// Should be in warm.
	v, err := warm.Get("key1")
	if err != nil || v != "hot-val" {
		t.Errorf("warm after demote: v=%q err=%v", v, err)
	}

	// Should be removed from hot.
	_, _, err = hot.Get("key1")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("hot after demote: err = %v, want ErrKeyNotFound", err)
	}
}

func TestTieredMemoryDemoteHotToCold(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("key1", "hot-val")
	cold := newMockColdStore()

	tm := NewTieredMemory(hot, nil, cold)

	if err := tm.Demote("key1", TierCold); err != nil {
		t.Fatalf("Demote: %v", err)
	}

	v, err := cold.Get("key1")
	if err != nil || v != "hot-val" {
		t.Errorf("cold after demote: v=%q err=%v", v, err)
	}

	_, _, err = hot.Get("key1")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("hot after demote: err = %v, want ErrKeyNotFound", err)
	}
}

func TestTieredMemoryDemoteToFasterFails(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	warm.data["key1"] = "val"

	tm := NewTieredMemory(hot, warm, nil)

	err := tm.Demote("key1", TierHot)
	if !errors.Is(err, ErrDemoteToFaster) {
		t.Errorf("err = %v, want ErrDemoteToFaster", err)
	}
}

func TestTieredMemoryDemoteMissing(t *testing.T) {
	hot := NewSharedMemory()
	tm := NewTieredMemory(hot, nil, nil)
	err := tm.Demote("nope", TierWarm)
	if !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
}

func TestTieredMemoryStats(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("h", "1")
	warm := newMockWarmStore()
	warm.data["w"] = "2"
	cold := newMockColdStore()
	cold.data["c"] = "3"

	tm := NewTieredMemory(hot, warm, cold)

	tm.Get("h")
	tm.Get("h")
	tm.Get("w")
	tm.Get("c")
	tm.Get("missing")

	stats := tm.Stats()
	if stats.HotHits != 2 {
		t.Errorf("hot hits = %d, want 2", stats.HotHits)
	}
	if stats.WarmHits != 1 {
		t.Errorf("warm hits = %d, want 1", stats.WarmHits)
	}
	if stats.ColdHits != 1 {
		t.Errorf("cold hits = %d, want 1", stats.ColdHits)
	}
	if stats.Misses != 1 {
		t.Errorf("misses = %d, want 1", stats.Misses)
	}
}

func TestTieredMemoryConcurrency(t *testing.T) {
	hot := NewSharedMemory()
	warm := newMockWarmStore()
	cold := newMockColdStore()
	tm := NewTieredMemory(hot, warm, cold)

	// Seed some data.
	tm.Put("shared", "initial", TierHot)
	tm.Put("warm-key", "warm-val", TierWarm)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			tm.Get("shared")
		}()
		go func() {
			defer wg.Done()
			tm.Put("shared", "updated", TierHot)
		}()
		go func() {
			defer wg.Done()
			tm.Stats()
		}()
	}
	wg.Wait()
}

func TestMemoryTierString(t *testing.T) {
	cases := []struct {
		tier MemoryTier
		want string
	}{
		{TierHot, "hot"},
		{TierWarm, "warm"},
		{TierCold, "cold"},
		{MemoryTier(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.tier.String(); got != tc.want {
			t.Errorf("MemoryTier(%d).String() = %q, want %q", tc.tier, got, tc.want)
		}
	}
}

func TestTieredMemoryPromoteSameTier(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("key1", "val")
	tm := NewTieredMemory(hot, nil, nil)

	// Promoting from hot to hot should fail (not slower, but equal).
	err := tm.Promote("key1", TierHot)
	if !errors.Is(err, ErrPromoteToSlower) {
		t.Errorf("promote same tier: err = %v, want ErrPromoteToSlower", err)
	}
}

func TestTieredMemoryDemoteSameTier(t *testing.T) {
	hot := NewSharedMemory()
	hot.Set("key1", "val")
	tm := NewTieredMemory(hot, nil, nil)

	err := tm.Demote("key1", TierHot)
	if !errors.Is(err, ErrDemoteToFaster) {
		t.Errorf("demote same tier: err = %v, want ErrDemoteToFaster", err)
	}
}
