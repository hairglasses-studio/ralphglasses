package notify

import (
	"fmt"
	"strings"
)

// Template defines a notification message template.
type Template struct {
	Title string
	Body  string
}

// DefaultTemplates maps event types to their default notification templates.
var DefaultTemplates = map[EventType]Template{
	EventSessionComplete: {
		Title: "Session Complete",
		Body:  "Session {{.SessionID}} finished for {{.Repo}} (cost: ${{.Cost}})",
	},
	EventBudgetWarning: {
		Title: "Budget Warning",
		Body:  "Session {{.SessionID}}: spent ${{.Spent}} of ${{.Budget}} ({{.Pct}}%)",
	},
	EventCircuitBreakerTrip: {
		Title: "Circuit Breaker Tripped",
		Body:  "Provider {{.Provider}} circuit breaker opened after {{.Failures}} failures",
	},
	EventCrash: {
		Title: "Session Crashed",
		Body:  "Session {{.SessionID}} crashed: {{.Error}}",
	},
	EventRestart: {
		Title: "Session Restarted",
		Body:  "Session {{.SessionID}} restarted (attempt {{.Attempt}})",
	},
}

// RenderTemplate fills a template with the provided values.
// Placeholders are {{.Key}} format, replaced from the values map.
func RenderTemplate(tmpl Template, values map[string]string) (title, body string) {
	title = renderString(tmpl.Title, values)
	body = renderString(tmpl.Body, values)
	return
}

// SendTemplated sends a notification using the template for the given event type.
func SendTemplated(eventType EventType, values map[string]string) error {
	tmpl, ok := DefaultTemplates[eventType]
	if !ok {
		return fmt.Errorf("no template for event type %s", eventType)
	}
	title, body := RenderTemplate(tmpl, values)
	return Send(title, body)
}

func renderString(s string, values map[string]string) string {
	result := s
	for k, v := range values {
		result = strings.ReplaceAll(result, "{{."+k+"}}", v)
	}
	return result
}
