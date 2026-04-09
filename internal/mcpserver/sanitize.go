package mcpserver

import "regexp"

var secretRegexes = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[a-zA-Z0-9_-]+`),   // Anthropic
	regexp.MustCompile(`sk-[a-zA-Z0-9]{32,}`),     // OpenAI
	regexp.MustCompile(`AIza[0-9A-Za-z\-_]{20,}`), // Google
	regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{20,}`), // Slack
}

// RedactSecrets redacts known API keys and secrets from the given string.
func RedactSecrets(s string) string {
	for _, re := range secretRegexes {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}
