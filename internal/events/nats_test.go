package events

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- Mock infrastructure ---

// mockMsg implements NATSMsg for testing.
type mockMsg struct {
	data    []byte
	subject string
	seq     uint64
	acked   bool
	naked   bool
	mu      sync.Mutex
}

func (m *mockMsg) Data() []byte     { return m.data }
func (m *mockMsg) Subject() string  { return m.subject }
func (m *mockMsg) Sequence() uint64 { return m.seq }

func (m *mockMsg) Ack() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.acked = true
	return nil
}

func (m *mockMsg) Nak() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.naked = true
	return nil
}

func (m *mockMsg) wasAcked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.acked
}

func (m *mockMsg) wasNaked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.naked
}

// mockSub implements NATSSubscription for testing.
type mockSub struct {
	stopped bool
	mu      sync.Mutex
}

func (s *mockSub) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
}

func (s *mockSub) isStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

// mockConn implements NATSConnection for testing. It records published
// messages and allows tests to deliver them to subscribers.
type mockConn struct {
	mu          sync.Mutex
	seq         uint64
	published   []mockPublished
	subscribers []mockSubscriber
	closed      bool
	publishErr  error // when non-nil, Publish returns this error
}

type mockPublished struct {
	subject string
	data    []byte
	seq     uint64
}

type mockSubscriber struct {
	subject      string
	consumerName string
	startSeq     uint64
	handler      func(NATSMsg)
	sub          *mockSub
}

func newMockConn() *mockConn {
	return &mockConn{}
}

func (c *mockConn) Publish(_ context.Context, subject string, data []byte) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, fmt.Errorf("connection closed")
	}
	if c.publishErr != nil {
		return 0, c.publishErr
	}

	c.seq++
	c.published = append(c.published, mockPublished{
		subject: subject,
		data:    data,
		seq:     c.seq,
	})

	// Deliver to matching subscribers.
	seq := c.seq
	for _, s := range c.subscribers {
		if !s.sub.isStopped() && subjectMatches(s.subject, subject) {
			msg := &mockMsg{data: data, subject: subject, seq: seq}
			go s.handler(msg)
		}
	}

	return seq, nil
}

func (c *mockConn) Subscribe(_ context.Context, subject string, consumerName string, startSeq uint64, handler func(NATSMsg)) (NATSSubscription, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("connection closed")
	}

	sub := &mockSub{}
	c.subscribers = append(c.subscribers, mockSubscriber{
		subject:      subject,
		consumerName: consumerName,
		startSeq:     startSeq,
		handler:      handler,
		sub:          sub,
	})

	// Replay stored messages from startSeq if requested.
	if startSeq > 0 {
		for _, p := range c.published {
			if p.seq >= startSeq && subjectMatches(subject, p.subject) {
				msg := &mockMsg{data: p.data, subject: p.subject, seq: p.seq}
				go handler(msg)
			}
		}
	}

	return sub, nil
}

func (c *mockConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// subjectMatches checks if a NATS subject pattern matches a concrete subject.
// Supports the ">" wildcard at the end (matches any number of tokens).
func subjectMatches(pattern, subject string) bool {
	if pattern == subject {
		return true
	}
	// "foo.>" matches "foo.bar", "foo.bar.baz", etc.
	if len(pattern) > 1 && pattern[len(pattern)-1] == '>' {
		prefix := pattern[:len(pattern)-1] // includes the trailing dot
		return len(subject) >= len(prefix) && subject[:len(prefix)] == prefix
	}
	return false
}

// --- Tests ---

func TestNATSBus_PublishSubscribe(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	ch, err := bus.Subscribe(context.Background(), "test-sub", nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	event := Event{Type: SessionStarted, RepoName: "repo1"}
	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case e := <-ch:
		if e.Type != SessionStarted {
			t.Errorf("type = %q, want %q", e.Type, SessionStarted)
		}
		if e.RepoName != "repo1" {
			t.Errorf("repo = %q, want repo1", e.RepoName)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestNATSBus_FilteredSubscribe(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	// Only accept cost events.
	filter := func(e Event) bool { return e.Type == CostUpdate }
	ch, err := bus.Subscribe(context.Background(), "cost-watcher", filter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Publish a session event (should be filtered out) and a cost event.
	_ = bus.Publish(context.Background(), Event{Type: SessionStarted})
	_ = bus.Publish(context.Background(), Event{Type: CostUpdate, Data: map[string]any{"amount": 1.5}})

	select {
	case e := <-ch:
		if e.Type != CostUpdate {
			t.Errorf("expected CostUpdate, got %q", e.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cost event")
	}

	// Ensure no session event leaked through.
	select {
	case e := <-ch:
		t.Errorf("unexpected event: %q", e.Type)
	case <-time.After(100 * time.Millisecond):
		// Good — no extra events.
	}
}

func TestNATSBus_ConsumerGroups(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	ch1, err := bus.Subscribe(context.Background(), "group-a", nil)
	if err != nil {
		t.Fatalf("Subscribe group-a: %v", err)
	}
	ch2, err := bus.Subscribe(context.Background(), "group-b", nil)
	if err != nil {
		t.Fatalf("Subscribe group-b: %v", err)
	}

	_ = bus.Publish(context.Background(), Event{Type: LoopStarted})

	// Both consumer groups should receive the event.
	for _, pair := range []struct {
		name string
		ch   <-chan Event
	}{
		{"group-a", ch1},
		{"group-b", ch2},
	} {
		select {
		case e := <-pair.ch:
			if e.Type != LoopStarted {
				t.Errorf("%s: type = %q, want loop.started", pair.name, e.Type)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: timeout", pair.name)
		}
	}
}

func TestNATSBus_DuplicateSubscribe(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	ch1, err := bus.Subscribe(context.Background(), "dup", nil)
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}

	ch2, err := bus.Subscribe(context.Background(), "dup", nil)
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	// Should return the same channel.
	if fmt.Sprintf("%p", ch1) != fmt.Sprintf("%p", ch2) {
		t.Error("duplicate Subscribe should return the same channel")
	}
}

func TestNATSBus_Unsubscribe(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	ch, err := bus.Subscribe(context.Background(), "ephemeral", nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	bus.Unsubscribe("ephemeral")

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after Unsubscribe")
	}

	// Unsubscribing again should not panic.
	bus.Unsubscribe("ephemeral")
}

func TestNATSBus_ReplayFromSequence(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	// Publish 3 events before subscribing.
	_ = bus.Publish(context.Background(), Event{Type: SessionStarted, SessionID: "s1"})
	_ = bus.Publish(context.Background(), Event{Type: CostUpdate, SessionID: "s2"})
	_ = bus.Publish(context.Background(), Event{Type: LoopStarted, SessionID: "s3"})

	// Subscribe with replay from sequence 2 (should get events 2 and 3).
	ch, err := bus.SubscribeFrom(context.Background(), "replayer", 2, nil)
	if err != nil {
		t.Fatalf("SubscribeFrom: %v", err)
	}

	seen := make(map[string]bool)
	timeout := time.After(2 * time.Second)
	for i := range 2 {
		select {
		case e := <-ch:
			seen[e.SessionID] = true
		case <-timeout:
			t.Fatalf("timeout waiting for replayed event %d", i)
		}
	}

	if len(seen) != 2 {
		t.Fatalf("expected 2 distinct replayed events, got %d", len(seen))
	}
	if !seen["s2"] {
		t.Error("expected replayed event with SessionID s2")
	}
	if !seen["s3"] {
		t.Error("expected replayed event with SessionID s3")
	}

	// Ensure s1 (seq 1, before startSeq 2) was NOT replayed.
	select {
	case e := <-ch:
		t.Errorf("unexpected extra event: session=%q", e.SessionID)
	case <-time.After(100 * time.Millisecond):
		// Good.
	}
}

func TestNATSBus_FleetPartitioning(t *testing.T) {
	conn := newMockConn()

	fleet1 := NewNATSBus(conn, NATSBusConfig{FleetID: "fleet-alpha"})
	fleet2 := NewNATSBus(conn, NATSBusConfig{FleetID: "fleet-beta"})

	// Verify subjects are partitioned.
	s1 := fleet1.SubjectFor(SessionStarted)
	s2 := fleet2.SubjectFor(SessionStarted)

	if s1 == s2 {
		t.Errorf("fleet subjects should differ: fleet1=%q fleet2=%q", s1, s2)
	}
	if s1 != "ralph.events.fleet-alpha.session.started" {
		t.Errorf("fleet1 subject = %q, want ralph.events.fleet-alpha.session.started", s1)
	}
	if s2 != "ralph.events.fleet-beta.session.started" {
		t.Errorf("fleet2 subject = %q, want ralph.events.fleet-beta.session.started", s2)
	}

	// Verify FleetID accessor.
	if fleet1.FleetID() != "fleet-alpha" {
		t.Errorf("FleetID = %q, want fleet-alpha", fleet1.FleetID())
	}
}

func TestNATSBus_FleetIsolation(t *testing.T) {
	conn := newMockConn()

	fleet1 := NewNATSBus(conn, NATSBusConfig{FleetID: "alpha"})
	fleet2 := NewNATSBus(conn, NATSBusConfig{FleetID: "beta"})

	ch1, _ := fleet1.Subscribe(context.Background(), "alpha-sub", nil)
	ch2, _ := fleet2.Subscribe(context.Background(), "beta-sub", nil)

	// Publish to fleet1 only.
	_ = fleet1.Publish(context.Background(), Event{Type: SessionStarted, SessionID: "alpha-only"})

	// fleet1's subscriber should receive it.
	select {
	case e := <-ch1:
		if e.SessionID != "alpha-only" {
			t.Errorf("fleet1 got session = %q, want alpha-only", e.SessionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fleet1 subscriber timeout")
	}

	// fleet2's subscriber should NOT receive it (different subject partition).
	select {
	case e := <-ch2:
		t.Errorf("fleet2 should not receive fleet1 event, got session=%q", e.SessionID)
	case <-time.After(200 * time.Millisecond):
		// Good — isolated.
	}
}

func TestNATSBus_PublishError(t *testing.T) {
	conn := newMockConn()
	conn.publishErr = fmt.Errorf("simulated NATS failure")

	bus := NewNATSBus(conn, NATSBusConfig{})

	err := bus.Publish(context.Background(), Event{Type: SessionStarted})
	if err == nil {
		t.Fatal("expected error from publish")
	}
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

func TestNATSBus_PublishAfterClose(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})
	_ = bus.Close()

	err := bus.Publish(context.Background(), Event{Type: SessionStarted})
	if err == nil {
		t.Fatal("expected error publishing after close")
	}
}

func TestNATSBus_SubscribeAfterClose(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})
	_ = bus.Close()

	_, err := bus.Subscribe(context.Background(), "late", nil)
	if err == nil {
		t.Fatal("expected error subscribing after close")
	}
}

func TestNATSBus_CloseIdempotent(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	if err := bus.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := bus.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestNATSBus_LastSequence(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	if seq := bus.LastSequence(); seq != 0 {
		t.Errorf("initial LastSequence = %d, want 0", seq)
	}

	_ = bus.Publish(context.Background(), Event{Type: SessionStarted})
	_ = bus.Publish(context.Background(), Event{Type: SessionEnded})

	if seq := bus.LastSequence(); seq != 2 {
		t.Errorf("LastSequence after 2 publishes = %d, want 2", seq)
	}
}

func TestNATSBus_CloseStopsConsumers(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	ch, _ := bus.Subscribe(context.Background(), "will-close", nil)
	_ = bus.Close()

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("subscriber channel should be closed after bus Close")
	}
}

func TestNATSBus_DefaultTimestampAndVersion(t *testing.T) {
	conn := newMockConn()
	bus := NewNATSBus(conn, NATSBusConfig{})

	ch, _ := bus.Subscribe(context.Background(), "defaults", nil)

	before := time.Now()
	_ = bus.Publish(context.Background(), Event{Type: CostUpdate})

	select {
	case e := <-ch:
		if e.Timestamp.Before(before) {
			t.Error("timestamp should be >= publish time")
		}
		if e.Version != 1 {
			t.Errorf("version = %d, want 1", e.Version)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestNATSBus_SubjectMapping(t *testing.T) {
	tests := []struct {
		name    string
		fleetID string
		et      EventType
		want    string
	}{
		{"no fleet", "", SessionStarted, "ralph.events.session.started"},
		{"with fleet", "prod", SessionStarted, "ralph.events.prod.session.started"},
		{"with fleet cost", "staging", CostUpdate, "ralph.events.staging.cost.update"},
		{"nested event type", "", LoopIterated, "ralph.events.loop.iterated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := NewNATSBus(newMockConn(), NATSBusConfig{FleetID: tt.fleetID})
			got := bus.SubjectFor(tt.et)
			if got != tt.want {
				t.Errorf("SubjectFor(%q) = %q, want %q", tt.et, got, tt.want)
			}
		})
	}
}

func TestNATSBusConfig_Defaults(t *testing.T) {
	cfg := NATSBusConfig{}
	cfg.defaults()

	if cfg.SubjectPrefix != "ralph.events" {
		t.Errorf("SubjectPrefix = %q, want ralph.events", cfg.SubjectPrefix)
	}
	if cfg.ChannelBuffer != 100 {
		t.Errorf("ChannelBuffer = %d, want 100", cfg.ChannelBuffer)
	}
}

func TestNATSBusConfig_DefaultsPreserveExplicit(t *testing.T) {
	cfg := NATSBusConfig{
		SubjectPrefix: "custom",
		ChannelBuffer: 50,
		FleetID:       "my-fleet",
	}
	cfg.defaults()

	if cfg.SubjectPrefix != "custom" {
		t.Errorf("SubjectPrefix = %q, want custom", cfg.SubjectPrefix)
	}
	if cfg.ChannelBuffer != 50 {
		t.Errorf("ChannelBuffer = %d, want 50", cfg.ChannelBuffer)
	}
	if cfg.FleetID != "my-fleet" {
		t.Errorf("FleetID = %q, want my-fleet", cfg.FleetID)
	}
}

// TestSubjectMatches validates the test helper used by mockConn.
func TestSubjectMatches(t *testing.T) {
	tests := []struct {
		pattern string
		subject string
		want    bool
	}{
		{"ralph.events.>", "ralph.events.session.started", true},
		{"ralph.events.>", "ralph.events.cost.update", true},
		{"ralph.events.>", "other.prefix.session.started", false},
		{"ralph.events.alpha.>", "ralph.events.alpha.session.started", true},
		{"ralph.events.alpha.>", "ralph.events.beta.session.started", false},
		{"exact.match", "exact.match", true},
		{"exact.match", "exact.mismatch", false},
	}

	for _, tt := range tests {
		got := subjectMatches(tt.pattern, tt.subject)
		if got != tt.want {
			t.Errorf("subjectMatches(%q, %q) = %v, want %v", tt.pattern, tt.subject, got, tt.want)
		}
	}
}

// TestMockConnInterfaces verifies the mock types satisfy their interfaces.
var (
	_ NATSConnection   = (*mockConn)(nil)
	_ NATSMsg          = (*mockMsg)(nil)
	_ NATSSubscription = (*mockSub)(nil)
)
