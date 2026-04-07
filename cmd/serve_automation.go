package cmd

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

type serveAutomationRuntime struct {
	supervisors []*session.Supervisor
	gateway     *session.DocsResearchGateway
}

func startServeAutomationRuntime(ctx context.Context, scanRoot string, bus *events.Bus, mgr *session.Manager) (*serveAutomationRuntime, error) {
	if !serveAutomation || mgr == nil {
		return nil, nil
	}

	repos, err := discovery.Scan(ctx, scanRoot)
	if err != nil {
		return nil, err
	}

	runtime := &serveAutomationRuntime{}
	docsRoot := defaultServeDocsRoot(scanRoot)
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
		// Run one evaluation immediately so overdue schedules and parked resumes
		// do not wait for the first supervisor tick after process start.
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

func (r *serveAutomationRuntime) Stop() {
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

func defaultServeDocsRoot(scanRoot string) string {
	parent := filepath.Dir(scanRoot)
	return filepath.Join(parent, "docs")
}
