// E3.3: Confirmation Flow for Destructive Operations — requires explicit
// confirmation before executing NL commands that stop sessions, clear budgets,
// or scale the fleet to zero.
package session

import (
	"fmt"
	"strings"
)

// ConfirmationLevel indicates how dangerous a command is.
type ConfirmationLevel int

const (
	// ConfirmNone means the command is safe and needs no confirmation.
	ConfirmNone ConfirmationLevel = iota
	// ConfirmAck means the command needs a simple yes/no confirmation.
	ConfirmAck
	// ConfirmExplicit means the command needs the user to type the target name.
	ConfirmExplicit
)

// ConfirmationRequest is generated when a destructive NL command is parsed.
type ConfirmationRequest struct {
	Level       ConfirmationLevel `json:"level"`
	Message     string            `json:"message"`      // human-readable prompt
	Command     *Command          `json:"command"`       // the parsed command awaiting confirmation
	RequiredAck string            `json:"required_ack"`  // what the user must type (for ConfirmExplicit)
}

// destructiveActions maps actions to their confirmation requirements.
var destructiveActions = map[string]ConfirmationLevel{
	ActionStop:  ConfirmAck,
	ActionScale: ConfirmAck,
}

// RequiresConfirmation checks if a parsed command needs user confirmation
// before execution. Returns nil if no confirmation is needed.
func RequiresConfirmation(cmd *Command) *ConfirmationRequest {
	if cmd == nil {
		return nil
	}

	level, ok := destructiveActions[cmd.Action]
	if !ok {
		return nil
	}

	// Escalate to explicit confirmation for fleet-wide destructive ops
	target := strings.ToLower(cmd.Target)
	if target == "fleet" || target == "all" {
		level = ConfirmExplicit
	}

	// Scale to zero is especially dangerous
	if cmd.Action == ActionScale {
		if count, ok := cmd.Parameters["count"]; ok && count == "0" {
			level = ConfirmExplicit
		}
	}

	switch level {
	case ConfirmAck:
		return &ConfirmationRequest{
			Level:   ConfirmAck,
			Message: fmt.Sprintf("Confirm: %s %s? (yes/no)", cmd.Action, cmd.Target),
			Command: cmd,
		}
	case ConfirmExplicit:
		ack := fmt.Sprintf("%s %s", cmd.Action, cmd.Target)
		return &ConfirmationRequest{
			Level:       ConfirmExplicit,
			Message:     fmt.Sprintf("This will %s %s. Type '%s' to confirm:", cmd.Action, cmd.Target, ack),
			Command:     cmd,
			RequiredAck: ack,
		}
	default:
		return nil
	}
}

// ValidateConfirmation checks if the user's response satisfies the confirmation.
func ValidateConfirmation(req *ConfirmationRequest, response string) bool {
	if req == nil {
		return true
	}

	response = strings.TrimSpace(strings.ToLower(response))

	switch req.Level {
	case ConfirmNone:
		return true
	case ConfirmAck:
		return response == "yes" || response == "y"
	case ConfirmExplicit:
		return response == strings.ToLower(req.RequiredAck)
	default:
		return false
	}
}
