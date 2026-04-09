package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	tenantJSON                    bool
	tenantDisplayName             string
	tenantAllowedRoots            []string
	tenantBudgetCapUSD            float64
	tenantLeaderboardLimit        int
	tenantLeaderboardIncludeEnded bool
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage workspace tenants",
}

var tenantListCmd = &cobra.Command{
	Use:   "list",
	Short: "List known tenants",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := initManagerWithStore(nil)
		tenants, err := mgr.ListTenants(context.Background())
		if err != nil {
			return err
		}
		if tenantJSON {
			data, err := json.MarshalIndent(tenants, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		if len(tenants) == 0 {
			fmt.Println("No tenants found.")
			return nil
		}
		for _, tenant := range tenants {
			fmt.Printf("%s\t%s\troots=%d\tbudget_cap=$%.2f\n", tenant.ID, tenant.DisplayName, len(tenant.AllowedRepoRoots), tenant.BudgetCapUSD)
		}
		return nil
	},
}

var tenantCreateCmd = &cobra.Command{
	Use:   "create [tenant-id]",
	Short: "Create or update a tenant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := initManagerWithStore(nil)
		tenant, err := mgr.SaveTenant(context.Background(), &session.Tenant{
			ID:               args[0],
			DisplayName:      tenantDisplayName,
			AllowedRepoRoots: tenantAllowedRoots,
			BudgetCapUSD:     tenantBudgetCapUSD,
		})
		if err != nil {
			return err
		}
		if tenantJSON {
			data, err := json.MarshalIndent(tenant, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("Tenant %s saved.\n", tenant.ID)
		return nil
	},
}

var tenantStatusCmd = &cobra.Command{
	Use:   "status [tenant-id]",
	Short: "Show tenant status and current usage",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tenantID := session.NormalizeTenantID(args[0])
		mgr := initManagerWithStore(nil)
		sp := util.ExpandHome(scanPath)
		loadManagerExternalSessions(mgr, sp)

		tenant, err := mgr.GetTenant(context.Background(), tenantID)
		if err != nil {
			return err
		}

		sessions := mgr.ListByTenant("", tenantID)
		teams := mgr.ListTeamsForTenant(tenantID)
		totalSpend := 0.0
		if store := mgr.Store(); store != nil {
			totalSpend, _ = store.AggregateSpend(context.Background(), tenantID, "")
		}
		payload := map[string]any{
			"tenant":                  tenant,
			"active_sessions":         len(sessions),
			"teams":                   len(teams),
			"total_spend_usd":         totalSpend,
			"allowed_repo_root_count": len(tenant.AllowedRepoRoots),
		}
		if tenantJSON {
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("Tenant: %s (%s)\n", tenant.ID, tenant.DisplayName)
		fmt.Printf("Repo roots: %d\n", len(tenant.AllowedRepoRoots))
		fmt.Printf("Budget cap: $%.2f\n", tenant.BudgetCapUSD)
		fmt.Printf("Sessions: %d\n", len(sessions))
		fmt.Printf("Teams: %d\n", len(teams))
		fmt.Printf("Total spend: $%.2f\n", totalSpend)
		return nil
	},
}

var tenantRotateTriggerTokenCmd = &cobra.Command{
	Use:   "rotate-trigger-token [tenant-id]",
	Short: "Rotate the bearer token for trigger HTTP auth",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := initManagerWithStore(nil)
		token, tenant, err := mgr.RotateTenantTriggerToken(context.Background(), args[0])
		if err != nil {
			return err
		}
		payload := map[string]any{
			"tenant_id":     tenant.ID,
			"trigger_token": token,
		}
		if tenantJSON {
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		fmt.Printf("Tenant %s trigger token: %s\n", tenant.ID, token)
		return nil
	},
}

var tenantLeaderboardCmd = &cobra.Command{
	Use:   "leaderboard [tenant-id]",
	Short: "Generate top-role leaderboards for one tenant or all tenants",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr := initManagerWithStore(nil)
		sp := util.ExpandHome(scanPath)
		loadManagerExternalSessions(mgr, sp)

		opts := session.RoleLeaderboardOptions{
			Limit:        tenantLeaderboardLimit,
			IncludeEnded: tenantLeaderboardIncludeEnded,
		}

		var boards []*session.TenantRoleLeaderboard
		if len(args) == 1 {
			board, err := mgr.BuildRoleLeaderboard(context.Background(), args[0], opts)
			if err != nil {
				return err
			}
			boards = []*session.TenantRoleLeaderboard{board}
		} else {
			allBoards, err := mgr.BuildRoleLeaderboards(context.Background(), opts)
			if err != nil {
				return err
			}
			boards = allBoards
		}

		payload := map[string]any{
			"tenants":       boards,
			"count":         len(boards),
			"limit":         opts.Limit,
			"include_ended": opts.IncludeEnded,
		}
		if tenantJSON {
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		if len(boards) == 0 {
			fmt.Println("No tenants found.")
			return nil
		}

		for i, board := range boards {
			if i > 0 {
				fmt.Println()
			}
			if board.DisplayName != "" && board.DisplayName != board.TenantID {
				fmt.Printf("Tenant: %s (%s)\n", board.TenantID, board.DisplayName)
			} else {
				fmt.Printf("Tenant: %s\n", board.TenantID)
			}
			fmt.Printf("Total sessions: %d\n", board.TotalSessions)
			if len(board.Roles) == 0 {
				fmt.Println("No role activity.")
				continue
			}
			fmt.Printf("%-20s  %-8s  %-8s  %-9s  %-9s  %-8s\n", "ROLE", "SESSIONS", "ACTIVE", "DONE", "SPEND", "TURNS")
			fmt.Println(strings.Repeat("-", 76))
			for _, entry := range board.Roles {
				done := entry.Completed + entry.Errored + entry.Stopped + entry.Interrupted
				fmt.Printf("%-20s  %-8d  %-8d  %-9d  $%-8.2f  %-8d\n",
					entry.Role, entry.Sessions, entry.Active, done, entry.SpendUSD, entry.Turns)
			}
		}
		return nil
	},
}

func init() {
	tenantCmd.PersistentFlags().BoolVar(&tenantJSON, "json", false, "Output as JSON")
	tenantCreateCmd.Flags().StringVar(&tenantDisplayName, "display-name", "", "Tenant display name")
	tenantCreateCmd.Flags().StringArrayVar(&tenantAllowedRoots, "allowed-repo-root", nil, "Allowed repo root (repeatable)")
	tenantCreateCmd.Flags().Float64Var(&tenantBudgetCapUSD, "budget-cap-usd", 0, "Budget cap in USD")
	tenantLeaderboardCmd.Flags().IntVar(&tenantLeaderboardLimit, "limit", 10, "Maximum leaderboard entries per tenant")
	tenantLeaderboardCmd.Flags().BoolVar(&tenantLeaderboardIncludeEnded, "include-ended", true, "Include persisted ended sessions")
	tenantCmd.AddCommand(tenantListCmd, tenantCreateCmd, tenantStatusCmd, tenantRotateTriggerTokenCmd, tenantLeaderboardCmd)
	rootCmd.AddCommand(tenantCmd)
}
