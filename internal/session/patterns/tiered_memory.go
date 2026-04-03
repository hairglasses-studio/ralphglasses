package patterns

import (
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
)

// MemoryTier identifies which tier a value lives in.
type MemoryTier int

const (
	TierHot  MemoryTier = iota // in-memory, sub-millisecond
	TierWarm                    // SQLite WAL, millisecond
	TierCold                    // fleet store, 10ms+
)

// String returns a human-readable tier name.
func (t MemoryTier) String() string {
	switch t {
	case TierHot:
		return "hot"
	case TierWarm:
		return "warm"
	case TierCold:
		return "cold"
	default:
		return "unknown"
	}
}

// WarmStore is an interface for the warm tier (SQLite-backed KV).
// *session.SharedState satisfies this interface.
type WarmStore interface {
	Get(key string) (string, error)
	Put(key, value string) error
	Delete(key string) error
}

// ColdStore is an interface for the cold tier (fleet-level storage).
type ColdStore interface {
	Get(key string) (string, error)
	Put(key, value string) error
	Delete(key string) error
}

// Errors for tiered memory operations.
var (
	ErrInvalidTier     = errors.New("patterns: invalid memory tier")
	ErrPromoteToSlower = errors.New("patterns: cannot promote to a slower tier")
	ErrDemoteToFaster  = errors.New("patterns: cannot demote to a faster tier")
)

// TieredMemoryStats tracks hit rates per tier.
type TieredMemoryStats struct {
	HotHits  int64 `json:"hot_hits"`
	WarmHits int64 `json:"warm_hits"`
	ColdHits int64 `json:"cold_hits"`
	Misses   int64 `json:"misses"`
}

// TieredMemory provides a three-tier hot/warm/cold memory hierarchy.
// Based on Codified Context (ArXiv 2602.20478).
//
// Hot tier: in-memory SharedMemory (sub-millisecond).
// Warm tier: SQLite WAL-backed KV (millisecond).
// Cold tier: fleet-level storage via ColdStore interface (10ms+).
type TieredMemory struct {
	hot  *SharedMemory
	warm WarmStore
	cold ColdStore
	mu   sync.RWMutex

	// Stats tracked with atomics for lock-free reads.
	hotHits  atomic.Int64
	warmHits atomic.Int64
	coldHits atomic.Int64
	misses   atomic.Int64
}

// NewTieredMemory creates a tiered memory with the given stores.
// The hot tier is required. Warm and cold tiers may be nil (lookups
// will skip nil tiers).
func NewTieredMemory(hot *SharedMemory, warm WarmStore, cold ColdStore) *TieredMemory {
	return &TieredMemory{
		hot:  hot,
		warm: warm,
		cold: cold,
	}
}

// Get retrieves a value by checking hot, warm, then cold tiers in order.
// Returns the value, the tier it was found in, and any error.
// Returns ErrKeyNotFound if the key is not in any tier.
func (tm *TieredMemory) Get(key string) (string, MemoryTier, error) {
	// Hot tier: in-memory, sub-millisecond.
	v, _, err := tm.hot.Get(key)
	if err == nil {
		tm.hotHits.Add(1)
		return v, TierHot, nil
	}
	if !errors.Is(err, ErrKeyNotFound) {
		return "", TierHot, err
	}

	// Warm tier: SQLite WAL.
	if tm.warm != nil {
		v, err = tm.warm.Get(key)
		if err == nil {
			tm.warmHits.Add(1)
			return v, TierWarm, nil
		}
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrKeyNotFound) {
			return "", TierWarm, err
		}
	}

	// Cold tier: fleet store.
	if tm.cold != nil {
		v, err = tm.cold.Get(key)
		if err == nil {
			tm.coldHits.Add(1)
			return v, TierCold, nil
		}
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrKeyNotFound) {
			return "", TierCold, err
		}
	}

	tm.misses.Add(1)
	return "", TierHot, ErrKeyNotFound
}

// Put writes a value to the specified tier.
func (tm *TieredMemory) Put(key, value string, tier MemoryTier) error {
	switch tier {
	case TierHot:
		tm.hot.Set(key, value)
		return nil
	case TierWarm:
		if tm.warm == nil {
			return ErrInvalidTier
		}
		return tm.warm.Put(key, value)
	case TierCold:
		if tm.cold == nil {
			return ErrInvalidTier
		}
		return tm.cold.Put(key, value)
	default:
		return ErrInvalidTier
	}
}

// Promote moves a key from its current tier to a faster (lower-numbered) tier.
// The key is copied to the target tier and removed from the source tier.
func (tm *TieredMemory) Promote(key string, to MemoryTier) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Find the key in the tiers below 'to'.
	value, currentTier, err := tm.getUnlocked(key)
	if err != nil {
		return err
	}

	if currentTier <= to {
		return ErrPromoteToSlower
	}

	// Write to the target tier.
	if err := tm.putUnlocked(key, value, to); err != nil {
		return err
	}

	// Remove from the source tier.
	return tm.deleteUnlocked(key, currentTier)
}

// Demote moves a key from its current tier to a slower (higher-numbered) tier.
// The key is copied to the target tier and removed from the source tier.
func (tm *TieredMemory) Demote(key string, to MemoryTier) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	value, currentTier, err := tm.getUnlocked(key)
	if err != nil {
		return err
	}

	if currentTier >= to {
		return ErrDemoteToFaster
	}

	// Write to the target tier.
	if err := tm.putUnlocked(key, value, to); err != nil {
		return err
	}

	// Remove from the source tier.
	return tm.deleteUnlocked(key, currentTier)
}

// Stats returns a snapshot of hit rates per tier.
func (tm *TieredMemory) Stats() TieredMemoryStats {
	return TieredMemoryStats{
		HotHits:  tm.hotHits.Load(),
		WarmHits: tm.warmHits.Load(),
		ColdHits: tm.coldHits.Load(),
		Misses:   tm.misses.Load(),
	}
}

// getUnlocked finds a key across all tiers. Caller must hold tm.mu.
func (tm *TieredMemory) getUnlocked(key string) (string, MemoryTier, error) {
	v, _, err := tm.hot.Get(key)
	if err == nil {
		return v, TierHot, nil
	}

	if tm.warm != nil {
		v, err = tm.warm.Get(key)
		if err == nil {
			return v, TierWarm, nil
		}
	}

	if tm.cold != nil {
		v, err = tm.cold.Get(key)
		if err == nil {
			return v, TierCold, nil
		}
	}

	return "", TierHot, ErrKeyNotFound
}

// putUnlocked writes to a specific tier. Caller must hold tm.mu.
func (tm *TieredMemory) putUnlocked(key, value string, tier MemoryTier) error {
	switch tier {
	case TierHot:
		tm.hot.Set(key, value)
		return nil
	case TierWarm:
		if tm.warm == nil {
			return ErrInvalidTier
		}
		return tm.warm.Put(key, value)
	case TierCold:
		if tm.cold == nil {
			return ErrInvalidTier
		}
		return tm.cold.Put(key, value)
	default:
		return ErrInvalidTier
	}
}

// deleteUnlocked removes a key from a specific tier. Caller must hold tm.mu.
func (tm *TieredMemory) deleteUnlocked(key string, tier MemoryTier) error {
	switch tier {
	case TierHot:
		tm.hot.Delete(key)
		return nil
	case TierWarm:
		if tm.warm == nil {
			return ErrInvalidTier
		}
		return tm.warm.Delete(key)
	case TierCold:
		if tm.cold == nil {
			return ErrInvalidTier
		}
		return tm.cold.Delete(key)
	default:
		return ErrInvalidTier
	}
}
