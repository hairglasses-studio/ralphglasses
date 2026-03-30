package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ReplayEventType classifies what kind of session event was recorded.
type ReplayEventType string

const (
	ReplayInput  ReplayEventType = "input"
	ReplayOutput ReplayEventType = "output"
	ReplayTool   ReplayEventType = "tool"
	ReplayStatus ReplayEventType = "status"
)

// ReplayEvent is a single recorded event in a session replay log.
type ReplayEvent struct {
	Timestamp time.Time       `json:"timestamp"`
	Type      ReplayEventType `json:"type"`
	Data      string          `json:"data"`
	SessionID string          `json:"session_id"`
}

// Recorder appends session events to a JSONL replay file.
type Recorder struct {
	sessionID string
	path      string
	mu        sync.Mutex
	f         *os.File
	enc       *json.Encoder
}

// NewRecorder creates a Recorder that writes JSONL to path.
// The file is created (or appended to) on the first call to Record.
func NewRecorder(sessionID, path string) *Recorder {
	return &Recorder{
		sessionID: sessionID,
		path:      path,
	}
}

// Record appends a single event to the replay log.
func (r *Recorder) Record(event ReplayEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.f == nil {
		f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("replay: open %s: %w", r.path, err)
		}
		r.f = f
		r.enc = json.NewEncoder(f)
	}

	// Ensure the event carries the recorder's session ID.
	event.SessionID = r.sessionID
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	return r.enc.Encode(event)
}

// Close flushes and closes the underlying file.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		err := r.f.Close()
		r.f = nil
		r.enc = nil
		return err
	}
	return nil
}

// Player reads a JSONL replay file and supports playback and search.
type Player struct {
	events []ReplayEvent
}

// NewPlayer loads all events from the JSONL file at path.
func NewPlayer(path string) (*Player, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("replay: open %s: %w", path, err)
	}
	defer f.Close()

	var events []ReplayEvent
	scanner := bufio.NewScanner(f)
	// Allow large lines (1 MB).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev ReplayEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("replay: decode line: %w", err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("replay: scan %s: %w", path, err)
	}
	return &Player{events: events}, nil
}

// Events returns all loaded replay events.
func (p *Player) Events() []ReplayEvent {
	return p.events
}

// Duration returns the wall-clock duration from first to last event.
// Returns 0 if fewer than 2 events exist.
func (p *Player) Duration() time.Duration {
	if len(p.events) < 2 {
		return 0
	}
	return p.events[len(p.events)-1].Timestamp.Sub(p.events[0].Timestamp)
}

// Search returns events whose Data contains query (case-insensitive).
func (p *Player) Search(query string) []ReplayEvent {
	q := strings.ToLower(query)
	var matches []ReplayEvent
	for _, ev := range p.events {
		if strings.Contains(strings.ToLower(ev.Data), q) {
			matches = append(matches, ev)
		}
	}
	return matches
}

// Play replays events by calling handler for each event, sleeping between
// events proportional to the original timing divided by speed.
// A speed of 0 or less replays instantly with no sleep.
// Play respects context cancellation.
func (p *Player) Play(ctx context.Context, speed float64, handler func(ReplayEvent)) error {
	for i, ev := range p.events {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		handler(ev)

		// Sleep between events to simulate original timing.
		if speed > 0 && i+1 < len(p.events) {
			gap := p.events[i+1].Timestamp.Sub(ev.Timestamp)
			if gap > 0 {
				scaled := time.Duration(float64(gap) / speed)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(scaled):
				}
			}
		}
	}
	return nil
}
