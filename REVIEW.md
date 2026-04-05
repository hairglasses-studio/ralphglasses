# Review Guidelines — ralphglasses

Inherits from org-wide [REVIEW.md](https://github.com/hairglasses-studio/.github/blob/main/REVIEW.md).

## Additional Focus
- **Multi-provider isolation**: Sessions across Claude/Gemini/Codex must not leak context between providers
- **Cost tracking**: Verify token counting accuracy per provider, flag any unbounded API calls
- **TUI thread safety**: bubbletea model updates must be immutable — never mutate shared state
- **126 MCP tools**: Tool routing must handle name collisions across servers
