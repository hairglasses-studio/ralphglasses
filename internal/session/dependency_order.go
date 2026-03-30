package session

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrCyclicDependency indicates the dependency graph contains a cycle.
	ErrCyclicDependency = errors.New("cyclic dependency detected")

	// ErrDuplicateTask indicates a task was added more than once.
	ErrDuplicateTask = errors.New("duplicate task")

	// ErrMissingDependency indicates a dependency references a task not in the graph.
	ErrMissingDependency = errors.New("missing dependency")
)

// DependencyGraph is a DAG-based dependency tracker for fleet session tasks.
// It supports topological ordering, cycle detection, ready-task queries, and
// critical-path computation. All methods are safe for concurrent use.
type DependencyGraph struct {
	mu    sync.RWMutex
	nodes map[string][]string // taskID -> list of dependency taskIDs
}

// NewDependencyGraph creates an empty DependencyGraph.
func NewDependencyGraph() *DependencyGraph {
	return &DependencyGraph{
		nodes: make(map[string][]string),
	}
}

// AddTask registers a task with its dependencies. Returns ErrDuplicateTask
// if the task ID already exists.
func (g *DependencyGraph) AddTask(id string, deps []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.nodes[id]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTask, id)
	}
	// Copy deps to avoid caller mutation.
	d := make([]string, len(deps))
	copy(d, deps)
	g.nodes[id] = d
	return nil
}

// TopologicalSort returns a valid execution order for all tasks. Returns
// ErrCyclicDependency if the graph contains a cycle. Missing dependencies
// (referenced but never added) are treated as already-satisfied.
func (g *DependencyGraph) TopologicalSort() ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Kahn's algorithm.
	inDegree := make(map[string]int, len(g.nodes))
	reverse := make(map[string][]string) // dep -> dependents

	for id := range g.nodes {
		if _, ok := inDegree[id]; !ok {
			inDegree[id] = 0
		}
	}

	for id, deps := range g.nodes {
		for _, dep := range deps {
			if _, inGraph := g.nodes[dep]; !inGraph {
				continue // missing dep treated as satisfied
			}
			inDegree[id]++
			reverse[dep] = append(reverse[dep], id)
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// Stable ordering: sort queue each round so output is deterministic.
	sortStrings(queue)

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		dependents := reverse[node]
		sortStrings(dependents)
		for _, dep := range dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				sortStrings(queue)
			}
		}
	}

	if len(order) != len(g.nodes) {
		return nil, ErrCyclicDependency
	}
	return order, nil
}

// DetectCycles returns all elementary cycles found in the graph. Each cycle
// is a slice of task IDs forming the loop. Returns nil if the graph is acyclic.
func (g *DependencyGraph) DetectCycles() [][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int, len(g.nodes))
	parent := make(map[string]string)
	var cycles [][]string

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		deps := g.nodes[node]
		sortedDeps := make([]string, len(deps))
		copy(sortedDeps, deps)
		sortStrings(sortedDeps)

		for _, dep := range sortedDeps {
			if _, inGraph := g.nodes[dep]; !inGraph {
				continue
			}
			switch color[dep] {
			case white:
				parent[dep] = node
				dfs(dep)
			case gray:
				// Back edge: extract cycle.
				var cycle []string
				cur := node
				for cur != dep {
					cycle = append([]string{cur}, cycle...)
					cur = parent[cur]
				}
				cycle = append([]string{dep}, cycle...)
				cycles = append(cycles, cycle)
			}
		}
		color[node] = black
	}

	ids := g.sortedNodeIDs()
	for _, id := range ids {
		if color[id] == white {
			dfs(id)
		}
	}
	return cycles
}

// ReadyTasks returns the set of tasks whose dependencies have all been
// completed. A task that is itself in the completed set is not returned.
func (g *DependencyGraph) ReadyTasks(completed map[string]bool) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var ready []string
	for id, deps := range g.nodes {
		if completed[id] {
			continue
		}
		allDone := true
		for _, dep := range deps {
			// Missing deps (not in graph) count as satisfied.
			if _, inGraph := g.nodes[dep]; inGraph && !completed[dep] {
				allDone = false
				break
			}
		}
		if allDone {
			ready = append(ready, id)
		}
	}
	sortStrings(ready)
	return ready
}

// CriticalPath returns the longest path through the DAG, measured by node
// count. Returns ErrCyclicDependency if the graph contains a cycle.
func (g *DependencyGraph) CriticalPath() ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	order, err := g.topSortLocked()
	if err != nil {
		return nil, err
	}

	// Build forward adjacency: dep -> tasks that depend on it.
	forward := make(map[string][]string)
	for id, deps := range g.nodes {
		for _, dep := range deps {
			if _, ok := g.nodes[dep]; ok {
				forward[dep] = append(forward[dep], id)
			}
		}
	}

	dist := make(map[string]int, len(g.nodes))
	prev := make(map[string]string, len(g.nodes))
	for _, id := range order {
		dist[id] = 1 // self
	}

	for _, u := range order {
		children := forward[u]
		sortStrings(children)
		for _, v := range children {
			if dist[u]+1 > dist[v] {
				dist[v] = dist[u] + 1
				prev[v] = u
			}
		}
	}

	// Find the node with maximum distance.
	var maxNode string
	maxDist := 0
	for _, id := range order {
		if dist[id] > maxDist {
			maxDist = dist[id]
			maxNode = id
		}
	}

	if maxNode == "" {
		return nil, nil
	}

	// Backtrack to build path.
	var path []string
	for cur := maxNode; cur != ""; cur = prev[cur] {
		path = append([]string{cur}, path...)
	}
	return path, nil
}

// topSortLocked is TopologicalSort without acquiring the lock (caller must hold RLock).
func (g *DependencyGraph) topSortLocked() ([]string, error) {
	inDegree := make(map[string]int, len(g.nodes))
	reverse := make(map[string][]string)

	for id := range g.nodes {
		if _, ok := inDegree[id]; !ok {
			inDegree[id] = 0
		}
	}

	for id, deps := range g.nodes {
		for _, dep := range deps {
			if _, inGraph := g.nodes[dep]; !inGraph {
				continue
			}
			inDegree[id]++
			reverse[dep] = append(reverse[dep], id)
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sortStrings(queue)

	var order []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		dependents := reverse[node]
		sortStrings(dependents)
		for _, dep := range dependents {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
				sortStrings(queue)
			}
		}
	}

	if len(order) != len(g.nodes) {
		return nil, ErrCyclicDependency
	}
	return order, nil
}

// sortedNodeIDs returns deterministically sorted node IDs.
func (g *DependencyGraph) sortedNodeIDs() []string {
	ids := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	sortStrings(ids)
	return ids
}

// sortStrings sorts a string slice in place using insertion sort (good for
// small slices, avoids importing sort package for a simple helper).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
