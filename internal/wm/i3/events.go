package i3

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// EventType identifies the kind of i3 IPC event.
type EventType uint32

const (
	// WorkspaceEvent fires on workspace changes (focus, init, empty, etc.).
	WorkspaceEvent EventType = 0
	// OutputEvent fires when an output (monitor) is added or removed.
	OutputEvent EventType = 1
	// ModeEvent fires when the binding mode changes.
	ModeEvent EventType = 2
	// WindowEvent fires on window changes (new, close, focus, move, etc.).
	WindowEvent EventType = 3
)

// eventNames maps EventType to the i3 IPC subscription string.
var eventNames = map[EventType]string{
	WorkspaceEvent: "workspace",
	OutputEvent:    "output",
	ModeEvent:      "mode",
	WindowEvent:    "window",
}

// String returns the event type name used in i3 IPC subscriptions.
func (e EventType) String() string {
	if s, ok := eventNames[e]; ok {
		return s
	}
	return fmt.Sprintf("unknown(%d)", e)
}

// i3 IPC subscribe message type and event bit.
const (
	msgTypeSubscribe uint32 = 2
	eventBit         uint32 = 1 << 31 // high bit set in event response type
)

// Event represents an i3 IPC event.
type Event struct {
	Type    EventType       `json:"type"`
	Change  string          `json:"change"`
	Payload json.RawMessage `json:"payload"`
}

// EventListener subscribes to and receives i3 IPC events.
type EventListener struct {
	socketPath string
	conn       net.Conn
	eventTypes []EventType

	mu     sync.Mutex
	closed bool

	// reconnectDelay controls backoff between reconnect attempts.
	reconnectDelay time.Duration
	maxReconnect   time.Duration
}

// NewEventListener creates a new EventListener connected to the i3 IPC socket.
// It finds the socket path from the I3SOCK environment variable, or by running
// `i3 --get-socketpath`.
func NewEventListener() (*EventListener, error) {
	sockPath, err := findSocketPath()
	if err != nil {
		return nil, fmt.Errorf("i3 events: %w", err)
	}
	return NewEventListenerFromPath(sockPath)
}

// NewEventListenerFromPath creates a new EventListener using the given socket path.
func NewEventListenerFromPath(socketPath string) (*EventListener, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("i3 events connect: %w", err)
	}
	return &EventListener{
		socketPath:     socketPath,
		conn:           conn,
		reconnectDelay: 100 * time.Millisecond,
		maxReconnect:   5 * time.Second,
	}, nil
}

// findSocketPath returns the i3 IPC socket path.
func findSocketPath() (string, error) {
	if sock := os.Getenv("I3SOCK"); sock != "" {
		return sock, nil
	}
	out, err := exec.Command("i3", "--get-socketpath").Output()
	if err != nil {
		return "", fmt.Errorf("cannot determine i3 socket path: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Subscribe tells i3 which event types to deliver on this connection.
// Must be called before Listen.
func (el *EventListener) Subscribe(eventTypes ...EventType) error {
	el.mu.Lock()
	defer el.mu.Unlock()

	if el.closed {
		return fmt.Errorf("i3 events: listener closed")
	}

	el.eventTypes = eventTypes
	return el.subscribe()
}

// subscribe sends the subscribe message on the current connection.
// Caller must hold el.mu.
func (el *EventListener) subscribe() error {
	names := make([]string, len(el.eventTypes))
	for i, et := range el.eventTypes {
		names[i] = et.String()
	}
	payload, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("i3 events marshal subscribe: %w", err)
	}

	if err := writeMessage(el.conn, msgTypeSubscribe, payload); err != nil {
		return err
	}

	resp, _, err := readMessage(el.conn)
	if err != nil {
		return fmt.Errorf("i3 events subscribe response: %w", err)
	}

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("i3 events parse subscribe: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("i3 events: subscribe rejected")
	}
	return nil
}

// Listen blocks, reading events from i3 and calling handler for each one.
// It returns when ctx is cancelled or an unrecoverable error occurs.
// On transient socket errors it attempts to reconnect.
func (el *EventListener) Listen(ctx context.Context, handler func(Event)) error {
	for {
		err := el.listenOnce(ctx, handler)
		if err == nil {
			return nil // context cancelled
		}

		// Check if we were closed or context done.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		el.mu.Lock()
		if el.closed {
			el.mu.Unlock()
			return fmt.Errorf("i3 events: listener closed")
		}
		el.mu.Unlock()

		// Attempt reconnect with backoff.
		if reconnErr := el.reconnect(ctx); reconnErr != nil {
			return fmt.Errorf("i3 events reconnect: %w", reconnErr)
		}
	}
}

// listenOnce reads events until an error or context cancellation.
func (el *EventListener) listenOnce(ctx context.Context, handler func(Event)) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		el.mu.Lock()
		conn := el.conn
		el.mu.Unlock()

		if conn == nil {
			return fmt.Errorf("no connection")
		}

		// Set a read deadline so we periodically check ctx.
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			return fmt.Errorf("i3 events set deadline: %w", err)
		}

		payload, msgType, err := readMessage(conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // deadline exceeded, check ctx and loop
			}
			return err
		}

		// Event responses have the high bit set in the message type.
		if msgType&eventBit == 0 {
			continue // not an event, skip
		}
		evtType := EventType(msgType &^ eventBit)

		// Parse the change field from the payload.
		var partial struct {
			Change string `json:"change"`
		}
		_ = json.Unmarshal(payload, &partial)

		handler(Event{
			Type:    evtType,
			Change:  partial.Change,
			Payload: json.RawMessage(payload),
		})
	}
}

// reconnect closes the old connection and establishes a new one with backoff.
func (el *EventListener) reconnect(ctx context.Context) error {
	el.mu.Lock()
	if el.conn != nil {
		el.conn.Close()
		el.conn = nil
	}
	delay := el.reconnectDelay
	eventTypes := el.eventTypes
	el.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		conn, err := net.Dial("unix", el.socketPath)
		if err != nil {
			delay = delay * 2
			if delay > el.maxReconnect {
				delay = el.maxReconnect
			}
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

		// Re-subscribe if we had event types.
		if len(eventTypes) > 0 {
			el.mu.Lock()
			err := el.subscribe()
			el.mu.Unlock()
			if err != nil {
				continue
			}
		}
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

// writeMessage writes an i3 IPC message (header + payload) to the connection.
func writeMessage(conn net.Conn, msgType uint32, payload []byte) error {
	header := make([]byte, headerLen)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[6:10], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[10:14], msgType)

	if _, err := conn.Write(header); err != nil {
		return fmt.Errorf("i3 ipc write header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := conn.Write(payload); err != nil {
			return fmt.Errorf("i3 ipc write payload: %w", err)
		}
	}
	return nil
}

// readMessage reads a full i3 IPC message (header + payload) from the connection.
// It returns the payload, the message type, and any error.
func readMessage(conn net.Conn) ([]byte, uint32, error) {
	hdr := make([]byte, headerLen)
	if _, err := readFull(conn, hdr); err != nil {
		return nil, 0, fmt.Errorf("i3 ipc read header: %w", err)
	}

	// Validate magic bytes.
	for i := 0; i < len(magic); i++ {
		if hdr[i] != magic[i] {
			return nil, 0, fmt.Errorf("i3 ipc: invalid magic in response")
		}
	}

	payloadLen := binary.LittleEndian.Uint32(hdr[6:10])
	msgType := binary.LittleEndian.Uint32(hdr[10:14])

	if payloadLen == 0 {
		return nil, msgType, nil
	}

	payload := make([]byte, payloadLen)
	if _, err := readFull(conn, payload); err != nil {
		return nil, 0, fmt.Errorf("i3 ipc read payload: %w", err)
	}
	return payload, msgType, nil
}
