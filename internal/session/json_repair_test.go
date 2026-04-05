package session

import (
	"encoding/json"
	"testing"
)

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no fences",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "json fence",
			input: "```json\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "plain fence",
			input: "```\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
		{
			name:  "fence with whitespace",
			input: "  ```json\n  {\"key\": \"value\"}  \n  ```  ",
			want:  `{"key": "value"}`,
		},
		{
			name:  "fence with array",
			input: "```json\n[{\"title\":\"test\"}]\n```",
			want:  `[{"title":"test"}]`,
		},
		{
			name:  "inline fence in prose",
			input: "Here is the result:\n```json\n{\"key\": \"value\"}\n```\nDone.",
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON fence uppercase",
			input: "```JSON\n{\"key\": \"value\"}\n```",
			want:  `{"key": "value"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripMarkdownFences(tt.input)
			if got != tt.want {
				t.Errorf("stripMarkdownFences() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractJSONFromProse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no prose",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "leading prose",
			input: `Here is the JSON: {"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "trailing prose",
			input: `{"key": "value"} Hope this helps!`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "both sides prose",
			input: "Here is the result:\n{\"key\": \"value\"}\nHope this helps!",
			want:  `{"key": "value"}`,
		},
		{
			name:  "array with prose",
			input: "Tasks:\n[{\"title\":\"test\"}]\nEnd.",
			want:  `[{"title":"test"}]`,
		},
		{
			name:  "no JSON",
			input: "just plain text",
			want:  "just plain text",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSONFromProse(tt.input)
			if got != tt.want {
				t.Errorf("extractJSONFromProse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizePythonLiterals(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "True to true",
			input: `{"active": True}`,
			want:  `{"active": true}`,
		},
		{
			name:  "False to false",
			input: `{"active": False}`,
			want:  `{"active": false}`,
		},
		{
			name:  "None to null",
			input: `{"value": None}`,
			want:  `{"value": null}`,
		},
		{
			name:  "mixed booleans",
			input: `{"a": True, "b": False, "c": None}`,
			want:  `{"a": true, "b": false, "c": null}`,
		},
		{
			name:  "in array",
			input: `[True, False, None]`,
			want:  `[true, false, null]`,
		},
		{
			name:  "no change needed",
			input: `{"active": true, "value": null}`,
			want:  `{"active": true, "value": null}`,
		},
		{
			name:  "True in string preserved",
			input: `{"msg": "True story"}`,
			want:  `{"msg": "True story"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePythonLiterals(tt.input)
			if got != tt.want {
				t.Errorf("normalizePythonLiterals() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripTrailingCommas(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trailing comma before brace",
			input: `{"name": "test",}`,
			want:  `{"name": "test"}`,
		},
		{
			name:  "trailing comma before bracket",
			input: `["a", "b",]`,
			want:  `["a", "b"]`,
		},
		{
			name:  "trailing comma with whitespace",
			input: "{\"name\": \"test\",\n}",
			want:  "{\"name\": \"test\"\n}",
		},
		{
			name:  "multiple trailing commas",
			input: `{"a": {"b": 1,}, "c": [1, 2,]}`,
			want:  `{"a": {"b": 1}, "c": [1, 2]}`,
		},
		{
			name:  "no trailing commas",
			input: `{"a": 1, "b": 2}`,
			want:  `{"a": 1, "b": 2}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTrailingCommas(tt.input)
			if got != tt.want {
				t.Errorf("stripTrailingCommas() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJsonRepair(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValid bool
	}{
		{
			name:      "valid JSON unchanged",
			input:     `{"title":"test","prompt":"do things"}`,
			wantValid: true,
		},
		{
			name:      "fenced JSON",
			input:     "```json\n{\"title\":\"test\",\"prompt\":\"do things\"}\n```",
			wantValid: true,
		},
		{
			name:      "prose wrapped JSON",
			input:     "Here is the task:\n{\"title\":\"test\",\"prompt\":\"do things\"}\nHope this helps!",
			wantValid: true,
		},
		{
			name:      "trailing comma",
			input:     `{"title":"test","prompt":"do things",}`,
			wantValid: true,
		},
		{
			name:      "Python booleans",
			input:     `{"active": True, "deleted": False}`,
			wantValid: true,
		},
		{
			name:      "combined: fence + trailing comma + Python bool",
			input:     "```json\n{\"title\":\"test\",\"active\": True,}\n```",
			wantValid: true,
		},
		{
			name:      "Codex prose wrapping with fences",
			input:     "Here is the JSON:\n```json\n{\"title\":\"fix bug\",\"prompt\":\"fix the null pointer\"}\n```\nLet me know if you need anything else.",
			wantValid: true,
		},
		{
			name:      "not JSON at all",
			input:     "this is just plain text with no JSON",
			wantValid: false,
		},
		{
			name:      "empty",
			input:     "",
			wantValid: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired := jsonRepair(tt.input)
			valid := json.Valid([]byte(repaired))
			if valid != tt.wantValid {
				t.Errorf("jsonRepair() valid = %v, want %v\n  input:    %q\n  repaired: %q",
					valid, tt.wantValid, tt.input, repaired)
			}
		})
	}
}

func TestTryUnmarshalWithRepair(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantKey string
	}{
		{
			name:    "valid JSON",
			input:   `{"title":"test"}`,
			wantErr: false,
			wantKey: "test",
		},
		{
			name:    "fenced JSON",
			input:   "```json\n{\"title\":\"fenced\"}\n```",
			wantErr: false,
			wantKey: "fenced",
		},
		{
			name:    "trailing comma",
			input:   `{"title":"commas",}`,
			wantErr: false,
			wantKey: "commas",
		},
		{
			name:    "prose wrapping",
			input:   "Here is your task:\n{\"title\":\"prose\"}\nGood luck!",
			wantErr: false,
			wantKey: "prose",
		},
		{
			name:    "not JSON",
			input:   "not json at all",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]any
			_, err := tryUnmarshalWithRepair(tt.input, &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("tryUnmarshalWithRepair() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result["title"] != tt.wantKey {
				t.Errorf("result[title] = %v, want %v", result["title"], tt.wantKey)
			}
		})
	}
}

func TestLooksLikeJSONOrFenced(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`{"key": "value"}`, true},
		{`[1, 2, 3]`, true},
		{"```json\n{\"key\": \"value\"}\n```", true},
		{"```\n[1,2]\n```", true},
		{"just text", false},
		{"", false},
		{"  {}", true},
		{"  ```json\n{}\n```  ", true},
		{"```python\nprint('hi')\n```", false},
	}
	for _, tt := range tests {
		t.Run(tt.input[:min(20, len(tt.input))], func(t *testing.T) {
			if got := looksLikeJSONOrFenced(tt.input); got != tt.want {
				t.Errorf("looksLikeJSONOrFenced(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripLineComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "comment after value",
			input: "  \"key\": \"value\" // this is a comment",
			want:  "  \"key\": \"value\"",
		},
		{
			name:  "no comment",
			input: `  "key": "value"`,
			want:  `  "key": "value"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripLineComments(tt.input)
			if got != tt.want {
				t.Errorf("stripLineComments() = %q, want %q", got, tt.want)
			}
		})
	}
}
