package session

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Action constants for natural language commands.
const (
	ActionStart  = "start"
	ActionStop   = "stop"
	ActionPause  = "pause"
	ActionResume = "resume"
	ActionScale  = "scale"
	ActionReport = "report"
	ActionStatus = "status"
)

// Command represents a parsed natural language fleet command.
type Command struct {
	Action     string            // canonical action (start, stop, pause, resume, scale, report, status)
	Target     string            // what the action applies to (fleet, session, cost, etc.)
	Parameters map[string]string // extracted parameters (count, provider, project, session_id, time_range, etc.)
}

// CommandHandler is a callback that executes a parsed command against a Manager.
type CommandHandler func(ctx context.Context, mgr *Manager, cmd *Command) error

// NLController parses and executes natural language fleet control commands.
type NLController struct {
	mgr        *Manager
	handlers   map[string]CommandHandler
	classifier *IntentClassifier // TF-IDF fallback for intent detection
}

// NewNLController creates a controller wired to the given Manager.
// It registers the default set of command handlers.
func NewNLController(mgr *Manager) *NLController {
	c := &NLController{
		mgr:        mgr,
		handlers:   make(map[string]CommandHandler),
		classifier: NewIntentClassifier(0),
	}
	c.registerDefaults()
	return c
}

// RegisterHandler adds or replaces a handler for the given action.
func (c *NLController) RegisterHandler(action string, h CommandHandler) {
	c.handlers[action] = h
}

// Parse converts a natural language string into a structured Command.
// It returns an error if no intent can be detected.
func (c *NLController) Parse(input string) (*Command, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, errors.New("empty input")
	}

	tokens := tokenize(input)
	if len(tokens) == 0 {
		return nil, errors.New("no recognizable tokens")
	}

	action := detectIntent(tokens)
	if action == "" {
		// TF-IDF classifier fallback
		if c.classifier != nil {
			result := c.classifier.Classify(input)
			if result.Confidence > 0 && result.Action != "" {
				action = result.Action
			}
		}
	}
	if action == "" {
		return nil, fmt.Errorf("could not determine intent from: %q", input)
	}

	cmd := &Command{
		Action:     action,
		Parameters: make(map[string]string),
	}

	// Extract entities based on action type.
	switch action {
	case ActionStart:
		if n := extractCount(tokens); n > 0 {
			cmd.Parameters["count"] = strconv.Itoa(n)
		}
		if p, ok := extractProvider(tokens); ok {
			cmd.Parameters["provider"] = string(p)
		}
		if proj := extractProject(tokens); proj != "" {
			cmd.Parameters["project"] = proj
		}
		cmd.Target = "sessions"

	case ActionStop:
		if p, ok := extractProvider(tokens); ok {
			cmd.Parameters["provider"] = string(p)
		}
		if sid := extractSessionID(tokens); sid != "" {
			cmd.Parameters["session_id"] = sid
			cmd.Target = "session"
		} else if containsToken(tokens, "all") {
			cmd.Parameters["all"] = "true"
			cmd.Target = "sessions"
		} else {
			cmd.Target = "sessions"
		}

	case ActionPause, ActionResume:
		if sid := extractSessionID(tokens); sid != "" {
			cmd.Parameters["session_id"] = sid
			cmd.Target = "session"
		}
		if p, ok := extractProvider(tokens); ok {
			cmd.Parameters["provider"] = string(p)
		}

	case ActionScale:
		if n := extractCount(tokens); n > 0 {
			cmd.Parameters["count"] = strconv.Itoa(n)
		}
		if extractFleetKeyword(tokens) {
			cmd.Target = "fleet"
		} else {
			cmd.Target = "fleet"
		}

	case ActionReport:
		subj := extractReportSubject(tokens)
		if subj != "" {
			cmd.Target = subj
		} else {
			cmd.Target = "status"
		}
		if tr := extractTimeRange(tokens); tr != "" {
			cmd.Parameters["time_range"] = tr
		}
		if p, ok := extractProvider(tokens); ok {
			cmd.Parameters["provider"] = string(p)
		}

	case ActionStatus:
		cmd.Target = "fleet"
		if p, ok := extractProvider(tokens); ok {
			cmd.Parameters["provider"] = string(p)
		}
	}

	return cmd, nil
}

// Execute runs the handler registered for cmd.Action.
func (c *NLController) Execute(ctx context.Context, cmd *Command) error {
	if cmd == nil {
		return errors.New("nil command")
	}
	h, ok := c.handlers[cmd.Action]
	if !ok {
		return fmt.Errorf("no handler registered for action %q", cmd.Action)
	}
	return h(ctx, c.mgr, cmd)
}

// registerDefaults wires up the built-in handlers.
func (c *NLController) registerDefaults() {
	c.handlers[ActionStart] = handleStart
	c.handlers[ActionStop] = handleStop
	c.handlers[ActionPause] = handlePause
	c.handlers[ActionResume] = handleResume
	c.handlers[ActionScale] = handleScale
	c.handlers[ActionReport] = handleReport
	c.handlers[ActionStatus] = handleStatus
}

// containsToken returns true if tok appears in tokens.
func containsToken(tokens []string, tok string) bool {
	for _, t := range tokens {
		if t == tok {
			return true
		}
	}
	return false
}

// --- default handlers ---

func handleStart(_ context.Context, _ *Manager, cmd *Command) error {
	count := 1
	if s, ok := cmd.Parameters["count"]; ok {
		if n, err := strconv.Atoi(s); err == nil {
			count = n
		}
	}
	if count < 1 {
		return errors.New("count must be at least 1")
	}
	if count > 100 {
		return fmt.Errorf("count %d exceeds maximum of 100", count)
	}
	// Actual launch would call mgr.Launch in a loop.
	// This handler validates the command and returns nil to indicate readiness.
	return nil
}

func handleStop(_ context.Context, _ *Manager, cmd *Command) error {
	if cmd.Parameters["session_id"] == "" && cmd.Parameters["all"] != "true" && cmd.Parameters["provider"] == "" {
		return errors.New("stop requires a session ID, provider filter, or 'all'")
	}
	return nil
}

func handlePause(_ context.Context, _ *Manager, cmd *Command) error {
	if cmd.Parameters["session_id"] == "" {
		return errors.New("pause requires a session ID")
	}
	return nil
}

func handleResume(_ context.Context, _ *Manager, cmd *Command) error {
	if cmd.Parameters["session_id"] == "" {
		return errors.New("resume requires a session ID")
	}
	return nil
}

func handleScale(_ context.Context, _ *Manager, cmd *Command) error {
	s, ok := cmd.Parameters["count"]
	if !ok {
		return errors.New("scale requires a target count")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid scale count: %q", s)
	}
	if n > 100 {
		return fmt.Errorf("scale count %d exceeds maximum of 100", n)
	}
	return nil
}

func handleReport(_ context.Context, _ *Manager, _ *Command) error {
	return nil
}

func handleStatus(_ context.Context, _ *Manager, _ *Command) error {
	return nil
}
