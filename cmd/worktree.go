package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var worktreeJSON bool

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees for ralph loops",
	Long:  `Create, list, and clean up git worktrees used by ralph loop iterations.`,
}

var worktreeListCmd = &cobra.Command{
	Use:   "list [repo-path]",
	Short: "List active worktrees",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath := util.ExpandHome(scanPath)
		if len(args) > 0 {
			repoPath = args[0]
		}

		wts, err := session.ListWorktrees(repoPath)
		if err != nil {
			return err
		}

		if worktreeJSON {
			data, err := json.MarshalIndent(wts, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		if len(wts) == 0 {
			fmt.Println("No worktrees found.")
			return nil
		}

		fmt.Printf("%-30s  %-15s  %-30s  %-5s  %s\n", "LOOP", "ITERATION", "BRANCH", "DIRTY", "MODIFIED")
		fmt.Println(strings.Repeat("-", 90))
		for _, wt := range wts {
			parts := strings.Split(wt.Path, "/")
			iter := ""
			if len(parts) > 0 {
				iter = parts[len(parts)-1]
			}
			dirty := ""
			if wt.Dirty {
				dirty = "yes"
			}
			fmt.Printf("%-30s  %-15s  %-30s  %-5s  %s\n",
				wt.Loop, iter, wt.Branch, dirty, wt.ModTime)
		}
		return nil
	},
}

var worktreeCreateCmd = &cobra.Command{
	Use:   "create <repo-path> <name>",
	Short: "Create a new worktree",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath := args[0]
		name := args[1]

		wtPath, branch, err := session.CreateWorktree(repoPath, name)
		if err != nil {
			return err
		}

		if worktreeJSON {
			data, _ := json.MarshalIndent(map[string]string{
				"path":   wtPath,
				"branch": branch,
			}, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Created worktree:\n  Path:   %s\n  Branch: %s\n", wtPath, branch)
		return nil
	},
}

var worktreeCleanCmd = &cobra.Command{
	Use:   "clean [repo-path]",
	Short: "Remove stale worktrees",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath := util.ExpandHome(scanPath)
		if len(args) > 0 {
			repoPath = args[0]
		}

		maxAge, _ := cmd.Flags().GetInt("max-age-hours")
		if maxAge < 1 {
			maxAge = 24
		}

		cleaned, err := session.CleanupStaleWorktrees(repoPath, time.Duration(maxAge)*time.Hour)
		if err != nil {
			return err
		}

		if worktreeJSON {
			data, _ := json.MarshalIndent(map[string]any{
				"cleaned":       cleaned,
				"max_age_hours": maxAge,
			}, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Cleaned %d stale worktrees (older than %dh).\n", cleaned, maxAge)
		return nil
	},
}

func init() {
	worktreeCmd.PersistentFlags().BoolVar(&worktreeJSON, "json", false, "Output as JSON")
	worktreeCleanCmd.Flags().Int("max-age-hours", 24, "Max age in hours before cleanup")
	worktreeCmd.AddCommand(worktreeListCmd, worktreeCreateCmd, worktreeCleanCmd)
	rootCmd.AddCommand(worktreeCmd)
}
