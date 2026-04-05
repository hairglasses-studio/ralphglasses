package session

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestDependencyGraph_AddTask(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()

	if err := g.AddTask("a", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := g.AddTask("b", []string{"a"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duplicate.
	err := g.AddTask("a", nil)
	if !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("expected ErrDuplicateTask, got %v", err)
	}
}

func TestDependencyGraph_TopologicalSort_Linear(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("c", []string{"b"})
	_ = g.AddTask("b", []string{"a"})
	_ = g.AddTask("a", nil)

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"a", "b", "c"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d tasks, got %d: %v", len(expected), len(order), order)
	}
	for i, id := range expected {
		if order[i] != id {
			t.Errorf("order[%d] = %q, want %q", i, order[i], id)
		}
	}
}

func TestDependencyGraph_TopologicalSort_Diamond(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", nil)
	_ = g.AddTask("b", []string{"a"})
	_ = g.AddTask("c", []string{"a"})
	_ = g.AddTask("d", []string{"b", "c"})

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("expected 4 tasks, got %d: %v", len(order), order)
	}

	// a must come before b, c; d must come last.
	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	if pos["a"] >= pos["b"] || pos["a"] >= pos["c"] {
		t.Errorf("a must precede b and c: %v", order)
	}
	if pos["b"] >= pos["d"] || pos["c"] >= pos["d"] {
		t.Errorf("b and c must precede d: %v", order)
	}
}

func TestDependencyGraph_TopologicalSort_Cycle(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", []string{"c"})
	_ = g.AddTask("b", []string{"a"})
	_ = g.AddTask("c", []string{"b"})

	_, err := g.TopologicalSort()
	if !errors.Is(err, ErrCyclicDependency) {
		t.Fatalf("expected ErrCyclicDependency, got %v", err)
	}
}

func TestDependencyGraph_TopologicalSort_Empty(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 0 {
		t.Fatalf("expected empty order, got %v", order)
	}
}

func TestDependencyGraph_TopologicalSort_MissingDep(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", []string{"external"}) // external not in graph

	order, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "a" {
		t.Fatalf("expected [a], got %v", order)
	}
}

func TestDependencyGraph_DetectCycles_NoCycle(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", nil)
	_ = g.AddTask("b", []string{"a"})

	cycles := g.DetectCycles()
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %v", cycles)
	}
}

func TestDependencyGraph_DetectCycles_WithCycle(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", []string{"b"})
	_ = g.AddTask("b", []string{"a"})

	cycles := g.DetectCycles()
	if len(cycles) == 0 {
		t.Fatal("expected at least one cycle")
	}
	// The cycle should contain both a and b.
	found := make(map[string]bool)
	for _, c := range cycles {
		for _, id := range c {
			found[id] = true
		}
	}
	if !found["a"] || !found["b"] {
		t.Errorf("expected cycle containing a and b, got %v", cycles)
	}
}

func TestDependencyGraph_ReadyTasks(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", nil)
	_ = g.AddTask("b", []string{"a"})
	_ = g.AddTask("c", []string{"a"})
	_ = g.AddTask("d", []string{"b", "c"})

	// Nothing completed: only a is ready.
	ready := g.ReadyTasks(map[string]bool{})
	if len(ready) != 1 || ready[0] != "a" {
		t.Fatalf("expected [a], got %v", ready)
	}

	// a completed: b and c are ready.
	ready = g.ReadyTasks(map[string]bool{"a": true})
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready tasks, got %v", ready)
	}
	if ready[0] != "b" || ready[1] != "c" {
		t.Errorf("expected [b c], got %v", ready)
	}

	// a, b completed: c is ready, d is not (c not done).
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true})
	if len(ready) != 1 || ready[0] != "c" {
		t.Fatalf("expected [c], got %v", ready)
	}

	// All but d completed: d is ready.
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true, "c": true})
	if len(ready) != 1 || ready[0] != "d" {
		t.Fatalf("expected [d], got %v", ready)
	}

	// All completed: nothing ready.
	ready = g.ReadyTasks(map[string]bool{"a": true, "b": true, "c": true, "d": true})
	if len(ready) != 0 {
		t.Fatalf("expected no ready tasks, got %v", ready)
	}
}

func TestDependencyGraph_CriticalPath_Linear(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", nil)
	_ = g.AddTask("b", []string{"a"})
	_ = g.AddTask("c", []string{"b"})

	path, err := g.CriticalPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []string{"a", "b", "c"}
	if len(path) != len(expected) {
		t.Fatalf("expected path %v, got %v", expected, path)
	}
	for i, id := range expected {
		if path[i] != id {
			t.Errorf("path[%d] = %q, want %q", i, path[i], id)
		}
	}
}

func TestDependencyGraph_CriticalPath_Diamond(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", nil)
	_ = g.AddTask("b", []string{"a"})
	_ = g.AddTask("c", []string{"a"})
	_ = g.AddTask("d", []string{"b", "c"})

	path, err := g.CriticalPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Length should be 3: a -> b|c -> d.
	if len(path) != 3 {
		t.Fatalf("expected path length 3, got %d: %v", len(path), path)
	}
	if path[0] != "a" {
		t.Errorf("path[0] = %q, want a", path[0])
	}
	if path[len(path)-1] != "d" {
		t.Errorf("path[last] = %q, want d", path[len(path)-1])
	}
}

func TestDependencyGraph_CriticalPath_Empty(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()

	path, err := g.CriticalPath()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(path) != 0 {
		t.Fatalf("expected empty path, got %v", path)
	}
}

func TestDependencyGraph_CriticalPath_Cycle(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()
	_ = g.AddTask("a", []string{"b"})
	_ = g.AddTask("b", []string{"a"})

	_, err := g.CriticalPath()
	if !errors.Is(err, ErrCyclicDependency) {
		t.Fatalf("expected ErrCyclicDependency, got %v", err)
	}
}

func TestDependencyGraph_Concurrent(t *testing.T) {
	t.Parallel()
	g := NewDependencyGraph()

	var wg sync.WaitGroup
	// Add tasks concurrently.
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("task-%03d", n)
			var deps []string
			if n > 0 {
				deps = []string{fmt.Sprintf("task-%03d", n-1)}
			}
			_ = g.AddTask(id, deps)
		}(i)
	}
	wg.Wait()

	// Read operations concurrent with nothing else — just ensure no panic.
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, _ = g.TopologicalSort()
	}()
	go func() {
		defer wg.Done()
		_ = g.DetectCycles()
	}()
	go func() {
		defer wg.Done()
		_ = g.ReadyTasks(map[string]bool{"task-000": true})
	}()
	wg.Wait()
}

func TestSortStrings(t *testing.T) {
	t.Parallel()
	s := []string{"c", "a", "b", "a"}
	sortStrings(s)
	expected := []string{"a", "a", "b", "c"}
	for i, v := range expected {
		if s[i] != v {
			t.Errorf("s[%d] = %q, want %q", i, s[i], v)
		}
	}
}
