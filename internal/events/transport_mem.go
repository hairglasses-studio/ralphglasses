package events

import (
	"context"
	"sync"
)

// memTransport is the default in-memory EventTransport.
// It fans out events to subscribers using non-blocking channel sends.
type memTransport struct {
	mu   sync.RWMutex
	subs map[string]memSub
}

type memSub struct {
	ch     chan Event
	filter func(Event) bool // nil means accept all
}

// newMemTransport creates an in-memory transport.
func newMemTransport() *memTransport {
	return &memTransport{
		subs: make(map[string]memSub),
	}
}

// Publish fans out an event to all matching subscribers (non-blocking).
func (m *memTransport) Publish(_ context.Context, event Event) error {
	m.mu.RLock()
	// Snapshot matching subscribers under lock
	chans := make([]chan Event, 0, len(m.subs))
	for _, s := range m.subs {
		if s.filter == nil || s.filter(event) {
			chans = append(chans, s.ch)
		}
	}
	m.mu.RUnlock()

	for _, ch := range chans {
		select {
		case ch <- event:
		default:
			// Drop on overflow — subscriber is too slow
		}
	}
	return nil
}

// Subscribe returns a buffered channel that receives events matching the filter.
func (m *memTransport) Subscribe(_ context.Context, subscriber string, filter func(Event) bool) (<-chan Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan Event, 100)
	m.subs[subscriber] = memSub{ch: ch, filter: filter}
	return ch, nil
}

// Unsubscribe removes a subscriber and closes its channel.
func (m *memTransport) Unsubscribe(subscriber string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.subs[subscriber]; ok {
		delete(m.subs, subscriber)
		close(s.ch)
	}
}

// Close removes all subscribers and closes their channels.
func (m *memTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range m.subs {
		close(s.ch)
		delete(m.subs, id)
	}
	return nil
}
