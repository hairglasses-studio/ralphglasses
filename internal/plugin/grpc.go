package plugin

import "context"

// Handshake constants for hashicorp/go-plugin protocol.
// The host and plugin must agree on these values for a connection to succeed.
const (
	MagicCookieKey   = "RALPHGLASSES_PLUGIN"
	MagicCookieValue = "v1"
)

// GRPCPlugin extends Plugin with gRPC-based tool call handling.
// This is the contract that external plugin binaries must satisfy.
// The actual gRPC transport (hashicorp/go-plugin wiring) is a follow-up;
// this interface establishes the protocol shape.
type GRPCPlugin interface {
	Plugin

	// HandleToolCall processes an MCP tool call from the host.
	// The name corresponds to a tool declared in the plugin's capabilities.
	HandleToolCall(ctx context.Context, name string, args map[string]any) (string, error)

	// Capabilities returns the list of tool names this plugin provides.
	Capabilities() []string
}
