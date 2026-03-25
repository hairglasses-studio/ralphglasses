package session

import "testing"

func FuzzNormalizeClaudeEvent(f *testing.F) {
	// Seed corpus with real Claude output patterns
	f.Add([]byte(`{"type":"result","session_id":"abc","cost_usd":0.12,"num_turns":5}`))
	f.Add([]byte(`{"type":"assistant","content":"hello"}`))
	f.Add([]byte(`{"type":"system","session_id":"test-123"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"type":"result","usage":{"cost_usd":0.05}}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(``))
	f.Add([]byte(`{"type":"subagent","description":"working on task"}`))
	f.Add([]byte(`{"type":"agent","message":"sub-agent running","content":"details"}`))
	f.Add([]byte(`{"type":"result","cost_usd":0.05,"usage":{"cost_usd":0.12,"total_cost_usd":0.15}}`))
	f.Add([]byte(`{"type":"result","duration_seconds":12.5,"metadata":{"duration_seconds":15.0}}`))
	f.Add([]byte(`{"type":"result","num_turns":3,"usage":{"turns":5}}`))
	f.Add([]byte(`{"type":"assistant","content":["part1","part2"]}`))
	f.Add([]byte(`{"type":"result","result":{"text":"nested result"}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic — errors are acceptable
		_, _ = normalizeClaudeEvent(data)
	})
}

func FuzzNormalizeGeminiEvent(f *testing.F) {
	// Seed with Gemini-style output
	f.Add([]byte(`{"type":"message","content":"response text"}`))
	f.Add([]byte(`{"event":"delta","text":"chunk"}`))
	f.Add([]byte(`{"type":"result","usage_metadata":{"prompt_token_count":100,"candidates_token_count":50}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`plain text output`))
	f.Add([]byte(``))
	f.Add([]byte(`{"event":"message","message":{"parts":[{"text":"hello"}]}}`))
	f.Add([]byte(`{"type":"result","usage":{"total_cost_usd":0.4,"turns":3},"session":{"id":"gem-456"}}`))
	f.Add([]byte(`{"type":"error","error":"rate limited","is_error":true}`))
	f.Add([]byte(`{"type":"result","result":{"text":"nested"},"metadata":{"model":"gemini-3-pro"}}`))
	f.Add([]byte(`{"candidate":{"content":{"parts":[{"text":"deep nested"}]}}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = normalizeGeminiEvent(data)
	})
}

func FuzzNormalizeCodexEvent(f *testing.F) {
	// Seed with Codex-style output
	f.Add([]byte(`{"type":"message","output_text":"code result"}`))
	f.Add([]byte(`{"item":{"type":"message"},"content":"hello"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"type":"error","error":"rate limited"}`))
	f.Add([]byte(``))
	f.Add([]byte(`{"event":"message","message":{"content":"Refactor complete"}}`))
	f.Add([]byte(`{"type":"result","usage":{"total_cost_usd":0.12,"turns":2}}`))
	f.Add([]byte(`{"type":"result","summary":"All done","is_error":false}`))
	f.Add([]byte("\x1b[32mRefactored 3 files successfully\x1b[0m"))

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = normalizeCodexEvent(data)
	})
}

func FuzzNormalizeEvent(f *testing.F) {
	// Test the dispatch function with provider routing
	f.Add([]byte(`{"type":"result"}`), "claude")
	f.Add([]byte(`{"type":"result"}`), "gemini")
	f.Add([]byte(`{"type":"result"}`), "codex")
	f.Add([]byte(`random bytes`), "unknown")
	f.Add([]byte(`{"type":"assistant","content":"hi"}`), "claude")
	f.Add([]byte(`{"event":"delta","text":"chunk"}`), "gemini")
	f.Add([]byte(`{"type":"message","output_text":"done"}`), "codex")
	f.Add([]byte(``), "claude")
	f.Add([]byte(`{}`), "")

	f.Fuzz(func(t *testing.T, data []byte, provider string) {
		_, _ = normalizeEvent(Provider(provider), data)
	})
}
