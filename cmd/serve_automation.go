package cmd

import (
	"context"

	"github.com/hairglasses-studio/ralphglasses/internal/automation"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

type serveAutomationRuntime = automation.Runtime

func startServeAutomationRuntime(ctx context.Context, scanRoot string, bus *events.Bus, mgr *session.Manager) (*serveAutomationRuntime, error) {
	return automation.StartServeRuntime(ctx, scanRoot, serveAutomation, bus, mgr)
}
