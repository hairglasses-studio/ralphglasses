package graph

import (
	"encoding/json"
	"fmt"
	"sync"
)

// NodeKind classifies nodes in the knowledge graph.
type NodeKind string

const (
	KindPackage   NodeKind = "package"
	KindFile      NodeKind = "file"
	KindFunction  NodeKind = "function"
	KindMethod    NodeKind = "method"
	KindType      NodeKind = "type"
	KindInterface NodeKind = "interface"
	KindField     NodeKind = "field"
	KindVariable  NodeKind = "variable"
	KindConstant  NodeKind = "constant"
)

// EdgeKind classifies relationships between nodes.
type EdgeKind string

const (
	EdgeImports    EdgeKind = "imports"
	EdgeCalls      EdgeKind = "calls"
	EdgeImplements EdgeKind = "implements"
	EdgeEmbeds     EdgeKind = "embeds"
	EdgeDeclaredIn EdgeKind = "declared_in"
	EdgeReturns    EdgeKind = "returns"
	EdgeReceives   EdgeKind = "receives"
	EdgeReferences EdgeKind = "references"
	EdgeDependsOn  EdgeKind = "depends_on"
)

// Node represents an entity in the knowledge graph.
type Node struct {
	ID       string            `json:"id"`
	Kind     NodeKind          `json:"kind"`
	Name     string            `json:"name"`
	Package  string            `json:"package,omitempty"`
	File     string            `json:"file,omitempty"`
	Line     int               `json:"line,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Edge represents a directed relationship between two nodes.
type Edge struct {
	From     string            `json:"from"`
	To       string            `json:"to"`
	Kind     EdgeKind          `json:"kind"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GraphStore is a thread-safe in-memory directed graph with adjacency lists.
type GraphStore struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges map[string][]*Edge // adjacency list keyed by from-node ID
	rev   map[string][]*Edge // reverse adjacency keyed by to-node ID
}

// NewGraphStore creates an empty graph store.
func NewGraphStore() *GraphStore {
	return &GraphStore{
		nodes: make(map[string]*Node),
		edges: make(map[string][]*Edge),
		rev:   make(map[string][]*Edge),
	}
}

// AddNode inserts or replaces a node in the graph.
func (g *GraphStore) AddNode(n *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[n.ID] = n
}

// RemoveNode deletes a node and all its incident edges.
func (g *GraphStore) RemoveNode(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.nodes, id)

	// Remove outgoing edges.
	for _, e := range g.edges[id] {
		g.removeRevEdge(e)
	}
	delete(g.edges, id)

	// Remove incoming edges.
	for _, e := range g.rev[id] {
		g.removeFwdEdge(e)
	}
	delete(g.rev, id)
}

func (g *GraphStore) removeFwdEdge(target *Edge) {
	out := g.edges[target.From]
	for i, e := range out {
		if e == target {
			g.edges[target.From] = append(out[:i], out[i+1:]...)
			return
		}
	}
}

func (g *GraphStore) removeRevEdge(target *Edge) {
	in := g.rev[target.To]
	for i, e := range in {
		if e == target {
			g.rev[target.To] = append(in[:i], in[i+1:]...)
			return
		}
	}
}

// GetNode returns a node by ID, or nil if not found.
func (g *GraphStore) GetNode(id string) *Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.nodes[id]
}

// AddEdge adds a directed edge between two existing nodes.
// Returns an error if either endpoint is missing.
func (g *GraphStore) AddEdge(e *Edge) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, ok := g.nodes[e.From]; !ok {
		return fmt.Errorf("graph: source node %q not found", e.From)
	}
	if _, ok := g.nodes[e.To]; !ok {
		return fmt.Errorf("graph: target node %q not found", e.To)
	}

	g.edges[e.From] = append(g.edges[e.From], e)
	g.rev[e.To] = append(g.rev[e.To], e)
	return nil
}

// RemoveEdge removes a specific edge instance from the graph.
func (g *GraphStore) RemoveEdge(e *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.removeFwdEdge(e)
	g.removeRevEdge(e)
}

// OutEdges returns all edges originating from the given node.
func (g *GraphStore) OutEdges(id string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	cp := make([]*Edge, len(g.edges[id]))
	copy(cp, g.edges[id])
	return cp
}

// InEdges returns all edges pointing to the given node.
func (g *GraphStore) InEdges(id string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	cp := make([]*Edge, len(g.rev[id]))
	copy(cp, g.rev[id])
	return cp
}

// Nodes returns all nodes in the graph.
func (g *GraphStore) Nodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	return out
}

// Edges returns all edges in the graph.
func (g *GraphStore) Edges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []*Edge
	for _, list := range g.edges {
		out = append(out, list...)
	}
	return out
}

// NodesByKind returns all nodes of the given kind.
func (g *GraphStore) NodesByKind(kind NodeKind) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []*Node
	for _, n := range g.nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

// EdgesByKind returns all edges of the given kind.
func (g *GraphStore) EdgesByKind(kind EdgeKind) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []*Edge
	for _, list := range g.edges {
		for _, e := range list {
			if e.Kind == kind {
				out = append(out, e)
			}
		}
	}
	return out
}

// serialization envelope for JSON round-trip.
type graphJSON struct {
	Nodes []*Node `json:"nodes"`
	Edges []*Edge `json:"edges"`
}

// MarshalJSON serializes the graph to JSON.
func (g *GraphStore) MarshalJSON() ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	env := graphJSON{
		Nodes: make([]*Node, 0, len(g.nodes)),
	}
	for _, n := range g.nodes {
		env.Nodes = append(env.Nodes, n)
	}
	for _, list := range g.edges {
		env.Edges = append(env.Edges, list...)
	}
	return json.Marshal(env)
}

// UnmarshalJSON deserializes JSON into the graph, replacing all contents.
func (g *GraphStore) UnmarshalJSON(data []byte) error {
	var env graphJSON
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.nodes = make(map[string]*Node, len(env.Nodes))
	g.edges = make(map[string][]*Edge)
	g.rev = make(map[string][]*Edge)

	for _, n := range env.Nodes {
		g.nodes[n.ID] = n
	}
	for _, e := range env.Edges {
		g.edges[e.From] = append(g.edges[e.From], e)
		g.rev[e.To] = append(g.rev[e.To], e)
	}
	return nil
}
