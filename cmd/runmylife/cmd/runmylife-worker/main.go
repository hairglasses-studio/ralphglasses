package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/db"
	"github.com/hairglasses-studio/runmylife/internal/events"
	"github.com/hairglasses-studio/runmylife/internal/finance"
	"github.com/hairglasses-studio/runmylife/internal/intelligence"
	"github.com/hairglasses-studio/runmylife/internal/jobs"
	"github.com/hairglasses-studio/runmylife/internal/knowledge"
	"github.com/hairglasses-studio/runmylife/internal/srs"
	"github.com/hairglasses-studio/runmylife/internal/notifications"
	"github.com/hairglasses-studio/runmylife/internal/resilience"
	"github.com/hairglasses-studio/runmylife/internal/scheduler"
	"github.com/hairglasses-studio/runmylife/internal/timecontext"
	"github.com/hairglasses-studio/runmylife/internal/worker"
)

// Worker-level circuit breakers for external APIs.
// Separate from MCP tool breakers (different process).
var (
	workerGmailCB = resilience.NewCircuitBreaker("worker_gmail_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          60 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerCalendarCB = resilience.NewCircuitBreaker("worker_calendar_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          60 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerTodoistCB = resilience.NewCircuitBreaker("worker_todoist_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          120 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerDiscordCB = resilience.NewCircuitBreaker("worker_discord_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          120 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerWeatherCB = resilience.NewCircuitBreaker("worker_weather_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          120 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerSpotifyCB = resilience.NewCircuitBreaker("worker_spotify_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          120 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerFitbitCB = resilience.NewCircuitBreaker("worker_fitbit_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          120 * time.Second,
		HalfOpenMaxCalls: 1,
	})
	workerMessagesCB = resilience.NewCircuitBreaker("worker_messages_api", resilience.CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          120 * time.Second,
		HalfOpenMaxCalls: 1,
	})
)

// isRetryableError checks if an error is retryable (rate limit or server error).
func isRetryableError(err error) bool {
	var httpErr *clients.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.IsRetryable()
	}
	return false
}

// workerRetryConfig returns the standard retry config for worker API calls.
func workerRetryConfig() resilience.RetryConfig {
	return resilience.RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		RetryOn:      isRetryableError,
	}
}

// withWorkerRetry wraps an API call with circuit breaker + exponential backoff retry.
func withWorkerRetry(ctx context.Context, cb *resilience.CircuitBreaker, fn func() error) error {
	return resilience.Retry(ctx, workerRetryConfig(), func() error {
		return cb.Execute(fn)
	})
}

// withTimeout runs fn with a context deadline and panic recovery.
func withTimeout(parent context.Context, timeout time.Duration, name string, fn func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[worker] PANIC in %s: %v", name, r)
		}
	}()

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[worker] PANIC in %s goroutine: %v", name, r)
			}
			close(done)
		}()
		fn(ctx)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("[worker] TIMEOUT: %s exceeded %v", name, timeout)
		}
	}
}

func main() {
	log.Println("[worker] Starting runmylife background worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("[worker] Shutting down...")
		cancel()
	}()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[worker] Failed to load config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("[worker] Failed to open database: %v", err)
	}
	defer database.ForceClose()

	sqlDB := database.SqlDB()

	// Initialize notification dispatcher
	notify := notifications.NewDispatcher(cfg, sqlDB)

	// Initialize event bus with persistence and built-in subscribers
	eventBus := events.NewBus(sqlDB)
	emitter := events.NewEmitter(eventBus)
	events.RegisterBuiltinSubscribers(eventBus, sqlDB, notify, emitter)
	log.Println("[worker] Event bus initialized with built-in subscribers")

	// Initialize workflow scheduler/executor
	workflowExec := scheduler.NewExecutor(sqlDB)

	// Log active configuration
	block := timecontext.CurrentBlock()
	log.Printf("[worker] Time block: %s | Priorities: %v", block.Label(), block.Priorities())

	// --- Startup burst: run all tasks once immediately ---
	log.Println("[worker] Running startup burst...")
	withTimeout(ctx, 120*time.Second, "gmail-poll", func(c context.Context) { runGmailPoll(c, sqlDB) })
	withTimeout(ctx, 120*time.Second, "calendar-sync", func(c context.Context) { runCalendarSync(c, sqlDB) })
	withTimeout(ctx, 60*time.Second, "task-sync", func(c context.Context) { runTaskSync(c, sqlDB, cfg, emitter) })
	withTimeout(ctx, 120*time.Second, "reply-radar", func(c context.Context) { runReplyRadar(c, sqlDB, cfg, notify) })
	withTimeout(ctx, 60*time.Second, "discord-sync", func(c context.Context) { runDiscordSync(c, sqlDB, cfg) })
	withTimeout(ctx, 30*time.Second, "spotify-state", func(c context.Context) { runSpotifyState(c, sqlDB, cfg) })
	withTimeout(ctx, 30*time.Second, "weather-cache", func(c context.Context) { runWeatherCache(c, sqlDB, cfg) })
	withTimeout(ctx, 120*time.Second, "fitness-sync", func(c context.Context) { runFitnessSync(c, sqlDB, cfg) })
	withTimeout(ctx, 60*time.Second, "social-health", func(c context.Context) { runSocialHealthRecalc(c, sqlDB) })
	withTimeout(ctx, 30*time.Second, "intelligence", func(c context.Context) { runIntelligenceEngine(c, sqlDB, emitter) })
	withTimeout(ctx, 30*time.Second, "overwhelm-check", func(c context.Context) { runOverwhelmCheck(c, sqlDB, notify, emitter) })
	withTimeout(ctx, 30*time.Second, "time-blindness", func(c context.Context) { runTimeBlindnessCheck(c, sqlDB, notify, emitter) })
	withTimeout(ctx, 30*time.Second, "outreach", func(c context.Context) { runOutreachReminders(c, sqlDB, notify) })
	withTimeout(ctx, 30*time.Second, "scheduler", func(c context.Context) { workflowExec.CheckAndRun(c) })
	withTimeout(ctx, 30*time.Second, "metrics-cleanup", func(c context.Context) { runMetricsCleanup(c, sqlDB) })
	withTimeout(ctx, 15*time.Second, "srs-reminder", func(c context.Context) { runSRSReminder(c, sqlDB, notify) })
	log.Println("[worker] Startup burst complete")

	// --- Create tickers ---
	gmailTicker := time.NewTicker(5 * time.Minute)
	defer gmailTicker.Stop()

	calendarTicker := time.NewTicker(15 * time.Minute)
	defer calendarTicker.Stop()

	taskTicker := time.NewTicker(10 * time.Minute)
	defer taskTicker.Stop()

	replyRadarTicker := time.NewTicker(15 * time.Minute)
	defer replyRadarTicker.Stop()

	discordTicker := time.NewTicker(15 * time.Minute)
	defer discordTicker.Stop()

	spotifyTicker := time.NewTicker(30 * time.Minute)
	defer spotifyTicker.Stop()

	weatherTicker := time.NewTicker(2 * time.Hour)
	defer weatherTicker.Stop()

	fitnessTicker := time.NewTicker(6 * time.Hour)
	defer fitnessTicker.Stop()

	socialHealthTicker := time.NewTicker(24 * time.Hour)
	defer socialHealthTicker.Stop()

	intelligenceTicker := time.NewTicker(1 * time.Hour)
	defer intelligenceTicker.Stop()

	morningBriefingTicker := time.NewTicker(1 * time.Minute) // check every minute for 7 AM
	defer morningBriefingTicker.Stop()

	outreachTicker := time.NewTicker(24 * time.Hour)
	defer outreachTicker.Stop()

	queueTicker := time.NewTicker(10 * time.Second)
	defer queueTicker.Stop()

	schedulerTicker := time.NewTicker(1 * time.Minute) // check workflows every minute
	defer schedulerTicker.Stop()

	overwhelmTicker := time.NewTicker(30 * time.Minute)
	defer overwhelmTicker.Stop()

	timeBlindnessTicker := time.NewTicker(5 * time.Minute)
	defer timeBlindnessTicker.Stop()

	metricsCleanupTicker := time.NewTicker(24 * time.Hour)
	defer metricsCleanupTicker.Stop()

	srsReminderTicker := time.NewTicker(2 * time.Hour)
	defer srsReminderTicker.Stop()

	log.Println("[worker] Entering main loop")

	var lastBriefingDate string // track last morning briefing date

	for {
		select {
		case <-ctx.Done():
			log.Println("[worker] Stopped")
			return

		case <-gmailTicker.C:
			withTimeout(ctx, 120*time.Second, "gmail-poll", func(c context.Context) { runGmailPoll(c, sqlDB) })

		case <-calendarTicker.C:
			withTimeout(ctx, 120*time.Second, "calendar-sync", func(c context.Context) { runCalendarSync(c, sqlDB) })

		case <-taskTicker.C:
			withTimeout(ctx, 60*time.Second, "task-sync", func(c context.Context) { runTaskSync(c, sqlDB, cfg, emitter) })

		case <-replyRadarTicker.C:
			withTimeout(ctx, 120*time.Second, "reply-radar", func(c context.Context) { runReplyRadar(c, sqlDB, cfg, notify) })

		case <-discordTicker.C:
			withTimeout(ctx, 60*time.Second, "discord-sync", func(c context.Context) { runDiscordSync(c, sqlDB, cfg) })

		case <-spotifyTicker.C:
			withTimeout(ctx, 30*time.Second, "spotify-state", func(c context.Context) { runSpotifyState(c, sqlDB, cfg) })

		case <-weatherTicker.C:
			withTimeout(ctx, 30*time.Second, "weather-cache", func(c context.Context) { runWeatherCache(c, sqlDB, cfg) })

		case <-fitnessTicker.C:
			withTimeout(ctx, 120*time.Second, "fitness-sync", func(c context.Context) { runFitnessSync(c, sqlDB, cfg) })

		case <-socialHealthTicker.C:
			withTimeout(ctx, 60*time.Second, "social-health", func(c context.Context) { runSocialHealthRecalc(c, sqlDB) })

		case <-intelligenceTicker.C:
			withTimeout(ctx, 30*time.Second, "intelligence", func(c context.Context) { runIntelligenceEngine(c, sqlDB, emitter) })

		case <-morningBriefingTicker.C:
			now := time.Now()
			today := now.Format("2006-01-02")
			if now.Hour() == 7 && now.Minute() < 2 && lastBriefingDate != today {
				lastBriefingDate = today
				withTimeout(ctx, 300*time.Second, "morning-briefing", func(c context.Context) { runMorningBriefing(c, sqlDB, notify, emitter) })
			}

		case <-outreachTicker.C:
			withTimeout(ctx, 30*time.Second, "outreach", func(c context.Context) { runOutreachReminders(c, sqlDB, notify) })

		case <-schedulerTicker.C:
			if !scheduler.SuppressNightBlock() {
				withTimeout(ctx, 30*time.Second, "scheduler", func(c context.Context) { workflowExec.CheckAndRun(c) })
			}

		case <-overwhelmTicker.C:
			withTimeout(ctx, 30*time.Second, "overwhelm-check", func(c context.Context) { runOverwhelmCheck(c, sqlDB, notify, emitter) })

		case <-timeBlindnessTicker.C:
			withTimeout(ctx, 30*time.Second, "time-blindness", func(c context.Context) { runTimeBlindnessCheck(c, sqlDB, notify, emitter) })

		case <-queueTicker.C:
			withTimeout(ctx, 60*time.Second, "queue-process", func(c context.Context) { runQueueProcessor(c, sqlDB, emitter, notify) })

		case <-metricsCleanupTicker.C:
			withTimeout(ctx, 30*time.Second, "metrics-cleanup", func(c context.Context) { runMetricsCleanup(c, sqlDB) })

		case <-srsReminderTicker.C:
			withTimeout(ctx, 15*time.Second, "srs-reminder", func(c context.Context) { runSRSReminder(c, sqlDB, notify) })
		}
	}
}

// recordSync logs a sync operation to sync_history.
func recordSync(ctx context.Context, db *sql.DB, source, status string, count int, errMsg string) {
	_, _ = db.ExecContext(ctx,
		"INSERT INTO sync_history (source, status, records_synced, error_message, completed_at) VALUES (?, ?, ?, ?, ?)",
		source, status, count, errMsg, time.Now().Format(time.RFC3339),
	)
}

// --- Gmail Poll ---

func runGmailPoll(ctx context.Context, sqlDB *sql.DB) {
	log.Println("[worker] Polling Gmail...")
	err := withWorkerRetry(ctx, workerGmailCB, func() error {
		client, err := clients.NewGmailAPIClient(ctx)
		if err != nil {
			return err
		}

		messages, err := client.FetchMessageHeaders(ctx, "in:inbox newer_than:7d", 100)
		if err != nil {
			return err
		}

		count := 0
		for _, m := range messages {
			_, err := sqlDB.ExecContext(ctx,
				`INSERT OR REPLACE INTO gmail_messages (id, thread_id, from_addr, subject, snippet, body, timestamp, labels, triaged)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
				m.ID, m.ThreadID, m.From, m.Subject, m.Snippet, m.Body,
				m.Date.Format(time.RFC3339), m.Labels,
			)
			if err == nil {
				count++
			}
		}
		recordSync(ctx, sqlDB, "gmail", "success", count, "")
		log.Printf("[worker] Gmail: synced %d messages", count)
		return nil
	})
	if err != nil {
		recordSync(ctx, sqlDB, "gmail", "error", 0, err.Error())
		log.Printf("[worker] Gmail poll error: %v", err)
	}
}

// --- Calendar Sync ---

func runCalendarSync(ctx context.Context, sqlDB *sql.DB) {
	log.Println("[worker] Syncing Calendar...")
	err := withWorkerRetry(ctx, workerCalendarCB, func() error {
		client, err := clients.NewCalendarAPIClient(ctx, "")
		if err != nil {
			return err
		}

		now := time.Now()
		events, err := client.FetchEvents(ctx, now.AddDate(0, 0, -7), now.AddDate(0, 0, 30), 100)
		if err != nil {
			return err
		}

		count := 0
		for _, e := range events {
			_, err := sqlDB.ExecContext(ctx,
				`INSERT OR REPLACE INTO calendar_events (id, summary, description, start_time, end_time, location, attendees)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				e.ID, e.Summary, e.Description,
				e.StartTime.Format(time.RFC3339), e.EndTime.Format(time.RFC3339),
				e.Location, e.Attendees,
			)
			if err == nil {
				count++
			}
		}
		recordSync(ctx, sqlDB, "calendar", "success", count, "")
		log.Printf("[worker] Calendar: synced %d events", count)
		return nil
	})
	if err != nil {
		recordSync(ctx, sqlDB, "calendar", "error", 0, err.Error())
		log.Printf("[worker] Calendar sync error: %v", err)
	}
}

// --- Task Sync (Todoist) ---

func runTaskSync(ctx context.Context, sqlDB *sql.DB, cfg *config.Config, emitter *events.Emitter) {
	token := cfg.Credentials["todoist"]
	if token == "" {
		log.Println("[worker] Task sync skipped — no Todoist token")
		return
	}
	log.Println("[worker] Syncing Todoist tasks...")

	// Snapshot active tasks before sync to detect completions
	activeBefore := make(map[string]string) // todoist_id -> title
	rows, err := sqlDB.QueryContext(ctx,
		"SELECT todoist_id, title FROM tasks WHERE completed = 0 AND todoist_id != ''")
	if err == nil {
		for rows.Next() {
			var id, title string
			if rows.Scan(&id, &title) == nil {
				activeBefore[id] = title
			}
		}
		rows.Close()
	}

	var syncedIDs map[string]bool
	err = withWorkerRetry(ctx, workerTodoistCB, func() error {
		client := clients.NewTodoistClient(token)
		tasks, err := client.ListTasks(ctx, "", "")
		if err != nil {
			return err
		}

		syncedIDs = make(map[string]bool, len(tasks))
		count := 0
		for _, t := range tasks {
			syncedIDs[t.ID] = true
			dueDate := ""
			if t.Due != nil {
				dueDate = t.Due.Date
			}
			_, err := sqlDB.ExecContext(ctx,
				`INSERT OR REPLACE INTO tasks (id, todoist_id, title, description, priority, project, due_date, completed)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				"todoist-"+t.ID, t.ID, t.Content, t.Description, t.Priority, t.ProjectID, dueDate, 0,
			)
			if err == nil {
				count++
			}
		}
		recordSync(ctx, sqlDB, "todoist", "success", count, "")
		log.Printf("[worker] Todoist: synced %d tasks", count)
		return nil
	})
	if err != nil {
		recordSync(ctx, sqlDB, "todoist", "error", 0, err.Error())
		log.Printf("[worker] Todoist sync error: %v", err)
		return
	}

	// Detect completions: tasks that were active but no longer in Todoist
	for id, title := range activeBefore {
		if !syncedIDs[id] {
			_, _ = sqlDB.ExecContext(ctx,
				"UPDATE tasks SET completed = 1 WHERE todoist_id = ?", id)
			emitter.TaskCompleted(ctx, id, title)
			log.Printf("[worker] Task completed: %s", title)
		}
	}
}

// --- Reply Radar ---

func runReplyRadar(ctx context.Context, sqlDB *sql.DB, cfg *config.Config, notify *notifications.Dispatcher) {
	log.Println("[worker] Scanning reply radar...")

	// Count pending replies across channels
	var gmailPending, discordPending, smsPending int

	_ = sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending' AND channel = 'gmail'`,
	).Scan(&gmailPending)

	_ = sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending' AND channel = 'discord'`,
	).Scan(&discordPending)

	_ = sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending' AND channel = 'sms'`,
	).Scan(&smsPending)

	total := gmailPending + discordPending + smsPending
	log.Printf("[worker] Reply radar: %d pending (gmail=%d, discord=%d, sms=%d)",
		total, gmailPending, discordPending, smsPending)

	// Notify if reply debt is growing
	if total >= 10 {
		notify.Send(ctx, notifications.Notification{
			Title:   "Reply debt is high",
			Message: fmt.Sprintf("You have %d pending replies (gmail=%d, discord=%d, sms=%d). Consider a reply session.", total, gmailPending, discordPending, smsPending),
			Urgency: notifications.UrgencyHigh,
			Source:  "reply_radar",
		})
	} else if total >= 5 {
		notify.Send(ctx, notifications.Notification{
			Title:   "Pending replies building up",
			Message: fmt.Sprintf("%d pending replies across channels.", total),
			Urgency: notifications.UrgencyNormal,
			Source:  "reply_radar",
		})
	}

	// Detect new unreplied messages from Gmail
	err := withWorkerRetry(ctx, workerGmailCB, func() error {
		client, err := clients.NewGmailAPIClient(ctx)
		if err != nil {
			return err
		}

		messages, err := client.FetchMessageHeaders(ctx, "in:inbox is:unread", 50)
		if err != nil {
			return err
		}

		newReplies := 0
		for _, m := range messages {
			var exists int
			_ = sqlDB.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM reply_tracker WHERE message_id = ?", m.ID,
			).Scan(&exists)
			if exists == 0 {
				_, err := sqlDB.ExecContext(ctx,
					`INSERT INTO reply_tracker (message_id, channel, from_addr, subject, received_at, status)
					 VALUES (?, 'gmail', ?, ?, ?, 'pending')`,
					m.ID, m.From, m.Subject, m.Date.Format(time.RFC3339),
				)
				if err == nil {
					newReplies++
				}
			}
		}
		if newReplies > 0 {
			log.Printf("[worker] Reply radar: tracked %d new Gmail messages", newReplies)
		}
		return nil
	})
	if err != nil {
		log.Printf("[worker] Reply radar Gmail scan error: %v", err)
	}
}

// --- Discord Sync ---

func runDiscordSync(ctx context.Context, sqlDB *sql.DB, cfg *config.Config) {
	token := cfg.Credentials["discord"]
	if token == "" {
		log.Println("[worker] Discord sync skipped — no bot token")
		return
	}
	log.Println("[worker] Syncing Discord...")

	err := withWorkerRetry(ctx, workerDiscordCB, func() error {
		client := clients.NewDiscordClient(token)
		guilds, err := client.GetGuilds(ctx)
		if err != nil {
			return err
		}

		serverCount, channelCount := 0, 0
		for _, g := range guilds {
			_, err := sqlDB.ExecContext(ctx,
				`INSERT OR REPLACE INTO discord_servers (id, name, icon_url, member_count, cached_at) VALUES (?, ?, ?, ?, ?)`,
				g.ID, g.Name, g.Icon, g.MemberCount, time.Now().Format(time.RFC3339),
			)
			if err == nil {
				serverCount++
			}

			channels, err := client.GetChannels(ctx, g.ID)
			if err != nil {
				continue
			}
			for _, ch := range channels {
				_, err := sqlDB.ExecContext(ctx,
					`INSERT OR REPLACE INTO discord_channels (id, server_id, name, type, topic, cached_at) VALUES (?, ?, ?, ?, ?, ?)`,
					ch.ID, g.ID, ch.Name, fmt.Sprintf("%d", ch.Type), ch.Topic, time.Now().Format(time.RFC3339),
				)
				if err == nil {
					channelCount++
				}
			}
		}
		recordSync(ctx, sqlDB, "discord", "success", serverCount+channelCount, "")
		log.Printf("[worker] Discord: cached %d servers, %d channels", serverCount, channelCount)
		return nil
	})
	if err != nil {
		recordSync(ctx, sqlDB, "discord", "error", 0, err.Error())
		log.Printf("[worker] Discord sync error: %v", err)
	}
}

// --- Spotify State ---

func runSpotifyState(ctx context.Context, sqlDB *sql.DB, cfg *config.Config) {
	token := cfg.Credentials["spotify"]
	if token == "" {
		return // silent skip
	}
	log.Println("[worker] Checking Spotify state...")

	err := withWorkerRetry(ctx, workerSpotifyCB, func() error {
		client := clients.NewSpotifyClient(token)
		np, err := client.NowPlaying(ctx)
		if err != nil {
			return err
		}
		if np == nil || !np.IsPlaying {
			log.Println("[worker] Spotify: no active playback")
			return nil
		}
		trackName := ""
		if np.Track != nil {
			trackName = np.Track.Name
		}
		log.Printf("[worker] Spotify: playing %q on %s", trackName, np.DeviceName)
		return nil
	})
	if err != nil {
		log.Printf("[worker] Spotify state error: %v", err)
	}
}

// --- Weather Cache ---

func runWeatherCache(ctx context.Context, sqlDB *sql.DB, cfg *config.Config) {
	if cfg.Location == nil {
		return // silent skip
	}
	log.Println("[worker] Refreshing weather cache...")

	err := withWorkerRetry(ctx, workerWeatherCB, func() error {
		client := clients.NewWeatherClient(cfg.Location.Latitude, cfg.Location.Longitude)
		current, err := client.GetCurrent(ctx)
		if err != nil {
			return err
		}

		dataJSON, _ := json.Marshal(current)
		locationKey := fmt.Sprintf("%.4f,%.4f", cfg.Location.Latitude, cfg.Location.Longitude)

		_, _ = sqlDB.ExecContext(ctx,
			`INSERT INTO weather_cache (location_key, data_json, forecast_type, fetched_at) VALUES (?, ?, 'current', ?)`,
			locationKey, string(dataJSON), time.Now().Format(time.RFC3339),
		)

		log.Printf("[worker] Weather: %.1f°C", current.Temperature)
		recordSync(ctx, sqlDB, "weather", "success", 1, "")
		return nil
	})
	if err != nil {
		recordSync(ctx, sqlDB, "weather", "error", 0, err.Error())
		log.Printf("[worker] Weather cache error: %v", err)
	}
}

// --- Fitness Sync (Fitbit) ---

func runFitnessSync(ctx context.Context, sqlDB *sql.DB, cfg *config.Config) {
	token := cfg.Credentials["fitbit"]
	if token == "" {
		return // silent skip
	}
	log.Println("[worker] Syncing Fitbit data...")

	err := withWorkerRetry(ctx, workerFitbitCB, func() error {
		client := clients.NewFitbitClient(token)
		today := time.Now().Format("2006-01-02")

		stats, err := client.DailyStats(ctx, today)
		if err != nil {
			return err
		}

		_, _ = sqlDB.ExecContext(ctx,
			`INSERT OR REPLACE INTO fitness_daily_stats (date, steps, calories, distance, active_minutes, data_json, synced_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			today, stats.Steps, stats.Calories, stats.Distance, stats.ActiveMinutes,
			"", time.Now().Format(time.RFC3339),
		)

		recordSync(ctx, sqlDB, "fitbit", "success", 1, "")
		log.Printf("[worker] Fitbit: %d steps, %d cal", stats.Steps, stats.Calories)
		return nil
	})
	if err != nil {
		recordSync(ctx, sqlDB, "fitbit", "error", 0, err.Error())
		log.Printf("[worker] Fitbit sync error: %v", err)
	}
}

// --- Social Health Recalculation ---

func runSocialHealthRecalc(ctx context.Context, sqlDB *sql.DB) {
	log.Println("[worker] Recalculating social health scores...")

	// Update relationship health based on recent interactions
	_, _ = sqlDB.ExecContext(ctx, `
		UPDATE relationship_health SET
			health_score = CASE
				WHEN last_interaction_at > datetime('now', '-7 days') THEN MIN(health_score + 5, 100)
				WHEN last_interaction_at > datetime('now', '-30 days') THEN health_score
				WHEN last_interaction_at > datetime('now', '-90 days') THEN MAX(health_score - 10, 0)
				ELSE MAX(health_score - 25, 0)
			END,
			updated_at = datetime('now')
		WHERE contact_id IS NOT NULL
	`)

	var total, healthy, declining int
	_ = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health").Scan(&total)
	_ = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health WHERE health_score >= 70").Scan(&healthy)
	_ = sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health WHERE health_score < 40").Scan(&declining)

	log.Printf("[worker] Social health: %d total, %d healthy, %d declining", total, healthy, declining)
}

// --- Intelligence Engine ---

func runIntelligenceEngine(ctx context.Context, sqlDB *sql.DB, emitter *events.Emitter) {
	block := timecontext.CurrentBlock()
	log.Printf("[worker] Running intelligence engine (block: %s)...", block)

	// 1. Generate rule-based suggestions
	engine := intelligence.NewEngine(sqlDB)
	suggestions := engine.GenerateSuggestions(ctx)

	// 2. Convert finance anomalies to suggestions
	anomalies := finance.DetectAnomalies(ctx, sqlDB)
	for _, a := range anomalies {
		priority := 0.4
		if a.Severity == finance.SeverityAlert {
			priority = 0.8
		} else if a.Severity == finance.SeverityWarning {
			priority = 0.6
		}
		suggestions = append(suggestions, intelligence.Suggestion{
			Category:    "finances",
			Priority:    priority,
			Title:       a.Message,
			Description: fmt.Sprintf("[%s] %s", a.Type, a.Description),
			ActionHint:  "runmylife_finances(domain=analytics, action=anomalies)",
		})
	}

	// 3. Cross-module correlations
	for _, c := range intelligence.AnalyzeMoodSleep(ctx, sqlDB, 14) {
		suggestions = append(suggestions, intelligence.Suggestion{
			Category:    "wellness",
			Priority:    0.5,
			Title:       c.Insight,
			Description: c.Pattern,
			ActionHint:  "runmylife_wellness(domain=mood, action=trend)",
		})
	}
	for _, c := range intelligence.AnalyzeFocusEnergy(ctx, sqlDB, 14) {
		suggestions = append(suggestions, intelligence.Suggestion{
			Category:    "wellness",
			Priority:    0.45,
			Title:       c.Insight,
			Description: c.Pattern,
			ActionHint:  "runmylife_wellness(domain=energy, action=optimize)",
		})
	}
	for _, c := range intelligence.AnalyzeExerciseMood(ctx, sqlDB, 14) {
		suggestions = append(suggestions, intelligence.Suggestion{
			Category:    "wellness",
			Priority:    0.45,
			Title:       c.Insight,
			Description: c.Pattern,
			ActionHint:  "runmylife_wellness(domain=mood, action=trend)",
		})
	}

	// 4. Refresh knowledge graph links
	links, _ := knowledge.BuildFromDB(ctx, sqlDB)
	if links > 0 {
		log.Printf("[worker] Intelligence: refreshed %d knowledge graph links", links)
	}

	// 5. Enrich suggestions with knowledge graph context
	suggestions = intelligence.EnrichFromGraph(ctx, sqlDB, suggestions)

	// 6. Add graph-based suggestions (stale contacts, batchable tasks)
	graphSuggestions := intelligence.GraphBasedSuggestions(ctx, sqlDB)
	suggestions = append(suggestions, graphSuggestions...)

	// 7. Persist and prune
	intelligence.PersistSuggestions(ctx, sqlDB, suggestions)
	pruned := intelligence.PruneSuggestions(ctx, sqlDB, time.Now().AddDate(0, 0, -30))

	log.Printf("[worker] Intelligence: %d suggestions generated, %d old pruned", len(suggestions), pruned)

	if len(suggestions) > 0 {
		emitter.ReviewGenerated(ctx, "intelligence")
	}
}

// --- Morning Briefing ---

func runMorningBriefing(ctx context.Context, sqlDB *sql.DB, notify *notifications.Dispatcher, emitter *events.Emitter) {
	log.Println("[worker] Generating morning briefing...")

	var tasksDue, unreplied, eventsToday int
	_ = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date = date('now')").Scan(&tasksDue)
	_ = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&unreplied)
	_ = sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM calendar_events WHERE date(start_time) = date('now')").Scan(&eventsToday)

	log.Printf("[worker] Morning briefing: %d tasks due, %d unreplied, %d events today",
		tasksDue, unreplied, eventsToday)

	// Store briefing as a daily snapshot
	briefingJSON, _ := json.Marshal(map[string]int{
		"tasks_due":    tasksDue,
		"unreplied":    unreplied,
		"events_today": eventsToday,
	})
	_, _ = sqlDB.ExecContext(ctx,
		`INSERT OR REPLACE INTO daily_snapshots (date, category, data_json, created_at)
		 VALUES (date('now'), 'morning_briefing', ?, datetime('now'))`,
		string(briefingJSON),
	)

	// Send morning briefing notification
	notify.Send(ctx, notifications.Notification{
		Title: "Morning Briefing",
		Message: fmt.Sprintf("Tasks due: %d | Pending replies: %d | Events today: %d",
			tasksDue, unreplied, eventsToday),
		Urgency: notifications.UrgencyHigh,
		Source:  "morning_briefing",
	})

	emitter.ReviewGenerated(ctx, "morning")
}

// --- Outreach Reminders ---

func runOutreachReminders(ctx context.Context, sqlDB *sql.DB, notify *notifications.Dispatcher) {
	log.Println("[worker] Checking outreach reminders...")

	var overdue int
	_ = sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM outreach_reminders
		 WHERE status = 'pending' AND next_outreach_at <= datetime('now')`,
	).Scan(&overdue)

	if overdue > 0 {
		log.Printf("[worker] Outreach: %d overdue reminders", overdue)
		notify.Send(ctx, notifications.Notification{
			Title:   "Outreach reminders overdue",
			Message: fmt.Sprintf("You have %d overdue outreach reminders. Check your social circle.", overdue),
			Urgency: notifications.UrgencyNormal,
			Source:  "outreach_reminders",
		})
	}
}

// --- Job Queue Processor ---

func runQueueProcessor(ctx context.Context, sqlDB *sql.DB, emitter *events.Emitter, notify *notifications.Dispatcher) {
	pending, err := jobs.ListPending(ctx, sqlDB, 10)
	if err != nil {
		log.Printf("[worker] Queue processor error: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}

	log.Printf("[worker] Processing %d queued jobs...", len(pending))

	for _, job := range pending {
		// Lock the job
		_, err := sqlDB.ExecContext(ctx,
			"UPDATE job_queue SET status = 'running', locked_at = datetime('now'), attempts = attempts + 1 WHERE id = ? AND status IN ('pending', 'failed')",
			job.ID,
		)
		if err != nil {
			continue
		}

		// Execute based on type
		jobErr := executeJob(ctx, sqlDB, emitter, notify, job)

		if jobErr != nil {
			if job.Attempts+1 >= job.MaxAttempts {
				// Dead letter
				_, _ = sqlDB.ExecContext(ctx,
					"UPDATE job_queue SET status = 'dead', error_message = ? WHERE id = ?",
					jobErr.Error(), job.ID,
				)
				log.Printf("[worker] Job %d (%s) dead-lettered: %v", job.ID, job.Type, jobErr)
			} else {
				// Retry with backoff
				backoff := time.Duration(1<<uint(job.Attempts)) * time.Minute
				nextRun := time.Now().Add(backoff).Format("2006-01-02T15:04:05")
				_, _ = sqlDB.ExecContext(ctx,
					"UPDATE job_queue SET status = 'failed', error_message = ?, next_run_at = ?, locked_at = NULL WHERE id = ?",
					jobErr.Error(), nextRun, job.ID,
				)
				log.Printf("[worker] Job %d (%s) failed, retry in %v: %v", job.ID, job.Type, backoff, jobErr)
			}
		} else {
			_, _ = sqlDB.ExecContext(ctx,
				"UPDATE job_queue SET status = 'completed', completed_at = datetime('now'), locked_at = NULL WHERE id = ?",
				job.ID,
			)
		}
	}
}

// executeJob dispatches a job to the appropriate handler.
func executeJob(ctx context.Context, sqlDB *sql.DB, emitter *events.Emitter, notify *notifications.Dispatcher, job jobs.Job) error {
	switch job.Type {
	case "sync_gmail":
		runGmailPoll(ctx, sqlDB)
	case "sync_calendar":
		runCalendarSync(ctx, sqlDB)
	case "sync_todoist":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runTaskSync(ctx, sqlDB, cfg, emitter)
	case "sync_discord":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runDiscordSync(ctx, sqlDB, cfg)
	case "sync_weather":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		runWeatherCache(ctx, sqlDB, cfg)
	default:
		jc := &worker.JobContext{DB: sqlDB, Emitter: emitter, Notify: notify}
		return worker.HandleJob(ctx, jc, job.Type, job.Payload)
	}
	return nil
}

// --- ADHD: Overwhelm Check ---

func runOverwhelmCheck(ctx context.Context, sqlDB *sql.DB, notify *notifications.Dispatcher, emitter *events.Emitter) {
	score, err := adhd.RunOverwhelmCheck(ctx, sqlDB)
	if err != nil {
		log.Printf("[worker] Overwhelm check error: %v", err)
		return
	}

	emitter.OverwhelmDetected(ctx, score.CompositeScore, score.TriageActivated)

	// Derive energy level as inverse of overwhelm (10 = low overwhelm, 1 = high overwhelm)
	energyLevel := 10 - int(score.CompositeScore*9)
	if energyLevel < 1 {
		energyLevel = 1
	}
	emitter.EnergyRecorded(ctx, energyLevel)

	if score.TriageActivated {
		tasks, _ := adhd.GetTopTriageTasks(ctx, sqlDB)
		msg := fmt.Sprintf("Overwhelm score: %.0f%%. Focus only on these:\n", score.CompositeScore*100)
		for i, t := range tasks {
			msg += fmt.Sprintf("%d. %s\n", i+1, t)
		}
		notify.Send(ctx, notifications.Notification{
			Title:   "Triage mode activated",
			Message: msg,
			Urgency: notifications.UrgencyHigh,
			Source:  "overwhelm_detector",
		})
	}
}

// --- ADHD: Time Blindness Check ---

func runTimeBlindnessCheck(ctx context.Context, sqlDB *sql.DB, notify *notifications.Dispatcher, emitter *events.Emitter) {
	// Check for upcoming events
	alerts, err := adhd.CheckTimeBlinds(ctx, sqlDB, 30)
	if err != nil {
		return
	}
	for _, alert := range alerts {
		notify.Send(ctx, notifications.Notification{
			Title:   "Upcoming event",
			Message: alert,
			Urgency: notifications.UrgencyHigh,
			Source:  "time_blindness",
		})
	}

	// Check for hyperfocus (>3 hours in same category)
	category, minutes, ok := adhd.CheckFocusSession(ctx, sqlDB)
	if ok && minutes >= 180 {
		emitter.FocusEnded(ctx, category, minutes)
		notify.Send(ctx, notifications.Notification{
			Title:   "Hyperfocus alert",
			Message: fmt.Sprintf("You've been in %q for %d minutes. Take a break or check if you need to switch.", category, minutes),
			Urgency: notifications.UrgencyHigh,
			Source:  "hyperfocus_detector",
		})
	} else if ok && minutes >= 90 {
		emitter.FocusStarted(ctx, category)
		notify.Send(ctx, notifications.Notification{
			Title:   "Focus check-in",
			Message: fmt.Sprintf("You've been in %q for %d minutes. Still on track?", category, minutes),
			Urgency: notifications.UrgencyNormal,
			Source:  "hyperfocus_detector",
		})
	}
}

// --- Metrics Cleanup ---

func runMetricsCleanup(ctx context.Context, sqlDB *sql.DB) {
	log.Println("[worker] Cleaning up old metrics...")

	// Clean old tool_metrics (>30 days)
	result, _ := sqlDB.ExecContext(ctx,
		"DELETE FROM tool_metrics WHERE recorded_at < datetime('now', '-30 days')")
	if result != nil {
		n, _ := result.RowsAffected()
		if n > 0 {
			log.Printf("[worker] Cleaned %d old tool_metrics rows", n)
		}
	}

	// Clean old sync_history (>30 days)
	result, _ = sqlDB.ExecContext(ctx,
		"DELETE FROM sync_history WHERE completed_at < datetime('now', '-30 days')")
	if result != nil {
		n, _ := result.RowsAffected()
		if n > 0 {
			log.Printf("[worker] Cleaned %d old sync_history rows", n)
		}
	}

	// Clean completed jobs (>7 days)
	cleaned, _ := jobs.ClearCompleted(ctx, sqlDB, 7*24*time.Hour)
	if cleaned > 0 {
		log.Printf("[worker] Cleaned %d completed jobs", cleaned)
	}
}

// --- SRS Reminder ---

func runSRSReminder(ctx context.Context, sqlDB *sql.DB, notify *notifications.Dispatcher) {
	count := srs.CountDueCards(ctx, sqlDB)
	if count == 0 {
		return
	}

	cards := srs.GetDueCardsSummary(ctx, sqlDB, 5)
	msg := srs.FormatReminder(count, cards)

	urgency := notifications.UrgencyNormal
	if count >= 20 {
		urgency = notifications.UrgencyHigh
	}

	notify.Send(ctx, notifications.Notification{
		Title:   fmt.Sprintf("%d SRS cards due", count),
		Message: msg,
		Urgency: urgency,
		Source:  "srs_reminder",
	})
	log.Printf("[worker] SRS reminder: %d cards due", count)
}
