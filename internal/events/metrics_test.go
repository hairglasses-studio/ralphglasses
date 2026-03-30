package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMetrics_PublishCounting(t *testing.T) {
	bus := NewBus(100)

	bus.Publish(Event{Type: SessionStarted})
	bus.Publish(Event{Type: SessionStarted})
	bus.Publish(Event{Type: SessionEnded})
	bus.Publish(Event{Type: CostUpdate})

	m := bus.Metrics()
	if m.TotalPublished != 4 {
		t.Errorf("TotalPublished = %d, want 4", m.TotalPublished)
	}
	if m.PublishedByType[SessionStarted] != 2 {
		t.Errorf("PublishedByType[SessionStarted] = %d, want 2", m.PublishedByType[SessionStarted])
	}
	if m.PublishedByType[SessionEnded] != 1 {
		t.Errorf("PublishedByType[SessionEnded] = %d, want 1", m.PublishedByType[SessionEnded])
	}
	if m.PublishedByType[CostUpdate] != 1 {
		t.Errorf("PublishedByType[CostUpdate] = %d, want 1", m.PublishedByType[CostUpdate])
	}
}

func TestMetrics_SubscriberCount(t *testing.T) {
	bus := NewBus(100)

	m := bus.Metrics()
	if m.SubscriberCount != 0 {
		t.Errorf("initial SubscriberCount = %d, want 0", m.SubscriberCount)
	}

	bus.Subscribe("s1")
	bus.Subscribe("s2")
	bus.SubscribeFiltered("s3", SessionStarted)

	m = bus.Metrics()
	if m.SubscriberCount != 3 {
		t.Errorf("SubscriberCount after 3 subscribes = %d, want 3", m.SubscriberCount)
	}

	bus.Unsubscribe("s1")
	m = bus.Metrics()
	if m.SubscriberCount != 2 {
		t.Errorf("SubscriberCount after unsubscribe = %d, want 2", m.SubscriberCount)
	}

	bus.Unsubscribe("s2")
	bus.Unsubscribe("s3")
	m = bus.Metrics()
	if m.SubscriberCount != 0 {
		t.Errorf("SubscriberCount after all unsubscribed = %d, want 0", m.SubscriberCount)
	}
}

func TestMetrics_UnsubscribeNonexistent(t *testing.T) {
	bus := NewBus(100)
	bus.Subscribe("s1")

	// Unsubscribing a non-existent subscriber should not decrement.
	bus.Unsubscribe("does-not-exist")

	m := bus.Metrics()
	if m.SubscriberCount != 1 {
		t.Errorf("SubscriberCount = %d, want 1 (non-existent unsub should be no-op)", m.SubscriberCount)
	}
}

func TestMetrics_DroppedEvents(t *testing.T) {
	// Use a bus with a subscriber that has a tiny buffer.
	bus := NewBus(1000)

	// Subscribe creates a channel with buffer 100.
	// Publish 150 events without reading — should drop at least some.
	bus.Subscribe("slow")

	for i := 0; i < 150; i++ {
		bus.Publish(Event{Type: SessionStarted})
	}

	m := bus.Metrics()
	if m.TotalPublished != 150 {
		t.Errorf("TotalPublished = %d, want 150", m.TotalPublished)
	}
	// Channel buffer is 100, so at least 50 should be dropped.
	if m.TotalDropped < 50 {
		t.Errorf("TotalDropped = %d, want >= 50", m.TotalDropped)
	}
}

func TestMetrics_DroppedFilteredEvents(t *testing.T) {
	bus := NewBus(1000)

	// Filtered subscriber with buffer 100.
	bus.SubscribeFiltered("slow-filtered", SessionStarted)

	for i := 0; i < 150; i++ {
		bus.Publish(Event{Type: SessionStarted})
	}

	m := bus.Metrics()
	if m.TotalDropped < 50 {
		t.Errorf("TotalDropped (filtered) = %d, want >= 50", m.TotalDropped)
	}
}

func TestMetrics_LatencyHistogram(t *testing.T) {
	bus := NewBus(100)

	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: SessionStarted})
	}

	m := bus.Metrics()

	if m.Latency.Count != 10 {
		t.Errorf("Latency.Count = %d, want 10", m.Latency.Count)
	}
	if m.Latency.Sum <= 0 {
		t.Errorf("Latency.Sum = %v, want > 0", m.Latency.Sum)
	}

	// The +Inf bucket should contain all observations.
	infBucket := m.Latency.Buckets[len(m.Latency.Buckets)-1]
	if infBucket.Le != "+Inf" {
		t.Errorf("last bucket label = %q, want +Inf", infBucket.Le)
	}
	if infBucket.Count != 10 {
		t.Errorf("+Inf bucket count = %d, want 10", infBucket.Count)
	}

	// Buckets must be monotonically non-decreasing (cumulative).
	for i := 1; i < len(m.Latency.Buckets); i++ {
		if m.Latency.Buckets[i].Count < m.Latency.Buckets[i-1].Count {
			t.Errorf("bucket[%d].Count (%d) < bucket[%d].Count (%d); cumulative invariant broken",
				i, m.Latency.Buckets[i].Count, i-1, m.Latency.Buckets[i-1].Count)
		}
	}
}

func TestMetrics_LatencyBucketCount(t *testing.T) {
	bus := NewBus(100)
	bus.Publish(Event{Type: SessionStarted})

	m := bus.Metrics()
	if len(m.Latency.Buckets) != latencyBucketCount {
		t.Errorf("len(Latency.Buckets) = %d, want %d", len(m.Latency.Buckets), latencyBucketCount)
	}

	// Verify expected labels.
	expectedLabels := []string{"10us", "50us", "100us", "500us", "1ms", "5ms", "10ms", "50ms", "100ms", "+Inf"}
	for i, b := range m.Latency.Buckets {
		if b.Le != expectedLabels[i] {
			t.Errorf("bucket[%d].Le = %q, want %q", i, b.Le, expectedLabels[i])
		}
	}
}

func TestMetrics_SnapshotIsIsolated(t *testing.T) {
	bus := NewBus(100)
	bus.Publish(Event{Type: SessionStarted})

	m1 := bus.Metrics()

	// Publish more after taking the snapshot.
	bus.Publish(Event{Type: SessionEnded})
	bus.Publish(Event{Type: SessionEnded})

	m2 := bus.Metrics()

	if m1.TotalPublished != 1 {
		t.Errorf("m1.TotalPublished = %d, want 1", m1.TotalPublished)
	}
	if m2.TotalPublished != 3 {
		t.Errorf("m2.TotalPublished = %d, want 3", m2.TotalPublished)
	}

	// Modifying the returned map should not affect the bus.
	m1.PublishedByType[LoopStarted] = 999
	m3 := bus.Metrics()
	if m3.PublishedByType[LoopStarted] != 0 {
		t.Errorf("snapshot mutation leaked into bus: LoopStarted = %d", m3.PublishedByType[LoopStarted])
	}
}

func TestMetrics_ZeroValue(t *testing.T) {
	bus := NewBus(100)
	m := bus.Metrics()

	if m.TotalPublished != 0 {
		t.Errorf("TotalPublished = %d, want 0", m.TotalPublished)
	}
	if m.TotalDropped != 0 {
		t.Errorf("TotalDropped = %d, want 0", m.TotalDropped)
	}
	if m.SubscriberCount != 0 {
		t.Errorf("SubscriberCount = %d, want 0", m.SubscriberCount)
	}
	if m.Latency.Count != 0 {
		t.Errorf("Latency.Count = %d, want 0", m.Latency.Count)
	}
	if m.Latency.Sum != 0 {
		t.Errorf("Latency.Sum = %v, want 0", m.Latency.Sum)
	}
	if len(m.PublishedByType) != 0 {
		t.Errorf("PublishedByType should be empty, got %v", m.PublishedByType)
	}
}

func TestMetrics_ConcurrentPublish(t *testing.T) {
	bus := NewBus(10000)
	const goroutines = 8
	const perGoroutine = 500

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				bus.Publish(Event{Type: SessionStarted})
			}
		}()
	}
	wg.Wait()

	m := bus.Metrics()
	want := int64(goroutines * perGoroutine)
	if m.TotalPublished != want {
		t.Errorf("TotalPublished = %d, want %d", m.TotalPublished, want)
	}
	if m.PublishedByType[SessionStarted] != want {
		t.Errorf("PublishedByType[SessionStarted] = %d, want %d", m.PublishedByType[SessionStarted], want)
	}
	if m.Latency.Count != want {
		t.Errorf("Latency.Count = %d, want %d", m.Latency.Count, want)
	}
}

func TestMetrics_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	bus := NewBus(100)
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Subscribe goroutines
	for g := 0; g < goroutines; g++ {
		id := string(rune('A' + g))
		go func(id string) {
			defer wg.Done()
			bus.Subscribe(id)
		}(id)
	}

	// Filtered subscribe goroutines
	for g := 0; g < goroutines; g++ {
		id := "f-" + string(rune('A'+g))
		go func(id string) {
			defer wg.Done()
			bus.SubscribeFiltered(id, SessionStarted)
		}(id)
	}
	wg.Wait()

	m := bus.Metrics()
	if m.SubscriberCount != int64(goroutines*2) {
		t.Errorf("SubscriberCount = %d, want %d", m.SubscriberCount, goroutines*2)
	}

	// Unsubscribe all
	wg.Add(goroutines * 2)
	for g := 0; g < goroutines; g++ {
		id := string(rune('A' + g))
		go func(id string) {
			defer wg.Done()
			bus.Unsubscribe(id)
		}(id)
	}
	for g := 0; g < goroutines; g++ {
		id := "f-" + string(rune('A'+g))
		go func(id string) {
			defer wg.Done()
			bus.Unsubscribe(id)
		}(id)
	}
	wg.Wait()

	m = bus.Metrics()
	if m.SubscriberCount != 0 {
		t.Errorf("SubscriberCount after all unsub = %d, want 0", m.SubscriberCount)
	}
}

func TestMetrics_PublishCtxCancelled(t *testing.T) {
	bus := NewBus(100)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bus.PublishCtx(ctx, Event{Type: SessionStarted})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	// Cancelled publishes should NOT be counted.
	m := bus.Metrics()
	if m.TotalPublished != 0 {
		t.Errorf("TotalPublished = %d, want 0 (cancelled publish should not count)", m.TotalPublished)
	}
}

func TestMetrics_MultipleEventTypes(t *testing.T) {
	bus := NewBus(100)

	types := []EventType{
		SessionStarted, SessionEnded, CostUpdate,
		LoopStarted, LoopStopped, BudgetAlert,
	}
	for _, et := range types {
		bus.Publish(Event{Type: et})
	}

	m := bus.Metrics()
	if m.TotalPublished != int64(len(types)) {
		t.Errorf("TotalPublished = %d, want %d", m.TotalPublished, len(types))
	}
	for _, et := range types {
		if m.PublishedByType[et] != 1 {
			t.Errorf("PublishedByType[%s] = %d, want 1", et, m.PublishedByType[et])
		}
	}
}

func TestMetrics_WithTransportSubscribe(t *testing.T) {
	mt := newMemTransport()
	bus := NewBus(100, WithTransport(mt))

	bus.Subscribe("t1")
	bus.SubscribeFiltered("t2", SessionStarted)

	m := bus.Metrics()
	if m.SubscriberCount != 2 {
		t.Errorf("SubscriberCount with transport = %d, want 2", m.SubscriberCount)
	}

	bus.Unsubscribe("t1")
	m = bus.Metrics()
	if m.SubscriberCount != 1 {
		t.Errorf("SubscriberCount after unsub with transport = %d, want 1", m.SubscriberCount)
	}
}

func TestMetrics_NoDropsWhenNoSubscribers(t *testing.T) {
	bus := NewBus(100)

	for i := 0; i < 50; i++ {
		bus.Publish(Event{Type: SessionStarted})
	}

	m := bus.Metrics()
	if m.TotalDropped != 0 {
		t.Errorf("TotalDropped = %d, want 0 (no subscribers means no drops)", m.TotalDropped)
	}
}

func TestMetrics_LatencySumAccumulates(t *testing.T) {
	bus := NewBus(100)

	bus.Publish(Event{Type: SessionStarted})
	m1 := bus.Metrics()

	bus.Publish(Event{Type: SessionStarted})
	m2 := bus.Metrics()

	if m2.Latency.Sum < m1.Latency.Sum {
		t.Errorf("Latency.Sum should grow: m1=%v, m2=%v", m1.Latency.Sum, m2.Latency.Sum)
	}
	if m2.Latency.Count != m1.Latency.Count+1 {
		t.Errorf("Latency.Count should increment: m1=%d, m2=%d", m1.Latency.Count, m2.Latency.Count)
	}
}

// TestMetrics_recordLatency_unit directly tests the histogram bucketing logic.
func TestMetrics_recordLatency_unit(t *testing.T) {
	m := newMetrics()

	// Record a 5 microsecond latency — should land in the 10us bucket.
	m.recordLatency(5 * time.Microsecond)

	s := m.snapshot()
	// 10us bucket (index 0) should have count 1.
	if s.Latency.Buckets[0].Count != 1 {
		t.Errorf("10us bucket = %d, want 1", s.Latency.Buckets[0].Count)
	}
	// All higher buckets should also be 1 (cumulative).
	for i := 1; i < len(s.Latency.Buckets); i++ {
		if s.Latency.Buckets[i].Count != 1 {
			t.Errorf("bucket[%d] (%s) = %d, want 1 (cumulative)", i, s.Latency.Buckets[i].Le, s.Latency.Buckets[i].Count)
		}
	}

	// Record a 2ms latency — should land in the 5ms bucket (index 5).
	m.recordLatency(2 * time.Millisecond)
	s = m.snapshot()

	// Buckets below 5ms boundary: 10us, 50us, 100us, 500us, 1ms (indices 0-4) should still be 1.
	for i := 0; i < 5; i++ {
		if s.Latency.Buckets[i].Count != 1 {
			t.Errorf("bucket[%d] (%s) = %d, want 1 after 2ms observation", i, s.Latency.Buckets[i].Le, s.Latency.Buckets[i].Count)
		}
	}
	// 5ms bucket (index 5) and above should be 2.
	for i := 5; i < len(s.Latency.Buckets); i++ {
		if s.Latency.Buckets[i].Count != 2 {
			t.Errorf("bucket[%d] (%s) = %d, want 2 after 2ms observation", i, s.Latency.Buckets[i].Le, s.Latency.Buckets[i].Count)
		}
	}

	if s.Latency.Count != 2 {
		t.Errorf("Latency.Count = %d, want 2", s.Latency.Count)
	}
}

// TestMetrics_recordLatency_largeDuration tests that very large latencies
// end up in the +Inf bucket.
func TestMetrics_recordLatency_largeDuration(t *testing.T) {
	m := newMetrics()

	m.recordLatency(500 * time.Millisecond)

	s := m.snapshot()
	// Only the +Inf bucket (last) should have count 1.
	for i := 0; i < len(s.Latency.Buckets)-1; i++ {
		// All buckets below 100ms should be 0.
		if latencyBoundaries[i] > 0 && 500_000 > latencyBoundaries[i] && s.Latency.Buckets[i].Count != 0 {
			t.Errorf("bucket[%d] (%s) = %d, want 0 for 500ms observation",
				i, s.Latency.Buckets[i].Le, s.Latency.Buckets[i].Count)
		}
	}
	// +Inf bucket must have count 1.
	inf := s.Latency.Buckets[len(s.Latency.Buckets)-1]
	if inf.Count != 1 {
		t.Errorf("+Inf bucket = %d, want 1", inf.Count)
	}
}

func TestMetrics_ConcurrentPublishAndMetrics(t *testing.T) {
	bus := NewBus(10000)
	const publishers = 4
	const readers = 4
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(publishers + readers)

	// Publishers
	for g := 0; g < publishers; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				bus.Publish(Event{Type: SessionStarted})
			}
		}()
	}

	// Concurrent readers of Metrics()
	for g := 0; g < readers; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				m := bus.Metrics()
				// Sanity: totals should never be negative.
				if m.TotalPublished < 0 {
					t.Errorf("TotalPublished < 0: %d", m.TotalPublished)
				}
				if m.TotalDropped < 0 {
					t.Errorf("TotalDropped < 0: %d", m.TotalDropped)
				}
			}
		}()
	}

	wg.Wait()
}
