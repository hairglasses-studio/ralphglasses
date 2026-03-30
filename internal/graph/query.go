package graph

// QueryEngine provides graph traversal and analysis operations.
type QueryEngine struct {
	store *GraphStore
}

// NewQueryEngine creates a query engine backed by the given store.
func NewQueryEngine(store *GraphStore) *QueryEngine {
	return &QueryEngine{store: store}
}

// Dependencies returns all nodes that the given node depends on (outgoing edges).
// If kinds is non-empty, only edges of those kinds are followed.
func (q *QueryEngine) Dependencies(nodeID string, kinds ...EdgeKind) []*Node {
	filter := makeKindSet(kinds)
	var out []*Node
	seen := map[string]bool{}

	for _, e := range q.store.OutEdges(nodeID) {
		if len(filter) > 0 && !filter[e.Kind] {
			continue
		}
		if !seen[e.To] {
			seen[e.To] = true
			if n := q.store.GetNode(e.To); n != nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// Dependents returns all nodes that depend on the given node (incoming edges).
// If kinds is non-empty, only edges of those kinds are followed.
func (q *QueryEngine) Dependents(nodeID string, kinds ...EdgeKind) []*Node {
	filter := makeKindSet(kinds)
	var out []*Node
	seen := map[string]bool{}

	for _, e := range q.store.InEdges(nodeID) {
		if len(filter) > 0 && !filter[e.Kind] {
			continue
		}
		if !seen[e.From] {
			seen[e.From] = true
			if n := q.store.GetNode(e.From); n != nil {
				out = append(out, n)
			}
		}
	}
	return out
}

// TransitiveDependencies returns all nodes reachable from nodeID via outgoing edges.
func (q *QueryEngine) TransitiveDependencies(nodeID string, kinds ...EdgeKind) []*Node {
	filter := makeKindSet(kinds)
	visited := map[string]bool{nodeID: true}
	var result []*Node
	queue := []string{nodeID}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range q.store.OutEdges(cur) {
			if len(filter) > 0 && !filter[e.Kind] {
				continue
			}
			if !visited[e.To] {
				visited[e.To] = true
				if n := q.store.GetNode(e.To); n != nil {
					result = append(result, n)
				}
				queue = append(queue, e.To)
			}
		}
	}
	return result
}

// ShortestPath returns the shortest path (sequence of node IDs) from src to dst
// using BFS. Returns nil if no path exists. Edge kind filtering is optional.
func (q *QueryEngine) ShortestPath(src, dst string, kinds ...EdgeKind) []string {
	if src == dst {
		return []string{src}
	}

	filter := makeKindSet(kinds)
	type entry struct {
		id   string
		path []string
	}

	visited := map[string]bool{src: true}
	queue := []entry{{id: src, path: []string{src}}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, e := range q.store.OutEdges(cur.id) {
			if len(filter) > 0 && !filter[e.Kind] {
				continue
			}
			if visited[e.To] {
				continue
			}
			newPath := make([]string, len(cur.path)+1)
			copy(newPath, cur.path)
			newPath[len(cur.path)] = e.To

			if e.To == dst {
				return newPath
			}
			visited[e.To] = true
			queue = append(queue, entry{id: e.To, path: newPath})
		}
	}
	return nil
}

// DetectCycles returns all cycles found in the graph using DFS.
// Each cycle is represented as a slice of node IDs forming the cycle.
// If kinds is non-empty, only edges of those kinds are followed.
func (q *QueryEngine) DetectCycles(kinds ...EdgeKind) [][]string {
	filter := makeKindSet(kinds)

	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully processed
	)

	color := map[string]int{}
	parent := map[string]string{}
	var cycles [][]string

	// Reconstruct cycle from the back-edge target up to the current node.
	extractCycle := func(from, to string) []string {
		cycle := []string{to}
		cur := from
		for cur != to {
			cycle = append([]string{cur}, cycle...)
			cur = parent[cur]
		}
		cycle = append([]string{to}, cycle...) // close the cycle
		return cycle
	}

	var dfs func(nodeID string)
	dfs = func(nodeID string) {
		color[nodeID] = gray
		for _, e := range q.store.OutEdges(nodeID) {
			if len(filter) > 0 && !filter[e.Kind] {
				continue
			}
			switch color[e.To] {
			case white:
				parent[e.To] = nodeID
				dfs(e.To)
			case gray:
				// Back edge — cycle detected.
				cycles = append(cycles, extractCycle(nodeID, e.To))
			}
		}
		color[nodeID] = black
	}

	q.store.mu.RLock()
	nodeIDs := make([]string, 0, len(q.store.nodes))
	for id := range q.store.nodes {
		nodeIDs = append(nodeIDs, id)
	}
	q.store.mu.RUnlock()

	for _, id := range nodeIDs {
		if color[id] == white {
			dfs(id)
		}
	}
	return cycles
}

// Subgraph extracts a new GraphStore containing only the specified nodes
// and all edges between them.
func (q *QueryEngine) Subgraph(nodeIDs []string) *GraphStore {
	sub := NewGraphStore()
	idSet := make(map[string]bool, len(nodeIDs))

	for _, id := range nodeIDs {
		idSet[id] = true
		if n := q.store.GetNode(id); n != nil {
			sub.AddNode(n)
		}
	}

	for _, id := range nodeIDs {
		for _, e := range q.store.OutEdges(id) {
			if idSet[e.To] {
				_ = sub.AddEdge(e)
			}
		}
	}
	return sub
}

// ReachableFrom returns all node IDs reachable from the given start node.
func (q *QueryEngine) ReachableFrom(nodeID string, kinds ...EdgeKind) []string {
	filter := makeKindSet(kinds)
	visited := map[string]bool{nodeID: true}
	queue := []string{nodeID}
	var result []string

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range q.store.OutEdges(cur) {
			if len(filter) > 0 && !filter[e.Kind] {
				continue
			}
			if !visited[e.To] {
				visited[e.To] = true
				result = append(result, e.To)
				queue = append(queue, e.To)
			}
		}
	}
	return result
}

func makeKindSet(kinds []EdgeKind) map[EdgeKind]bool {
	if len(kinds) == 0 {
		return nil
	}
	m := make(map[EdgeKind]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}
