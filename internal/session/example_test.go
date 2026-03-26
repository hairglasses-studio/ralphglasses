package session_test

import (
	"fmt"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func ExampleManager() {
	// Create a new session manager.
	mgr := session.NewManager()

	// List all sessions (empty at start).
	sessions := mgr.List("")
	fmt.Printf("sessions: %d\n", len(sessions))

	// Look up a session by ID (returns false when not found).
	_, found := mgr.Get("nonexistent")
	fmt.Printf("found: %v\n", found)

	// Output:
	// sessions: 0
	// found: false
}
