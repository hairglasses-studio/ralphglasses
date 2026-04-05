// Package knowledge provides an in-memory directed graph of code entities
// (packages, types, functions, files) for codebase-aware prompt enhancement.
package knowledge

import (
	"maps"
	"sort"
	"strings"
	"sync"
)

// NodeKind identifies the type of code entity.
type NodeKind string

const (
	KindPackage  NodeKind = "package"
	KindType     NodeKind = "type"
	KindFunction NodeKind = "function"
	KindFile     NodeKind = "file"
)

// EdgeType identifies the relationship between two nodes.
type EdgeType string

const (
	EdgeImports    EdgeType = "imports"
	EdgeDefines    EdgeType = "defines"
	EdgeCalls      EdgeType = "calls"
	EdgeImplements EdgeType = "implements"
)

// Node represents a code entity in the knowledge graph.
type Node struct {
	ID       string            `json:"id"`
	Kind     NodeKind          `json:"kind"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Edge represents a directed relationship between two nodes.
type Edge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	EdgeType EdgeType `json:"edge_type"`
}

// Graph is a thread-safe in-memory directed graph of code entities.
type Graph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges []Edge
	// adjacency lists for fast lookup
	outEdges map[string][]Edge // from -> edges
	inEdges  map[string][]Edge // to -> edges
}

// NewGraph creates an empty knowledge graph.
func NewGraph() *Graph {
	return &Graph{
		nodes:    make(map[string]*Node),
		outEdges: make(map[string][]Edge),
		inEdges:  make(map[string][]Edge),
	}
}

// AddNode adds a node to the graph. If a node with the same ID already exists,
// its metadata is merged (new values overwrite old).
func (g *Graph) AddNode(id string, kind NodeKind, metadata map[string]string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if existing, ok := g.nodes[id]; ok {
		maps.Copy(existing.Metadata, metadata)
		return
	}

	md := make(map[string]string, len(metadata))
	maps.Copy(md, metadata)
	g.nodes[id] = &Node{
		ID:       id,
		Kind:     kind,
		Metadata: md,
	}
}

// AddEdge adds a directed edge. Duplicate edges are allowed.
func (g *Graph) AddEdge(from, to string, edgeType EdgeType) {
	g.mu.Lock()
	defer g.mu.Unlock()

	e := Edge{From: from, To: to, EdgeType: edgeType}
	g.edges = append(g.edges, e)
	g.outEdges[from] = append(g.outEdges[from], e)
	g.inEdges[to] = append(g.inEdges[to], e)
}

// NodeCount returns the number of nodes in the graph.
func (g *Graph) NodeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgeCount returns the number of edges in the graph.
func (g *Graph) EdgeCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.edges)
}

// GetNode returns the node with the given ID, or nil if not found.
func (g *Graph) GetNode(id string) *Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n := g.nodes[id]
	if n == nil {
		return nil
	}
	// Return a copy to avoid data races
	cp := *n
	cp.Metadata = make(map[string]string, len(n.Metadata))
	maps.Copy(cp.Metadata, n.Metadata)
	return &cp
}

// Query returns the neighbors (outgoing and incoming) of a node,
// along with the node's own metadata.
type QueryResult struct {
	Node      *Node  `json:"node"`
	Outgoing  []Edge `json:"outgoing"`
	Incoming  []Edge `json:"incoming"`
	Neighbors []Node `json:"neighbors"`
}

// Query retrieves a node and its immediate neighbors.
func (g *Graph) Query(nodeID string) *QueryResult {
	g.mu.RLock()
	defer g.mu.RUnlock()

	n, ok := g.nodes[nodeID]
	if !ok {
		return nil
	}

	result := &QueryResult{
		Node:     copyNode(n),
		Outgoing: append([]Edge(nil), g.outEdges[nodeID]...),
		Incoming: append([]Edge(nil), g.inEdges[nodeID]...),
	}

	// Collect unique neighbor nodes
	seen := make(map[string]bool)
	seen[nodeID] = true
	for _, e := range result.Outgoing {
		if !seen[e.To] {
			seen[e.To] = true
			if neighbor, ok := g.nodes[e.To]; ok {
				result.Neighbors = append(result.Neighbors, *copyNode(neighbor))
			}
		}
	}
	for _, e := range result.Incoming {
		if !seen[e.From] {
			seen[e.From] = true
			if neighbor, ok := g.nodes[e.From]; ok {
				result.Neighbors = append(result.Neighbors, *copyNode(neighbor))
			}
		}
	}

	return result
}

// Subgraph returns a new graph containing only the specified nodes
// and edges between them.
func (g *Graph) Subgraph(nodeIDs []string) *Graph {
	g.mu.RLock()
	defer g.mu.RUnlock()

	sub := NewGraph()
	keep := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		keep[id] = true
	}

	for id := range keep {
		if n, ok := g.nodes[id]; ok {
			sub.nodes[id] = copyNode(n)
		}
	}

	for _, e := range g.edges {
		if keep[e.From] && keep[e.To] {
			sub.edges = append(sub.edges, e)
			sub.outEdges[e.From] = append(sub.outEdges[e.From], e)
			sub.inEdges[e.To] = append(sub.inEdges[e.To], e)
		}
	}

	return sub
}

// RelatedContext finds nodes relevant to the given query string.
// It performs keyword matching against node IDs and metadata,
// then expands to immediate neighbors. Returns up to maxNodes results,
// sorted by relevance score (higher = more relevant).
func (g *Graph) RelatedContext(query string, maxNodes int) []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if maxNodes <= 0 {
		maxNodes = 10
	}

	queryLower := strings.ToLower(query)
	keywords := strings.Fields(queryLower)
	if len(keywords) == 0 {
		return nil
	}

	type scored struct {
		node  Node
		score int
	}

	scores := make(map[string]int)

	// Score each node based on keyword matches
	for id, n := range g.nodes {
		idLower := strings.ToLower(id)
		s := 0
		for _, kw := range keywords {
			if strings.Contains(idLower, kw) {
				s += 3
			}
			for _, v := range n.Metadata {
				if strings.Contains(strings.ToLower(v), kw) {
					s += 1
				}
			}
		}
		if s > 0 {
			scores[id] = s
		}
	}

	// Expand to neighbors of matched nodes (with reduced score)
	expanded := make(map[string]int)
	for id, s := range scores {
		expanded[id] = s
		for _, e := range g.outEdges[id] {
			if _, already := expanded[e.To]; !already {
				expanded[e.To] = s / 2
			}
		}
		for _, e := range g.inEdges[id] {
			if _, already := expanded[e.From]; !already {
				expanded[e.From] = s / 2
			}
		}
	}

	// Merge expanded back, keeping the higher score
	for id, s := range expanded {
		if s > scores[id] {
			scores[id] = s
		} else if _, ok := scores[id]; !ok {
			scores[id] = s
		}
	}

	// Sort by score descending
	var results []scored
	for id, s := range scores {
		if s <= 0 {
			continue
		}
		if n, ok := g.nodes[id]; ok {
			results = append(results, scored{node: *copyNode(n), score: s})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].node.ID < results[j].node.ID
	})

	if len(results) > maxNodes {
		results = results[:maxNodes]
	}

	nodes := make([]Node, len(results))
	for i, r := range results {
		nodes[i] = r.node
	}
	return nodes
}

// AllNodes returns a copy of all nodes in the graph.
func (g *Graph) AllNodes() []Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, *copyNode(n))
	}
	return nodes
}

func copyNode(n *Node) *Node {
	cp := *n
	cp.Metadata = make(map[string]string, len(n.Metadata))
	maps.Copy(cp.Metadata, n.Metadata)
	return &cp
}
