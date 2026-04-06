// Package provider defines the LLM provider interface and adapter
// implementations for Claude, Codex, Gemini, Goose, Amp, and Crush.
//
// Each adapter translates between ralphglasses session lifecycle
// operations (launch, send, stream, terminate) and the provider-specific
// CLI or API protocol. Provider normalization ensures that cost, token,
// and status data from heterogeneous providers is mapped to a common
// schema before reaching the rest of the session subsystem.
package provider
