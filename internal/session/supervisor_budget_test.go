package session

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestBudgetEnvelopeCanSpend(t *testing.T) {
	be := NewBudgetEnvelope(10)
	if !be.CanSpend(5) {
		t.Error("expected CanSpend($5) true with $10 budget")
	}
	if be.CanSpend(15) {
		t.Error("expected CanSpend($15) false with $10 budget")
	}
	if !be.CanSpend(10) {
		t.Error("expected CanSpend($10) true with $10 budget (exact)")
	}
}

func TestBudgetEnvelopeRecordSpend(t *testing.T) {
	be := NewBudgetEnvelope(10)
	be.RecordSpend(3)
	be.RecordSpend(4)

	if r := be.Remaining(); math.Abs(r-3) > 0.001 {
		t.Errorf("expected Remaining()=3, got %f", r)
	}
	if be.CanSpend(4) {
		t.Error("expected CanSpend($4) false after spending $7 of $10")
	}
	if !be.CanSpend(3) {
		t.Error("expected CanSpend($3) true with $3 remaining")
	}
}

func TestBudgetEnvelopePerCycleCapDefault(t *testing.T) {
	be := NewBudgetEnvelope(100)
	if cap := be.PerCycleCap(); math.Abs(cap-10) > 0.001 {
		t.Errorf("expected PerCycleCap()=10 (100/10), got %f", cap)
	}
}

func TestBudgetEnvelopePerCycleCapCustom(t *testing.T) {
	be := NewBudgetEnvelope(100)
	be.PerCycleBudgetUSD = 25
	if cap := be.PerCycleCap(); math.Abs(cap-25) > 0.001 {
		t.Errorf("expected PerCycleCap()=25, got %f", cap)
	}
}

func TestBudgetEnvelopeConcurrency(t *testing.T) {
	be := NewBudgetEnvelope(1000)
	var wg sync.WaitGroup
	n := 100
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			be.RecordSpend(1)
			_ = be.Remaining()
			_ = be.Spent()
			_ = be.CanSpend(1)
			_ = be.PerCycleCap()
		}()
	}
	wg.Wait()

	if s := be.Spent(); math.Abs(s-float64(n)) > 0.001 {
		t.Errorf("expected Spent()=%d after %d goroutines, got %f", n, n, s)
	}
}

func TestBudgetEnvelopeLoadFromState(t *testing.T) {
	be := NewBudgetEnvelope(10)
	be.LoadFromState(7.5)
	if s := be.Spent(); math.Abs(s-7.5) > 0.001 {
		t.Errorf("expected Spent()=7.5 after LoadFromState, got %f", s)
	}
	if r := be.Remaining(); math.Abs(r-2.5) > 0.001 {
		t.Errorf("expected Remaining()=2.5, got %f", r)
	}
}

func TestBudgetEnvelopeZeroBudget(t *testing.T) {
	be := NewBudgetEnvelope(0)
	if be.CanSpend(1) {
		t.Error("expected CanSpend($1) false with $0 budget")
	}
	if !be.CanSpend(0) {
		t.Error("expected CanSpend($0) true with $0 budget")
	}
}

func TestBudgetEnvelopeMarshalJSON(t *testing.T) {
	be := NewBudgetEnvelope(50)
	be.RecordSpend(12.5)

	data, err := be.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var out map[string]float64
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["total_budget_usd"] != 50 {
		t.Errorf("expected total_budget_usd=50, got %f", out["total_budget_usd"])
	}
	if math.Abs(out["spent_usd"]-12.5) > 0.001 {
		t.Errorf("expected spent_usd=12.5, got %f", out["spent_usd"])
	}
	if math.Abs(out["remaining_usd"]-37.5) > 0.001 {
		t.Errorf("expected remaining_usd=37.5, got %f", out["remaining_usd"])
	}
}

func TestBudgetEnvelopeSubscribeToBus(t *testing.T) {
	be := NewBudgetEnvelope(100)
	bus := events.NewBus(100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		be.SubscribeToBus(ctx, bus)
		close(done)
	}()

	// Give goroutine time to subscribe before publishing.
	time.Sleep(20 * time.Millisecond)

	// Publish cumulative cost updates for a session.
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Data:      map[string]any{"spent_usd": 5.0},
	})
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Data:      map[string]any{"spent_usd": 8.0},
	})
	// Second session
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s2",
		Data:      map[string]any{"spent_usd": 3.0},
	})

	// Give goroutine time to process
	time.Sleep(50 * time.Millisecond)

	// s1 spent 8 (cumulative), s2 spent 3 => total = 11
	if s := be.Spent(); math.Abs(s-11) > 0.001 {
		t.Errorf("expected Spent()=11 after bus events, got %f", s)
	}

	cancel()
	<-done
}

func TestBudgetEnvelopeSubscribeToBusNilBus(t *testing.T) {
	be := NewBudgetEnvelope(10)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		be.SubscribeToBus(ctx, nil)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SubscribeToBus with nil bus should return on context cancel")
	}
}

func TestBudgetEnvelopeSubscribeToBusDuplicateSpend(t *testing.T) {
	// Ensure duplicate cumulative values don't double-count.
	be := NewBudgetEnvelope(100)
	bus := events.NewBus(100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		be.SubscribeToBus(ctx, bus)
		close(done)
	}()

	// Give goroutine time to subscribe before publishing.
	time.Sleep(20 * time.Millisecond)

	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Data:      map[string]any{"spent_usd": 5.0},
	})
	// Same value again (no new spend)
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Data:      map[string]any{"spent_usd": 5.0},
	})

	time.Sleep(50 * time.Millisecond)

	if s := be.Spent(); math.Abs(s-5) > 0.001 {
		t.Errorf("expected Spent()=5 (no double-count), got %f", s)
	}

	cancel()
	<-done
}
