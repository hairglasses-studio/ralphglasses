package hyprland

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// EventType identifies the kind of Hyprland event.
type EventType string

const (
	// EventWorkspace fires when the active workspace changes.
	EventWorkspace EventType = "workspace"
	// EventActiveWindow fires when window focus changes.
	EventActiveWindow EventType = "activewindow"
	// EventOpenWindow fires when a new window is created.
	EventOpenWindow EventType = "openwindow"
	// EventCloseWindow fires when a window is destroyed.
	EventCloseWindow EventType = "closewindow"
	// EventMonitorAdded fires when a display is hot-plugged.
	EventMonitorAdded EventType = "monitoradded"
	// EventMonitorRemoved fires when a display is removed.
	EventMonitorRemoved EventType = "monitorremoved"
	// EventSubmap fires when the binding submap changes.
	EventSubmap EventType = "submap"
	// EventFullscreen fires on fullscreen state change.
	EventFullscreen EventType = "fullscreen"
	// EventMoveWorkspace fires when a workspace is moved to a different monitor.
	EventMoveWorkspace EventType = "moveworkspace"
	// EventUrgent fires when a window sets the urgent hint.
	EventUrgent EventType = "urgent"
	// EventConfigReloaded fires after a config reload.
	EventConfigReloaded EventType = "configreloaded"
)

// Event represents a Hyprland event received from the event socket.
type Event struct {
	// Type is the event type (e.g., "workspace", "activewindow").
	Type EventType
	// Data is the raw event data after the ">>" separator.
	Data string
}

// EventListener subscribes to and receives Hyprland events from .socket2.sock.
//
// Unlike i3, Hyprland streams ALL events without a subscribe handshake.
// The protocol is simple: newline-delimited "EVENT>>DATA\n" text.
type EventListener struct {
	client *Client
	conn   net.Conn

	mu     sync.Mutex
	closed bool

	// Reconnect backoff parameters, matching i3/events.go pattern.
	reconnectDelay time.Duration
	maxReconnect   time.Duration
}

// NewEventListener creates a new EventListener for the running Hyprland instance.
func NewEventListener() (*EventListener, error) {
	client, err := NewClient()
	if err != nil {
		return nil, err
	}
	return newEventListenerFromClient(client)
}

// newEventListenerFromClient creates an EventListener using an existing client.
// Exported for internal testing.
func newEventListenerFromClient(client *Client) (*EventListener, error) {
	conn, err := net.Dial("unix", client.EventSocketPath())
	if err != nil {
		return nil, fmt.Errorf("hyprland events connect: %w", err)
	}
	return &EventListener{
		client:         client,
		conn:           conn,
		reconnectDelay: 100 * time.Millisecond,
		maxReconnect:   5 * time.Second,
	}, nil
}

// newEventListenerForTest creates an EventListener connected to a specific socket path.
func newEventListenerForTest(conn net.Conn, client *Client) *EventListener {
	return &EventListener{
		client:         client,
		conn:           conn,
		reconnectDelay: 50 * time.Millisecond,
		maxReconnect:   200 * time.Millisecond,
	}
}

// Listen blocks, reading events from Hyprland and calling handler for each one.
// It returns when ctx is cancelled or an unrecoverable error occurs.
// On transient socket errors it attempts to reconnect with exponential backoff.
func (el *EventListener) Listen(ctx context.Context, handler func(Event)) error {
	for {
		err := el.listenOnce(ctx, handler)
		if err == nil {
			return nil // context cancelled
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		el.mu.Lock()
		if el.closed {
			el.mu.Unlock()
			return fmt.Errorf("hyprland events: listener closed")
		}
		el.mu.Unlock()

		if reconnErr := el.reconnect(ctx); reconnErr != nil {
			return fmt.Errorf("hyprland events reconnect: %w", reconnErr)
		}
	}
}

// listenOnce reads events until an error or context cancellation.
func (el *EventListener) listenOnce(ctx context.Context, handler func(Event)) error {
	el.mu.Lock()
	conn := el.conn
	el.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("no connection")
	}

	scanner := bufio.NewScanner(conn)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Set a read deadline so we periodically check ctx.
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return fmt.Errorf("hyprland events set deadline: %w", err)
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // deadline exceeded, loop to check ctx
				}
				return err
			}
			return fmt.Errorf("hyprland events: connection closed")
		}

		line := scanner.Text()
		evt, ok := ParseEvent(line)
		if !ok {
			continue // skip malformed lines
		}
		handler(evt)
	}
}

// ParseEvent parses a single Hyprland event line ("EVENT>>DATA") into an Event.
// Returns false if the line does not contain the ">>" separator.
func ParseEvent(line string) (Event, bool) {
	before, after, ok := strings.Cut(line, ">>")
	if !ok {
		return Event{}, false
	}
	return Event{
		Type: EventType(before),
		Data: after,
	}, true
}

// reconnect closes the old connection and establishes a new one with backoff.
func (el *EventListener) reconnect(ctx context.Context) error {
	el.mu.Lock()
	if el.conn != nil {
		el.conn.Close()
		el.conn = nil
	}
	delay := el.reconnectDelay
	el.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		conn, err := net.Dial("unix", el.client.EventSocketPath())
		if err != nil {
			delay = min(delay*2, el.maxReconnect)
			continue
		}

		el.mu.Lock()
		if el.closed {
			conn.Close()
			el.mu.Unlock()
			return fmt.Errorf("listener closed during reconnect")
		}
		el.conn = conn
		el.mu.Unlock()
		return nil
	}
}

// Close shuts down the listener and closes the underlying connection.
func (el *EventListener) Close() error {
	el.mu.Lock()
	defer el.mu.Unlock()

	if el.closed {
		return nil
	}
	el.closed = true
	if el.conn != nil {
		return el.conn.Close()
	}
	return nil
}
