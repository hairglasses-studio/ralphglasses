package session

import (
	"os"
	"strings"
	"time"
)

// InterpolatePrompt replaces {{.Key}} placeholders in a prompt template
// with values from the provided map and built-in variables.
func InterpolatePrompt(template string, vars map[string]string) string {
	result := template

	// Built-in variables (lower priority than user-provided)
	builtins := map[string]string{
		"Date":     time.Now().Format("2006-01-02"),
		"Time":     time.Now().Format("15:04:05"),
		"Hostname": hostname(),
		"User":     os.Getenv("USER"),
		"Home":     os.Getenv("HOME"),
	}

	// User variables override builtins
	for k, v := range builtins {
		if _, exists := vars[k]; !exists {
			vars[k] = v
		}
	}

	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{."+k+"}}", v)
	}

	return result
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

// ListVariables returns all {{.Key}} placeholders found in a template.
func ListVariables(template string) []string {
	var vars []string
	seen := make(map[string]bool)
	i := 0
	for i < len(template) {
		idx := strings.Index(template[i:], "{{.")
		if idx < 0 {
			break
		}
		start := i + idx + 3
		end := strings.Index(template[start:], "}}")
		if end < 0 {
			break
		}
		key := template[start : start+end]
		if !seen[key] {
			vars = append(vars, key)
			seen[key] = true
		}
		i = start + end + 2
	}
	return vars
}
