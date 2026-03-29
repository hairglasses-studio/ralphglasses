package tui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// Update handles incoming messages and returns the updated model and any follow-up commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Table.Width = msg.Width
		m.Table.Height = msg.Height
		m.SessionTable.Width = msg.Width
		m.SessionTable.Height = msg.Height
		m.TeamTable.Width = msg.Width
		m.TeamTable.Height = msg.Height
		m.LoopListTable.Width = msg.Width
		m.LoopListTable.Height = msg.Height
		m.LogView.SetDimensions(msg.Width, msg.Height)
		m.HelpView.SetDimensions(msg.Width, msg.Height-4)
		m.RepoDetailView.SetDimensions(msg.Width, msg.Height-4)
		m.LoopHealthView.SetDimensions(msg.Width, msg.Height-4)
		m.SessionDetailView.SetDimensions(msg.Width, msg.Height-4)
		m.TeamDetailView.SetDimensions(msg.Width, msg.Height-4)
		m.FleetView.SetDimensions(msg.Width, msg.Height-4)
		m.DiffViewport.SetDimensions(msg.Width, msg.Height-4)
		m.TimelineViewport.SetDimensions(msg.Width, msg.Height-4)
		m.LoopDetailView.SetDimensions(msg.Width, msg.Height-4)
		m.LoopControlView.SetDimensions(msg.Width, msg.Height-4)
		m.ObservationViewport.SetDimensions(msg.Width, msg.Height-4)
		m.RDCycleView.SetDimensions(msg.Width, msg.Height-4)
		m.StatusBar.Width = msg.Width
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case tickMsg:
		// Check for external shutdown (e.g. SIGINT/SIGTERM cancelled the context).
		if m.Ctx != nil {
			select {
			case <-m.Ctx.Done():
				return m, tea.Quit
			default:
			}
		}
		m.TickFrame++
		var cmds []tea.Cmd
		cmds = append(cmds, m.refreshAllRepos()...)
		// Load sessions persisted by other processes (e.g. MCP server)
		if m.SessMgr != nil {
			m.SessMgr.LoadExternalSessions()
		}
		// Refresh loop observation and gate caches (TTL-gated, not every tick)
		m.refreshObsCache()
		m.refreshGateCache()
		m.drainRegressionEvents()
		m.refreshLoopView()
		m.refreshLoopControlData()
		cmds = append(cmds, m.loopListCmd())
		m.updateTable()
		m.updateSessionTable()
		m.updateTeamTable()
		m.LastRefresh = components.NowFunc()
		cmds = append(cmds, m.tickCmd())
		// If viewing logs, tail the log
		if m.Nav.CurrentView == ViewLogs && m.Sel.RepoIdx < len(m.Repos) {
			cmds = append(cmds, process.TailLog(m.Repos[m.Sel.RepoIdx].Path, &m.LogOffset))
		}
		return m, tea.Batch(cmds...)

	case LoopListMsg:
		rows := views.LoopRunsToRows([]*session.LoopRun(msg), m.TickFrame)
		m.LoopListTable.SetRows(rows)
		return m, nil

	case LoopStepResultMsg:
		if msg.Err != nil {
			m.Notify.Show(fmt.Sprintf("Step error: %v", msg.Err), 3*time.Second)
		} else {
			m.Notify.Show(fmt.Sprintf("Stepped loop %s", truncateID(msg.LoopID)), 3*time.Second)
		}
		return m, m.loopListCmd()

	case LoopToggleResultMsg:
		if msg.Err != nil {
			m.Notify.Show(fmt.Sprintf("Toggle error: %v", msg.Err), 3*time.Second)
		} else if msg.Started {
			m.Notify.Show(fmt.Sprintf("Started loop %s", truncateID(msg.LoopID)), 3*time.Second)
		} else {
			m.Notify.Show(fmt.Sprintf("Stopped loop %s", truncateID(msg.LoopID)), 3*time.Second)
		}
		return m, m.loopListCmd()

	case LoopPauseResultMsg:
		if msg.Err != nil {
			m.Notify.Show(fmt.Sprintf("Pause error: %v", msg.Err), 3*time.Second)
		} else if msg.Paused {
			m.Notify.Show(fmt.Sprintf("Paused loop %s", truncateID(msg.LoopID)), 3*time.Second)
		} else {
			m.Notify.Show(fmt.Sprintf("Resumed loop %s", truncateID(msg.LoopID)), 3*time.Second)
		}
		return m, m.loopListCmd()

	case RefreshErrorMsg:
		m.Notify.Show(fmt.Sprintf("⚠ %s: parse errors", filepath.Base(msg.RepoPath)), 5*time.Second)
		return m, nil

	case scanResultMsg:
		if msg.err != nil {
			m.Notify.Show(fmt.Sprintf("Scan error: %v", msg.err), 5*time.Second)
			return m, nil
		}
		m.Repos = msg.repos
		// Recover any orphaned processes from PID files
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		if n := m.ProcMgr.Recover(paths); n > 0 {
			m.Notify.Show(fmt.Sprintf("Recovered %d orphaned loop(s)", n), 5*time.Second)
		}
		m.updateTable()
		// Start watching status files
		return m, process.WatchStatusFiles(paths)

	case process.WatcherErrorMsg:
		m.WatcherFails++
		if m.WatcherFails >= 5 && !m.WatcherDisabled {
			// Too many consecutive failures — fall back to polling only
			m.WatcherDisabled = true
			m.Notify.Show("⚠ Watcher failed repeatedly, using polling mode", 5*time.Second)
			return m, nil
		}
		m.Notify.Show(fmt.Sprintf("⚠ Watcher error: %v", msg.Err), 4*time.Second)
		// Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s
		delay := time.Duration(1<<uint(m.WatcherFails-1)) * time.Second
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		return m, tea.Tick(delay, func(time.Time) tea.Msg {
			return watcherBackoffMsg{}
		})

	case watcherBackoffMsg:
		if m.WatcherDisabled {
			return m, nil
		}
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		return m, process.WatchStatusFiles(paths)

	case process.FileChangedMsg:
		// Successful watch — reset failure counter
		m.WatcherFails = 0
		// Reactive update for a single repo
		var cmds []tea.Cmd
		for _, r := range m.Repos {
			if r.Path == msg.RepoPath {
				if errs := model.RefreshRepo(r); len(errs) > 0 {
					repoPath := r.Path
					errs := errs
					cmds = append(cmds, func() tea.Msg {
						return RefreshErrorMsg{RepoPath: repoPath, Errors: errs}
					})
				}
				break
			}
		}
		m.updateTable()
		// Re-watch
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		cmds = append(cmds, process.WatchStatusFiles(paths))
		return m, tea.Batch(cmds...)

	case LogLoadedMsg:
		if msg.Err != nil {
			m.Notify.Show(fmt.Sprintf("Log load error: %v", msg.Err), 3*time.Second)
		} else if m.LogView != nil {
			m.LogView.SetLines(msg.Lines)
		}
		return m, nil

	case process.LogLinesMsg:
		if len(msg.Lines) > 0 {
			m.LogView.AppendLines(msg.Lines)
		}
		return m, nil

	case process.ProcessExitMsg:
		applyProcessExit(msg, m.Repos)
		// Re-arm the listener for the next exit.
		if m.ProcMgr != nil {
			return m, process.WaitForProcessExit(m.ProcMgr.ExitChan())
		}
		return m, nil

	case components.ConfirmResultMsg:
		return m.handleConfirmResult(msg)

	case components.ActionResultMsg:
		return m.handleActionResult(msg)

	case components.LaunchResultMsg:
		return m.handleLaunchResult(msg)

	case SessionOutputMsg:
		if msg.SessionID == m.Stream.SessionID && m.Stream.OutputView != nil {
			m.Stream.OutputView.AppendLines([]string{msg.Line})
		}
		return m, m.streamSessionOutput(msg.SessionID)

	case SessionOutputDoneMsg:
		m.Stream.Active = false
		m.Notify.Show("Session output stream ended", 3*time.Second)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// applyProcessExit updates the status of the repo matching msg.RepoPath
// based on the process exit code.
func applyProcessExit(msg process.ProcessExitMsg, repos []*model.Repo) {
	for _, r := range repos {
		if r.Path == msg.RepoPath {
			if r.Status == nil {
				r.Status = &model.LoopStatus{}
			}
			r.Status.Status = model.RepoStatusFromExitCode(msg.ExitCode, msg.Error)
			return
		}
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Modal overlays take priority
	if m.Modals.ConfirmDialog != nil && m.Modals.ConfirmDialog.Active {
		return m.handleConfirmKey(msg)
	}
	if m.Modals.ActionMenu != nil && m.Modals.ActionMenu.Active {
		return m.handleActionMenuKey(msg)
	}
	if m.Modals.Launcher != nil && m.Modals.Launcher.Active {
		return m.handleLauncherKey(msg)
	}

	// Command mode input
	if m.InputMode == ModeCommand {
		return m.handleCommandInput(msg)
	}

	// Filter mode input
	if m.InputMode == ModeFilter {
		return m.handleFilterInput(msg)
	}

	// Config editor in edit mode
	if m.Nav.CurrentView == ViewConfigEditor && m.ConfigEdit != nil && m.ConfigEdit.Editing {
		return m.handleConfigEditInput(msg)
	}

	// Global keys — iterate dispatch table; first match wins (preserves switch/case semantics).
	for _, entry := range KeyDispatch {
		if key.Matches(msg, entry.Binding(&m.Keys)) {
			return entry.Handler(&m, msg)
		}
	}

	// View-specific keys
	switch m.Nav.CurrentView {
	case ViewOverview:
		return m.handleOverviewKey(msg)
	case ViewRepoDetail:
		return m.handleDetailKey(msg)
	case ViewLogs:
		return m.handleLogKey(msg)
	case ViewConfigEditor:
		return m.handleConfigKey(msg)
	case ViewSessions:
		return m.handleSessionsKey(msg)
	case ViewSessionDetail:
		return m.handleSessionDetailKey(msg)
	case ViewTeams:
		return m.handleTeamsKey(msg)
	case ViewTeamDetail:
		return m.handleTeamDetailKey(msg)
	case ViewFleet:
		return m.handleFleetKey(msg)
	case ViewLoopList:
		return m.handleLoopListKey(msg)
	case ViewLoopDetail:
		return m.handleLoopDetailKey(msg)
	case ViewLoopControl:
		return m.handleLoopControlKey(msg)
	case ViewHelp:
		return m.handleHelpKey(msg)
	case ViewLoopHealth:
		return m.handleLoopHealthKey(msg)
	case ViewDiff:
		return m.handleDiffKey(msg)
	case ViewTimeline:
		return m.handleTimelineKey(msg)
	case ViewObservation:
		return m.handleObservationKey(msg)
	case ViewEventLog:
		return m.handleEventLogKey(msg)
	case ViewRDCycle:
		return m.handleRDCycleKey(msg)
	}

	return m, nil
}
