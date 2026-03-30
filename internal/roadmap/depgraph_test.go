package roadmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to build a Roadmap from inline tasks for graph testing.
func buildTestRoadmap(tasks []Task, phase, section string) *Roadmap {
	return &Roadmap{
		Title: "Test",
		Phases: []Phase{
			{
				Name: phase,
				Sections: []Section{
					{Name: section, Tasks: tasks},
				},
			},
		},
	}
}

// buildMultiPhaseRoadmap builds a roadmap with tasks spread across phases.
func buildMultiPhaseRoadmap(phases []struct {
	name  string
	tasks []Task
}) *Roadmap {
	rm := &Roadmap{Title: "Test"}
	for _, p := range phases {
		rm.Phases = append(rm.Phases, Phase{
			Name: p.name,
			Sections: []Section{
				{Name: p.name, Tasks: p.tasks},
			},
		})
	}
	return rm
}

func TestDepGraph_LinearChain(t *testing.T) {
	t.Parallel()

	// A -> B -> C (linear chain, all pending)
	tasks := []Task{
		{ID: "A", Description: "Task A", Done: false},
		{ID: "B", Description: "Task B", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Task C", Done: false, DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Chain")
	g := BuildGraph(rm)

	// Verify node count.
	if len(g.Nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(g.Nodes))
	}

	// Critical path should be the full chain A -> B -> C.
	cp := g.CriticalPath()
	if len(cp) != 3 {
		t.Fatalf("critical path length = %d, want 3", len(cp))
	}
	if cp[0].ID != "A" || cp[1].ID != "B" || cp[2].ID != "C" {
		t.Errorf("critical path = %v, want [A, B, C]", nodeIDs(cp))
	}

	// Unblocked: only A (no deps).
	ub := g.Unblocked()
	if len(ub) != 1 || ub[0].ID != "A" {
		t.Errorf("unblocked = %v, want [A]", nodeIDs(ub))
	}

	// Bottleneck: A blocks B and C (fan-out 2), B blocks C (fan-out 1).
	bn := g.Bottlenecks()
	if len(bn) < 2 {
		t.Fatalf("bottlenecks = %d, want >= 2", len(bn))
	}
	if bn[0].ID != "A" {
		t.Errorf("top bottleneck = %s, want A", bn[0].ID)
	}
	if bn[0].FanOut != 2 {
		t.Errorf("A fan-out = %d, want 2", bn[0].FanOut)
	}

	// Topological sort: A before B before C.
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	idxA, idxB, idxC := indexOf(sorted, "A"), indexOf(sorted, "B"), indexOf(sorted, "C")
	if idxA >= idxB || idxB >= idxC {
		t.Errorf("topo order: A@%d, B@%d, C@%d — want A < B < C", idxA, idxB, idxC)
	}
}

func TestDepGraph_Diamond(t *testing.T) {
	t.Parallel()

	// Diamond: A -> B, A -> C, B -> D, C -> D
	tasks := []Task{
		{ID: "A", Description: "Root", Done: false},
		{ID: "B", Description: "Left", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Right", Done: false, DependsOn: []string{"A"}},
		{ID: "D", Description: "Sink", Done: false, DependsOn: []string{"B", "C"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Diamond")
	g := BuildGraph(rm)

	// Critical path: A -> B -> D or A -> C -> D (both length 3).
	cp := g.CriticalPath()
	if len(cp) != 3 {
		t.Fatalf("critical path length = %d, want 3", len(cp))
	}
	if cp[0].ID != "A" {
		t.Errorf("critical path start = %s, want A", cp[0].ID)
	}
	if cp[2].ID != "D" {
		t.Errorf("critical path end = %s, want D", cp[2].ID)
	}

	// Unblocked: only A.
	ub := g.Unblocked()
	if len(ub) != 1 || ub[0].ID != "A" {
		t.Errorf("unblocked = %v, want [A]", nodeIDs(ub))
	}

	// A is the biggest bottleneck (blocks B, C, D = fan-out 3).
	bn := g.Bottlenecks()
	if len(bn) == 0 {
		t.Fatal("expected bottlenecks")
	}
	if bn[0].ID != "A" || bn[0].FanOut != 3 {
		t.Errorf("top bottleneck = %s (fan-out %d), want A (3)", bn[0].ID, bn[0].FanOut)
	}

	// Parallelizable groups: [A], [B, C], [D].
	groups := g.ParallelizableGroups()
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if len(groups[0]) != 1 || groups[0][0].ID != "A" {
		t.Errorf("group 0 = %v, want [A]", nodeIDs(groups[0]))
	}
	if len(groups[1]) != 2 {
		t.Errorf("group 1 size = %d, want 2", len(groups[1]))
	}
	g1IDs := nodeIDs(groups[1])
	if g1IDs[0] != "B" || g1IDs[1] != "C" {
		t.Errorf("group 1 = %v, want [B, C]", g1IDs)
	}
	if len(groups[2]) != 1 || groups[2][0].ID != "D" {
		t.Errorf("group 2 = %v, want [D]", nodeIDs(groups[2]))
	}
}

func TestDepGraph_UnblockedWithCompletedDeps(t *testing.T) {
	t.Parallel()

	// A is done, B depends on A, C has no deps. Both B and C should be unblocked.
	tasks := []Task{
		{ID: "A", Description: "Done task", Done: true},
		{ID: "B", Description: "Depends on done", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Independent", Done: false},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Mixed")
	g := BuildGraph(rm)

	ub := g.Unblocked()
	ids := nodeIDs(ub)
	if len(ids) != 2 {
		t.Fatalf("unblocked = %v, want [B, C]", ids)
	}
	if ids[0] != "B" || ids[1] != "C" {
		t.Errorf("unblocked = %v, want [B, C]", ids)
	}
}

func TestDepGraph_UnblockedSkipsDoneNodes(t *testing.T) {
	t.Parallel()

	// All done — unblocked should be empty.
	tasks := []Task{
		{ID: "A", Description: "Done", Done: true},
		{ID: "B", Description: "Also done", Done: true, DependsOn: []string{"A"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Done")
	g := BuildGraph(rm)

	ub := g.Unblocked()
	if len(ub) != 0 {
		t.Errorf("unblocked = %v, want empty", nodeIDs(ub))
	}
}

func TestDepGraph_BottleneckIdentification(t *testing.T) {
	t.Parallel()

	// Hub topology: X blocks Y1, Y2, Y3. Z is independent.
	tasks := []Task{
		{ID: "X", Description: "Hub", Done: false},
		{ID: "Y1", Description: "Spoke 1", Done: false, DependsOn: []string{"X"}},
		{ID: "Y2", Description: "Spoke 2", Done: false, DependsOn: []string{"X"}},
		{ID: "Y3", Description: "Spoke 3", Done: false, DependsOn: []string{"X"}},
		{ID: "Z", Description: "Independent", Done: false},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Hub")
	g := BuildGraph(rm)

	bn := g.Bottlenecks()
	if len(bn) != 1 {
		t.Fatalf("bottlenecks = %d, want 1 (only X has dependents)", len(bn))
	}
	if bn[0].ID != "X" {
		t.Errorf("bottleneck = %s, want X", bn[0].ID)
	}
	if bn[0].FanOut != 3 {
		t.Errorf("X pending fan-out = %d, want 3", bn[0].FanOut)
	}
}

func TestDepGraph_CycleDetection(t *testing.T) {
	t.Parallel()

	// A -> B -> C -> A (cycle).
	tasks := []Task{
		{ID: "A", Description: "Cycle A", Done: false, DependsOn: []string{"C"}},
		{ID: "B", Description: "Cycle B", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Cycle C", Done: false, DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Cycle")
	g := BuildGraph(rm)

	_, err := g.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want to contain 'cycle'", err.Error())
	}
}

func TestDepGraph_ParallelizableGroups(t *testing.T) {
	t.Parallel()

	// Two independent chains: A->B and C->D, plus E standalone.
	tasks := []Task{
		{ID: "A", Description: "Chain 1 root", Done: false},
		{ID: "B", Description: "Chain 1 leaf", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Chain 2 root", Done: false},
		{ID: "D", Description: "Chain 2 leaf", Done: false, DependsOn: []string{"C"}},
		{ID: "E", Description: "Standalone", Done: false},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Parallel")
	g := BuildGraph(rm)

	groups := g.ParallelizableGroups()
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}

	// Group 0: A, C, E (all roots / no deps).
	g0 := nodeIDs(groups[0])
	if len(g0) != 3 {
		t.Errorf("group 0 size = %d, want 3", len(g0))
	}

	// Group 1: B, D (both unblocked once A, C done).
	g1 := nodeIDs(groups[1])
	if len(g1) != 2 {
		t.Errorf("group 1 size = %d, want 2", len(g1))
	}
}

func TestDepGraph_ParallelizableGroupsWithDone(t *testing.T) {
	t.Parallel()

	// A is done, B depends on A, C depends on B, D is independent.
	tasks := []Task{
		{ID: "A", Description: "Done root", Done: true},
		{ID: "B", Description: "Unblocked", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Blocked by B", Done: false, DependsOn: []string{"B"}},
		{ID: "D", Description: "Independent", Done: false},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Mixed")
	g := BuildGraph(rm)

	groups := g.ParallelizableGroups()
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}

	// Group 0: B and D (both unblocked — A is already done).
	g0 := nodeIDs(groups[0])
	if len(g0) != 2 {
		t.Fatalf("group 0 = %v, want [B, D]", g0)
	}
	if g0[0] != "B" || g0[1] != "D" {
		t.Errorf("group 0 = %v, want [B, D]", g0)
	}

	// Group 1: C.
	g1 := nodeIDs(groups[1])
	if len(g1) != 1 || g1[0] != "C" {
		t.Errorf("group 1 = %v, want [C]", g1)
	}
}

func TestDepGraph_TopologicalSortNoCycle(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "A", Description: "Root"},
		{ID: "B", Description: "Mid", DependsOn: []string{"A"}},
		{ID: "C", Description: "Leaf", DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Topo")
	g := BuildGraph(rm)

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 3 {
		t.Fatalf("sorted length = %d, want 3", len(sorted))
	}

	// A must come before B, B before C.
	idxA, idxB, idxC := indexOf(sorted, "A"), indexOf(sorted, "B"), indexOf(sorted, "C")
	if idxA >= idxB || idxB >= idxC {
		t.Errorf("topo order: A@%d, B@%d, C@%d — want A < B < C", idxA, idxB, idxC)
	}
}

func TestDepGraph_EmptyGraph(t *testing.T) {
	t.Parallel()

	rm := &Roadmap{Title: "Empty"}
	g := BuildGraph(rm)

	if len(g.Nodes) != 0 {
		t.Errorf("nodes = %d, want 0", len(g.Nodes))
	}
	if cp := g.CriticalPath(); len(cp) != 0 {
		t.Errorf("critical path should be empty")
	}
	if ub := g.Unblocked(); len(ub) != 0 {
		t.Errorf("unblocked should be empty")
	}
	if bn := g.Bottlenecks(); len(bn) != 0 {
		t.Errorf("bottlenecks should be empty")
	}
	groups := g.ParallelizableGroups()
	if len(groups) != 0 {
		t.Errorf("groups should be empty")
	}
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(sorted) != 0 {
		t.Errorf("sorted should be empty")
	}
}

func TestDepGraph_AllDone(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "A", Description: "Done A", Done: true},
		{ID: "B", Description: "Done B", Done: true, DependsOn: []string{"A"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Done")
	g := BuildGraph(rm)

	if cp := g.CriticalPath(); len(cp) != 0 {
		t.Errorf("critical path should be empty when all done, got %v", nodeIDs(cp))
	}
	if ub := g.Unblocked(); len(ub) != 0 {
		t.Errorf("unblocked should be empty when all done")
	}
	if bn := g.Bottlenecks(); len(bn) != 0 {
		t.Errorf("bottlenecks should be empty when all done")
	}
}

func TestDepGraph_SyntheticIDs(t *testing.T) {
	t.Parallel()

	// Tasks without IDs get synthetic IDs.
	tasks := []Task{
		{Description: "No ID task 1"},
		{Description: "No ID task 2"},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "NoID")
	g := BuildGraph(rm)

	if len(g.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(g.Nodes))
	}

	// Verify nodes exist with synthetic IDs.
	for _, n := range g.Nodes {
		if !strings.HasPrefix(n.ID, "_auto_") {
			t.Errorf("expected synthetic ID, got %q", n.ID)
		}
	}
}

func TestDepGraph_UnresolvedDependency(t *testing.T) {
	t.Parallel()

	// B depends on "X" which doesn't exist. B should be treated as unblocked.
	tasks := []Task{
		{ID: "A", Description: "Task A"},
		{ID: "B", Description: "Task B", DependsOn: []string{"X"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Unresolved")
	g := BuildGraph(rm)

	ub := g.Unblocked()
	ids := nodeIDs(ub)
	if len(ids) != 2 {
		t.Fatalf("unblocked = %v, want [A, B] (X is unresolved so B is unblocked)", ids)
	}
}

func TestDepGraph_CriticalPathSkipsDone(t *testing.T) {
	t.Parallel()

	// Chain: A(done) -> B(pending) -> C(pending). Critical path = [B, C].
	tasks := []Task{
		{ID: "A", Description: "Done", Done: true},
		{ID: "B", Description: "Pending", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Pending", Done: false, DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Mixed")
	g := BuildGraph(rm)

	cp := g.CriticalPath()
	if len(cp) != 2 {
		t.Fatalf("critical path = %v, want [B, C]", nodeIDs(cp))
	}
	if cp[0].ID != "B" || cp[1].ID != "C" {
		t.Errorf("critical path = %v, want [B, C]", nodeIDs(cp))
	}
}

func TestDepGraph_Visualize(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "A", Description: "Root task", Done: true},
		{ID: "B", Description: "Middle task", Done: false, DependsOn: []string{"A"}},
		{ID: "C", Description: "Leaf task", Done: false, DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Viz")
	g := BuildGraph(rm)

	viz := g.Visualize()

	// Should contain the header.
	if !strings.Contains(viz, "Dependency Graph") {
		t.Error("missing header in visualization")
	}

	// Should contain node IDs.
	for _, id := range []string{"A", "B", "C"} {
		if !strings.Contains(viz, id) {
			t.Errorf("missing node %s in visualization", id)
		}
	}

	// Should contain status markers.
	if !strings.Contains(viz, "[x]") {
		t.Error("missing [x] marker for done node")
	}
	if !strings.Contains(viz, "[ ]") {
		t.Error("missing [ ] marker for pending node")
	}

	// Should contain summary counts.
	if !strings.Contains(viz, "Nodes: 3") {
		t.Error("missing node count")
	}
}

func TestDepGraph_VisualizeCycle(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "A", Description: "Cycle", DependsOn: []string{"B"}},
		{ID: "B", Description: "Cycle", DependsOn: []string{"A"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Cycle")
	g := BuildGraph(rm)

	viz := g.Visualize()
	if !strings.Contains(viz, "cycle") {
		t.Error("visualization should indicate cycle")
	}
}

func TestDepGraph_MultiPhase(t *testing.T) {
	t.Parallel()

	phases := []struct {
		name  string
		tasks []Task
	}{
		{
			name: "Phase 1",
			tasks: []Task{
				{ID: "1.1", Description: "Foundation", Done: true},
			},
		},
		{
			name: "Phase 2",
			tasks: []Task{
				{ID: "2.1", Description: "Build on 1.1", Done: false, DependsOn: []string{"1.1"}},
				{ID: "2.2", Description: "Independent", Done: false},
			},
		},
	}
	rm := buildMultiPhaseRoadmap(phases)
	g := BuildGraph(rm)

	if len(g.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3", len(g.Nodes))
	}

	// 2.1 should be unblocked (1.1 is done), 2.2 is also unblocked.
	ub := g.Unblocked()
	if len(ub) != 2 {
		t.Fatalf("unblocked = %v, want 2 items", nodeIDs(ub))
	}

	// Verify phase is preserved on nodes.
	if g.Nodes["1.1"].Phase != "Phase 1" {
		t.Errorf("node 1.1 phase = %q", g.Nodes["1.1"].Phase)
	}
	if g.Nodes["2.1"].Phase != "Phase 2" {
		t.Errorf("node 2.1 phase = %q", g.Nodes["2.1"].Phase)
	}
}

func TestDepGraph_WideBottleneck(t *testing.T) {
	t.Parallel()

	// Two bottlenecks: A blocks 3, B blocks 2. A should rank higher.
	tasks := []Task{
		{ID: "A", Description: "Big bottleneck"},
		{ID: "A1", Description: "Dep A1", DependsOn: []string{"A"}},
		{ID: "A2", Description: "Dep A2", DependsOn: []string{"A"}},
		{ID: "A3", Description: "Dep A3", DependsOn: []string{"A"}},
		{ID: "B", Description: "Small bottleneck"},
		{ID: "B1", Description: "Dep B1", DependsOn: []string{"B"}},
		{ID: "B2", Description: "Dep B2", DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Wide")
	g := BuildGraph(rm)

	bn := g.Bottlenecks()
	if len(bn) < 2 {
		t.Fatalf("bottlenecks = %d, want >= 2", len(bn))
	}
	if bn[0].ID != "A" {
		t.Errorf("top bottleneck = %s, want A", bn[0].ID)
	}
	if bn[0].FanOut != 3 {
		t.Errorf("A fan-out = %d, want 3", bn[0].FanOut)
	}
	if bn[1].ID != "B" {
		t.Errorf("second bottleneck = %s, want B", bn[1].ID)
	}
	if bn[1].FanOut != 2 {
		t.Errorf("B fan-out = %d, want 2", bn[1].FanOut)
	}
}

func TestDepGraph_TransitiveFanOut(t *testing.T) {
	t.Parallel()

	// A -> B -> C -> D. A's transitive fan-out should be 3.
	tasks := []Task{
		{ID: "A", Description: "Root"},
		{ID: "B", Description: "Mid 1", DependsOn: []string{"A"}},
		{ID: "C", Description: "Mid 2", DependsOn: []string{"B"}},
		{ID: "D", Description: "Leaf", DependsOn: []string{"C"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Trans")
	g := BuildGraph(rm)

	bn := g.Bottlenecks()
	// A should have fan-out 3, B should have 2, C should have 1.
	if len(bn) < 3 {
		t.Fatalf("bottlenecks = %d, want 3", len(bn))
	}
	fanOuts := make(map[string]int)
	for _, n := range bn {
		fanOuts[n.ID] = n.FanOut
	}
	if fanOuts["A"] != 3 {
		t.Errorf("A fan-out = %d, want 3", fanOuts["A"])
	}
	if fanOuts["B"] != 2 {
		t.Errorf("B fan-out = %d, want 2", fanOuts["B"])
	}
	if fanOuts["C"] != 1 {
		t.Errorf("C fan-out = %d, want 1", fanOuts["C"])
	}
}

func TestDepGraph_IntegrationWithParse(t *testing.T) {
	t.Parallel()

	// Use the testRoadmap constant from parse_test.go.
	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(path, []byte(testRoadmap), 0644); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	g := BuildGraph(rm)

	// The test roadmap has:
	// Phase 0: 3 completed tasks (no IDs)
	// Phase 1: 1.1.1, 1.1.2 [BLOCKED BY 1.1.1], 1.1.3 (done),
	//          1.2.1, 1.2.2 [BLOCKED BY 1.1.1, 1.2.1]
	// Phase 2: 2 tasks (no IDs)

	// Should have nodes for all 10 tasks.
	if len(g.Nodes) != 10 {
		t.Fatalf("nodes = %d, want 10", len(g.Nodes))
	}

	// 1.1.1 should block 1.1.2 and 1.2.2.
	n111 := g.Nodes["1.1.1"]
	if n111 == nil {
		t.Fatal("node 1.1.1 not found")
	}
	if len(n111.Dependents) != 2 {
		t.Errorf("1.1.1 dependents = %d, want 2", len(n111.Dependents))
	}

	// 1.1.2 depends on 1.1.1.
	n112 := g.Nodes["1.1.2"]
	if n112 == nil {
		t.Fatal("node 1.1.2 not found")
	}
	if len(n112.DependsOn) != 1 || n112.DependsOn[0] != "1.1.1" {
		t.Errorf("1.1.2 depends_on = %v, want [1.1.1]", n112.DependsOn)
	}

	// 1.2.2 depends on both 1.1.1 and 1.2.1.
	n122 := g.Nodes["1.2.2"]
	if n122 == nil {
		t.Fatal("node 1.2.2 not found")
	}
	if len(n122.DependsOn) != 2 {
		t.Errorf("1.2.2 depends_on = %v, want 2 deps", n122.DependsOn)
	}

	// Unblocked should include all pending items not blocked by pending deps.
	ub := g.Unblocked()
	if len(ub) == 0 {
		t.Error("expected some unblocked items")
	}

	// 1.1.1 and 1.2.1 should be unblocked (pending, no deps).
	ubIDs := make(map[string]bool)
	for _, n := range ub {
		ubIDs[n.ID] = true
	}
	if !ubIDs["1.1.1"] {
		t.Error("1.1.1 should be unblocked")
	}
	if !ubIDs["1.2.1"] {
		t.Error("1.2.1 should be unblocked")
	}
	// 1.1.2 should NOT be unblocked (depends on pending 1.1.1).
	if ubIDs["1.1.2"] {
		t.Error("1.1.2 should be blocked by 1.1.1")
	}

	// Topological sort should succeed (no cycles).
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort: %v", err)
	}
	if len(sorted) != 10 {
		t.Errorf("sorted = %d, want 10", len(sorted))
	}

	// Visualization should produce output.
	viz := g.Visualize()
	if len(viz) < 50 {
		t.Errorf("visualization too short: %d chars", len(viz))
	}
}

func TestDepGraph_RealROADMAP(t *testing.T) {
	t.Parallel()

	// Integration test with the actual ROADMAP.md file.
	// Skip if file doesn't exist (e.g., in CI without repo checkout).
	path := filepath.Join("..", "..", "ROADMAP.md")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("ROADMAP.md not found at %s: %v", path, err)
	}

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	g := BuildGraph(rm)

	// Basic sanity checks.
	if len(g.Nodes) < 10 {
		t.Errorf("expected at least 10 nodes from real roadmap, got %d", len(g.Nodes))
	}

	// Topological sort should work (roadmap should not have cycles).
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort on real roadmap: %v", err)
	}
	if len(sorted) != len(g.Nodes) {
		t.Errorf("sorted %d nodes, but graph has %d", len(sorted), len(g.Nodes))
	}

	// Should have some unblocked items.
	ub := g.Unblocked()
	if len(ub) == 0 {
		t.Error("expected some unblocked items in real roadmap")
	}

	// Should have some bottlenecks.
	bn := g.Bottlenecks()
	t.Logf("Real roadmap: %d nodes, %d unblocked, %d bottlenecks", len(g.Nodes), len(ub), len(bn))
	if len(bn) > 0 {
		t.Logf("Top bottleneck: %s (fan-out: %d)", bn[0].ID, bn[0].FanOut)
	}

	// Critical path should exist.
	cp := g.CriticalPath()
	if len(cp) > 0 {
		t.Logf("Critical path length: %d, start: %s, end: %s", len(cp), cp[0].ID, cp[len(cp)-1].ID)
	}

	// Parallelizable groups.
	groups := g.ParallelizableGroups()
	t.Logf("Parallelizable groups: %d", len(groups))
	if len(groups) > 0 {
		t.Logf("First group size: %d", len(groups[0]))
	}

	// Visualization should produce substantial output.
	viz := g.Visualize()
	if len(viz) < 100 {
		t.Errorf("visualization too short for real roadmap: %d chars", len(viz))
	}
}

func TestDepGraph_Depth(t *testing.T) {
	t.Parallel()

	// A(depth 0) -> B(depth 1) -> C(depth 2)
	tasks := []Task{
		{ID: "A", Description: "Root"},
		{ID: "B", Description: "Mid", DependsOn: []string{"A"}},
		{ID: "C", Description: "Leaf", DependsOn: []string{"B"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Depth")
	g := BuildGraph(rm)

	if g.Nodes["A"].Depth != 0 {
		t.Errorf("A depth = %d, want 0", g.Nodes["A"].Depth)
	}
	if g.Nodes["B"].Depth != 1 {
		t.Errorf("B depth = %d, want 1", g.Nodes["B"].Depth)
	}
	if g.Nodes["C"].Depth != 2 {
		t.Errorf("C depth = %d, want 2", g.Nodes["C"].Depth)
	}
}

func TestDepGraph_DiamondDepth(t *testing.T) {
	t.Parallel()

	// Diamond: D's depth should be 2 (longest path through A->B->D or A->C->D).
	tasks := []Task{
		{ID: "A", Description: "Root"},
		{ID: "B", Description: "Left", DependsOn: []string{"A"}},
		{ID: "C", Description: "Right", DependsOn: []string{"A"}},
		{ID: "D", Description: "Sink", DependsOn: []string{"B", "C"}},
	}
	rm := buildTestRoadmap(tasks, "Phase 1", "Diamond")
	g := BuildGraph(rm)

	if g.Nodes["D"].Depth != 2 {
		t.Errorf("D depth = %d, want 2", g.Nodes["D"].Depth)
	}
}

// nodeIDs extracts IDs from a slice of Nodes.
func nodeIDs(nodes []Node) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

// indexOf finds the position of a node with the given ID.
func indexOf(nodes []Node, id string) int {
	for i, n := range nodes {
		if n.ID == id {
			return i
		}
	}
	return -1
}
