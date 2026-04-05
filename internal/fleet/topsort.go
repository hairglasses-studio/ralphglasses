package fleet

import (
	"fmt"
	"slices"
	"sort"
)

// TaskNode represents a task with dependencies in a scheduling graph.
type TaskNode struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Dependencies []string `json:"dependencies,omitempty"` // IDs of tasks this depends on
	Priority     int      `json:"priority"`
}

// TaskGraph holds task nodes and their dependency relationships for scheduling.
type TaskGraph struct {
	nodes map[string]TaskNode // keyed by ID
	edges map[string][]string // adjacency: from -> []to (from depends on to)
	order []string            // insertion order for determinism
}

// NewTaskGraph creates an empty task graph.
func NewTaskGraph() *TaskGraph {
	return &TaskGraph{
		nodes: make(map[string]TaskNode),
		edges: make(map[string][]string),
	}
}

// AddNode inserts a task node into the graph. If a node with the same ID
// already exists, it is replaced. Any dependencies listed on the node are
// registered as edges automatically.
func (g *TaskGraph) AddNode(node TaskNode) {
	if _, exists := g.nodes[node.ID]; !exists {
		g.order = append(g.order, node.ID)
	}
	g.nodes[node.ID] = node
	for _, dep := range node.Dependencies {
		g.addEdge(node.ID, dep)
	}
}

// AddDependency declares that task `from` depends on task `to` (i.e., `to`
// must complete before `from` can start). Returns an error if either node ID
// is unknown.
func (g *TaskGraph) AddDependency(from, to string) error {
	if _, ok := g.nodes[from]; !ok {
		return fmt.Errorf("topsort: unknown node %q", from)
	}
	if _, ok := g.nodes[to]; !ok {
		return fmt.Errorf("topsort: unknown node %q", to)
	}
	g.addEdge(from, to)
	return nil
}

func (g *TaskGraph) addEdge(from, to string) {
	if slices.Contains(g.edges[from], to) {
		return // already present
	}
	g.edges[from] = append(g.edges[from], to)
}

// Sort returns nodes in topological order using Kahn's algorithm (BFS-based).
// Tasks with no unmet dependencies appear first. Returns an error if the graph
// contains a cycle.
func (g *TaskGraph) Sort() ([]TaskNode, error) {
	if len(g.nodes) == 0 {
		return nil, nil
	}

	// Build in-degree map: for each node, count how many nodes depend on it
	// being a dependency. Actually we need the reverse: in-degree counts how
	// many prerequisites a node has.
	inDegree := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		inDegree[id] = 0
	}
	for from, deps := range g.edges {
		if _, ok := g.nodes[from]; !ok {
			continue
		}
		for _, to := range deps {
			if _, ok := g.nodes[to]; !ok {
				continue
			}
		}
		inDegree[from] = len(deps)
	}

	// But we actually need: for each node, the number of dependencies it has
	// that are also in the graph. Let's recompute properly.
	for id := range g.nodes {
		count := 0
		for _, dep := range g.edges[id] {
			if _, ok := g.nodes[dep]; ok {
				count++
			}
		}
		inDegree[id] = count
	}

	// Seed queue with nodes that have zero in-degree (no dependencies).
	var queue []string
	for _, id := range g.order {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	// Sort initial queue by priority descending for deterministic output.
	sort.Slice(queue, func(i, j int) bool {
		return g.nodes[queue[i]].Priority > g.nodes[queue[j]].Priority
	})

	var result []TaskNode
	for len(queue) > 0 {
		// Pop front.
		current := queue[0]
		queue = queue[1:]
		result = append(result, g.nodes[current])

		// For every node that depends on `current`, decrement its in-degree.
		for _, id := range g.order {
			for _, dep := range g.edges[id] {
				if dep == current {
					inDegree[id]--
					if inDegree[id] == 0 {
						queue = append(queue, id)
					}
				}
			}
		}
		// Re-sort queue by priority for deterministic batch ordering.
		sort.Slice(queue, func(i, j int) bool {
			return g.nodes[queue[i]].Priority > g.nodes[queue[j]].Priority
		})
	}

	if len(result) != len(g.nodes) {
		return nil, fmt.Errorf("topsort: cycle detected — sorted %d of %d nodes", len(result), len(g.nodes))
	}
	return result, nil
}

// DetectCycles returns all cycles in the graph as paths. Each cycle is a slice
// of node IDs forming the loop. Returns nil if the graph is acyclic.
func (g *TaskGraph) DetectCycles() [][]string {
	if len(g.nodes) == 0 {
		return nil
	}

	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully explored
	)

	color := make(map[string]int, len(g.nodes))
	parent := make(map[string]string, len(g.nodes))
	var cycles [][]string

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		for _, dep := range g.edges[node] {
			if _, ok := g.nodes[dep]; !ok {
				continue
			}
			if color[dep] == gray {
				// Found a cycle: trace back from node to dep.
				cycle := []string{dep, node}
				cur := node
				for cur != dep {
					cur = parent[cur]
					if cur == dep {
						break
					}
					cycle = append(cycle, cur)
				}
				// Reverse to get the natural order.
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				cycles = append(cycles, cycle)
			} else if color[dep] == white {
				parent[dep] = node
				dfs(dep)
			}
		}
		color[node] = black
	}

	for _, id := range g.order {
		if color[id] == white {
			dfs(id)
		}
	}
	return cycles
}

// CriticalPath returns the longest dependency chain in the graph. For a DAG
// this is the critical path that determines minimum sequential execution time.
// Returns nil for an empty graph.
func (g *TaskGraph) CriticalPath() []TaskNode {
	sorted, err := g.Sort()
	if err != nil || len(sorted) == 0 {
		return nil
	}

	// Build a map of node ID to its position in the topological order.
	dist := make(map[string]int, len(sorted))
	prev := make(map[string]string, len(sorted))

	for _, node := range sorted {
		dist[node.ID] = 0
	}

	// Process in topological order. For each node, look at nodes that depend
	// on it and update their distance.
	for _, node := range sorted {
		for _, id := range g.order {
			for _, dep := range g.edges[id] {
				if dep == node.ID {
					newDist := dist[node.ID] + 1
					if newDist > dist[id] {
						dist[id] = newDist
						prev[id] = node.ID
					}
				}
			}
		}
	}

	// Find the node with the largest distance.
	var endID string
	maxDist := -1
	for _, id := range g.order {
		if dist[id] > maxDist {
			maxDist = dist[id]
			endID = id
		}
	}

	if endID == "" {
		return nil
	}

	// Trace back to build the path.
	var path []TaskNode
	cur := endID
	for cur != "" {
		path = append(path, g.nodes[cur])
		cur = prev[cur]
	}

	// Reverse to get start->end order.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	return path
}

// ReadyTasks returns all tasks whose dependencies have been fully satisfied
// (all dependency IDs are present in the completed set). Results are sorted
// by priority descending.
func (g *TaskGraph) ReadyTasks(completed map[string]bool) []TaskNode {
	var ready []TaskNode
	for _, id := range g.order {
		if completed[id] {
			continue
		}
		node := g.nodes[id]
		allMet := true
		for _, dep := range g.edges[id] {
			if _, inGraph := g.nodes[dep]; inGraph && !completed[dep] {
				allMet = false
				break
			}
		}
		if allMet {
			ready = append(ready, node)
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].Priority > ready[j].Priority
	})
	return ready
}

// Nodes returns all nodes in insertion order.
func (g *TaskGraph) Nodes() []TaskNode {
	result := make([]TaskNode, 0, len(g.nodes))
	for _, id := range g.order {
		result = append(result, g.nodes[id])
	}
	return result
}

// SchedulePlan groups tasks into ordered batches of parallelizable work.
// Tasks within the same batch have no mutual dependencies and can run
// concurrently. Batches must execute sequentially.
type SchedulePlan struct {
	Batches    []ScheduleBatch `json:"batches"`
	TotalTasks int             `json:"total_tasks"`
	Depth      int             `json:"depth"` // number of sequential batches
}

// ScheduleBatch is a set of tasks that can execute in parallel.
type ScheduleBatch struct {
	Level int        `json:"level"` // 0-indexed batch number
	Tasks []TaskNode `json:"tasks"`
}

// BuildSchedule groups the graph's tasks into ordered, parallelizable batches.
// Tasks in the same batch share no dependencies and can run concurrently.
// Returns an error if the graph contains a cycle.
func BuildSchedule(graph *TaskGraph) (*SchedulePlan, error) {
	if len(graph.nodes) == 0 {
		return &SchedulePlan{}, nil
	}

	// Compute in-degree (number of in-graph dependencies) for each node.
	inDegree := make(map[string]int, len(graph.nodes))
	for _, id := range graph.order {
		count := 0
		for _, dep := range graph.edges[id] {
			if _, ok := graph.nodes[dep]; ok {
				count++
			}
		}
		inDegree[id] = count
	}

	// Collect initial zero-degree nodes.
	var currentBatch []string
	for _, id := range graph.order {
		if inDegree[id] == 0 {
			currentBatch = append(currentBatch, id)
		}
	}

	var plan SchedulePlan
	processed := 0
	level := 0

	for len(currentBatch) > 0 {
		// Sort batch by priority descending.
		sort.Slice(currentBatch, func(i, j int) bool {
			return graph.nodes[currentBatch[i]].Priority > graph.nodes[currentBatch[j]].Priority
		})

		batch := ScheduleBatch{Level: level}
		for _, id := range currentBatch {
			batch.Tasks = append(batch.Tasks, graph.nodes[id])
		}
		plan.Batches = append(plan.Batches, batch)
		processed += len(currentBatch)

		// Find next batch: nodes whose in-degree drops to zero.
		var nextBatch []string
		for _, doneID := range currentBatch {
			for _, id := range graph.order {
				for _, dep := range graph.edges[id] {
					if dep == doneID {
						inDegree[id]--
						if inDegree[id] == 0 {
							nextBatch = append(nextBatch, id)
						}
					}
				}
			}
		}

		// Deduplicate nextBatch.
		seen := make(map[string]bool, len(nextBatch))
		var deduped []string
		for _, id := range nextBatch {
			if !seen[id] {
				seen[id] = true
				deduped = append(deduped, id)
			}
		}

		currentBatch = deduped
		level++
	}

	if processed != len(graph.nodes) {
		return nil, fmt.Errorf("topsort: cycle detected — scheduled %d of %d tasks", processed, len(graph.nodes))
	}

	plan.TotalTasks = processed
	plan.Depth = len(plan.Batches)
	return &plan, nil
}
