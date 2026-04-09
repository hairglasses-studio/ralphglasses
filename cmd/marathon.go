package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"log/slog"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/resource"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	marathonBudget     float64
	marathonDuration   string
	marathonCheckpoint string
	marathonRepo       string
	marathonRoadmap    string
	marathonResume     bool
)

var marathonCmd = &cobra.Command{
	Use:   "marathon",
	Short: "Run continuous improvement cycles with budget and time limits",
	Long: `Launch a marathon supervisor session that runs improvement cycles
until the budget or duration limit is reached.

Checkpoints are saved at the specified interval for resumability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)

		dur, err := time.ParseDuration(marathonDuration)
		if err != nil {
			return fmt.Errorf("invalid --duration %q: %w", marathonDuration, err)
		}
		cpInterval, err := time.ParseDuration(marathonCheckpoint)
		if err != nil {
			return fmt.Errorf("invalid --checkpoint-interval %q: %w", marathonCheckpoint, err)
		}

		repoPath := marathonRepo
		if repoPath == "" {
			repoPath = filepath.Join(sp, "ralphglasses")
		}
		repoPath = util.ExpandHome(repoPath)

		ctx, cancel := context.WithTimeout(context.Background(), dur)
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "\nmarathon: shutting down...")
			cancel()
		}()

		bus := events.NewBus(1000)
		mgr := initManagerWithStore(bus)
		mgr.SetAutonomyLevel(session.LevelAutoOptimize, repoPath)

		// Pre-flight validation.
		result := session.ValidateConfig(repoPath)
		for _, w := range result.Warnings {
			fmt.Fprintf(os.Stderr, "marathon: WARNING: %s\n", w)
		}
		if !result.OK() {
			for _, e := range result.Errors {
				fmt.Fprintf(os.Stderr, "marathon: ERROR: %s\n", e)
			}
			return fmt.Errorf("pre-flight validation failed (%d errors)", len(result.Errors))
		}

		fmt.Fprintf(os.Stderr, "marathon: starting (budget=$%.2f, duration=%s, checkpoint=%s)\n",
			marathonBudget, dur, cpInterval)
		fmt.Fprintf(os.Stderr, "marathon: repo=%s\n", repoPath)

		// Create and configure supervisor.
		sup := session.NewSupervisor(mgr, repoPath)
		sup.MaxTotalCostUSD = marathonBudget
		sup.MaxDuration = dur
		sup.SetBus(bus)
		sup.SetMonitor(session.NewHealthMonitor(session.DefaultHealthThresholds()))
		sup.SetChainer(session.NewCycleChainer())

		if marathonRoadmap != "" {
			roadmapPath := filepath.Join(repoPath, marathonRoadmap)
			sup.SetSprintPlanner(session.NewSprintPlanner(roadmapPath))
			fmt.Fprintf(os.Stderr, "marathon: loaded roadmap from %s\n", roadmapPath)
		}

		// Resume from previous state if flag set.
		if marathonResume {
			if err := sup.ResumeFromState(); err != nil {
				slog.Warn("marathon: resume failed, starting fresh", "error", err)
			} else {
				fmt.Fprintln(os.Stderr, "marathon: resumed from previous state")
			}
		}

		// Start supervisor.
		if err := sup.Start(ctx); err != nil {
			return fmt.Errorf("supervisor start: %w", err)
		}
		defer sup.Stop()

		fmt.Fprintln(os.Stderr, "marathon: supervisor started")

		// Checkpoint ticker.
		cpTicker := time.NewTicker(cpInterval)
		defer cpTicker.Stop()

		// Resource monitoring ticker (every 60s).
		resTicker := time.NewTicker(60 * time.Second)
		defer resTicker.Stop()

		go func() {
			for {
				select {
				case <-cpTicker.C:
					fmt.Fprintf(os.Stderr, "marathon: checkpoint at %s\n", time.Now().Format(time.RFC3339))
				case <-resTicker.C:
					status := resource.Check(repoPath)
					if !status.IsHealthy() {
						for _, w := range status.Warnings {
							slog.Warn("marathon: resource warning", "warning", w)
							fmt.Fprintf(os.Stderr, "marathon: WARNING: %s\n", w)
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		<-ctx.Done()
		fmt.Fprintln(os.Stderr, "marathon: finished")
		return nil
	},
}

func init() {
	marathonCmd.Flags().Float64Var(&marathonBudget, "budget", 10.0,
		"Maximum budget in USD")
	marathonCmd.Flags().StringVar(&marathonDuration, "duration", "1h",
		"Maximum duration (e.g. 1h, 30m, 2h)")
	marathonCmd.Flags().StringVar(&marathonCheckpoint, "checkpoint-interval", "10m",
		"Checkpoint save interval (e.g. 5m, 10m)")
	marathonCmd.Flags().StringVar(&marathonRepo, "repo", "",
		"Target repository path (default: <scan-path>/ralphglasses)")
	marathonCmd.Flags().StringVar(&marathonRoadmap, "roadmap", "ROADMAP.md",
		"Roadmap file name within the repo (default: ROADMAP.md)")
	marathonCmd.Flags().BoolVar(&marathonResume, "resume", false,
		"Resume from previous supervisor state")
	rootCmd.AddCommand(marathonCmd)
}
