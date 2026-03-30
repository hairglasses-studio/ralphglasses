package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var sessionJSON bool

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage headless LLM sessions",
	Long:  `List, inspect, and stop sessions launched by the MCP server or TUI.`,
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		mgr := session.NewManager()
		mgr.SetStateDir(filepath.Join(sp, ".session-state"))
		mgr.LoadExternalSessions()

		sessions := mgr.List("")

		if sessionJSON {
			data, err := json.MarshalIndent(sessions, "", "  ")
			if err != nil {
				return fmt.Errorf("json marshal: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}

		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		fmt.Printf("%-20s  %-10s  %-10s  %-8s  %s\n", "ID", "STATUS", "PROVIDER", "SPEND", "REPO")
		fmt.Println(strings.Repeat("-", 72))
		for _, s := range sessions {
			s.Lock()
			id := s.ID
			if len(id) > 20 {
				id = id[:20]
			}
			fmt.Printf("%-20s  %-10s  %-10s  $%-7.2f  %s\n",
				id, s.Status, s.Provider, s.SpentUSD, s.RepoName)
			s.Unlock()
		}
		return nil
	},
}

var sessionStatusCmd = &cobra.Command{
	Use:   "status [session-id]",
	Short: "Show details for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		mgr := session.NewManager()
		mgr.SetStateDir(filepath.Join(sp, ".session-state"))
		mgr.LoadExternalSessions()

		s, ok := mgr.Get(args[0])
		if !ok {
			return fmt.Errorf("session %s not found", args[0])
		}

		s.Lock()
		defer s.Unlock()

		if sessionJSON {
			data, err := json.MarshalIndent(s, "", "  ")
			if err != nil {
				return fmt.Errorf("json marshal: %w", err)
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("ID:        %s\n", s.ID)
		fmt.Printf("Status:    %s\n", s.Status)
		fmt.Printf("Provider:  %s\n", s.Provider)
		fmt.Printf("Model:     %s\n", s.Model)
		fmt.Printf("Repo:      %s\n", s.RepoName)
		fmt.Printf("Spend:     $%.2f", s.SpentUSD)
		if s.BudgetUSD > 0 {
			fmt.Printf(" / $%.2f", s.BudgetUSD)
		}
		fmt.Println()
		fmt.Printf("Turns:     %d\n", s.TurnCount)
		fmt.Printf("Launched:  %s\n", s.LaunchedAt.Format(time.RFC3339))
		if s.EndedAt != nil {
			fmt.Printf("Ended:     %s\n", s.EndedAt.Format(time.RFC3339))
		}
		if s.Error != "" {
			fmt.Printf("Error:     %s\n", s.Error)
		}
		return nil
	},
}

var sessionStopCmd = &cobra.Command{
	Use:   "stop [session-id]",
	Short: "Stop a running session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		mgr := session.NewManager()
		mgr.SetStateDir(filepath.Join(sp, ".session-state"))
		mgr.LoadExternalSessions()

		if err := mgr.Stop(args[0]); err != nil {
			return err
		}
		fmt.Printf("Session %s stopped.\n", args[0])
		return nil
	},
}

func init() {
	sessionCmd.PersistentFlags().BoolVar(&sessionJSON, "json", false, "Output as JSON")
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionStatusCmd)
	sessionCmd.AddCommand(sessionStopCmd)
	rootCmd.AddCommand(sessionCmd)
}
