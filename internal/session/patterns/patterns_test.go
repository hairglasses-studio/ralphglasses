package patterns

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// --- Protocol tests ---

func TestMarshalDecodeEnvelope(t *testing.T) {
	task := TaskAssignment{
		TaskID:      "t1",
		Description: "implement feature X",
		Priority:    5,
		SkillTags:   []string{"backend"},
	}
	env, err := MarshalEnvelope("msg-1", "arch", "exec-1", task)
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}
	if env.Type != MsgTaskAssignment {
		t.Errorf("type = %q, want %q", env.Type, MsgTaskAssignment)
	}
	if env.From != "arch" || env.To != "exec-1" {
		t.Errorf("routing: from=%q to=%q", env.From, env.To)
	}

	decoded, err := env.DecodePayload()
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	got, ok := decoded.(*TaskAssignment)
	if !ok {
		t.Fatalf("decoded type = %T, want *TaskAssignment", decoded)
	}
	if got.TaskID != "t1" || got.Description != "implement feature X" {
		t.Errorf("decoded task = %+v", got)
	}
}

func TestEnvelopeAllTypes(t *testing.T) {
	cases := []struct {
		name string
		msg  any
		want MessageType
	}{
		{"TaskAssignment", TaskAssignment{TaskID: "t"}, MsgTaskAssignment},
		{"ReviewRequest", ReviewRequest{TaskID: "t"}, MsgReviewRequest},
		{"ReviewResponse", ReviewResponse{TaskID: "t", Verdict: VerdictApproved}, MsgReviewResponse},
		{"MemoryUpdate", MemoryUpdate{Key: "k", Value: "v"}, MsgMemoryUpdate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := MarshalEnvelope("id", "from", "to", tc.msg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if env.Type != tc.want {
				t.Errorf("type = %q, want %q", env.Type, tc.want)
			}
			decoded, err := env.DecodePayload()
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded == nil {
				t.Fatal("decoded is nil")
			}
		})
	}
}

func TestEnvelopeJSON(t *testing.T) {
	env, _ := MarshalEnvelope("id1", "s1", "s2", TaskAssignment{TaskID: "t1"})
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var decoded Envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded.ID != "id1" || decoded.Type != MsgTaskAssignment {
		t.Errorf("roundtrip failed: %+v", decoded)
	}
}

func TestMarshalUnknownType(t *testing.T) {
	_, err := MarshalEnvelope("id", "from", "to", "not a message")
	if err != ErrUnknownMessageType {
		t.Errorf("err = %v, want ErrUnknownMessageType", err)
	}
}

// --- SharedMemory tests ---

func TestSharedMemorySetGet(t *testing.T) {
	sm := NewSharedMemory()
	rev := sm.Set("key1", "val1")
	if rev != 1 {
		t.Errorf("rev = %d, want 1", rev)
	}
	v, r, err := sm.Get("key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "val1" || r != 1 {
		t.Errorf("Get = (%q, %d), want (val1, 1)", v, r)
	}
}

func TestSharedMemoryGetMissing(t *testing.T) {
	sm := NewSharedMemory()
	_, _, err := sm.Get("nope")
	if err != ErrKeyNotFound {
		t.Errorf("err = %v, want ErrKeyNotFound", err)
	}
}

func TestSharedMemoryDelete(t *testing.T) {
	sm := NewSharedMemory()
	sm.Set("k", "v")
	if !sm.Delete("k") {
		t.Error("Delete returned false for existing key")
	}
	if sm.Delete("k") {
		t.Error("Delete returned true for already-deleted key")
	}
	_, _, err := sm.Get("k")
	if err != ErrKeyNotFound {
		t.Errorf("after delete, Get err = %v, want ErrKeyNotFound", err)
	}
}

func TestSharedMemoryKeys(t *testing.T) {
	sm := NewSharedMemory()
	sm.Set("a", "1")
	sm.Set("b", "2")
	keys := sm.Keys()
	if len(keys) != 2 {
		t.Fatalf("keys = %v, want 2 keys", keys)
	}
}

func TestSharedMemorySnapshot(t *testing.T) {
	sm := NewSharedMemory()
	sm.Set("x", "1")
	sm.Set("y", "2")
	snap := sm.Snapshot()
	if snap["x"] != "1" || snap["y"] != "2" {
		t.Errorf("snapshot = %v", snap)
	}
	// Mutating snapshot should not affect store.
	snap["x"] = "modified"
	v, _, _ := sm.Get("x")
	if v != "1" {
		t.Error("snapshot mutation leaked into store")
	}
}

func TestSharedMemoryWatch(t *testing.T) {
	sm := NewSharedMemory()
	ch := sm.Watch("key1", 10)
	sm.Set("key1", "hello")
	sm.Set("key1", "world")

	var updates []MemoryUpdate
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case u := <-ch:
			updates = append(updates, u)
			if len(updates) == 2 {
				goto done
			}
		case <-timeout:
			goto done
		}
	}
done:
	if len(updates) != 2 {
		t.Fatalf("got %d updates, want 2", len(updates))
	}
	if updates[0].Value != "hello" || updates[1].Value != "world" {
		t.Errorf("updates = %+v", updates)
	}
}

func TestSharedMemoryWatchDelete(t *testing.T) {
	sm := NewSharedMemory()
	ch := sm.Watch("k", 5)
	sm.Set("k", "v")
	sm.Delete("k")

	var updates []MemoryUpdate
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case u := <-ch:
			updates = append(updates, u)
			if len(updates) == 2 {
				goto done
			}
		case <-timeout:
			goto done
		}
	}
done:
	if len(updates) != 2 {
		t.Fatalf("got %d updates, want 2", len(updates))
	}
	if updates[1].Value != "" {
		t.Errorf("delete update value = %q, want empty", updates[1].Value)
	}
}

func TestSharedMemoryConcurrency(t *testing.T) {
	sm := NewSharedMemory()
	var wg sync.WaitGroup
	n := 100
	wg.Add(n * 2)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			sm.Set("counter", "value")
		}(i)
		go func(i int) {
			defer wg.Done()
			sm.Get("counter")
		}(i)
	}
	wg.Wait()
	// Just verifying no race condition panics.
}

// --- Architect Pattern tests ---

func TestArchitectPlanExecuteFlow(t *testing.T) {
	ap, err := NewArchitectPattern("arch-1", []string{"exec-1", "exec-2"}, nil)
	if err != nil {
		t.Fatalf("NewArchitectPattern: %v", err)
	}
	if ap.Phase() != PhasePlan {
		t.Errorf("initial phase = %q, want plan", ap.Phase())
	}
	if ap.ArchitectID() != "arch-1" {
		t.Errorf("architect = %q", ap.ArchitectID())
	}

	steps := []PlanStep{
		{ID: "s1", Description: "setup database"},
		{ID: "s2", Description: "implement API", DependsOn: []string{"s1"}},
		{ID: "s3", Description: "write tests", DependsOn: []string{"s2"}},
	}
	if err := ap.SetPlan(steps); err != nil {
		t.Fatalf("SetPlan: %v", err)
	}
	if ap.Phase() != PhaseExecute {
		t.Errorf("after SetPlan, phase = %q, want execute", ap.Phase())
	}

	// Only s1 should be ready (no deps).
	next := ap.NextTasks()
	if len(next) != 1 || next[0].ID != "s1" {
		t.Errorf("NextTasks = %v, want [s1]", next)
	}

	// Assign and complete s1.
	ap.AssignTask("s1", "exec-1")
	ap.CompleteTask("s1", "done")

	// Now s2 should be ready.
	next = ap.NextTasks()
	if len(next) != 1 || next[0].ID != "s2" {
		t.Errorf("after s1 complete, NextTasks = %v, want [s2]", next)
	}

	ap.AssignTask("s2", "exec-2")
	ap.CompleteTask("s2", "done")

	next = ap.NextTasks()
	if len(next) != 1 || next[0].ID != "s3" {
		t.Errorf("after s2 complete, NextTasks = %v, want [s3]", next)
	}

	ap.AssignTask("s3", "exec-1")
	ap.CompleteTask("s3", "done")

	if !ap.IsComplete() {
		t.Error("expected IsComplete=true after all steps done")
	}
	if ap.Phase() != PhaseDone {
		t.Errorf("final phase = %q, want done", ap.Phase())
	}
}

func TestArchitectNoExecutors(t *testing.T) {
	_, err := NewArchitectPattern("arch", nil, nil)
	if err != ErrNoExecutors {
		t.Errorf("err = %v, want ErrNoExecutors", err)
	}
}

func TestArchitectSetPlanOnlyInPlanPhase(t *testing.T) {
	ap, _ := NewArchitectPattern("arch", []string{"exec"}, nil)
	ap.SetPlan([]PlanStep{{ID: "s1"}})
	err := ap.SetPlan([]PlanStep{{ID: "s2"}})
	if err == nil {
		t.Error("expected error when setting plan in execute phase")
	}
}

func TestArchitectFailTask(t *testing.T) {
	ap, _ := NewArchitectPattern("arch", []string{"exec"}, nil)
	ap.SetPlan([]PlanStep{{ID: "s1"}, {ID: "s2"}})
	ap.FailTask("s1", "compile error")
	ap.CompleteTask("s2", "ok")
	if !ap.IsComplete() {
		t.Error("expected complete after all tasks resolved (even with failure)")
	}
}

func TestArchitectParallelTasks(t *testing.T) {
	ap, _ := NewArchitectPattern("arch", []string{"e1", "e2"}, nil)
	// Two independent steps should both be ready.
	ap.SetPlan([]PlanStep{
		{ID: "a", Description: "independent A"},
		{ID: "b", Description: "independent B"},
	})
	next := ap.NextTasks()
	if len(next) != 2 {
		t.Errorf("NextTasks = %d, want 2 parallel tasks", len(next))
	}
}

// --- Review Chain tests ---

func TestReviewChainOrdering(t *testing.T) {
	rc, err := NewReviewChainPattern("task-1", []string{"author", "rev1", "rev2"})
	if err != nil {
		t.Fatalf("NewReviewChainPattern: %v", err)
	}

	chain := rc.Chain()
	if len(chain) != 3 {
		t.Fatalf("chain len = %d, want 3", len(chain))
	}
	if chain[0].Role != RoleAuthor || chain[0].SessionID != "author" {
		t.Errorf("chain[0] = %+v", chain[0])
	}
	if chain[1].Role != RoleReviewer || chain[2].Role != RoleReviewer {
		t.Error("expected reviewer roles for chain[1] and chain[2]")
	}

	// Author produces content.
	rc.SetContent("initial implementation")
	if rc.Content() != "initial implementation" {
		t.Errorf("content = %q", rc.Content())
	}

	// First reviewer reviews.
	req, err := rc.BuildReviewRequest()
	if err != nil {
		t.Fatalf("BuildReviewRequest: %v", err)
	}
	if req.ChainStep != 1 {
		t.Errorf("chain_step = %d, want 1", req.ChainStep)
	}

	err = rc.SubmitReview(ReviewResponse{
		TaskID:  "task-1",
		Verdict: VerdictNeedsChanges,
		Score:   0.7,
	}, "revised implementation")
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}

	if rc.Content() != "revised implementation" {
		t.Errorf("content after review = %q", rc.Content())
	}
	if rc.IsDone() {
		t.Error("chain should not be done after first reviewer")
	}

	// Second reviewer approves.
	err = rc.SubmitReview(ReviewResponse{
		TaskID:  "task-1",
		Verdict: VerdictApproved,
		Score:   0.95,
	}, "")
	if err != nil {
		t.Fatalf("SubmitReview 2: %v", err)
	}

	if !rc.IsDone() {
		t.Error("chain should be done after all reviewers")
	}

	results := rc.Results()
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2", len(results))
	}
	if results[0].ReviewerID != "rev1" || results[1].ReviewerID != "rev2" {
		t.Errorf("reviewer ordering: %+v", results)
	}

	verdict, err := rc.FinalVerdict()
	if err != nil {
		t.Fatalf("FinalVerdict: %v", err)
	}
	if verdict != VerdictNeedsChanges {
		t.Errorf("verdict = %q, want needs_changes", verdict)
	}
}

func TestReviewChainTooFewSessions(t *testing.T) {
	_, err := NewReviewChainPattern("t", []string{"only-one"})
	if err != ErrEmptyChain {
		t.Errorf("err = %v, want ErrEmptyChain", err)
	}
}

func TestReviewChainRejection(t *testing.T) {
	rc, _ := NewReviewChainPattern("t", []string{"a", "r1", "r2"})
	rc.SetContent("code")
	rc.SubmitReview(ReviewResponse{Verdict: VerdictApproved, Score: 1.0}, "")
	rc.SubmitReview(ReviewResponse{Verdict: VerdictRejected, Score: 0.1}, "")
	v, _ := rc.FinalVerdict()
	if v != VerdictRejected {
		t.Errorf("verdict = %q, want rejected", v)
	}
}

func TestReviewChainAllApproved(t *testing.T) {
	rc, _ := NewReviewChainPattern("t", []string{"a", "r1", "r2"})
	rc.SetContent("code")
	rc.SubmitReview(ReviewResponse{Verdict: VerdictApproved, Score: 0.9}, "")
	rc.SubmitReview(ReviewResponse{Verdict: VerdictApproved, Score: 0.95}, "")
	v, _ := rc.FinalVerdict()
	if v != VerdictApproved {
		t.Errorf("verdict = %q, want approved", v)
	}
}

// --- Specialist / SkillRouter tests ---

func TestSkillRouterBasicRouting(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "fe-1", SkillTags: []string{"frontend", "css"}},
		{SessionID: "be-1", SkillTags: []string{"backend", "database"}},
		{SessionID: "test-1", SkillTags: []string{"testing", "backend"}},
	})

	matches, err := router.Route([]string{"backend"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(matches))
	}
	// be-1 should score 1.0 (1/1 tags), test-1 also 1.0 (1/1 tags).
	if matches[0].Score != 1.0 {
		t.Errorf("top score = %f, want 1.0", matches[0].Score)
	}
}

func TestSkillRouterPartialMatch(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "full", SkillTags: []string{"frontend", "backend", "testing"}},
		{SessionID: "fe", SkillTags: []string{"frontend"}},
	})

	matches, err := router.Route([]string{"frontend", "backend"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if matches[0].Specialist.SessionID != "full" {
		t.Errorf("best = %q, want full", matches[0].Specialist.SessionID)
	}
	if matches[0].Score != 1.0 {
		t.Errorf("full score = %f, want 1.0", matches[0].Score)
	}
	if matches[1].Score != 0.5 {
		t.Errorf("fe score = %f, want 0.5", matches[1].Score)
	}
}

func TestSkillRouterCapacity(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "busy", SkillTags: []string{"backend"}, Capacity: 1, Active: 1},
		{SessionID: "free", SkillTags: []string{"backend"}, Capacity: 2, Active: 0},
	})

	matches, err := router.Route([]string{"backend"})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(matches) != 1 || matches[0].Specialist.SessionID != "free" {
		t.Errorf("expected only free specialist, got %+v", matches)
	}
}

func TestSkillRouterNoMatch(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "fe", SkillTags: []string{"frontend"}},
	})
	_, err := router.Route([]string{"database"})
	if err != ErrNoMatchingSpecialist {
		t.Errorf("err = %v, want ErrNoMatchingSpecialist", err)
	}
}

func TestSkillRouterBest(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "a", SkillTags: []string{"go", "testing"}},
		{SessionID: "b", SkillTags: []string{"go", "testing", "backend"}},
	})
	m, err := router.Best([]string{"go", "testing", "backend"})
	if err != nil {
		t.Fatalf("Best: %v", err)
	}
	if m.Specialist.SessionID != "b" || m.Score != 1.0 {
		t.Errorf("best = %+v", m)
	}
}

func TestSkillRouterRegisterUnregister(t *testing.T) {
	router := NewSkillRouter(nil)
	router.Register(Specialist{SessionID: "s1", SkillTags: []string{"go"}})
	if len(router.Specialists()) != 1 {
		t.Fatal("expected 1 specialist after Register")
	}
	router.Unregister("s1")
	if len(router.Specialists()) != 0 {
		t.Fatal("expected 0 specialists after Unregister")
	}
}

func TestSkillRouterEmptyTags(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "any", SkillTags: []string{"go"}},
	})
	matches, err := router.Route(nil)
	if err != nil {
		t.Fatalf("Route with nil tags: %v", err)
	}
	if len(matches) != 1 || matches[0].Score != 1.0 {
		t.Errorf("expected all specialists with score 1.0, got %+v", matches)
	}
}

func TestSpecialistPatternRouteTask(t *testing.T) {
	sp := NewSpecialistPattern([]Specialist{
		{SessionID: "fe", SkillTags: []string{"frontend"}},
		{SessionID: "be", SkillTags: []string{"backend"}},
	}, nil)

	task := TaskAssignment{
		TaskID:      "t1",
		Description: "build API endpoint",
		SkillTags:   []string{"backend"},
	}
	match, env, err := sp.RouteTask("orchestrator", task)
	if err != nil {
		t.Fatalf("RouteTask: %v", err)
	}
	if match.Specialist.SessionID != "be" {
		t.Errorf("routed to %q, want be", match.Specialist.SessionID)
	}
	if env.To != "be" || env.Type != MsgTaskAssignment {
		t.Errorf("envelope = %+v", env)
	}
}

// --- Concurrency stress tests ---

func TestSkillRouterConcurrency(t *testing.T) {
	router := NewSkillRouter([]Specialist{
		{SessionID: "s1", SkillTags: []string{"go", "backend"}},
		{SessionID: "s2", SkillTags: []string{"go", "frontend"}},
	})
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			router.Route([]string{"go"})
		}()
		go func() {
			defer wg.Done()
			router.Register(Specialist{SessionID: "dynamic", SkillTags: []string{"go"}})
		}()
	}
	wg.Wait()
}

func TestArchitectConcurrency(t *testing.T) {
	ap, _ := NewArchitectPattern("arch", []string{"e1", "e2"}, nil)
	ap.SetPlan([]PlanStep{
		{ID: "s1"}, {ID: "s2"}, {ID: "s3"}, {ID: "s4"},
	})
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ap.NextTasks()
		}()
		go func() {
			defer wg.Done()
			ap.Plan()
		}()
	}
	wg.Wait()
}
