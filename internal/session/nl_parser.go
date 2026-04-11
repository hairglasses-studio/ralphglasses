package session

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// tokenize splits input into lowercase tokens, stripping punctuation.
func tokenize(input string) []string {
	input = strings.ToLower(strings.TrimSpace(input))
	var tokens []string
	var cur strings.Builder
	for _, r := range input {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			cur.WriteRune(r)
		} else {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// stopWords are common English words to skip during entity extraction.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "to": true, "for": true,
	"on": true, "in": true, "at": true, "of": true, "with": true,
	"all": true, "my": true, "from": true, "and": true, "or": true,
}

// actionKeywords maps trigger words to canonical action names.
var actionKeywords = map[string]string{
	// start
	"start": "start", "launch": "start", "run": "start", "begin": "start", "create": "start", "spin": "start",
	// stop
	"stop": "stop", "kill": "stop", "terminate": "stop", "end": "stop", "halt": "stop", "shutdown": "stop",
	// pause / resume
	"pause": "pause", "suspend": "pause", "freeze": "pause",
	"resume": "resume", "unpause": "resume", "continue": "resume", "unfreeze": "resume",
	// scale
	"scale": "scale", "resize": "scale",
	// report / show
	"show": "report", "report": "report", "display": "report", "list": "report", "get": "report", "view": "report",
	// status
	"status": "status",
}

// providerAliases maps aliases to canonical Provider values.
var providerAliases = map[string]Provider{
	"claude": ProviderClaude, "anthropic": ProviderClaude,
	"gemini": ProviderGemini, "google": ProviderGemini,
	"codex": ProviderCodex, "openai": ProviderCodex,
	"antigravity": ProviderAntigravity,
}

// timeRangeKeywords maps natural-language time references to canonical keys.
var timeRangeKeywords = map[string]string{
	"today":     "today",
	"yesterday": "yesterday",
	"week":      "week",
	"month":     "month",
}

// reportSubjects maps nouns to report topics.
var reportSubjects = map[string]string{
	"cost":     "cost",
	"costs":    "cost",
	"spend":    "cost",
	"spending": "cost",
	"budget":   "cost",
	"status":   "status",
	"sessions": "sessions",
	"session":  "sessions",
	"fleet":    "fleet",
	"health":   "health",
}

var numberRe = regexp.MustCompile(`\d+`)

// detectIntent returns the canonical action string from a token list.
func detectIntent(tokens []string) string {
	for _, t := range tokens {
		if action, ok := actionKeywords[t]; ok {
			return action
		}
	}
	return ""
}

// extractCount finds the first integer in a token list.
func extractCount(tokens []string) int {
	for _, t := range tokens {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// extractProvider returns the Provider referenced in the token list, if any.
func extractProvider(tokens []string) (Provider, bool) {
	for _, t := range tokens {
		if p, ok := providerAliases[t]; ok {
			return p, true
		}
	}
	return "", false
}

// extractTimeRange returns the canonical time range from a token list.
func extractTimeRange(tokens []string) string {
	for _, t := range tokens {
		if tr, ok := timeRangeKeywords[t]; ok {
			return tr
		}
	}
	return ""
}

// extractReportSubject returns the report topic from a token list.
func extractReportSubject(tokens []string) string {
	for _, t := range tokens {
		if subj, ok := reportSubjects[t]; ok {
			return subj
		}
	}
	return ""
}

// extractSessionID looks for "session N" pattern and returns the ID string.
func extractSessionID(tokens []string) string {
	for i, t := range tokens {
		if t == "session" && i+1 < len(tokens) {
			if _, err := strconv.Atoi(tokens[i+1]); err == nil {
				return tokens[i+1]
			}
		}
	}
	return ""
}

// extractProject looks for "project X" or "on X" patterns where X is not a
// stop word or keyword. Falls back to the last non-keyword token after
// removing known action/provider/time tokens.
func extractProject(tokens []string) string {
	// "project X" pattern
	for i, t := range tokens {
		if t == "project" && i+1 < len(tokens) {
			return tokens[i+1]
		}
	}
	// "on <project>" pattern — but skip if the next token is another keyword
	for i, t := range tokens {
		if t == "on" && i+1 < len(tokens) {
			next := tokens[i+1]
			if !stopWords[next] && actionKeywords[next] == "" && providerAliases[next] == "" && timeRangeKeywords[next] == "" {
				return next
			}
		}
	}
	return ""
}

// extractFleetKeyword returns true if "fleet" appears in the tokens.
func extractFleetKeyword(tokens []string) bool {
	return slices.Contains(tokens, "fleet")
}
