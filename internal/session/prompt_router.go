package session

import "context"

// PromptRouter is an interface satisfied by promptdj.PromptDJRouter.
// It breaks the import cycle (session cannot import promptdj, but promptdj
// imports session). The Manager stores the router via this interface.
type PromptRouter interface {
	// RoutePrompt routes a prompt and returns a JSON-serializable decision.
	RoutePrompt(ctx context.Context, prompt, repo string, score int) (any, error)
}
