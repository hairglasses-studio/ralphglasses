# ADR 003: Plugin Architecture with hashicorp/go-plugin

## Status

Accepted

## Context

Ralphglasses manages fleets of LLM agent sessions across multiple repositories. Users and ecosystem tools need to extend behavior -- adding custom event handlers, tool providers, or policy hooks -- without modifying the core binary.

Key requirements:

- **Process isolation** -- A misbehaving plugin must not crash the host process
- **Language flexibility** -- Plugins could eventually be written in languages other than Go
- **Versioned protocol** -- Host and plugin must negotiate compatibility at connect time
- **Security** -- Plugin binaries should be verified before execution

We considered Go's native `plugin` package (`.so` shared objects) but rejected it due to build-tag fragility, lack of isolation, and no cross-platform support on macOS/Windows.

## Decision

We adopted the hashicorp/go-plugin model for external plugin execution, implemented in `internal/plugin/`.

The architecture has three layers:

1. **Plugin interface** (`internal/plugin/plugin.go`) -- All plugins implement `Name()`, `Version()`, and `OnEvent()`. The `Event` type is decoupled from internal event types for API stability.

2. **Manifest system** (`internal/plugin/manifest.go`) -- Each plugin is a directory containing a `plugin.json` manifest with name, version, binary path, SHA-256 checksum, and protocol (`"grpc"` or `"builtin"`). `ValidateManifest()` verifies checksums before execution.

3. **gRPC contract** (`internal/plugin/grpc.go`) -- The `GRPCPlugin` interface extends `Plugin` with `HandleToolCall()` and `Capabilities()`. Magic cookie handshake constants (`MagicCookieKey`, `MagicCookieValue`) enforce protocol agreement between host and plugin.

4. **Discovery** (`internal/plugin/loader.go`) -- `LoadDir()` scans a directory for manifest-based plugins, logging legacy `.so` files but not loading them. `ScanPluginDir()` returns validated manifests.

Builtin plugins are registered in `internal/plugin/builtin/` via a registry pattern, avoiding process overhead for first-party extensions.

## Consequences

**Positive:**

- Crash isolation: plugin failures do not bring down the TUI or MCP server
- SHA-256 checksum verification prevents tampered binaries from loading
- Dual protocol support (builtin + gRPC) allows both in-process and out-of-process plugins
- Manifest-based discovery is simple and filesystem-driven

**Negative:**

- gRPC transport adds latency compared to in-process calls
- Full hashicorp/go-plugin client wiring is not yet complete (noted in `loader.go` as follow-up)
- Plugin authors must build a separate binary and maintain a manifest

**Current state:**

- Manifest loading, validation, and discovery are fully implemented and tested
- gRPC plugin interface is defined but client-side launch is pending
- Builtin plugin registry provides the immediate extension mechanism
