package trigger

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Schedule defines a recurring trigger configuration.
type Schedule struct {
	Name     string         `json:"name"`
	Cron     string         `json:"cron"`      // simplified cron: "@every 5m", "@hourly", or "HH:MM" daily
	Source   string         `json:"source"`     // trigger source label
	Event    string         `json:"event"`      // trigger event label
	Payload  map[string]any `json:"payload,omitempty"`
	Enabled  bool           `json:"enabled"`
}

// CronDaemon reads schedule configs and fires triggers on schedule.
// It supports simplified schedule expressions:
//   - "@every <duration>" (e.g. "@every 5m", "@every 1h")
//   - "@hourly" (every hour on the hour)
//   - "@daily" (every day at midnight)
//   - "HH:MM" (daily at the specified time, 24h format)
type CronDaemon struct {
	mu        sync.RWMutex
	schedules []Schedule
	launcher  SessionLauncher
	stops     []context.CancelFunc
	running   bool
}

// NewCronDaemon creates a cron daemon that fires triggers via the given launcher.
func NewCronDaemon(launcher SessionLauncher) *CronDaemon {
	return &CronDaemon{
		launcher: launcher,
	}
}

// AddSchedule registers a schedule. The daemon must be restarted for new
// schedules to take effect.
func (d *CronDaemon) AddSchedule(s Schedule) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.schedules = append(d.schedules, s)
}

// SetSchedules replaces all schedules. The daemon must be restarted for
// changes to take effect.
func (d *CronDaemon) SetSchedules(schedules []Schedule) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.schedules = make([]Schedule, len(schedules))
	copy(d.schedules, schedules)
}

// Schedules returns a copy of all registered schedules.
func (d *CronDaemon) Schedules() []Schedule {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]Schedule, len(d.schedules))
	copy(out, d.schedules)
	return out
}

// Start launches a goroutine for each enabled schedule. It blocks until
// the context is cancelled.
func (d *CronDaemon) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("cron daemon already running")
	}
	d.running = true

	var schedules []Schedule
	for _, s := range d.schedules {
		if s.Enabled {
			schedules = append(schedules, s)
		}
	}
	d.mu.Unlock()

	if len(schedules) == 0 {
		slog.Info("cron daemon: no enabled schedules, waiting for context cancellation")
		<-ctx.Done()
		return nil
	}

	slog.Info("cron daemon started", "schedules", len(schedules))

	var wg sync.WaitGroup
	for _, sched := range schedules {
		sched := sched
		childCtx, cancel := context.WithCancel(ctx)

		d.mu.Lock()
		d.stops = append(d.stops, cancel)
		d.mu.Unlock()

		wg.Add(1)
		go func() {
			defer wg.Done()
			d.runSchedule(childCtx, sched)
		}()
	}

	wg.Wait()

	d.mu.Lock()
	d.running = false
	d.stops = nil
	d.mu.Unlock()

	return nil
}

// Stop cancels all running schedule goroutines.
func (d *CronDaemon) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, cancel := range d.stops {
		cancel()
	}
}

// Running returns whether the daemon is currently active.
func (d *CronDaemon) Running() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// runSchedule runs a single schedule loop until the context is cancelled.
func (d *CronDaemon) runSchedule(ctx context.Context, sched Schedule) {
	interval, daily, err := parseCron(sched.Cron)
	if err != nil {
		slog.Error("cron: invalid schedule expression", "name", sched.Name, "cron", sched.Cron, "error", err)
		return
	}

	slog.Info("cron: schedule active", "name", sched.Name, "cron", sched.Cron)

	if daily != nil {
		d.runDailySchedule(ctx, sched, *daily)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.fire(ctx, sched)
		}
	}
}

// dailyTime represents a specific time of day (hour, minute).
type dailyTime struct {
	Hour   int
	Minute int
}

// runDailySchedule fires a trigger at a specific time each day.
func (d *CronDaemon) runDailySchedule(ctx context.Context, sched Schedule, dt dailyTime) {
	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), dt.Hour, dt.Minute, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}

		wait := time.Until(next)
		timer := time.NewTimer(wait)

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			d.fire(ctx, sched)
		}
	}
}

func (d *CronDaemon) fire(ctx context.Context, sched Schedule) {
	req := TriggerRequest{
		Source:  sched.Source,
		Event:   sched.Event,
		Payload: sched.Payload,
	}

	if d.launcher == nil {
		slog.Warn("cron: no launcher configured", "name", sched.Name)
		return
	}

	runID, err := d.launcher(ctx, req)
	if err != nil {
		slog.Error("cron: trigger failed", "name", sched.Name, "error", err)
		return
	}
	slog.Info("cron: triggered", "name", sched.Name, "run_id", runID)
}

// parseCron parses a simplified cron expression.
// Returns either a fixed interval or a daily time, plus any error.
func parseCron(expr string) (time.Duration, *dailyTime, error) {
	expr = strings.TrimSpace(expr)

	switch expr {
	case "@hourly":
		return time.Hour, nil, nil
	case "@daily":
		return 0, &dailyTime{Hour: 0, Minute: 0}, nil
	}

	// "@every <duration>"
	if strings.HasPrefix(expr, "@every ") {
		durStr := strings.TrimPrefix(expr, "@every ")
		d, err := time.ParseDuration(durStr)
		if err != nil {
			return 0, nil, fmt.Errorf("invalid duration %q: %w", durStr, err)
		}
		if d <= 0 {
			return 0, nil, fmt.Errorf("duration must be positive: %s", durStr)
		}
		return d, nil, nil
	}

	// "HH:MM" daily schedule
	if parts := strings.SplitN(expr, ":", 2); len(parts) == 2 {
		hour, err := strconv.Atoi(parts[0])
		if err != nil || hour < 0 || hour > 23 {
			return 0, nil, fmt.Errorf("invalid hour in %q", expr)
		}
		minute, err := strconv.Atoi(parts[1])
		if err != nil || minute < 0 || minute > 59 {
			return 0, nil, fmt.Errorf("invalid minute in %q", expr)
		}
		return 0, &dailyTime{Hour: hour, Minute: minute}, nil
	}

	return 0, nil, fmt.Errorf("unsupported cron expression: %q (use @every <dur>, @hourly, @daily, or HH:MM)", expr)
}
