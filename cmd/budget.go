package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var budgetJSON bool
var budgetTenantID string

var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "View and manage session budgets",
}

var budgetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show aggregate budget status across all sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		mgr := initManagerWithStore(nil)
		loadManagerExternalSessions(mgr, sp)

		sessions := mgr.ListByTenant("", session.NormalizeTenantID(budgetTenantID))
		var totalSpent, totalBudget float64
		var active, completed int
		for _, s := range sessions {
			s.Lock()
			totalSpent += s.SpentUSD
			totalBudget += s.BudgetUSD
			if s.Status.IsTerminal() {
				completed++
			} else {
				active++
			}
			s.Unlock()
		}

		if budgetJSON {
			data, err := json.MarshalIndent(map[string]any{
				"total_spent_usd":  totalSpent,
				"total_budget_usd": totalBudget,
				"sessions_active":  active,
				"sessions_done":    completed,
			}, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("Total spend:  $%.2f", totalSpent)
		if totalBudget > 0 {
			fmt.Printf(" / $%.2f (%.0f%%)", totalBudget, totalSpent/totalBudget*100)
		}
		fmt.Println()
		fmt.Printf("Sessions:     %d active, %d completed\n", active, completed)
		return nil
	},
}

var budgetSetCmd = &cobra.Command{
	Use:   "set [session-id] [amount]",
	Short: "Set budget for a session (USD)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		mgr := initManagerWithStore(nil)
		loadManagerExternalSessions(mgr, sp)

		s, ok := mgr.GetForTenant(args[0], session.NormalizeTenantID(budgetTenantID))
		if !ok {
			return fmt.Errorf("session %s not found", args[0])
		}

		var amount float64
		if _, err := fmt.Sscanf(args[1], "%f", &amount); err != nil {
			return fmt.Errorf("invalid amount %q: %w", args[1], err)
		}
		if amount < 0 {
			return fmt.Errorf("budget must be non-negative")
		}

		s.Lock()
		s.BudgetUSD = amount
		s.Unlock()
		fmt.Printf("Session %s budget set to $%.2f\n", args[0], amount)
		return nil
	},
}

var budgetResetCmd = &cobra.Command{
	Use:   "reset [session-id]",
	Short: "Reset spend tracking for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		mgr := initManagerWithStore(nil)
		loadManagerExternalSessions(mgr, sp)

		s, ok := mgr.GetForTenant(args[0], session.NormalizeTenantID(budgetTenantID))
		if !ok {
			return fmt.Errorf("session %s not found", args[0])
		}

		s.Lock()
		s.SpentUSD = 0
		s.CostHistory = nil
		s.Unlock()
		fmt.Printf("Session %s spend reset to $0.00\n", args[0])
		return nil
	},
}

func init() {
	budgetCmd.PersistentFlags().BoolVar(&budgetJSON, "json", false, "Output as JSON")
	budgetCmd.PersistentFlags().StringVar(&budgetTenantID, "tenant-id", session.DefaultTenantID, "Tenant ID")
	budgetSetCmd.ValidArgsFunction = sessionIDCompletion
	budgetResetCmd.ValidArgsFunction = sessionIDCompletion
	budgetCmd.AddCommand(budgetStatusCmd, budgetSetCmd, budgetResetCmd)
	rootCmd.AddCommand(budgetCmd)
}

// Ensure budgetCmd has proper help with subcommands listed.
var _ = func() string {
	return strings.Join([]string{"status", "set", "reset"}, ", ")
}
