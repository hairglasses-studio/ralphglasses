package tui

import "strings"

// Command represents a parsed : command.
type Command struct {
	Name string
	Args []string
}

// ParseCommand parses a command string like "start mesmer" into a Command.
func ParseCommand(input string) Command {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return Command{}
	}
	return Command{
		Name: parts[0],
		Args: parts[1:],
	}
}
