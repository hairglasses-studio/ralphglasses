package session

import (
	"testing"
)

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
