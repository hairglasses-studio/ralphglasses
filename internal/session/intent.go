package session

import (
	"fmt"
	"strings"
)

// IntentType classifies the purpose of a user or system action.
type IntentType string

const (
	IntentLaunch    IntentType = "launch"
	IntentStop      IntentType = "stop"
	IntentPause     IntentType = "pause"
	IntentResume    IntentType = "resume"
	IntentScale     IntentType = "scale"
	IntentConfigure IntentType = "configure"
	IntentQuery     IntentType = "query"
	IntentEscalate  IntentType = "escalate"
)

// Intent represents a typed, validated action request that can be routed
// through the session lifecycle. Every tool call or NL command is first
// converted to an Intent before execution.
type Intent struct {
	Type        IntentType        `json:"type"`
	SessionID   string            `json:"session_id,omitempty"`
	Provider    Provider          `json:"provider,omitempty"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	Destructive bool              `json:"destructive"` // requires confirmation
	Source      string            `json:"source"`      // "nl", "mcp", "api", "schedule"
}

// Validate checks that the intent has all required fields for its type.
func (i *Intent) Validate() error {
	switch i.Type {
	case IntentLaunch:
		if i.Provider == "" {
			return fmt.Errorf("launch intent requires provider")
		}
	case IntentStop, IntentPause, IntentResume:
		if i.SessionID == "" {
			return fmt.Errorf("%s intent requires session_id", i.Type)
		}
	case IntentScale:
		if _, ok := i.Parameters["count"]; !ok {
			return fmt.Errorf("scale intent requires count parameter")
		}
	case IntentConfigure:
		if _, ok := i.Parameters["key"]; !ok {
			return fmt.Errorf("configure intent requires key parameter")
		}
	case IntentQuery:
		// No required fields beyond type.
	case IntentEscalate:
		if _, ok := i.Parameters["question"]; !ok {
			return fmt.Errorf("escalate intent requires question parameter")
		}
	default:
		return fmt.Errorf("unknown intent type: %s", i.Type)
	}
	return nil
}

// ToEvent converts the intent into a SessionEvent for the reducer.
func (i *Intent) ToEvent() SessionEvent {
	switch i.Type {
	case IntentLaunch:
		return SessionEvent{Type: EventCreated, SessionID: i.SessionID}
	case IntentStop:
		return SessionEvent{Type: EventStopped, SessionID: i.SessionID, ExitReason: "manual_stop"}
	case IntentPause:
		return SessionEvent{Type: EventPaused, SessionID: i.SessionID}
	case IntentResume:
		return SessionEvent{Type: EventResumed, SessionID: i.SessionID}
	default:
		return SessionEvent{Type: EventConfigChanged, SessionID: i.SessionID}
	}
}

// destructiveIntents lists intent types that require user confirmation.
var destructiveIntents = map[IntentType]bool{
	IntentStop:  true,
	IntentScale: true,
}

// IntentRouter classifies inputs into typed intents. It handles both
// structured (MCP/API) and unstructured (NL) sources.
type IntentRouter struct{}

// NewIntentRouter creates a new router.
func NewIntentRouter() *IntentRouter {
	return &IntentRouter{}
}

// RouteNL classifies a natural language command into a typed intent.
// Returns an error if the command cannot be classified.
func (r *IntentRouter) RouteNL(text string) (*Intent, error) {
	lower := strings.ToLower(strings.TrimSpace(text))

	intent := &Intent{Source: "nl", Parameters: make(map[string]string)}

	switch {
	case strings.HasPrefix(lower, "start ") || strings.HasPrefix(lower, "launch "):
		intent.Type = IntentLaunch
		rest := strings.TrimPrefix(strings.TrimPrefix(lower, "start "), "launch ")
		intent.Provider = classifyProvider(rest)
		intent.Destructive = false

	case strings.HasPrefix(lower, "stop ") || strings.HasPrefix(lower, "kill "):
		intent.Type = IntentStop
		intent.Destructive = true
		intent.SessionID = extractSessionRef(lower)

	case strings.HasPrefix(lower, "pause "):
		intent.Type = IntentPause
		intent.SessionID = extractSessionRef(lower)

	case strings.HasPrefix(lower, "resume "):
		intent.Type = IntentResume
		intent.SessionID = extractSessionRef(lower)

	case strings.HasPrefix(lower, "scale "):
		intent.Type = IntentScale
		intent.Destructive = true
		intent.Parameters["count"] = extractNumber(lower)

	case strings.HasPrefix(lower, "status") || strings.HasPrefix(lower, "list") || strings.HasPrefix(lower, "show"):
		intent.Type = IntentQuery

	case strings.HasPrefix(lower, "help") || strings.Contains(lower, "?"):
		intent.Type = IntentEscalate
		intent.Parameters["question"] = text

	default:
		return nil, fmt.Errorf("cannot classify command: %q", text)
	}

	return intent, nil
}

// RouteMCP converts an MCP tool name and arguments into a typed intent.
func (r *IntentRouter) RouteMCP(toolName string, args map[string]string) (*Intent, error) {
	intent := &Intent{Source: "mcp", Parameters: args}

	switch {
	case strings.Contains(toolName, "session_launch") || strings.Contains(toolName, "start"):
		intent.Type = IntentLaunch
		intent.Provider = Provider(args["provider"])
	case strings.Contains(toolName, "stop"):
		intent.Type = IntentStop
		intent.SessionID = args["session_id"]
		intent.Destructive = true
	case strings.Contains(toolName, "pause"):
		intent.Type = IntentPause
		intent.SessionID = args["session_id"]
	case strings.Contains(toolName, "resume"):
		intent.Type = IntentResume
		intent.SessionID = args["session_id"]
	case strings.Contains(toolName, "status") || strings.Contains(toolName, "list"):
		intent.Type = IntentQuery
	case strings.Contains(toolName, "config"):
		intent.Type = IntentConfigure
	default:
		intent.Type = IntentQuery
	}

	if destructiveIntents[intent.Type] {
		intent.Destructive = true
	}

	return intent, nil
}

func classifyProvider(text string) Provider {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "claude"):
		return ProviderClaude
	case strings.Contains(lower, "gemini"):
		return ProviderGemini
	case strings.Contains(lower, "ollama") || strings.Contains(lower, "local"):
		return ProviderOllama
	case strings.Contains(lower, "openai"):
		return ProviderCodex
	case strings.Contains(lower, "codex"):
		return ProviderCodex
	case strings.Contains(lower, "antigravity"):
		return ProviderAntigravity
	default:
		return ProviderClaude // default
	}
}

func extractSessionRef(text string) string {
	parts := strings.Fields(text)
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return ""
}

func extractNumber(text string) string {
	for _, word := range strings.Fields(text) {
		if len(word) > 0 && word[0] >= '0' && word[0] <= '9' {
			return word
		}
	}
	return "1"
}
