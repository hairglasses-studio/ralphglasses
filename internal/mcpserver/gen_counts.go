package mcpserver

// Run `go generate ./internal/mcpserver/...` (or `make update-tool-counts`) to
// regenerate tool_counts_generated.go whenever tools are added or removed.

//go:generate go run github.com/hairglasses-studio/ralphglasses/internal/mcpserver/cmd/gen-tool-counts -output tool_counts_generated.go
