// gendoc generates man pages for all ralphglasses subcommands.
// Invoked via: go generate ./cmd/...
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra/doc"

	"github.com/hairglasses-studio/ralphglasses/cmd"
)

var manpageDate = time.Date(2026, time.April, 8, 0, 0, 0, 0, time.UTC)

func main() {
	dir := "man/man1"
	if err := renderManpages(dir); err != nil {
		fmt.Fprintf(os.Stderr, "gendoc: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Man pages generated in %s/\n", dir)
}

func renderManpages(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	header := &doc.GenManHeader{
		Title:   "RALPHGLASSES",
		Section: "1",
		Source:  "ralphglasses",
		Date:    &manpageDate,
	}

	root := cmd.RootCommand()
	if err := doc.GenManTree(root, header, dir); err != nil {
		return err
	}
	return nil
}
