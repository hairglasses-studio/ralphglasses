package roadmap

import (
	"fmt"
	"sort"
	"strings"
)

// Node represents a single item in the dependency graph.
type Node struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Phase       string   `json:"phase"`
	Section     string   `json:"section,omitempty"`
	Done        bool     `json:"done"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Dependents  []string `json:"dependents,omitempty"`
	Depth       int      `json:"depth"`       // longest path from a root
	FanOut      int      `json:"fan_out"`      // total transitive dependents
	Description string   `json:"description"`
}

// DepGraph is a directed acyclic graph of roadmap items keyed by task ID.
type DepGraph struct {
	Nodes map[string]*Node
	// adj stores edges: adj[A] contains B means A blocks B (A must finish before B).
	adj map[string][]string
	// radj stores reverse edges: radj[B] contains A means B depends on A.
	radj map[string][]string
}

// BuildGraph constructs a DepGraph from a parsed Roadmap.
// Tasks without an ID are assigned a synthetic ID from their phase/section index.
func BuildGraph(rm *Roadmap) *DepGraph {
	g := &DepGraph{
		Nodes: make(map[string]*Node),
		adj:   make(map[string][]string),
		radj:  make(map[string][]string),
	}

	// First pass: create nodes.
	syntheticIdx := 0
	for _, phase := range rm.Phases {
		for _, sec := range phase.Sections {
			for _, task := range sec.Tasks {
				id := task.ID
				if id == "" {
					syntheticIdx++
					id = fmt.Sprintf("_auto_%d", syntheticIdx)
				}
				// Avoid duplicates — first occurrence wins.
				if _, exists := g.Nodes[id]; exists {
					continue
				}
				g.Nodes[id] = &Node{
					ID:          id,
					Title:       task.Description,
					Phase:       phase.Name,
					Section:     sec.Name,
					Done:        task.Done,
					DependsOn:   task.DependsOn,
					Description: task.Description,
				}
			}
		}
	}

	// Second pass: build adjacency lists for edges that reference known nodes.
	for id, node := range g.Nodes {
		for _, dep := range node.DependsOn {
			if _, ok := g.Nodes[dep]; !ok {
				continue // skip unresolved references
			}
			g.adj[dep] = append(g.adj[dep], id)
			g.radj[id] = append(g.radj[id], dep)
		}
	}

	// Populate Dependents on each node.
	for id, targets := range g.adj {
		if n, ok := g.Nodes[id]; ok {
			n.Dependents = targets
		}
	}

	// Compute depths via BFS from roots.
	g.computeDepths()

	// Compute transitive fan-out for bottleneck analysis.
	g.computeFanOut()

	return g
}

// computeDepths sets Depth on every node to the longest path from any root.
func (g *DepGraph) computeDepths() {
	for _, n := range g.Nodes {
		n.Depth = 0
	}
	// Use dynamic programming on topological order.
	sorted, err := g.topSort()
	if err != nil {
		return // cycle — leave depths at 0
	}
	for _, id := range sorted {
		node := g.Nodes[id]
		for _, dep := range g.radj[id] {
			parentDepth := g.Nodes[dep].Depth
			if parentDepth+1 > node.Depth {
				node.Depth = parentDepth + 1
			}
		}
	}
}

// computeFanOut sets FanOut to the total number of transitive dependents.
func (g *DepGraph) computeFanOut() {
	// Memoized DFS.
	cache := make(map[string]int)
	var dfs func(string) int
	dfs = func(id string) int {
		if v, ok := cache[id]; ok {
			return v
		}
		cache[id] = 0 // guard against cycles during computation
		count := 0
		for _, child := range g.adj[id] {
			count += 1 + dfs(child)
		}
		cache[id] = count
		return count
	}
	for id := range g.Nodes {
		g.Nodes[id].FanOut = dfs(id)
	}
}

// TopologicalSort returns node IDs in dependency order. Returns an error if
// the graph contains a cycle.
func (g *DepGraph) TopologicalSort() ([]Node, error) {
	ids, err := g.topSort()
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, len(ids))
	for i, id := range ids {
		nodes[i] = *g.Nodes[id]
	}
	return nodes, nil
}

// topSort returns IDs in topological order using Kahn's algorithm.
func (g *DepGraph) topSort() ([]string, error) {
	inDegree := make(map[string]int, len(g.Nodes))
	for id := range g.Nodes {
		inDegree[id] = len(g.radj[id])
	}

	// Seed with roots (in-degree 0), sorted for determinism.
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	var result []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		result = append(result, id)

		children := make([]string, len(g.adj[id]))
		copy(children, g.adj[id])
		sort.Strings(children)

		for _, child := range children {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(result) != len(g.Nodes) {
		// Find nodes involved in cycles for a useful error message.
		var cycleNodes []string
		for id, deg := range inDegree {
			if deg > 0 {
				cycleNodes = append(cycleNodes, id)
			}
		}
		sort.Strings(cycleNodes)
		return nil, fmt.Errorf("dependency cycle detected involving: %s",
			strings.Join(cycleNodes, ", "))
	}
	return result, nil
}

// CriticalPath returns the longest chain of blocking dependencies in the graph.
// Only pending (not done) nodes are considered. The path is returned from the
// deepest root to the furthest leaf.
func (g *DepGraph) CriticalPath() []Node {
	// Build a subgraph of pending nodes only.
	pending := make(map[string]bool)
	for id, n := range g.Nodes {
		if !n.Done {
			pending[id] = true
		}
	}

	if len(pending) == 0 {
		return nil
	}

	// Compute longest path in the pending subgraph.
	// dist[id] = length of longest path ending at id (counting nodes).
	dist := make(map[string]int)
	prev := make(map[string]string) // backpointer for path reconstruction

	// Process in topological order restricted to pending nodes.
	sorted, err := g.topSort()
	if err != nil {
		return nil
	}

	var pendingSorted []string
	for _, id := range sorted {
		if pending[id] {
			pendingSorted = append(pendingSorted, id)
		}
	}

	for _, id := range pendingSorted {
		dist[id] = 1 // count the node itself
		for _, dep := range g.radj[id] {
			if !pending[dep] {
				continue
			}
			if dist[dep]+1 > dist[id] {
				dist[id] = dist[dep] + 1
				prev[id] = dep
			}
		}
	}

	// Find the node with the longest distance.
	maxDist := 0
	endID := ""
	for id, d := range dist {
		if d > maxDist {
			maxDist = d
			endID = id
		}
	}

	if endID == "" {
		return nil
	}

	// Reconstruct path backwards.
	var path []Node
	for cur := endID; cur != ""; cur = prev[cur] {
		path = append(path, *g.Nodes[cur])
	}

	// Reverse so root comes first.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// Unblocked returns pending nodes whose dependencies are all satisfied
// (either completed or absent from the graph). These are immediately workable.
func (g *DepGraph) Unblocked() []Node {
	var result []Node
	for _, node := range g.Nodes {
		if node.Done {
			continue
		}
		blocked := false
		for _, dep := range node.DependsOn {
			depNode, ok := g.Nodes[dep]
			if ok && !depNode.Done {
				blocked = true
				break
			}
		}
		if !blocked {
			result = append(result, *node)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// Bottlenecks returns pending nodes sorted by the number of transitive
// downstream items they block. Only nodes with at least one dependent are
// included. The result is sorted descending by FanOut.
func (g *DepGraph) Bottlenecks() []Node {
	var result []Node
	for _, node := range g.Nodes {
		if node.Done {
			continue
		}
		// Recompute pending fan-out (only count pending dependents).
		pendingFanOut := g.pendingFanOut(node.ID)
		if pendingFanOut == 0 {
			continue
		}
		n := *node
		n.FanOut = pendingFanOut
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].FanOut != result[j].FanOut {
			return result[i].FanOut > result[j].FanOut
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// pendingFanOut counts transitive pending dependents.
func (g *DepGraph) pendingFanOut(id string) int {
	visited := make(map[string]bool)
	var dfs func(string)
	count := 0
	dfs = func(cur string) {
		for _, child := range g.adj[cur] {
			if visited[child] {
				continue
			}
			visited[child] = true
			n := g.Nodes[child]
			if n != nil && !n.Done {
				count++
			}
			dfs(child)
		}
	}
	dfs(id)
	return count
}

// ParallelizableGroups returns groups of pending nodes that can be worked
// simultaneously. Each group contains nodes at the same depth level whose
// dependencies are all satisfied by completed nodes or nodes in earlier groups.
func (g *DepGraph) ParallelizableGroups() [][]Node {
	// Simulate execution: repeatedly collect unblocked pending nodes,
	// then mark them "done" for the next round.
	done := make(map[string]bool)
	for id, n := range g.Nodes {
		if n.Done {
			done[id] = true
		}
	}

	pending := make(map[string]bool)
	for id, n := range g.Nodes {
		if !n.Done {
			pending[id] = true
		}
	}

	var groups [][]Node
	for len(pending) > 0 {
		var group []Node
		for id := range pending {
			blocked := false
			for _, dep := range g.Nodes[id].DependsOn {
				if _, known := g.Nodes[dep]; known && !done[dep] {
					blocked = true
					break
				}
			}
			if !blocked {
				group = append(group, *g.Nodes[id])
			}
		}

		if len(group) == 0 {
			// Remaining items form a cycle or have unresolvable deps — break.
			break
		}

		// Sort within group for determinism.
		sort.Slice(group, func(i, j int) bool {
			return group[i].ID < group[j].ID
		})
		groups = append(groups, group)

		// Mark this batch as done.
		for _, n := range group {
			done[n.ID] = true
			delete(pending, n.ID)
		}
	}

	return groups
}

// Visualize renders an ASCII-art representation of the dependency graph.
// The output shows each root and its dependency tree with indentation.
func (g *DepGraph) Visualize() string {
	sorted, err := g.topSort()
	if err != nil {
		return fmt.Sprintf("(cycle detected: %v)", err)
	}

	// Find roots (nodes with no pending dependencies).
	var roots []string
	for _, id := range sorted {
		if len(g.radj[id]) == 0 {
			roots = append(roots, id)
		}
	}
	sort.Strings(roots)

	var b strings.Builder
	b.WriteString("Dependency Graph\n")
	b.WriteString(strings.Repeat("=", 50))
	b.WriteString("\n\n")

	// Write summary.
	total, done, pending := 0, 0, 0
	for _, n := range g.Nodes {
		total++
		if n.Done {
			done++
		} else {
			pending++
		}
	}
	b.WriteString(fmt.Sprintf("Nodes: %d (done: %d, pending: %d)\n", total, done, pending))
	b.WriteString(fmt.Sprintf("Edges: %d\n\n", g.edgeCount()))

	// Print each root's tree.
	visited := make(map[string]bool)
	for _, root := range roots {
		g.printTree(&b, root, "", true, visited)
	}

	// Print any remaining unvisited nodes (part of cycles or disconnected).
	var remaining []string
	for id := range g.Nodes {
		if !visited[id] {
			remaining = append(remaining, id)
		}
	}
	sort.Strings(remaining)
	if len(remaining) > 0 {
		b.WriteString("\nDisconnected / Cyclic:\n")
		for _, id := range remaining {
			n := g.Nodes[id]
			b.WriteString(fmt.Sprintf("  %s %s\n", g.statusIcon(n), id))
		}
	}

	return b.String()
}

func (g *DepGraph) printTree(b *strings.Builder, id, prefix string, isLast bool, visited map[string]bool) {
	if visited[id] {
		// Already printed — show reference.
		connector := "|-- "
		if isLast {
			connector = "`-- "
		}
		b.WriteString(fmt.Sprintf("%s%s(-> %s)\n", prefix, connector, id))
		return
	}
	visited[id] = true

	node := g.Nodes[id]
	connector := "|-- "
	if isLast {
		connector = "`-- "
	}
	if prefix == "" {
		// Root node.
		b.WriteString(fmt.Sprintf("%s %s", g.statusIcon(node), id))
	} else {
		b.WriteString(fmt.Sprintf("%s%s%s %s", prefix, connector, g.statusIcon(node), id))
	}

	title := node.Title
	if len(title) > 50 {
		title = title[:47] + "..."
	}
	if title != "" {
		b.WriteString(fmt.Sprintf(" -- %s", title))
	}
	b.WriteString("\n")

	children := make([]string, len(g.adj[id]))
	copy(children, g.adj[id])
	sort.Strings(children)

	childPrefix := prefix
	if prefix != "" {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "|   "
		}
	} else {
		childPrefix = "    "
	}

	for i, child := range children {
		g.printTree(b, child, childPrefix, i == len(children)-1, visited)
	}
}

func (g *DepGraph) statusIcon(n *Node) string {
	if n.Done {
		return "[x]"
	}
	return "[ ]"
}

func (g *DepGraph) edgeCount() int {
	count := 0
	for _, targets := range g.adj {
		count += len(targets)
	}
	return count
}
