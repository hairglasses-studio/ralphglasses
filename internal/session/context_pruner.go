package session

import (
	"github.com/hairglasses-studio/mcpkit/ralph"
)

type Message = ralph.Message
type PruneStats = ralph.PruneStats
type ContextPruner = ralph.ContextPruner

func NewContextPruner(maxTokens int) *ContextPruner {
	return ralph.NewContextPruner(maxTokens)
}

func EstimateTokens(text string) int {
	return ralph.EstimateTokens(text)
}
