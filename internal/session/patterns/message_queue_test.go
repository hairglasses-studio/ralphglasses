package patterns

import (
	"os"
	"path/filepath"
	"testing"
)

func tempDBPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test_mq.db")
}

func TestMessageQueueNewClose(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	if err := mq.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestMessageQueueSendReceiveAck(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	// Send a task assignment from orchestrator to worker-1.
	env, err := MarshalEnvelope("msg-1", "orchestrator", "worker-1", TaskAssignment{
		TaskID:      "t1",
		Description: "build API",
		Priority:    5,
	})
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}

	if err := mq.Send(env); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Receive as worker-1.
	msgs, err := mq.Receive("worker-1", 10)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	if msgs[0].From != "orchestrator" || msgs[0].To != "worker-1" {
		t.Errorf("routing: from=%q to=%q", msgs[0].From, msgs[0].To)
	}
	if msgs[0].Type != MsgTaskAssignment {
		t.Errorf("type = %q, want task_assignment", msgs[0].Type)
	}

	// Decode the payload.
	decoded, err := msgs[0].DecodePayload()
	if err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	ta, ok := decoded.(*TaskAssignment)
	if !ok {
		t.Fatalf("payload type = %T, want *TaskAssignment", decoded)
	}
	if ta.TaskID != "t1" || ta.Description != "build API" {
		t.Errorf("payload = %+v", ta)
	}

	// Ack the message (use DB row ID = 1 since it's the first insert).
	if err := mq.Ack(1); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// After ack, receive should return nothing.
	msgs, err = mq.Receive("worker-1", 10)
	if err != nil {
		t.Fatalf("Receive after ack: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("got %d messages after ack, want 0", len(msgs))
	}
}

func TestMessageQueueBroadcast(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	// Broadcast message (to = "").
	env, err := MarshalEnvelope("msg-b", "orchestrator", "", MemoryUpdate{
		Key: "status", Value: "ready", Revision: 1,
	})
	if err != nil {
		t.Fatalf("MarshalEnvelope: %v", err)
	}
	if err := mq.Send(env); err != nil {
		t.Fatalf("Send broadcast: %v", err)
	}

	// Any session should receive broadcast messages.
	msgs, err := mq.Receive("worker-1", 10)
	if err != nil {
		t.Fatalf("Receive worker-1: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("worker-1 got %d, want 1", len(msgs))
	}

	msgs, err = mq.Receive("worker-2", 10)
	if err != nil {
		t.Fatalf("Receive worker-2: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("worker-2 got %d, want 1", len(msgs))
	}
}

func TestMessageQueueReceiveAll(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	// Send messages to different recipients.
	for _, to := range []string{"w1", "w2", ""} {
		env, _ := MarshalEnvelope("id", "orch", to, TaskAssignment{TaskID: "t"})
		if err := mq.Send(env); err != nil {
			t.Fatalf("Send to=%q: %v", to, err)
		}
	}

	msgs, err := mq.ReceiveAll(100)
	if err != nil {
		t.Fatalf("ReceiveAll: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("got %d messages, want 3", len(msgs))
	}
}

func TestMessageQueueReceiveAllLimit(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	for range 5 {
		env, _ := MarshalEnvelope("id", "orch", "w1", TaskAssignment{TaskID: "t"})
		mq.Send(env)
	}

	msgs, err := mq.ReceiveAll(2)
	if err != nil {
		t.Fatalf("ReceiveAll: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("got %d messages, want 2 (limit)", len(msgs))
	}
}

func TestMessageQueuePending(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	// No messages yet.
	count, err := mq.Pending("worker-1")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 0 {
		t.Errorf("pending = %d, want 0", count)
	}

	// Send 3 messages: 2 to worker-1, 1 broadcast.
	for range 2 {
		env, _ := MarshalEnvelope("id", "orch", "worker-1", TaskAssignment{TaskID: "t"})
		mq.Send(env)
	}
	env, _ := MarshalEnvelope("id", "orch", "", TaskAssignment{TaskID: "t"})
	mq.Send(env)

	count, err = mq.Pending("worker-1")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if count != 3 {
		t.Errorf("pending = %d, want 3 (2 direct + 1 broadcast)", count)
	}

	// Ack one direct message.
	mq.Ack(1)
	count, _ = mq.Pending("worker-1")
	if count != 2 {
		t.Errorf("pending after ack = %d, want 2", count)
	}
}

func TestMessageQueueReceiveLimit(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	for range 10 {
		env, _ := MarshalEnvelope("id", "orch", "w1", TaskAssignment{TaskID: "t"})
		mq.Send(env)
	}

	msgs, err := mq.Receive("w1", 3)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("got %d messages, want 3 (limit)", len(msgs))
	}
}

func TestMessageQueueDirectNotVisibleToOther(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	// Send a direct message to worker-1.
	env, _ := MarshalEnvelope("id", "orch", "worker-1", TaskAssignment{TaskID: "t"})
	mq.Send(env)

	// worker-2 should not see worker-1's direct messages.
	msgs, err := mq.Receive("worker-2", 10)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("worker-2 got %d messages meant for worker-1", len(msgs))
	}
}

func TestMessageQueueAckNonExistent(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	err = mq.Ack(999)
	if err == nil {
		t.Error("Ack(999) should return error for non-existent message")
	}
}

func TestMessageQueueAllMessageTypes(t *testing.T) {
	mq, err := NewMessageQueue(tempDBPath(t))
	if err != nil {
		t.Fatalf("NewMessageQueue: %v", err)
	}
	defer mq.Close()

	cases := []struct {
		name string
		msg  any
		want MessageType
	}{
		{"TaskAssignment", TaskAssignment{TaskID: "t1"}, MsgTaskAssignment},
		{"ReviewRequest", ReviewRequest{TaskID: "t2", Content: "code"}, MsgReviewRequest},
		{"ReviewResponse", ReviewResponse{TaskID: "t3", Verdict: VerdictApproved, Score: 0.9}, MsgReviewResponse},
		{"MemoryUpdate", MemoryUpdate{Key: "k", Value: "v", Revision: 1}, MsgMemoryUpdate},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, err := MarshalEnvelope("id", "sender", "receiver", tc.msg)
			if err != nil {
				t.Fatalf("MarshalEnvelope: %v", err)
			}
			if err := mq.Send(env); err != nil {
				t.Fatalf("Send: %v", err)
			}
		})
	}

	msgs, err := mq.Receive("receiver", 100)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}
	for i, tc := range cases {
		if msgs[i].Type != tc.want {
			t.Errorf("msg[%d] type = %q, want %q", i, msgs[i].Type, tc.want)
		}
	}
}

func TestMessageQueueCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	deepPath := filepath.Join(dir, "sub", "dir", "mq.db")
	mq, err := NewMessageQueue(deepPath)
	if err != nil {
		t.Fatalf("NewMessageQueue with nested dir: %v", err)
	}
	defer mq.Close()

	// Verify the directory was created.
	if _, err := os.Stat(filepath.Dir(deepPath)); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}
