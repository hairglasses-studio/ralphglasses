package tui

import "strings"

// Command represents a parsed : command.
type Command struct {
	Name string
	Args []string
}

// builtinAliases maps short aliases to their canonical command names.
var builtinAliases = map[string]string{
	"rp": "repos",
	"ss": "sessions",
	"tm": "teams",
	"fl": "fleet",
}

// ParseCommand parses a command string like "start mesmer" into a Command.
// Built-in aliases (e.g. :rp → :repos) are expanded automatically.
func ParseCommand(input string) Command {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return Command{}
	}
	name := parts[0]
	if expanded, ok := builtinAliases[name]; ok {
		name = expanded
	}
	return Command{
		Name: name,
		Args: parts[1:],
	}
}
