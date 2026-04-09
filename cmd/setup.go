package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Smart Setup Wizard (Scout/Validate) for project onboarding",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Phase 1: Detecting tech stack...")
		if _, err := os.Stat("go.mod"); err == nil {
			fmt.Println("Detected Go project.")
		} else if _, err := os.Stat("package.json"); err == nil {
			fmt.Println("Detected Node.js project.")
		} else {
			fmt.Println("Unknown project type.")
		}

		fmt.Println("\nPhase 2: Clarifying questions...")
		var primaryCmd string
		fmt.Print("What is the primary run command? (e.g., 'go run .'): ")
		fmt.Scanln(&primaryCmd)

		var testCmd string
		fmt.Print("What is the primary test command? (e.g., 'go test ./...'): ")
		fmt.Scanln(&testCmd)

		fmt.Println("\nPhase 3: Running dry-run task...")
		if primaryCmd == "" {
			primaryCmd = "echo 'No run command provided'"
		}
		fmt.Printf("Executing dry-run: %s\n", primaryCmd)
		
		parts := strings.Fields(primaryCmd)
		if len(parts) > 0 {
			c := exec.Command(parts[0], parts[1:]...)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			_ = c.Run()
		}

		fmt.Println("\nSetup complete!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
