package safety

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// KillSwitch is an emergency stop mechanism that halts all autonomous
// fleet operations when engaged. It emits events on the bus so that
// other subsystems (TUI, supervisor, loop engine) can react.
type KillSwitch struct {
	mu       sync.RWMutex
	bus      *events.Bus
	engaged  bool
	at       *time.Time
	reason   string
}

// NewKillSwitch creates a kill switch wired to the given event bus.
func NewKillSwitch(bus *events.Bus) *KillSwitch {
	return &KillSwitch{bus: bus}
}

// Engage activates the kill switch with the given reason. It emits an
// EmergencyStop event on the bus and logs the action. Calling Engage
// when already engaged is a no-op.
func (ks *KillSwitch) Engage(reason string) {
	ks.mu.Lock()
	if ks.engaged {
		ks.mu.Unlock()
		return
	}
	now := time.Now()
	ks.engaged = true
	ks.at = &now
	ks.reason = reason
	ks.mu.Unlock()

	slog.Warn("kill switch engaged", "reason", reason)

	if ks.bus != nil {
		_ = ks.bus.PublishCtx(context.Background(), events.Event{
			Type:      events.EmergencyStop,
			Timestamp: now,
			Data: map[string]any{
				"reason": reason,
			},
		})
	}
}

// Disengage deactivates the kill switch and emits an EmergencyResume
// event. Calling Disengage when not engaged is a no-op.
func (ks *KillSwitch) Disengage() {
	ks.mu.Lock()
	if !ks.engaged {
		ks.mu.Unlock()
		return
	}
	ks.engaged = false
	ks.at = nil
	ks.reason = ""
	ks.mu.Unlock()

	slog.Info("kill switch disengaged")

	if ks.bus != nil {
		_ = ks.bus.PublishCtx(context.Background(), events.Event{
			Type:      events.EmergencyResume,
			Timestamp: time.Now(),
		})
	}
}

// IsEngaged returns true if the kill switch is currently active.
func (ks *KillSwitch) IsEngaged() bool {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.engaged
}

// EngagedAt returns the time the kill switch was engaged, or nil if not engaged.
func (ks *KillSwitch) EngagedAt() *time.Time {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	if ks.at == nil {
		return nil
	}
	t := *ks.at
	return &t
}

// Reason returns the reason the kill switch was engaged, or empty string if not engaged.
func (ks *KillSwitch) Reason() string {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	return ks.reason
}
