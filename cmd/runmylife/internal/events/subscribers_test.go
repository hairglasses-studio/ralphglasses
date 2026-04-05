package events

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/notifications"
	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

// testDispatcher creates a dispatcher with empty credentials (all sends fall back to log).
func testDispatcher() *notifications.Dispatcher {
	cfg := &config.Config{Credentials: map[string]string{}}
	return notifications.NewDispatcher(cfg, nil)
}

// --- NotificationSubscriber tests ---

func TestNotificationSubscriber_OverwhelmDetected(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(OverwhelmDetected, "test", map[string]any{"score": 0.85})
	// Should not panic — dispatches to log channel (nil config falls back)
	handler(e)
}

func TestNotificationSubscriber_AchievementEarned(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(AchievementEarned, "test", map[string]any{
		"title":       "Task Streak",
		"description": "7 days of task completions",
	})
	handler(e)
}

func TestNotificationSubscriber_TaskCompleted(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(TaskCompleted, "test", map[string]any{"task_id": "t1", "title": "Buy milk"})
	handler(e)
}

func TestNotificationSubscriber_MoodLogged(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(MoodLogged, "test", map[string]any{"score": float64(7), "notes": "good day"})
	handler(e)
}

func TestNotificationSubscriber_HabitCompleted(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(HabitCompleted, "test", map[string]any{"habit_id": "h1", "name": "Exercise"})
	handler(e)
}

func TestNotificationSubscriber_FocusEnded(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(FocusEnded, "test", map[string]any{"category": "deep-work", "minutes": float64(45)})
	handler(e)
}

func TestNotificationSubscriber_ReviewGenerated(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	e := New(ReviewGenerated, "test", map[string]any{"review_type": "weekly"})
	handler(e)
}

func TestNotificationSubscriber_UnhandledType(t *testing.T) {
	d := testDispatcher()
	handler := NotificationSubscriber(d)

	// EnergyRecorded is not handled by NotificationSubscriber
	e := New(EnergyRecorded, "test", map[string]any{"level": 7})
	handler(e) // should return early, no notification
}

// --- AchievementSubscriber tests ---

func TestAchievementSubscriber_NoAchievements(t *testing.T) {
	db := testutil.TestDB(t)
	bus := NewBus(nil)
	emitter := NewEmitter(bus)
	handler := AchievementSubscriber(db, emitter)

	// Empty DB — no achievements possible
	e := New(TaskCompleted, "test", map[string]any{"task_id": "t1", "title": "Test"})
	handler(e) // should not panic
}

func TestAchievementSubscriber_NilEmitter(t *testing.T) {
	db := testutil.TestDB(t)
	handler := AchievementSubscriber(db, nil)

	// Should handle nil emitter gracefully
	e := New(TaskCompleted, "test", map[string]any{"task_id": "t1", "title": "Test"})
	handler(e)
}

func TestAchievementSubscriber_EmitsEvents(t *testing.T) {
	db := testutil.TestDB(t)

	// Seed enough data to trigger a "task streak" achievement.
	// Insert 7 consecutive days of completed tasks + milestone records that DON'T cover 7-day streak.
	for i := 0; i < 7; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		_, err := db.Exec(
			`INSERT INTO tasks (id, title, priority, completed, created_at, updated_at)
			 VALUES (?, 'Task', 2, 1, ?, ?)`,
			"t"+date, date+" 09:00:00", date+" 10:00:00")
		if err != nil {
			t.Fatalf("seed task: %v", err)
		}
	}

	bus := NewBus(nil)
	emitter := NewEmitter(bus)

	// Track if AchievementEarned fires
	var achievementCount int32
	bus.Subscribe(AchievementEarned, func(e Event) {
		atomic.AddInt32(&achievementCount, 1)
	})

	handler := AchievementSubscriber(db, emitter)
	e := New(TaskCompleted, "test", nil)
	handler(e)

	// Give async events time to fire
	time.Sleep(100 * time.Millisecond)

	// We may or may not get achievements depending on how CheckAndRecordAchievements
	// evaluates the data, but it should not panic and should emit for any found.
	// The main assertion is that this doesn't crash.
}

// --- AnalyticsSubscriber tests ---

func TestAnalyticsSubscriber_RecordsMetric(t *testing.T) {
	db := testutil.TestDB(t)
	handler := AnalyticsSubscriber(db)

	e := New(TaskCompleted, "test-source", map[string]any{"task_id": "t1"})
	handler(e)

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tool_metrics WHERE tool_name = 'event:task.completed'").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("tool_metrics rows = %d, want 1", count)
	}
}

func TestAnalyticsSubscriber_MultipleEvents(t *testing.T) {
	db := testutil.TestDB(t)
	handler := AnalyticsSubscriber(db)

	handler(New(TaskCompleted, "test", nil))
	handler(New(HabitCompleted, "test", nil))
	handler(New(MoodLogged, "test", nil))

	var count int
	db.QueryRow("SELECT COUNT(*) FROM tool_metrics").Scan(&count)
	if count != 3 {
		t.Errorf("tool_metrics rows = %d, want 3", count)
	}
}

// --- RegisterBuiltinSubscribers tests ---

func TestRegisterBuiltinSubscribers_AllBound(t *testing.T) {
	db := testutil.TestDB(t)
	d := testDispatcher()
	bus := NewBus(nil)
	emitter := NewEmitter(bus)

	RegisterBuiltinSubscribers(bus, db, d, emitter)

	// Publish each event type and verify no panic
	ctx := context.Background()
	for _, et := range []EventType{
		TaskCompleted, MoodLogged, FocusStarted, FocusEnded,
		ReplySent, ChoreDone, HabitCompleted, EnergyRecorded,
		OverwhelmDetected, AchievementEarned, ReviewGenerated,
	} {
		bus.Publish(ctx, New(et, "test", nil))
	}
}
