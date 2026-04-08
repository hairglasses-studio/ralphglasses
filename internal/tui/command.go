package tui

import (
	"os"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
	"gopkg.in/yaml.v3"
)

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

// LoadUserAliases reads custom aliases from the shared Ralph config path.
// The file format is a simple YAML map of alias -> command name.
func LoadUserAliases() map[string]string {
	data, err := os.ReadFile(ralphpath.AliasesYAMLPath())
	if err != nil {
		return nil
	}
	var aliases map[string]string
	if err := yaml.Unmarshal(data, &aliases); err != nil {
		return nil
	}
	return aliases
}

// ParseCommand parses a command string like "start mesmer" into a Command.
// Built-in aliases (e.g. :rp → :repos) are expanded automatically.
// User aliases from the shared Ralph config path take precedence.
func ParseCommand(input string) Command {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return Command{}
	}
	name := parts[0]
	// User aliases take precedence over built-in.
	if userAliases := LoadUserAliases(); userAliases != nil {
		if expanded, ok := userAliases[name]; ok {
			name = expanded
		}
	}
	if expanded, ok := builtinAliases[name]; ok {
		name = expanded
	}
	return Command{
		Name: name,
		Args: parts[1:],
	}
}
