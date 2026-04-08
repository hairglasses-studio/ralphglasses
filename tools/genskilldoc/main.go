// genskilldoc generates docs/SKILLS.md from the MCP server's tool group
// registrations using the ExportSkillMarkdown function.
//
// Usage:
//
//	go run ./tools/genskilldoc                    # stdout
//	go run ./tools/genskilldoc -output docs/SKILLS.md
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

func main() {
	output := flag.String("output", "", "Output file path (default: stdout)")
	flag.Parse()

	md := renderSkillMarkdown(".")

	if *output == "" {
		fmt.Print(md)
		return
	}

	if err := os.WriteFile(*output, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", *output, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes)\n", *output, len(md))
}

func renderSkillMarkdown(repoRoot string) string {
	srv := mcpserver.NewServer(repoRoot)
	return mcpserver.ExportSkillMarkdownFromContract(srv.ToolGroups(), srv.ManagementTools())
}
