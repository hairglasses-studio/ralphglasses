// Package ctxbudget manages context window budgets, pruning, prefetch
// hooks, error context enrichment, and token counting for LLM sessions.
//
// The name "ctxbudget" avoids shadowing the stdlib "context" package.
//
// Context budget management ensures that prompts sent to LLM providers
// stay within model token limits by tracking token usage, pruning stale
// context, prefetching relevant context before tool calls, and enriching
// error messages with actionable context for retry attempts.
package ctxbudget
