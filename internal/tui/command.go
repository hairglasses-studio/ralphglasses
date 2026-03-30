package tui

import (
	"os"
	"path/filepath"
	"strings"

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

// LoadUserAliases reads custom aliases from ~/.config/ralphglasses/aliases.yml.
// The file format is a simple YAML map of alias → command name.
func LoadUserAliases() map[string]string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(dir, "ralphglasses", "aliases.yml"))
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
// User aliases from ~/.config/ralphglasses/aliases.yml take precedence.
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
