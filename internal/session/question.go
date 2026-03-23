package session

import "strings"

// questionPatterns are phrases that indicate the worker is asking a question
// instead of acting autonomously. In headless mode, no human will answer.
var questionPatterns = []string{
	"should i",
	"would you like",
	"do you want",
	"can you clarify",
	"which approach",
	"what would you prefer",
	"shall i",
	"how should i",
	"which option",
	"could you specify",
	"do you have a preference",
	"what is your",
	"would you prefer",
	"could you provide",
	"please clarify",
	"is this what you",
	"did you mean",
}

// DetectQuestions scans output text for question patterns that indicate the
// worker asked for human input instead of acting autonomously.
// Returns whether any questions were found and the total count.
func DetectQuestions(output string) (bool, int) {
	lower := strings.ToLower(output)
	count := 0
	for _, pattern := range questionPatterns {
		count += strings.Count(lower, pattern)
	}
	return count > 0, count
}
