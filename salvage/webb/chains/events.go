package chains

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// EventType represents the type of event
type EventType string

const (
	EventIncidentCreated   EventType = "incident.created"
	EventIncidentUpdated   EventType = "incident.updated"
	EventIncidentResolved  EventType = "incident.resolved"
	EventTicketCreated     EventType = "ticket.created"
	EventTicketUpdated     EventType = "ticket.updated"
	EventTicketUrgent      EventType = "ticket.urgent"
	EventAlertFiring       EventType = "alert.firing"
	EventAlertResolved     EventType = "alert.resolved"
	EventDeployStarted     EventType = "deploy.started"
	EventDeployCompleted   EventType = "deploy.completed"
	EventDeployFailed      EventType = "deploy.failed"
	EventPROpened          EventType = "pr.opened"
	EventPRMerged          EventType = "pr.merged"
	EventHealthDegraded    EventType = "health.degraded"
	EventHealthCritical    EventType = "health.critical"
	EventShiftStart        EventType = "shift.start"
	EventShiftEnd          EventType = "shift.end"
	// Ephemeral cluster events (v132)
	EventEphemeralReady     EventType = "ephemeral.ready"
	EventEphemeralFailed    EventType = "ephemeral.failed"
	EventEphemeralDestroyed EventType = "ephemeral.destroyed"
)

// Event represents an event that can trigger chains
type Event struct {
	Type      EventType              `json:"type"`
	Source    string                 `json:"source"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// EventEmitter handles event emission and chain triggering
type EventEmitter struct {
	scheduler *Scheduler
	mu        sync.RWMutex
	handlers  map[EventType][]EventHandler
	history   []Event
	maxHistory int
}

// EventHandler is a function that handles an event
type EventHandler func(event Event) error

// NewEventEmitter creates a new event emitter
func NewEventEmitter(scheduler *Scheduler) *EventEmitter {
	return &EventEmitter{
		scheduler:  scheduler,
		handlers:   make(map[EventType][]EventHandler),
		history:    make([]Event, 0),
		maxHistory: 100,
	}
}

// Emit emits an event and triggers any registered chains
func (e *EventEmitter) Emit(event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Store in history
	e.mu.Lock()
	e.history = append(e.history, event)
	if len(e.history) > e.maxHistory {
		e.history = e.history[1:]
	}
	e.mu.Unlock()

	log.Printf("[chains] Event emitted: %s from %s", event.Type, event.Source)

	// Trigger chains via scheduler
	if e.scheduler != nil {
		if err := e.scheduler.TriggerEvent(string(event.Type), event.Data); err != nil {
			log.Printf("[chains] Failed to trigger chains for event %s: %v", event.Type, err)
		}
	}

	// Call registered handlers
	e.mu.RLock()
	handlers := e.handlers[event.Type]
	e.mu.RUnlock()

	for _, handler := range handlers {
		go func(h EventHandler) {
			if err := h(event); err != nil {
				log.Printf("[chains] Event handler error for %s: %v", event.Type, err)
			}
		}(handler)
	}

	return nil
}

// On registers a handler for an event type
func (e *EventEmitter) On(eventType EventType, handler EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[eventType] = append(e.handlers[eventType], handler)
}

// GetHistory returns recent events
func (e *EventEmitter) GetHistory(limit int) []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if limit <= 0 || limit > len(e.history) {
		limit = len(e.history)
	}

	// Return most recent first
	result := make([]Event, limit)
	for i := 0; i < limit; i++ {
		result[i] = e.history[len(e.history)-1-i]
	}
	return result
}

// GetHistoryByType returns recent events of a specific type
func (e *EventEmitter) GetHistoryByType(eventType EventType, limit int) []Event {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Event, 0)
	for i := len(e.history) - 1; i >= 0 && len(result) < limit; i-- {
		if e.history[i].Type == eventType {
			result = append(result, e.history[i])
		}
	}
	return result
}

// EmitIncidentCreated emits an incident created event
func (e *EventEmitter) EmitIncidentCreated(incidentID, customer, severity, title string) error {
	return e.Emit(Event{
		Type:   EventIncidentCreated,
		Source: "incident.io",
		Data: map[string]interface{}{
			"incident_id": incidentID,
			"customer":    customer,
			"severity":    severity,
			"title":       title,
		},
	})
}

// EmitTicketCreated emits a ticket created event
func (e *EventEmitter) EmitTicketCreated(ticketID, customer, priority, subject string) error {
	return e.Emit(Event{
		Type:   EventTicketCreated,
		Source: "pylon",
		Data: map[string]interface{}{
			"ticket_id": ticketID,
			"customer":  customer,
			"priority":  priority,
			"subject":   subject,
		},
	})
}

// EmitAlertFiring emits an alert firing event
func (e *EventEmitter) EmitAlertFiring(alertName, cluster, severity string) error {
	return e.Emit(Event{
		Type:   EventAlertFiring,
		Source: "grafana",
		Data: map[string]interface{}{
			"alert_name": alertName,
			"cluster":    cluster,
			"severity":   severity,
		},
	})
}

// EmitDeployCompleted emits a deploy completed event
func (e *EventEmitter) EmitDeployCompleted(release, cluster, namespace, version string) error {
	return e.Emit(Event{
		Type:   EventDeployCompleted,
		Source: "helm",
		Data: map[string]interface{}{
			"release":   release,
			"cluster":   cluster,
			"namespace": namespace,
			"version":   version,
		},
	})
}

// EmitHealthDegraded emits a health degraded event
func (e *EventEmitter) EmitHealthDegraded(cluster string, healthScore int, issues []string) error {
	return e.Emit(Event{
		Type:   EventHealthDegraded,
		Source: "health-monitor",
		Data: map[string]interface{}{
			"cluster":      cluster,
			"health_score": healthScore,
			"issues":       issues,
		},
	})
}

// EmitEphemeralClusterReady emits an ephemeral cluster ready event (v132)
func (e *EventEmitter) EmitEphemeralClusterReady(clusterID, cloud, owner, contextName string, ttlHours int) error {
	return e.Emit(Event{
		Type:   EventEphemeralReady,
		Source: "ephemeral-cluster",
		Data: map[string]interface{}{
			"cluster_id":   clusterID,
			"cloud":        cloud,
			"owner":        owner,
			"context_name": contextName,
			"ttl_hours":    ttlHours,
		},
	})
}

// EventBridge connects external systems to the event emitter
type EventBridge struct {
	emitter *EventEmitter
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewEventBridge creates a new event bridge
func NewEventBridge(emitter *EventEmitter) *EventBridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &EventBridge{
		emitter: emitter,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start begins listening for events from external sources
func (b *EventBridge) Start() error {
	log.Printf("[chains] Event bridge started")
	return nil
}

// Stop halts the event bridge
func (b *EventBridge) Stop() {
	b.cancel()
	log.Printf("[chains] Event bridge stopped")
}

// ProcessWebhook processes an incoming webhook and emits appropriate events
func (b *EventBridge) ProcessWebhook(source string, payload map[string]interface{}) error {
	switch source {
	case "incident.io":
		return b.processIncidentIOWebhook(payload)
	case "pylon":
		return b.processPylonWebhook(payload)
	case "grafana":
		return b.processGrafanaWebhook(payload)
	case "github":
		return b.processGitHubWebhook(payload)
	default:
		return fmt.Errorf("unknown webhook source: %s", source)
	}
}

func (b *EventBridge) processIncidentIOWebhook(payload map[string]interface{}) error {
	eventType, _ := payload["event_type"].(string)
	incident, _ := payload["incident"].(map[string]interface{})

	if incident == nil {
		return fmt.Errorf("missing incident data")
	}

	incidentID, _ := incident["id"].(string)
	severity, _ := incident["severity"].(string)
	title, _ := incident["title"].(string)
	customer, _ := incident["customer"].(string)

	var evtType EventType
	switch eventType {
	case "incident.created":
		evtType = EventIncidentCreated
	case "incident.updated":
		evtType = EventIncidentUpdated
	case "incident.resolved":
		evtType = EventIncidentResolved
	default:
		return nil // Ignore unknown event types
	}

	return b.emitter.Emit(Event{
		Type:   evtType,
		Source: "incident.io",
		Data: map[string]interface{}{
			"incident_id": incidentID,
			"severity":    severity,
			"title":       title,
			"customer":    customer,
		},
	})
}

func (b *EventBridge) processPylonWebhook(payload map[string]interface{}) error {
	eventType, _ := payload["event"].(string)
	ticket, _ := payload["ticket"].(map[string]interface{})

	if ticket == nil {
		return fmt.Errorf("missing ticket data")
	}

	ticketID, _ := ticket["id"].(string)
	priority, _ := ticket["priority"].(string)
	subject, _ := ticket["subject"].(string)
	customer, _ := ticket["customer"].(string)

	var evtType EventType
	switch eventType {
	case "ticket.created":
		evtType = EventTicketCreated
	case "ticket.updated":
		evtType = EventTicketUpdated
	default:
		return nil
	}

	// Check for urgent priority
	if priority == "urgent" || priority == "p0" {
		evtType = EventTicketUrgent
	}

	return b.emitter.Emit(Event{
		Type:   evtType,
		Source: "pylon",
		Data: map[string]interface{}{
			"ticket_id": ticketID,
			"priority":  priority,
			"subject":   subject,
			"customer":  customer,
		},
	})
}

func (b *EventBridge) processGrafanaWebhook(payload map[string]interface{}) error {
	status, _ := payload["status"].(string)
	alertName, _ := payload["alertname"].(string)
	labels, _ := payload["labels"].(map[string]interface{})

	cluster := ""
	severity := "warning"
	if labels != nil {
		cluster, _ = labels["cluster"].(string)
		if s, ok := labels["severity"].(string); ok {
			severity = s
		}
	}

	var evtType EventType
	if status == "firing" {
		evtType = EventAlertFiring
	} else {
		evtType = EventAlertResolved
	}

	return b.emitter.Emit(Event{
		Type:   evtType,
		Source: "grafana",
		Data: map[string]interface{}{
			"alert_name": alertName,
			"cluster":    cluster,
			"severity":   severity,
			"status":     status,
		},
	})
}

func (b *EventBridge) processGitHubWebhook(payload map[string]interface{}) error {
	action, _ := payload["action"].(string)
	pr, _ := payload["pull_request"].(map[string]interface{})

	if pr == nil {
		return nil // Not a PR event
	}

	prNumber, _ := pr["number"].(float64)
	title, _ := pr["title"].(string)
	merged, _ := pr["merged"].(bool)

	repo, _ := payload["repository"].(map[string]interface{})
	repoName := ""
	if repo != nil {
		repoName, _ = repo["full_name"].(string)
	}

	var evtType EventType
	switch action {
	case "opened":
		evtType = EventPROpened
	case "closed":
		if merged {
			evtType = EventPRMerged
		} else {
			return nil // PR closed without merge
		}
	default:
		return nil
	}

	return b.emitter.Emit(Event{
		Type:   evtType,
		Source: "github",
		Data: map[string]interface{}{
			"pr_number": int(prNumber),
			"title":     title,
			"repo":      repoName,
			"merged":    merged,
		},
	})
}
