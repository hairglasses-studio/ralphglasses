package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	serveCoordinator  bool
	servePort         int
	coordinatorURL    string
	fleetBudget       float64
	serveAutomation   bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run as a fleet node (coordinator or worker)",
	Long: `Start ralphglasses as a fleet node for distributed workload management.

Coordinator mode (one per fleet):
  ralphglasses serve --coordinator --port 9473

Worker mode (N per fleet):
  ralphglasses serve --coordinator-url http://100.x.y.z:9473

If --coordinator-url is not specified in worker mode, the node will probe
Tailscale peers on the fleet port to auto-discover the coordinator.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle signals
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Fprintln(os.Stderr, "\nshutting down...")
			cancel()
		}()

		bus, sessMgr, hostname := setupServe()
		runtime, err := startServeAutomationRuntime(ctx, sp, bus, sessMgr)
		if err != nil {
			return err
		}
		defer func() {
			if runtime != nil {
				runtime.Stop()
			}
		}()

		if serveCoordinator {
			return runCoordinator(ctx, sp, hostname, bus, sessMgr)
		}
		return runWorker(ctx, hostname, sp, bus, sessMgr)
	},
}

// setupServe creates the event bus, session manager, and resolves the hostname
// for fleet communication.
func setupServe() (*events.Bus, *session.Manager, string) {
	bus := events.NewBus(1000)
	sessMgr := initManagerWithStore(bus)
	hostname := fleet.GetHostname()
	return bus, sessMgr, hostname
}

func runCoordinator(ctx context.Context, scanRoot, hostname string, bus *events.Bus, sessMgr *session.Manager) error {
	nodeID := fmt.Sprintf("coord-%s", hostname)
	coord := fleet.NewCoordinatorWithPersistence(nodeID, hostname, servePort, version, bus, sessMgr, scanRoot)

	if fleetBudget > 0 {
		coord.SetBudgetLimit(fleetBudget)
	}

	fmt.Fprintf(os.Stderr, "starting coordinator %s on :%d\n", nodeID, servePort)

	errCh := make(chan error, 1)
	go func() {
		errCh <- coord.Start(ctx)
	}()

	select {
	case <-ctx.Done():
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5_000_000_000)
		defer shutCancel()
		return coord.Stop(shutCtx)
	case err := <-errCh:
		return err
	}
}

func runWorker(ctx context.Context, hostname, scanPath string, bus *events.Bus, sessMgr *session.Manager) error {
	coordURL := coordinatorURL

	// Auto-discover if not specified
	if coordURL == "" {
		fmt.Fprintln(os.Stderr, "no --coordinator-url specified, discovering via Tailscale...")
		coordURL = fleet.DiscoverCoordinator(servePort)
		if coordURL == "" {
			return fmt.Errorf("could not discover coordinator; specify --coordinator-url")
		}
		fmt.Fprintf(os.Stderr, "discovered coordinator at %s\n", coordURL)
	}

	worker := fleet.NewWorkerAgent(coordURL, hostname, servePort, version, scanPath, bus, sessMgr)

	fmt.Fprintf(os.Stderr, "starting worker, connecting to %s\n", coordURL)
	return worker.Run(ctx)
}

func init() {
	serveCmd.Flags().BoolVar(&serveCoordinator, "coordinator", false,
		"Run as fleet coordinator (default: worker)")
	serveCmd.Flags().IntVar(&servePort, "port", fleet.DefaultPort,
		"HTTP port for fleet communication")
	serveCmd.Flags().StringVar(&coordinatorURL, "coordinator-url", "",
		"Coordinator URL (auto-discover via Tailscale if empty)")
	serveCmd.Flags().Float64Var(&fleetBudget, "fleet-budget", 500,
		"Fleet-wide budget ceiling in USD")
	serveCmd.Flags().BoolVar(&serveAutomation, "automation", true,
		"Run repo-local subscription automation supervisors alongside fleet serving")
	rootCmd.AddCommand(serveCmd)
}
