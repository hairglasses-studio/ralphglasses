package events

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/notifications"
)

// RegisterBuiltinSubscribers wires up the standard event subscribers.
func RegisterBuiltinSubscribers(bus *Bus, db *sql.DB, notify *notifications.Dispatcher, emitter *Emitter) {
	// Notification routing for events that warrant user attention
	bus.Subscribe(TaskCompleted, NotificationSubscriber(notify))
	bus.Subscribe(OverwhelmDetected, NotificationSubscriber(notify))
	bus.Subscribe(AchievementEarned, NotificationSubscriber(notify))
	bus.Subscribe(MoodLogged, NotificationSubscriber(notify))
	bus.Subscribe(HabitCompleted, NotificationSubscriber(notify))
	bus.Subscribe(FocusEnded, NotificationSubscriber(notify))
	bus.Subscribe(ReviewGenerated, NotificationSubscriber(notify))

	// Achievement detection on completions — emits AchievementEarned back into the bus
	bus.Subscribe(TaskCompleted, AchievementSubscriber(db, emitter))
	bus.Subscribe(HabitCompleted, AchievementSubscriber(db, emitter))
	bus.Subscribe(FocusEnded, AchievementSubscriber(db, emitter))

	// Analytics tracking
	bus.Subscribe(TaskCompleted, AnalyticsSubscriber(db))
	bus.Subscribe(HabitCompleted, AnalyticsSubscriber(db))
	bus.Subscribe(FocusEnded, AnalyticsSubscriber(db))
	bus.Subscribe(ReplySent, AnalyticsSubscriber(db))
	bus.Subscribe(MoodLogged, AnalyticsSubscriber(db))
}

// NotificationSubscriber routes events to notification dispatch based on urgency.
func NotificationSubscriber(notify *notifications.Dispatcher) Handler {
	return func(e Event) {
		var n notifications.Notification
		n.Source = fmt.Sprintf("event:%s", e.Type)

		switch e.Type {
		case OverwhelmDetected:
			n.Title = "Overwhelm Alert"
			score, _ := e.Payload["score"].(float64)
			n.Message = fmt.Sprintf("Overwhelm score is %.0f — consider triaging to top-3 tasks only.", score)
			n.Urgency = notifications.UrgencyHigh

		case AchievementEarned:
			title, _ := e.Payload["title"].(string)
			desc, _ := e.Payload["description"].(string)
			n.Title = fmt.Sprintf("Achievement: %s", title)
			n.Message = desc
			n.Urgency = notifications.UrgencyNormal

		case TaskCompleted:
			title, _ := e.Payload["title"].(string)
			n.Title = "Task Done"
			n.Message = fmt.Sprintf("Completed: %s", title)
			n.Urgency = notifications.UrgencyLow

		case MoodLogged:
			score, _ := e.Payload["score"].(float64)
			n.Title = "Mood Logged"
			n.Message = fmt.Sprintf("Mood recorded: %.0f/10", score)
			n.Urgency = notifications.UrgencyLow

		case HabitCompleted:
			name, _ := e.Payload["name"].(string)
			n.Title = "Habit Done"
			n.Message = fmt.Sprintf("Completed: %s", name)
			n.Urgency = notifications.UrgencyLow

		case FocusEnded:
			category, _ := e.Payload["category"].(string)
			minutes, _ := e.Payload["minutes"].(float64)
			n.Title = "Focus Session Ended"
			n.Message = fmt.Sprintf("%s: %d minutes", category, int(minutes))
			n.Urgency = notifications.UrgencyLow

		case ReviewGenerated:
			reviewType, _ := e.Payload["review_type"].(string)
			n.Title = "Review Ready"
			n.Message = fmt.Sprintf("%s review generated", reviewType)
			n.Urgency = notifications.UrgencyLow

		default:
			return // no notification for unhandled types
		}

		notify.Send(context.Background(), n)
	}
}

// AchievementSubscriber triggers achievement checks on task/habit/focus completions.
// When new achievements are found, emits AchievementEarned events so notifications fire.
func AchievementSubscriber(db *sql.DB, emitter *Emitter) Handler {
	return func(e Event) {
		ctx := context.Background()
		celebrations := adhd.CheckAndRecordAchievements(ctx, db)
		if len(celebrations) > 0 {
			log.Printf("[events] %d new achievements earned", len(celebrations))
			if emitter != nil {
				for _, c := range celebrations {
					emitter.AchievementEarned(ctx, c.Title, c.Description)
				}
			}
		}
	}
}

// AnalyticsSubscriber records event occurrences in tool_metrics for
// throughput tracking alongside tool call metrics.
func AnalyticsSubscriber(db *sql.DB) Handler {
	return func(e Event) {
		metricKey := fmt.Sprintf("event:%s", e.Type)
		_, err := db.ExecContext(context.Background(),
			`INSERT INTO tool_metrics (tool_name, duration_ms, is_error, created_at)
			 VALUES (?, 0, FALSE, ?)`,
			metricKey, e.Timestamp.Format(time.RFC3339),
		)
		if err != nil {
			log.Printf("[events] analytics record error: %v", err)
		}
	}
}
