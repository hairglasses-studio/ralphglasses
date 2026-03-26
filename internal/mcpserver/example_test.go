package mcpserver_test

import (
	"fmt"

	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

func ExampleServer() {
	// Create an MCP server instance with a scan path.
	srv := mcpserver.NewServer("/tmp/example-scan")

	// Create the underlying mcp-go server.
	mcpSrv := server.NewMCPServer("ralphglasses", "0.1.0")

	// Register all tool groups at once.
	srv.RegisterAllTools(mcpSrv)

	fmt.Printf("scan path: %s\n", srv.ScanPath)
	fmt.Printf("deferred loading: %v\n", srv.DeferredLoading)

	// Output:
	// scan path: /tmp/example-scan
	// deferred loading: false
}
