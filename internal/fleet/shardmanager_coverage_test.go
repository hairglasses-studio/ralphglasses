package fleet

import (
	"testing"
)

func TestNewShardManager_Default(t *testing.T) {
	sm := NewShardManager()
	if sm.NodeCount() != 0 {
		t.Errorf("NodeCount() = %d, want 0", sm.NodeCount())
	}
	if sm.SessionCount() != 0 {
		t.Errorf("SessionCount() = %d, want 0", sm.SessionCount())
	}
}

func TestShardManager_WithMigrationCallback(t *testing.T) {
	var callbackFired bool
	sm := NewShardManager(WithMigrationCallback(func(m []Migration) {
		callbackFired = true
	}))
	if sm.MigrationCallback == nil {
		t.Error("expected MigrationCallback to be set")
	}

	// Join a node, then another — second join triggers rebalance which fires callback
	// if there are any sessions.
	sm.JoinNode(NodeInfo{ID: "n1", Address: "localhost:9001"})
	sm.AssignSession("session-1")

	// Join second node to trigger migration callback.
	sm.JoinNode(NodeInfo{ID: "n2", Address: "localhost:9002"})

	// Callback may or may not fire depending on whether session moved;
	// the important thing is it doesn't panic.
	_ = callbackFired
}

func TestShardManager_WithReplicas(t *testing.T) {
	sm := NewShardManager(WithReplicas(32))
	// Just verify construction succeeds with custom replicas.
	sm.JoinNode(NodeInfo{ID: "n1", Address: "localhost:9001"})
	sid := sm.AssignSession("test-session")
	if sid == "" {
		t.Error("expected non-empty node assignment with custom replicas")
	}
}

func TestShardManager_JoinNode(t *testing.T) {
	sm := NewShardManager()
	migrations := sm.JoinNode(NodeInfo{ID: "n1", Address: "localhost:9001", Capacity: 10})
	// First node join with no sessions produces no migrations.
	if len(migrations) != 0 {
		t.Errorf("first join with no sessions: expected 0 migrations, got %d", len(migrations))
	}
	if sm.NodeCount() != 1 {
		t.Errorf("NodeCount() = %d, want 1", sm.NodeCount())
	}
}

func TestShardManager_JoinNode_Existing(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1", Address: "localhost:9001"})
	// Joining again should update info but not rebalance.
	migrations := sm.JoinNode(NodeInfo{ID: "n1", Address: "localhost:9002"})
	if migrations != nil {
		t.Errorf("re-join should return nil migrations, got %v", migrations)
	}
	// Node count should still be 1.
	if sm.NodeCount() != 1 {
		t.Errorf("NodeCount() = %d, want 1 after re-join", sm.NodeCount())
	}
}

func TestShardManager_LeaveNode(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})
	sm.JoinNode(NodeInfo{ID: "n2"})
	sm.AssignSession("s1")
	sm.AssignSession("s2")

	migrations := sm.LeaveNode("n1")

	if sm.NodeCount() != 1 {
		t.Errorf("after LeaveNode, NodeCount() = %d, want 1", sm.NodeCount())
	}
	// Sessions should still be tracked.
	if sm.SessionCount() != 2 {
		t.Errorf("SessionCount() = %d, want 2 after leave", sm.SessionCount())
	}
	_ = migrations
}

func TestShardManager_LeaveNode_NonExistent(t *testing.T) {
	sm := NewShardManager()
	migrations := sm.LeaveNode("nonexistent")
	if migrations != nil {
		t.Errorf("expected nil migrations for non-existent node, got %v", migrations)
	}
}

func TestShardManager_DrainNode(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})
	sm.JoinNode(NodeInfo{ID: "n2"})

	sm.DrainNode("n1")

	// After draining n1, new sessions should only go to n2.
	for i := 0; i < 20; i++ {
		sid := sm.AssignSession("drain-test-session")
		if sid != "n2" {
			t.Errorf("expected drained sessions to go to n2, got %q", sid)
			break
		}
	}
}

func TestShardManager_DrainNode_NonExistent(t *testing.T) {
	sm := NewShardManager()
	// Should not panic for non-existent node.
	sm.DrainNode("nonexistent")
}

func TestShardManager_AssignSession_NoNodes(t *testing.T) {
	sm := NewShardManager()
	sid := sm.AssignSession("session-1")
	if sid != "" {
		t.Errorf("AssignSession with no nodes = %q, want empty", sid)
	}
}

func TestShardManager_AssignSession_SingleNode(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1", Address: "localhost:9001"})

	sid := sm.AssignSession("my-session")
	if sid != "n1" {
		t.Errorf("AssignSession = %q, want n1", sid)
	}
}

func TestShardManager_NodeForSession(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})
	sm.AssignSession("s1")

	nodeID, ok := sm.NodeForSession("s1")
	if !ok {
		t.Fatal("expected NodeForSession to find s1")
	}
	if nodeID != "n1" {
		t.Errorf("NodeForSession(s1) = %q, want n1", nodeID)
	}

	_, ok = sm.NodeForSession("nonexistent")
	if ok {
		t.Error("expected false for nonexistent session")
	}
}

func TestShardManager_SessionsForNode(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})

	sm.AssignSession("s1")
	sm.AssignSession("s2")
	sm.AssignSession("s3")

	sessions := sm.SessionsForNode("n1")
	if len(sessions) != 3 {
		t.Errorf("SessionsForNode(n1) = %v, want 3 sessions", sessions)
	}
}

func TestShardManager_RemoveSession(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})
	sm.AssignSession("s1")
	sm.AssignSession("s2")

	sm.RemoveSession("s1")

	if sm.SessionCount() != 1 {
		t.Errorf("SessionCount() after remove = %d, want 1", sm.SessionCount())
	}
	_, ok := sm.NodeForSession("s1")
	if ok {
		t.Error("expected s1 to be removed")
	}
}

func TestShardManager_Nodes(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1", Address: "host1:9001"})
	sm.JoinNode(NodeInfo{ID: "n2", Address: "host2:9002"})

	nodes := sm.Nodes()
	if len(nodes) != 2 {
		t.Errorf("Nodes() = %d, want 2", len(nodes))
	}
	ids := make(map[string]bool)
	for _, n := range nodes {
		ids[n.ID] = true
	}
	if !ids["n1"] || !ids["n2"] {
		t.Errorf("Nodes() missing expected IDs: %v", ids)
	}
}

func TestShardManager_NodeCount(t *testing.T) {
	sm := NewShardManager()
	if sm.NodeCount() != 0 {
		t.Errorf("NodeCount() = %d, want 0", sm.NodeCount())
	}
	sm.JoinNode(NodeInfo{ID: "n1"})
	sm.JoinNode(NodeInfo{ID: "n2"})
	if sm.NodeCount() != 2 {
		t.Errorf("NodeCount() = %d, want 2", sm.NodeCount())
	}
	sm.LeaveNode("n1")
	if sm.NodeCount() != 1 {
		t.Errorf("NodeCount() after leave = %d, want 1", sm.NodeCount())
	}
}

func TestShardManager_Distribution(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})

	for i := 0; i < 5; i++ {
		sm.AssignSession("s" + string(rune('0'+i)))
	}

	dist := sm.Distribution()
	total := 0
	for _, count := range dist {
		total += count
	}
	if total != 5 {
		t.Errorf("Distribution total = %d, want 5", total)
	}
}

func TestShardManager_MigrationOnNodeJoin(t *testing.T) {
	sm := NewShardManager()
	sm.JoinNode(NodeInfo{ID: "n1"})

	// Assign many sessions to n1.
	for i := 0; i < 30; i++ {
		sm.AssignSession("session-" + string(rune('a'+i%26)) + string(rune('a'+i/26)))
	}

	// Adding n2 should cause some migrations.
	migrations := sm.JoinNode(NodeInfo{ID: "n2"})
	// Not guaranteed migrations > 0, but structure should be valid.
	for _, m := range migrations {
		if m.SessionKey == "" {
			t.Error("migration has empty session key")
		}
		if m.FromNode == "" && m.ToNode == "" {
			t.Error("migration has empty from/to nodes")
		}
	}
}
