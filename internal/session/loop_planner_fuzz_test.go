package session

import (
	"strings"
	"testing"
)

// Run: go test -fuzz=FuzzParsePlannerTask -fuzztime=30s ./internal/session/...
func FuzzParsePlannerTask(f *testing.F) {
	// Valid JSON task
	f.Add(`{"title": "fix bug", "prompt": "Fix the login bug"}`)
	// Fenced code block
	f.Add("```json\n{\"title\": \"test\", \"prompt\": \"do something\"}\n```")
	// Empty
	f.Add("")
	f.Add("{}")
	f.Add("[]")
	// Missing fields
	f.Add(`{"title": "only title"}`)
	f.Add(`{"prompt": "only prompt"}`)
	// Garbage surrounding valid JSON
	f.Add(`Here is the task: {"title": "t", "prompt": "p"} done!`)
	// Nested braces
	f.Add(`{"title": "t", "prompt": "use map[string]int{\"a\": 1}"}`)
	// Very long title
	f.Add(`{"title": "` + string(make([]byte, 500)) + `", "prompt": "p"}`)
	// Null values
	f.Add(`{"title": null, "prompt": null}`)
	// Numeric values where strings expected
	f.Add(`{"title": 123, "prompt": 456}`)
	// Array instead of object
	f.Add(`[{"title": "a", "prompt": "b"}]`)
	// Plain text (no JSON)
	f.Add("just some plain text\nwith multiple lines")
	// Malformed JSON
	f.Add(`{invalid`)
	// Unicode content
	f.Add(`{"title": "こんにちは世界 🎉", "prompt": "Unicode prompt"}`)
	// Null bytes
	f.Add("\x00\x00")
	// Just whitespace
	f.Add("   \n\t  ")
	// Valid JSON with extra/unexpected fields
	f.Add(`{"title": "t", "prompt": "p", "priority": 1, "tags": ["a","b"], "nested": {"deep": true}}`)
	// Very long prompt
	f.Add(`{"title": "t", "prompt": "` + strings.Repeat("long ", 200) + `"}`)

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic on any input
		task, err := parsePlannerTask(input)
		if err == nil {
			// If parsing succeeded, title and prompt must be non-empty
			if task.Title == "" {
				t.Error("parsePlannerTask returned nil error but empty title")
			}
			if task.Prompt == "" {
				t.Error("parsePlannerTask returned nil error but empty prompt")
			}
		}
	})
}

// Run: go test -fuzz=FuzzParsePlannerTasks -fuzztime=30s ./internal/session/...
func FuzzParsePlannerTasks(f *testing.F) {
	// Valid JSON array of tasks
	f.Add(`[{"title": "task1", "prompt": "do thing 1"}, {"title": "task2", "prompt": "do thing 2"}]`)
	// Fenced code block with array
	f.Add("```json\n[{\"title\": \"a\", \"prompt\": \"b\"}]\n```")
	// Empty
	f.Add("")
	f.Add("[]")
	f.Add("[{}]")
	// Single object (not array)
	f.Add(`{"title": "t", "prompt": "p"}`)
	// Mixed valid/invalid tasks
	f.Add(`[{"title": "good", "prompt": "ok"}, {"title": "", "prompt": ""}]`)
	// Surrounding text
	f.Add(`The tasks are: [{"title": "x", "prompt": "y"}] end.`)
	// Deeply nested
	f.Add(`[{"title": "t", "prompt": "use [1,2,3] array"}]`)
	// Null array elements
	f.Add(`[null, {"title": "a", "prompt": "b"}]`)
	// Plain text
	f.Add("no json here at all")
	// Large array
	f.Add(`[{"title":"a","prompt":"b"},{"title":"c","prompt":"d"},{"title":"e","prompt":"f"},{"title":"g","prompt":"h"}]`)
	// Malformed JSON
	f.Add(`[{invalid`)
	// Unicode content
	f.Add(`[{"title": "こんにちは世界 🎉", "prompt": "Unicode task"}]`)
	// Null bytes
	f.Add("\x00\x00")
	// Just whitespace
	f.Add("   \n\t  ")
	// Extra/unexpected fields in array items
	f.Add(`[{"title": "t", "prompt": "p", "extra": true, "score": 99}]`)
	// Very long input
	f.Add(`[` + strings.Repeat(`{"title":"t","prompt":"p"},`, 100) + `{"title":"last","prompt":"end"}]`)

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic on any input
		tasks, err := parsePlannerTasks(input)
		if err == nil {
			if len(tasks) == 0 {
				t.Error("parsePlannerTasks returned nil error but empty slice")
			}
			for i, task := range tasks {
				if task.Title == "" {
					t.Errorf("task[%d] has empty title", i)
				}
				if task.Prompt == "" {
					t.Errorf("task[%d] has empty prompt", i)
				}
			}
		}
	})
}
