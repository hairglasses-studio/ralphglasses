package trigger

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseCron_Every(t *testing.T) {
	d, daily, err := parseCron("@every 5m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if daily != nil {
		t.Fatal("expected nil daily time")
	}
	if d != 5*time.Minute {
		t.Errorf("duration = %v, want 5m", d)
	}
}

func TestParseCron_Hourly(t *testing.T) {
	d, daily, err := parseCron("@hourly")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if daily != nil {
		t.Fatal("expected nil daily time")
	}
	if d != time.Hour {
		t.Errorf("duration = %v, want 1h", d)
	}
}

func TestParseCron_Daily(t *testing.T) {
	_, daily, err := parseCron("@daily")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if daily == nil {
		t.Fatal("expected non-nil daily time")
	}
	if daily.Hour != 0 || daily.Minute != 0 {
		t.Errorf("daily = %d:%02d, want 00:00", daily.Hour, daily.Minute)
	}
}

func TestParseCron_TimeOfDay(t *testing.T) {
	_, daily, err := parseCron("14:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if daily == nil {
		t.Fatal("expected non-nil daily time")
	}
	if daily.Hour != 14 || daily.Minute != 30 {
		t.Errorf("daily = %d:%02d, want 14:30", daily.Hour, daily.Minute)
	}
}

func TestParseCron_InvalidDuration(t *testing.T) {
	_, _, err := parseCron("@every nope")
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestParseCron_NegativeDuration(t *testing.T) {
	_, _, err := parseCron("@every -5m")
	if err == nil {
		t.Fatal("expected error for negative duration")
	}
}

func TestParseCron_InvalidHour(t *testing.T) {
	_, _, err := parseCron("25:00")
	if err == nil {
		t.Fatal("expected error for invalid hour")
	}
}

func TestParseCron_InvalidMinute(t *testing.T) {
	_, _, err := parseCron("12:99")
	if err == nil {
		t.Fatal("expected error for invalid minute")
	}
}

func TestParseCron_UnsupportedExpression(t *testing.T) {
	_, _, err := parseCron("*/5 * * * *")
	if err == nil {
		t.Fatal("expected error for unsupported cron expression")
	}
}

func TestCronDaemon_AddSchedule(t *testing.T) {
	d := NewCronDaemon(nil)
	d.AddSchedule(Schedule{Name: "test", Cron: "@every 1m", Enabled: true})

	schedules := d.Schedules()
	if len(schedules) != 1 {
		t.Fatalf("len(schedules) = %d, want 1", len(schedules))
	}
	if schedules[0].Name != "test" {
		t.Errorf("name = %q, want %q", schedules[0].Name, "test")
	}
}

func TestCronDaemon_SetSchedules(t *testing.T) {
	d := NewCronDaemon(nil)
	d.AddSchedule(Schedule{Name: "old", Cron: "@hourly", Enabled: true})
	d.SetSchedules([]Schedule{
		{Name: "new1", Cron: "@every 5m", Enabled: true},
		{Name: "new2", Cron: "@daily", Enabled: true},
	})

	schedules := d.Schedules()
	if len(schedules) != 2 {
		t.Fatalf("len(schedules) = %d, want 2", len(schedules))
	}
}

func TestCronDaemon_FiresOnInterval(t *testing.T) {
	var count atomic.Int32

	d := NewCronDaemon(func(_ context.Context, req TriggerRequest) (string, error) {
		count.Add(1)
		return "run-ok", nil
	})
	d.AddSchedule(Schedule{
		Name:    "fast",
		Cron:    "@every 50ms",
		Source:  "test",
		Event:   "tick",
		Enabled: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = d.Start(ctx)

	c := count.Load()
	if c < 2 {
		t.Errorf("fire count = %d, want >= 2", c)
	}
}

func TestCronDaemon_DisabledSchedulesSkipped(t *testing.T) {
	var count atomic.Int32

	d := NewCronDaemon(func(_ context.Context, _ TriggerRequest) (string, error) {
		count.Add(1)
		return "", nil
	})
	d.AddSchedule(Schedule{
		Name:    "disabled",
		Cron:    "@every 10ms",
		Source:  "test",
		Event:   "tick",
		Enabled: false,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = d.Start(ctx)

	if c := count.Load(); c != 0 {
		t.Errorf("disabled schedule fired %d times", c)
	}
}

func TestCronDaemon_StopCancels(t *testing.T) {
	d := NewCronDaemon(func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	})
	d.AddSchedule(Schedule{
		Name:    "longrun",
		Cron:    "@every 1h",
		Source:  "test",
		Event:   "tick",
		Enabled: true,
	})

	done := make(chan struct{})
	go func() {
		_ = d.Start(context.Background())
		close(done)
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	if !d.Running() {
		t.Error("daemon should be running")
	}

	d.Stop()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop within 2s")
	}
}

func TestCronDaemon_NoEnabledSchedules(t *testing.T) {
	d := NewCronDaemon(nil)
	d.AddSchedule(Schedule{Name: "off", Cron: "@hourly", Enabled: false})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := d.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCronDaemon_DoubleStartReturnsError(t *testing.T) {
	d := NewCronDaemon(func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	})
	d.AddSchedule(Schedule{Name: "test", Cron: "@every 1h", Source: "t", Event: "e", Enabled: true})

	go func() {
		_ = d.Start(context.Background())
	}()

	// Give it time to set running = true
	time.Sleep(20 * time.Millisecond)

	err := d.Start(context.Background())
	if err == nil {
		t.Fatal("expected error on double start")
	}

	d.Stop()
}
