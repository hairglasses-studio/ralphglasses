package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ToolResult captures the output of a tool invocation for deduplication.
type ToolResult struct {
	ToolName  string         `json:"tool_name"`
	Args      map[string]any `json:"args"`
	Output    string         `json:"output"`
	Timestamp time.Time      `json:"timestamp"`
	argsHash  string         // precomputed for fast comparison
}

// ToolDeduplicator detects repeated tool calls with identical arguments
// and returns cached results instead of re-executing. Each tool name has
// a configurable TTL; calls within the TTL window with matching args are
// considered duplicates.
type ToolDeduplicator struct {
	mu         sync.Mutex
	results    []ToolResult
	maxHistory int                    // max cached results to retain
	ttls       map[string]time.Duration // tool name -> TTL override
	defaultTTL time.Duration
}

// NewToolDeduplicator creates a deduplicator with sensible defaults.
// Default TTL is 5 minutes; status-check tools get 30 seconds.
func NewToolDeduplicator() *ToolDeduplicator {
	return &ToolDeduplicator{
		maxHistory: 200,
		defaultTTL: 5 * time.Minute,
		ttls: map[string]time.Duration{
			"git_status":      30 * time.Second,
			"session_status":  30 * time.Second,
			"loop_status":     30 * time.Second,
			"fleet_status":    30 * time.Second,
			"process_status":  30 * time.Second,
			"health_check":    30 * time.Second,
		},
	}
}

// SetTTL configures the cache TTL for a specific tool name.
func (d *ToolDeduplicator) SetTTL(toolName string, ttl time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ttls == nil {
		d.ttls = make(map[string]time.Duration)
	}
	d.ttls[toolName] = ttl
}

// IsDuplicate checks whether a tool call with the given name and args
// matches a recent cached result within the TTL window. If it is a
// duplicate, the cached output is also returned.
func (d *ToolDeduplicator) IsDuplicate(toolName string, args map[string]any, prevResults []ToolResult) (bool, string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	hash := hashArgs(toolName, args)
	ttl := d.ttlFor(toolName)
	cutoff := time.Now().Add(-ttl)

	// Check internal cache first.
	for i := len(d.results) - 1; i >= 0; i-- {
		r := d.results[i]
		if r.Timestamp.Before(cutoff) {
			continue
		}
		if r.ToolName == toolName && r.argsHash == hash {
			return true, r.Output
		}
	}

	// Check caller-provided previous results.
	for i := len(prevResults) - 1; i >= 0; i-- {
		r := prevResults[i]
		if r.Timestamp.Before(cutoff) {
			continue
		}
		if r.ToolName == toolName && hashArgs(r.ToolName, r.Args) == hash {
			return true, r.Output
		}
	}

	return false, ""
}

// Record stores a tool result for future dedup checks. Old entries beyond
// maxHistory are evicted.
func (d *ToolDeduplicator) Record(toolName string, args map[string]any, output string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	r := ToolResult{
		ToolName:  toolName,
		Args:      args,
		Output:    output,
		Timestamp: time.Now(),
		argsHash:  hashArgs(toolName, args),
	}
	d.results = append(d.results, r)

	// Evict oldest if over capacity.
	if len(d.results) > d.maxHistory {
		d.results = d.results[len(d.results)-d.maxHistory:]
	}
}

// Evict removes all cached results for a specific tool, forcing the next
// call to execute fresh. Useful after mutations (e.g., git commit).
func (d *ToolDeduplicator) Evict(toolName string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	filtered := d.results[:0]
	for _, r := range d.results {
		if r.ToolName != toolName {
			filtered = append(filtered, r)
		}
	}
	d.results = filtered
}

// EvictExpired removes all results older than their tool's TTL.
func (d *ToolDeduplicator) EvictExpired() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	filtered := d.results[:0]
	evicted := 0
	for _, r := range d.results {
		ttl := d.ttlFor(r.ToolName)
		if now.Sub(r.Timestamp) > ttl {
			evicted++
			continue
		}
		filtered = append(filtered, r)
	}
	d.results = filtered
	return evicted
}

// Stats returns the current cache size and per-tool counts.
func (d *ToolDeduplicator) Stats() map[string]int {
	d.mu.Lock()
	defer d.mu.Unlock()

	counts := make(map[string]int)
	for _, r := range d.results {
		counts[r.ToolName]++
	}
	counts["_total"] = len(d.results)
	return counts
}

// ttlFor returns the TTL for a tool, falling back to the default.
func (d *ToolDeduplicator) ttlFor(toolName string) time.Duration {
	if ttl, ok := d.ttls[toolName]; ok {
		return ttl
	}
	return d.defaultTTL
}

// hashArgs produces a deterministic hash of tool name + args for fast
// equality checks. Args are JSON-serialized with sorted keys (Go's
// encoding/json sorts map keys by default).
func hashArgs(toolName string, args map[string]any) string {
	h := sha256.New()
	h.Write([]byte(toolName))
	h.Write([]byte{0}) // separator
	if len(args) > 0 {
		data, err := json.Marshal(args)
		if err == nil {
			h.Write(data)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
