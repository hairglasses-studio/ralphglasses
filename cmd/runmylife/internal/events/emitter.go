package events

import "context"

// Emitter provides type-safe event publishing with consistent payload shapes.
// Wraps a Bus to prevent typos in event types and payload keys.
type Emitter struct {
	bus *Bus
}

// NewEmitter creates an emitter bound to the given bus.
func NewEmitter(bus *Bus) *Emitter {
	return &Emitter{bus: bus}
}

// TaskCompleted emits when a task is marked done.
func (e *Emitter) TaskCompleted(ctx context.Context, taskID, title string) {
	e.bus.PublishAsync(ctx, New(TaskCompleted, "worker", map[string]any{
		"task_id": taskID,
		"title":   title,
	}))
}

// MoodLogged emits when a mood entry is recorded.
func (e *Emitter) MoodLogged(ctx context.Context, score int, notes string) {
	e.bus.PublishAsync(ctx, New(MoodLogged, "worker", map[string]any{
		"score": score,
		"notes": notes,
	}))
}

// FocusStarted emits when a focus session begins.
func (e *Emitter) FocusStarted(ctx context.Context, category string) {
	e.bus.PublishAsync(ctx, New(FocusStarted, "worker", map[string]any{
		"category": category,
	}))
}

// FocusEnded emits when a focus session ends.
func (e *Emitter) FocusEnded(ctx context.Context, category string, minutes int) {
	e.bus.PublishAsync(ctx, New(FocusEnded, "worker", map[string]any{
		"category": category,
		"minutes":  minutes,
	}))
}

// HabitCompleted emits when a habit is checked off.
func (e *Emitter) HabitCompleted(ctx context.Context, habitID, name string) {
	e.bus.PublishAsync(ctx, New(HabitCompleted, "worker", map[string]any{
		"habit_id": habitID,
		"name":     name,
	}))
}

// ChoreDone emits when a chore is completed.
func (e *Emitter) ChoreDone(ctx context.Context, choreID, name string) {
	e.bus.PublishAsync(ctx, New(ChoreDone, "worker", map[string]any{
		"chore_id": choreID,
		"name":     name,
	}))
}

// ReplySent emits when a reply is sent on any platform.
func (e *Emitter) ReplySent(ctx context.Context, platform, contactID string) {
	e.bus.PublishAsync(ctx, New(ReplySent, "worker", map[string]any{
		"platform":   platform,
		"contact_id": contactID,
	}))
}

// EnergyRecorded emits when energy level is estimated or logged.
func (e *Emitter) EnergyRecorded(ctx context.Context, level int) {
	e.bus.PublishAsync(ctx, New(EnergyRecorded, "worker", map[string]any{
		"level": level,
	}))
}

// OverwhelmDetected emits when overwhelm score exceeds threshold.
func (e *Emitter) OverwhelmDetected(ctx context.Context, score float64, triageActivated bool) {
	e.bus.PublishAsync(ctx, New(OverwhelmDetected, "worker", map[string]any{
		"score":             score,
		"triage_activated":  triageActivated,
	}))
}

// AchievementEarned emits when a milestone is reached.
func (e *Emitter) AchievementEarned(ctx context.Context, title, description string) {
	e.bus.PublishAsync(ctx, New(AchievementEarned, "worker", map[string]any{
		"title":       title,
		"description": description,
	}))
}

// ReviewGenerated emits when a weekly/monthly review is produced.
func (e *Emitter) ReviewGenerated(ctx context.Context, reviewType string) {
	e.bus.PublishAsync(ctx, New(ReviewGenerated, "worker", map[string]any{
		"review_type": reviewType,
	}))
}
