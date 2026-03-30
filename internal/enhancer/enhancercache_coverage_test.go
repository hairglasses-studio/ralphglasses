package enhancer

import (
	"testing"
	"time"
)

func TestNewEnhancerCache_Defaults(t *testing.T) {
	c := NewEnhancerCache(0, 0) // zero values → defaults
	stats := c.Stats()
	if stats.MaxSize <= 0 {
		t.Errorf("MaxSize = %d, want > 0 (default)", stats.MaxSize)
	}
	if stats.Size != 0 {
		t.Errorf("Size = %d, want 0 (empty)", stats.Size)
	}
}

func TestEnhancerCache_MissOnEmpty(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	result, ok := c.Get("test prompt", "claude")
	if ok || result != nil {
		t.Error("expected cache miss on empty cache")
	}
}

func TestEnhancerCache_PutAndGet(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	expected := &EnhanceResult{Enhanced: "improved prompt"}

	c.Put("hello world", "claude", expected)
	result, ok := c.Get("hello world", "claude")

	if !ok {
		t.Fatal("expected cache hit")
	}
	if result == nil || result.Enhanced != "improved prompt" {
		t.Errorf("Get returned %+v, want Enhanced=%q", result, "improved prompt")
	}
}

func TestEnhancerCache_NormalizePrompt(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	expected := &EnhanceResult{Enhanced: "normalized"}

	// Put with extra whitespace.
	c.Put("  hello   world  ", "claude", expected)

	// Get with normalized whitespace should hit.
	result, ok := c.Get("hello world", "claude")
	if !ok || result == nil {
		t.Error("expected cache hit after prompt normalization")
	}
}

func TestEnhancerCache_ProviderSeparation(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	claudeResult := &EnhanceResult{Enhanced: "claude version"}
	geminiResult := &EnhanceResult{Enhanced: "gemini version"}

	c.Put("write tests", string(ProviderClaude), claudeResult)
	c.Put("write tests", string(ProviderGemini), geminiResult)

	r1, ok1 := c.Get("write tests", string(ProviderClaude))
	r2, ok2 := c.Get("write tests", string(ProviderGemini))

	if !ok1 || r1 == nil || r1.Enhanced != "claude version" {
		t.Errorf("claude cache miss or wrong value: ok=%v, result=%+v", ok1, r1)
	}
	if !ok2 || r2 == nil || r2.Enhanced != "gemini version" {
		t.Errorf("gemini cache miss or wrong value: ok=%v, result=%+v", ok2, r2)
	}
}

func TestEnhancerCache_UpdateExisting(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	v1 := &EnhanceResult{Enhanced: "version 1"}
	v2 := &EnhanceResult{Enhanced: "version 2"}

	c.Put("prompt", "claude", v1)
	c.Put("prompt", "claude", v2)

	result, ok := c.Get("prompt", "claude")
	if !ok || result == nil || result.Enhanced != "version 2" {
		t.Errorf("expected v2 after update, got %+v", result)
	}
	// Size should still be 1 (updated, not duplicated).
	if stats := c.Stats(); stats.Size != 1 {
		t.Errorf("Size after update = %d, want 1", stats.Size)
	}
}

func TestEnhancerCache_LRUEviction(t *testing.T) {
	c := NewEnhancerCache(3, 10*time.Minute) // capacity 3

	c.Put("p1", "claude", &EnhanceResult{Enhanced: "r1"})
	c.Put("p2", "claude", &EnhanceResult{Enhanced: "r2"})
	c.Put("p3", "claude", &EnhanceResult{Enhanced: "r3"})

	// Access p1 to make it most recently used.
	c.Get("p1", "claude")

	// Add p4, which should evict the LRU (p2 or p3 — whichever wasn't recently accessed).
	c.Put("p4", "claude", &EnhanceResult{Enhanced: "r4"})

	// p1 and p4 should be in cache.
	_, ok1 := c.Get("p1", "claude")
	_, ok4 := c.Get("p4", "claude")
	if !ok1 {
		t.Error("p1 should be in cache (recently accessed)")
	}
	if !ok4 {
		t.Error("p4 should be in cache (just added)")
	}

	stats := c.Stats()
	if stats.Evictions == 0 {
		t.Error("expected at least one eviction")
	}
}

func TestEnhancerCache_TTLExpiration(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Millisecond)
	c.Put("expiring", "claude", &EnhanceResult{Enhanced: "soon gone"})

	// Should hit before TTL.
	_, ok := c.Get("expiring", "claude")
	if !ok {
		t.Error("expected hit before TTL")
	}

	time.Sleep(20 * time.Millisecond)

	// Should miss after TTL.
	_, ok = c.Get("expiring", "claude")
	if ok {
		t.Error("expected miss after TTL expiration")
	}
}

func TestEnhancerCache_Invalidate(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	c.Put("my prompt", string(ProviderClaude), &EnhanceResult{Enhanced: "claude"})
	c.Put("my prompt", string(ProviderGemini), &EnhanceResult{Enhanced: "gemini"})
	c.Put("other prompt", string(ProviderClaude), &EnhanceResult{Enhanced: "other"})

	c.Invalidate("my prompt")

	_, ok1 := c.Get("my prompt", string(ProviderClaude))
	_, ok2 := c.Get("my prompt", string(ProviderGemini))
	_, ok3 := c.Get("other prompt", string(ProviderClaude))

	if ok1 {
		t.Error("claude entry should be invalidated")
	}
	if ok2 {
		t.Error("gemini entry should be invalidated")
	}
	if !ok3 {
		t.Error("other prompt should still be cached")
	}
}

func TestEnhancerCache_Clear(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	c.Put("p1", "claude", &EnhanceResult{Enhanced: "r1"})
	c.Put("p2", "gemini", &EnhanceResult{Enhanced: "r2"})

	// Generate some hits/misses for stats.
	c.Get("p1", "claude")
	c.Get("missing", "claude")

	c.Clear()

	stats := c.Stats()
	if stats.Size != 0 {
		t.Errorf("after Clear, Size = %d, want 0", stats.Size)
	}
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Errorf("after Clear, Hits=%d Misses=%d, want both 0", stats.Hits, stats.Misses)
	}

	_, ok := c.Get("p1", "claude")
	if ok {
		t.Error("expected miss after Clear")
	}
}

func TestEnhancerCache_Stats_HitRate(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	c.Put("p", "claude", &EnhanceResult{Enhanced: "r"})

	// 2 hits, 1 miss.
	c.Get("p", "claude")
	c.Get("p", "claude")
	c.Get("miss", "claude")

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Errorf("Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
	expected := 2.0 / 3.0
	if stats.HitRate < expected-0.01 || stats.HitRate > expected+0.01 {
		t.Errorf("HitRate = %f, want ~%f", stats.HitRate, expected)
	}
}

func TestEnhancerCache_Stats_EmptyHitRate(t *testing.T) {
	c := NewEnhancerCache(100, 10*time.Minute)
	stats := c.Stats()
	if stats.HitRate != 0 {
		t.Errorf("HitRate on empty cache = %f, want 0", stats.HitRate)
	}
}

// TestNormalizePrompt tests the normalizePrompt helper.
func TestNormalizePrompt(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello world"},
		{"  hello   world  ", "hello world"},
		{"", ""},
		{"  ", ""},
		{"a\tb\nc", "a b c"},
	}
	for _, tt := range tests {
		got := normalizePrompt(tt.input)
		if got != tt.want {
			t.Errorf("normalizePrompt(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestEnhancerCacheKey verifies that same inputs produce same keys and
// different inputs produce different keys.
func TestEnhancerCacheKey(t *testing.T) {
	k1 := enhancerCacheKey("hello world", "claude")
	k2 := enhancerCacheKey("hello world", "claude")
	k3 := enhancerCacheKey("hello world", "gemini")
	k4 := enhancerCacheKey("different", "claude")

	if k1 != k2 {
		t.Error("same inputs should produce same key")
	}
	if k1 == k3 {
		t.Error("different provider should produce different key")
	}
	if k1 == k4 {
		t.Error("different prompt should produce different key")
	}
}

// TestEnhancerCacheKeyRaw verifies the raw key function with empty provider.
func TestEnhancerCacheKeyRaw(t *testing.T) {
	k1 := enhancerCacheKeyRaw("prompt", "")
	k2 := enhancerCacheKeyRaw("prompt", "claude")
	if k1 == k2 {
		t.Error("empty provider and 'claude' should produce different keys")
	}

	// Same inputs produce same output.
	if enhancerCacheKeyRaw("test", "gemini") != enhancerCacheKeyRaw("test", "gemini") {
		t.Error("same inputs should produce same key")
	}
}
