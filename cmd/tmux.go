package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var tmuxCmd = &cobra.Command{
	Use:   "tmux",
	Short: "Manage tmux sessions for agent panes",
}

var tmuxListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tmux sessions matching ralph prefix",
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_windows}\t#{session_attached}").Output()
		if err != nil {
			if strings.Contains(err.Error(), "no server running") || strings.Contains(string(out), "no server") {
				fmt.Println("No tmux server running.")
				return nil
			}
			return fmt.Errorf("tmux list-sessions: %w", err)
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		fmt.Printf("%-30s  %-8s  %s\n", "SESSION", "WINDOWS", "ATTACHED")
		fmt.Println(strings.Repeat("-", 50))
		for _, line := range lines {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) < 3 {
				continue
			}
			attached := "no"
			if parts[2] == "1" {
				attached = "yes"
			}
			fmt.Printf("%-30s  %-8s  %s\n", parts[0], parts[1], attached)
		}
		return nil
	},
}

var tmuxAttachCmd = &cobra.Command{
	Use:   "attach [session-name]",
	Short: "Attach to a tmux session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := exec.Command("tmux", "attach-session", "-t", args[0])
		c.Stdin = cmd.InOrStdin()
		c.Stdout = cmd.OutOrStdout()
		c.Stderr = cmd.ErrOrStderr()
		return c.Run()
	},
}

var tmuxDetachCmd = &cobra.Command{
	Use:   "detach [session-name]",
	Short: "Detach a tmux session (send detach key)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return exec.Command("tmux", "detach-client", "-s", args[0]).Run()
	},
}

func init() {
	tmuxCmd.AddCommand(tmuxListCmd, tmuxAttachCmd, tmuxDetachCmd)
	rootCmd.AddCommand(tmuxCmd)
}
