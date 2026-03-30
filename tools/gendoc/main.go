// gendoc generates man pages for all ralphglasses subcommands.
// Invoked via: go generate ./cmd/...
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra/doc"

	"github.com/hairglasses-studio/ralphglasses/cmd"
)

func main() {
	dir := "man/man1"
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	header := &doc.GenManHeader{
		Title:   "RALPHGLASSES",
		Section: "1",
		Source:  "ralphglasses",
	}

	root := cmd.RootCommand()
	if err := doc.GenManTree(root, header, dir); err != nil {
		fmt.Fprintf(os.Stderr, "gendoc: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Man pages generated in %s/\n", dir)
}
