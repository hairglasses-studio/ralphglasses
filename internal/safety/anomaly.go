package safety

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// AnomalyType classifies the kind of anomaly detected.
type AnomalyType string

const (
	CostSpike      AnomalyType = "cost_spike"       // Session spend > threshold * rolling average
	ErrorStorm     AnomalyType = "error_storm"       // Error rate > threshold in time window
	RunawaySession AnomalyType = "runaway_session"   // Session exceeds expected duration
	CascadeFailure AnomalyType = "cascade_failure"   // Multiple sessions fail in rapid succession
)

// Severity indicates how urgent an anomaly is.
type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Anomaly represents a detected fleet anomaly.
type Anomaly struct {
	Type              AnomalyType `json:"type"`
	Severity          Severity    `json:"severity"`
	SessionID         string      `json:"session_id,omitempty"`
	Description       string      `json:"description"`
	Timestamp         time.Time   `json:"timestamp"`
	RecommendedAction string      `json:"recommended_action"`
}

// AnomalyConfig controls the thresholds and behavior of the anomaly detector.
type AnomalyConfig struct {
	// WindowSize is the time window for rate calculations (default 5m).
	WindowSize time.Duration

	// CostSpikeThreshold is the multiplier above rolling average that
	// triggers a cost spike anomaly (default 3.0).
	CostSpikeThreshold float64

	// ErrorRateThreshold is the fraction (0-1) of events that are errors
	// before an error storm is flagged (default 0.5).
	ErrorRateThreshold float64

	// DurationMultiplier is the factor above expected session duration
	// that triggers a runaway detection (default 2.0).
	DurationMultiplier float64

	// CascadeCount is how many session failures within CascadeWindow
	// trigger a cascade failure anomaly (default 3).
	CascadeCount int

	// CascadeWindow is the time window for cascade failure detection (default 1m).
	CascadeWindow time.Duration

	// KillSwitchEnabled controls whether critical anomalies auto-engage
	// the kill switch (default false).
	KillSwitchEnabled bool
}

// DefaultAnomalyConfig returns a config with sensible defaults.
func DefaultAnomalyConfig() AnomalyConfig {
	return AnomalyConfig{
		WindowSize:         5 * time.Minute,
		CostSpikeThreshold: 3.0,
		ErrorRateThreshold: 0.5,
		DurationMultiplier: 2.0,
		CascadeCount:       3,
		CascadeWindow:      1 * time.Minute,
		KillSwitchEnabled:  false,
	}
}

// AnomalyDetector monitors fleet events for anomalous patterns and can
// optionally engage a kill switch when critical anomalies are detected.
type AnomalyDetector struct {
	mu        sync.Mutex
	bus       *events.Bus
	cfg       AnomalyConfig
	ks        *KillSwitch
	callbacks []func(Anomaly)

	// Cost tracking: session ID -> list of cost observations
	costHistory map[string][]costPoint

	// Error tracking: recent error and total event timestamps
	errorTimes []time.Time
	eventTimes []time.Time

	// Session duration tracking: session ID -> start time
	sessionStarts map[string]time.Time

	// Cascade tracking: recent failure timestamps
	failureTimes []time.Time

	// Lifecycle
	cancel context.CancelFunc
	done   chan struct{}
}

// costPoint records a single cost observation.
type costPoint struct {
	ts    time.Time
	value float64
}

// NewAnomalyDetector creates a detector that subscribes to the given event bus.
// Pass nil for ks if no kill switch integration is needed.
func NewAnomalyDetector(bus *events.Bus, cfg AnomalyConfig, ks *KillSwitch) *AnomalyDetector {
	return &AnomalyDetector{
		bus:           bus,
		cfg:           cfg,
		ks:            ks,
		costHistory:   make(map[string][]costPoint),
		sessionStarts: make(map[string]time.Time),
		done:          make(chan struct{}),
	}
}

// OnAnomaly registers a callback that fires whenever an anomaly is detected.
// Multiple callbacks can be registered; they are called in order.
func (d *AnomalyDetector) OnAnomaly(fn func(Anomaly)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callbacks = append(d.callbacks, fn)
}

// Start begins listening for events on the bus. It blocks until Stop is
// called or the context is cancelled.
func (d *AnomalyDetector) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()

	ch := d.bus.SubscribeFiltered("safety.anomaly",
		events.CostUpdate,
		events.SessionError,
		events.SessionStarted,
		events.SessionEnded,
		events.SessionStopped,
		events.BudgetExceeded,
	)

	go func() {
		defer close(d.done)
		for {
			select {
			case <-ctx.Done():
				d.bus.Unsubscribe("safety.anomaly")
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				d.handleEvent(ev)
			}
		}
	}()
}

// Stop cancels the event listener and waits for it to finish.
func (d *AnomalyDetector) Stop() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	<-d.done
}

// handleEvent dispatches an event to the appropriate detection logic.
func (d *AnomalyDetector) handleEvent(ev events.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := ev.Timestamp
	if now.IsZero() {
		now = time.Now()
	}

	switch ev.Type {
	case events.CostUpdate:
		d.checkCostSpike(ev, now)

	case events.SessionError:
		d.recordError(now)
		d.checkErrorStorm(ev, now)

	case events.SessionStarted:
		if ev.SessionID != "" {
			d.sessionStarts[ev.SessionID] = now
		}
		d.recordEvent(now)

	case events.SessionEnded:
		d.checkRunaway(ev, now)
		d.recordEvent(now)
		delete(d.sessionStarts, ev.SessionID)

	case events.SessionStopped:
		d.recordEvent(now)
		delete(d.sessionStarts, ev.SessionID)

	case events.BudgetExceeded:
		d.recordFailure(now)
		d.checkCascadeFailure(ev, now)
	}
}

// checkCostSpike detects when a session's cost rate exceeds the configured
// multiple of its rolling average.
func (d *AnomalyDetector) checkCostSpike(ev events.Event, now time.Time) {
	sid := ev.SessionID
	if sid == "" {
		return
	}

	var cost float64
	if v, ok := ev.Data["spent_usd"]; ok {
		switch val := v.(type) {
		case float64:
			cost = val
		case int:
			cost = float64(val)
		}
	}
	if cost <= 0 {
		return
	}

	d.costHistory[sid] = append(d.costHistory[sid], costPoint{ts: now, value: cost})

	// Trim to window
	cutoff := now.Add(-d.cfg.WindowSize)
	pts := d.costHistory[sid]
	trimIdx := 0
	for trimIdx < len(pts) && pts[trimIdx].ts.Before(cutoff) {
		trimIdx++
	}
	if trimIdx > 0 {
		d.costHistory[sid] = pts[trimIdx:]
		pts = d.costHistory[sid]
	}

	// Need at least 3 data points for a meaningful average
	if len(pts) < 3 {
		return
	}

	// Rolling average of all points except the latest
	var sum float64
	for _, p := range pts[:len(pts)-1] {
		sum += p.value
	}
	avg := sum / float64(len(pts)-1)
	if avg <= 0 {
		return
	}

	ratio := cost / avg
	if ratio >= d.cfg.CostSpikeThreshold {
		d.fireAnomaly(Anomaly{
			Type:              CostSpike,
			Severity:          SeverityWarning,
			SessionID:         sid,
			Description:       "session cost rate is %.1fx the rolling average",
			Timestamp:         now,
			RecommendedAction: "review session budget and throttle if needed",
		}, ratio)
	}
}

// recordError adds an error timestamp to the sliding window.
func (d *AnomalyDetector) recordError(now time.Time) {
	d.errorTimes = append(d.errorTimes, now)
	d.eventTimes = append(d.eventTimes, now)
	d.trimWindow(&d.errorTimes, now)
	d.trimWindow(&d.eventTimes, now)
}

// recordEvent adds a general event timestamp to the sliding window.
func (d *AnomalyDetector) recordEvent(now time.Time) {
	d.eventTimes = append(d.eventTimes, now)
	d.trimWindow(&d.eventTimes, now)
}

// recordFailure adds a failure timestamp for cascade tracking.
func (d *AnomalyDetector) recordFailure(now time.Time) {
	d.failureTimes = append(d.failureTimes, now)
	d.trimCascadeWindow(&d.failureTimes, now)
}

// checkErrorStorm detects when the error rate within the window exceeds the threshold.
func (d *AnomalyDetector) checkErrorStorm(ev events.Event, now time.Time) {
	totalEvents := len(d.eventTimes)
	totalErrors := len(d.errorTimes)

	if totalEvents < 4 {
		return // Need a meaningful sample
	}

	rate := float64(totalErrors) / float64(totalEvents)
	if rate >= d.cfg.ErrorRateThreshold {
		d.fireAnomaly(Anomaly{
			Type:              ErrorStorm,
			Severity:          SeverityCritical,
			SessionID:         ev.SessionID,
			Description:       "error rate in window exceeds threshold",
			Timestamp:         now,
			RecommendedAction: "investigate error sources; consider pausing fleet",
		}, rate)
	}
}

// checkRunaway detects sessions that exceed the expected duration.
func (d *AnomalyDetector) checkRunaway(ev events.Event, now time.Time) {
	sid := ev.SessionID
	if sid == "" {
		return
	}

	start, ok := d.sessionStarts[sid]
	if !ok {
		return
	}

	// Check for expected_duration in event data (seconds)
	var expectedDur time.Duration
	if v, ok := ev.Data["expected_duration_sec"]; ok {
		switch val := v.(type) {
		case float64:
			expectedDur = time.Duration(val) * time.Second
		case int:
			expectedDur = time.Duration(val) * time.Second
		}
	}

	// If no expected duration in event, use a default of 30 minutes
	if expectedDur <= 0 {
		expectedDur = 30 * time.Minute
	}

	actual := now.Sub(start)
	threshold := time.Duration(float64(expectedDur) * d.cfg.DurationMultiplier)

	if actual > threshold {
		d.fireAnomaly(Anomaly{
			Type:              RunawaySession,
			Severity:          SeverityWarning,
			SessionID:         sid,
			Description:       "session exceeded expected duration",
			Timestamp:         now,
			RecommendedAction: "check session for stall or infinite loop",
		}, 0)
	}
}

// checkCascadeFailure detects rapid successive failures across sessions.
func (d *AnomalyDetector) checkCascadeFailure(ev events.Event, now time.Time) {
	if len(d.failureTimes) >= d.cfg.CascadeCount {
		d.fireAnomaly(Anomaly{
			Type:              CascadeFailure,
			Severity:          SeverityCritical,
			SessionID:         ev.SessionID,
			Description:       "multiple sessions failed in rapid succession",
			Timestamp:         now,
			RecommendedAction: "halt fleet and investigate systemic issue",
		}, 0)
	}
}

// fireAnomaly publishes the anomaly to the bus, notifies callbacks, and
// optionally engages the kill switch for critical anomalies.
func (d *AnomalyDetector) fireAnomaly(a Anomaly, detail float64) {
	slog.Warn("anomaly detected",
		"type", a.Type,
		"severity", a.Severity,
		"session_id", a.SessionID,
		"description", a.Description,
	)

	// Publish to event bus
	if d.bus != nil {
		_ = d.bus.PublishCtx(context.Background(), events.Event{
			Type:      events.AnomalyDetected,
			Timestamp: a.Timestamp,
			SessionID: a.SessionID,
			Data: map[string]any{
				"anomaly_type": string(a.Type),
				"severity":     string(a.Severity),
				"description":  a.Description,
				"detail":       detail,
			},
		})
	}

	// Fire registered callbacks
	for _, fn := range d.callbacks {
		fn(a)
	}

	// Kill switch for critical anomalies
	if a.Severity == SeverityCritical && d.cfg.KillSwitchEnabled && d.ks != nil {
		d.ks.Engage("anomaly: " + string(a.Type) + " - " + a.Description)
	}
}

// trimWindow removes timestamps older than WindowSize from a slice.
func (d *AnomalyDetector) trimWindow(times *[]time.Time, now time.Time) {
	cutoff := now.Add(-d.cfg.WindowSize)
	idx := 0
	for idx < len(*times) && (*times)[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		*times = (*times)[idx:]
	}
}

// trimCascadeWindow removes timestamps older than CascadeWindow from a slice.
func (d *AnomalyDetector) trimCascadeWindow(times *[]time.Time, now time.Time) {
	cutoff := now.Add(-d.cfg.CascadeWindow)
	idx := 0
	for idx < len(*times) && (*times)[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		*times = (*times)[idx:]
	}
}
