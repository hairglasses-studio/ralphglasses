package notifications

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func emptyConfig() *config.Config {
	return &config.Config{Credentials: map[string]string{}}
}

func TestRouteByUrgency_Critical(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	channels := d.routeByUrgency(UrgencyCritical)
	if len(channels) != 2 {
		t.Fatalf("critical channels = %d, want 2", len(channels))
	}
	if channels[0] != ChannelDiscordDM {
		t.Errorf("channel[0] = %v, want discord_dm", channels[0])
	}
	if channels[1] != ChannelHA {
		t.Errorf("channel[1] = %v, want homeassistant", channels[1])
	}
}

func TestRouteByUrgency_High(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	channels := d.routeByUrgency(UrgencyHigh)
	if len(channels) != 1 || channels[0] != ChannelDiscordDM {
		t.Errorf("high channels = %v, want [discord_dm]", channels)
	}
}

func TestRouteByUrgency_Normal(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	channels := d.routeByUrgency(UrgencyNormal)
	if len(channels) != 1 || channels[0] != ChannelSlack {
		t.Errorf("normal channels = %v, want [slack]", channels)
	}
}

func TestRouteByUrgency_Low(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	channels := d.routeByUrgency(UrgencyLow)
	if len(channels) != 1 || channels[0] != ChannelLog {
		t.Errorf("low channels = %v, want [log]", channels)
	}
}

func TestRateLimiting_LogNeverLimited(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	// Log channel should never be rate limited
	for i := 0; i < 20; i++ {
		if d.isRateLimited(ChannelLog) {
			t.Fatalf("log channel was rate limited after %d calls", i)
		}
		d.recordSend(ChannelLog)
	}
}

func TestRateLimiting_ExceedsMax(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	// maxPerHour is 5
	for i := 0; i < 5; i++ {
		if d.isRateLimited(ChannelSlack) {
			t.Fatalf("slack limited after only %d sends", i)
		}
		d.recordSend(ChannelSlack)
	}
	// 6th should be limited
	if !d.isRateLimited(ChannelSlack) {
		t.Error("slack should be rate limited after 5 sends")
	}
}

func TestRateLimiting_DifferentChannels(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	// Fill up Slack
	for i := 0; i < 5; i++ {
		d.recordSend(ChannelSlack)
	}
	// Discord should still be fine
	if d.isRateLimited(ChannelDiscordDM) {
		t.Error("discord should not be limited when only slack is full")
	}
}

func TestSend_LowUrgency_LogOnly(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	delivered := d.Send(context.Background(), Notification{
		Title:   "Test",
		Message: "Low urgency test",
		Urgency: UrgencyLow,
		Source:  "test",
	})
	// Low urgency goes to log only
	if len(delivered) != 1 || delivered[0] != ChannelLog {
		t.Errorf("delivered = %v, want [log]", delivered)
	}
}

func TestSend_NormalUrgency_SlackFallback(t *testing.T) {
	// No slack_webhook configured — Slack fallback to log (returns nil error)
	d := NewDispatcher(emptyConfig(), nil)
	delivered := d.Send(context.Background(), Notification{
		Title:   "Test",
		Message: "Normal urgency test",
		Urgency: UrgencyNormal,
		Source:  "test",
	})
	// Slack with no webhook falls back to log, still counts as delivered
	if len(delivered) != 1 || delivered[0] != ChannelSlack {
		t.Errorf("delivered = %v, want [slack]", delivered)
	}
}

func TestSend_HighUrgency_DiscordFails(t *testing.T) {
	// No discord token — should fail gracefully
	d := NewDispatcher(emptyConfig(), nil)
	delivered := d.Send(context.Background(), Notification{
		Title:   "Test",
		Message: "High urgency test",
		Urgency: UrgencyHigh,
		Source:  "test",
	})
	// Discord DM will fail (no token), nothing delivered
	if len(delivered) != 0 {
		t.Errorf("delivered = %v, want [] (no discord creds)", delivered)
	}
}

func TestLogToDB(t *testing.T) {
	db := testutil.TestDB(t)
	d := NewDispatcher(emptyConfig(), db)

	d.logToDB(context.Background(), Notification{
		Title:   "Test Alert",
		Message: "Something happened",
		Urgency: UrgencyHigh,
		Source:  "test_source",
	}, []Channel{ChannelDiscordDM})

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM notification_log").Scan(&count)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Errorf("notification_log rows = %d, want 1", count)
	}

	var title, urgency, source, channels string
	db.QueryRow("SELECT title, urgency, source, channels FROM notification_log LIMIT 1").
		Scan(&title, &urgency, &source, &channels)
	if title != "Test Alert" {
		t.Errorf("title = %q, want 'Test Alert'", title)
	}
	if urgency != "high" {
		t.Errorf("urgency = %q, want 'high'", urgency)
	}
	if source != "test_source" {
		t.Errorf("source = %q, want 'test_source'", source)
	}
	if channels != "discord_dm" {
		t.Errorf("channels = %q, want 'discord_dm'", channels)
	}
}

func TestLogToDB_NilDB(t *testing.T) {
	d := NewDispatcher(emptyConfig(), nil)
	// Should not panic with nil DB
	d.logToDB(context.Background(), Notification{
		Title: "Test", Message: "msg", Urgency: UrgencyLow,
	}, nil)
}

func TestLogToDB_MultipleChannels(t *testing.T) {
	db := testutil.TestDB(t)
	d := NewDispatcher(emptyConfig(), db)

	d.logToDB(context.Background(), Notification{
		Title: "Multi", Message: "msg", Urgency: UrgencyCritical, Source: "test",
	}, []Channel{ChannelDiscordDM, ChannelHA})

	var channels string
	db.QueryRow("SELECT channels FROM notification_log LIMIT 1").Scan(&channels)
	if channels != "discord_dm,homeassistant" {
		t.Errorf("channels = %q, want 'discord_dm,homeassistant'", channels)
	}
}

func TestUrgencyString(t *testing.T) {
	tests := []struct {
		u    Urgency
		want string
	}{
		{UrgencyLow, "low"},
		{UrgencyNormal, "normal"},
		{UrgencyHigh, "high"},
		{UrgencyCritical, "critical"},
		{Urgency(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.u.String(); got != tt.want {
			t.Errorf("Urgency(%d).String() = %q, want %q", tt.u, got, tt.want)
		}
	}
}

func TestSend_WithDBLogging(t *testing.T) {
	db := testutil.TestDB(t)
	d := NewDispatcher(emptyConfig(), db)

	d.Send(context.Background(), Notification{
		Title:   "Integration",
		Message: "Full pipeline test",
		Urgency: UrgencyLow,
		Source:  "test",
	})

	var count int
	db.QueryRow("SELECT COUNT(*) FROM notification_log").Scan(&count)
	if count != 1 {
		t.Errorf("notification_log rows = %d, want 1 (Send should log)", count)
	}
}
