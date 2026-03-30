package fleet

import (
	"errors"
	"sort"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ErrNoMatch is returned when no nodes satisfy the required capabilities.
var ErrNoMatch = errors.New("no nodes match required capabilities")

// NodeCapabilities describes the hardware and software resources available on a
// fleet node. It is reported during registration and updated via heartbeats.
type NodeCapabilities struct {
	NodeID            string             `json:"node_id"`
	Providers         []session.Provider `json:"providers"`
	GPUCount          int                `json:"gpu_count"`
	MemoryGB          float64            `json:"memory_gb"`
	MaxConcurrent     int                `json:"max_concurrent"`
	SupportedLanguages []string          `json:"supported_languages"`
}

// HasProvider returns true if the node supports the given provider.
func (nc *NodeCapabilities) HasProvider(p session.Provider) bool {
	for _, prov := range nc.Providers {
		if prov == p {
			return true
		}
	}
	return false
}

// HasLanguage returns true if the node supports the given language (case-sensitive).
func (nc *NodeCapabilities) HasLanguage(lang string) bool {
	for _, l := range nc.SupportedLanguages {
		if l == lang {
			return true
		}
	}
	return false
}

// TaskRequirements specifies the minimum capabilities a node must have to run a
// particular task. Zero-value fields are treated as "no requirement".
type TaskRequirements struct {
	Providers      []session.Provider `json:"providers,omitempty"`       // node must support at least one
	MinGPUs        int                `json:"min_gpus,omitempty"`        // minimum GPU count
	MinMemoryGB    float64            `json:"min_memory_gb,omitempty"`   // minimum memory in GB
	MinConcurrent  int                `json:"min_concurrent,omitempty"`  // minimum concurrent session slots
	Languages      []string           `json:"languages,omitempty"`       // node must support all listed languages
}

// Satisfies returns true if the node capabilities meet all task requirements.
func (nc *NodeCapabilities) Satisfies(req TaskRequirements) bool {
	// Provider check: node must support at least one required provider.
	if len(req.Providers) > 0 {
		found := false
		for _, rp := range req.Providers {
			if nc.HasProvider(rp) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// GPU check.
	if req.MinGPUs > 0 && nc.GPUCount < req.MinGPUs {
		return false
	}

	// Memory check.
	if req.MinMemoryGB > 0 && nc.MemoryGB < req.MinMemoryGB {
		return false
	}

	// Concurrent session slots check.
	if req.MinConcurrent > 0 && nc.MaxConcurrent < req.MinConcurrent {
		return false
	}

	// Language check: all required languages must be supported.
	for _, lang := range req.Languages {
		if !nc.HasLanguage(lang) {
			return false
		}
	}

	return true
}

// CapabilityMatcher maintains a registry of node capabilities and filters nodes
// that satisfy task requirements. It is safe for concurrent use.
type CapabilityMatcher struct {
	mu    sync.RWMutex
	nodes map[string]*NodeCapabilities // node ID -> capabilities
}

// NewCapabilityMatcher creates an empty matcher.
func NewCapabilityMatcher() *CapabilityMatcher {
	return &CapabilityMatcher{
		nodes: make(map[string]*NodeCapabilities),
	}
}

// Register adds or updates capabilities for a node.
func (cm *CapabilityMatcher) Register(caps NodeCapabilities) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c := caps // copy
	cm.nodes[caps.NodeID] = &c
}

// Remove deletes a node from the registry.
func (cm *CapabilityMatcher) Remove(nodeID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.nodes, nodeID)
}

// Get returns the capabilities for a single node, or false if not registered.
func (cm *CapabilityMatcher) Get(nodeID string) (NodeCapabilities, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	c, ok := cm.nodes[nodeID]
	if !ok {
		return NodeCapabilities{}, false
	}
	return *c, true
}

// All returns capabilities for every registered node, sorted by node ID.
func (cm *CapabilityMatcher) All() []NodeCapabilities {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	result := make([]NodeCapabilities, 0, len(cm.nodes))
	for _, c := range cm.nodes {
		result = append(result, *c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].NodeID < result[j].NodeID
	})
	return result
}

// Match returns all registered nodes that satisfy the given requirements,
// sorted by node ID for deterministic output. Returns ErrNoMatch if none qualify.
func (cm *CapabilityMatcher) Match(req TaskRequirements) ([]NodeCapabilities, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var matched []NodeCapabilities
	for _, c := range cm.nodes {
		if c.Satisfies(req) {
			matched = append(matched, *c)
		}
	}
	if len(matched) == 0 {
		return nil, ErrNoMatch
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].NodeID < matched[j].NodeID
	})
	return matched, nil
}

// MatchIDs is a convenience wrapper that returns only node IDs of matching nodes.
func (cm *CapabilityMatcher) MatchIDs(req TaskRequirements) ([]string, error) {
	nodes, err := cm.Match(req)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.NodeID
	}
	return ids, nil
}

// RankByCapacity returns matching nodes sorted by available capacity
// (MaxConcurrent descending, then GPUCount descending, then MemoryGB descending).
// This is useful for assigning heavy workloads to the most capable nodes first.
func (cm *CapabilityMatcher) RankByCapacity(req TaskRequirements) ([]NodeCapabilities, error) {
	nodes, err := cm.Match(req)
	if err != nil {
		return nil, err
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].MaxConcurrent != nodes[j].MaxConcurrent {
			return nodes[i].MaxConcurrent > nodes[j].MaxConcurrent
		}
		if nodes[i].GPUCount != nodes[j].GPUCount {
			return nodes[i].GPUCount > nodes[j].GPUCount
		}
		return nodes[i].MemoryGB > nodes[j].MemoryGB
	})
	return nodes, nil
}
