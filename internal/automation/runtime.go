package automation

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Runtime owns the repo-local automation supervisors that can run alongside
// fleet coordinator/worker processes.
type Runtime struct {
	supervisors []*session.Supervisor
	gateway     *session.DocsResearchGateway
}

func defaultDocsRoot(scanRoot string) string {
	return filepath.Join(filepath.Dir(scanRoot), "docs")
}

// StartServeRuntime scans the workspace, attaches subscription automation
// supervisors for opted-in repos, and optionally wires the shared docs gateway.
func StartServeRuntime(ctx context.Context, scanRoot string, enabled bool, bus *events.Bus, mgr *session.Manager) (*Runtime, error) {
	if !enabled || mgr == nil {
		return nil, nil
	}

	repos, err := discovery.Scan(ctx, scanRoot)
	if err != nil {
		return nil, err
	}

	runtime := &Runtime{}
	docsRoot := defaultDocsRoot(scanRoot)
	if _, err := os.Stat(filepath.Join(docsRoot, ".docs.sqlite")); err == nil {
		gateway, gwErr := session.NewDocsResearchGateway(docsRoot)
		if gwErr != nil {
			slog.Warn("serve: research gateway unavailable", "docs_root", docsRoot, "error", gwErr)
		} else {
			runtime.gateway = gateway
		}
	}

	for _, repo := range repos {
		if repo == nil {
			continue
		}
		ctrl := mgr.EnsureSubscriptionAutomation(repo.Path)
		if ctrl == nil || !ctrl.Policy().Enabled {
			continue
		}

		sup := session.NewSupervisor(mgr, repo.Path)
		sup.SetBus(bus)
		sup.SetSubscriptionAutomation(ctrl)
		if runtime.gateway != nil {
			rd := session.NewResearchDaemon(runtime.gateway, session.DefaultResearchDaemonConfig())
			rd.SetBus(bus)
			sup.SetResearchDaemon(rd)
		}
		if err := sup.ResumeFromState(); err != nil {
			slog.Debug("serve: supervisor resume skipped", "repo", repo.Path, "error", err)
		}
		ctrl.Tick(ctx)
		if err := sup.Start(ctx); err != nil {
			slog.Warn("serve: automation supervisor start failed", "repo", repo.Path, "error", err)
			continue
		}
		runtime.supervisors = append(runtime.supervisors, sup)
		slog.Info("serve: automation supervisor started", "repo", repo.Path)
	}

	return runtime, nil
}

// Stop gracefully tears down all supervisors and closes the shared docs gateway.
func (r *Runtime) Stop() {
	if r == nil {
		return
	}
	for _, sup := range r.supervisors {
		if sup != nil {
			sup.Stop()
		}
	}
	if r.gateway != nil {
		_ = r.gateway.Close()
	}
}
