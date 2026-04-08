package tui

import (
	"fmt"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// EventBusMsg wraps an internal event for the BubbleTea loop.
type EventBusMsg events.Event

func (m *Model) handleEventBusMsg(e events.Event) {
	switch e.Type {
	case events.SessionStarted, events.SessionEnded, events.SessionStopped:
		if m.needsSessionTable() {
			m.updateSessionTable()
		}
		if m.needsTeamTable() {
			m.updateTeamTable()
		}
		m.refreshSessionStatusBar()

	case events.CostUpdate, events.BudgetAlert, events.BudgetExceeded:
		m.refreshCostStatusBar()
		if e.Type == events.BudgetAlert || e.Type == events.BudgetExceeded {
			if label, ok := e.Data["label"]; ok {
				id := e.SessionID
				if len(id) > 8 {
					id = id[:8]
				}
				m.Notify.Show(fmt.Sprintf("Budget alert: %v (session %s)", label, id), 5*time.Second)
			}
		}

	case events.LoopStarted, events.LoopStopped, events.LoopRestarted, events.LoopIterated:
		if m.ShowLoopPanel {
			m.refreshLoopView()
		}
		if m.Nav.CurrentView == ViewLoopControl {
			m.refreshLoopControlData()
		}
		if e.RepoPath != "" {
			for _, r := range m.Repos {
				if r.Path == e.RepoPath {
					model.RefreshRepo(m.Ctx, r)
					break
				}
			}
		}
		if m.needsRepoTable() {
			m.updateTable()
		} else {
			m.refreshStatusBarCounts()
		}

	case events.LoopRegression:
		m.drainRegressionEvents()

	case events.TeamCreated:
		if m.needsTeamTable() {
			m.updateTeamTable()
		}

	case events.AnomalyDetected:
		if msg, ok := e.Data["message"]; ok {
			m.Notify.Show(fmt.Sprintf("Anomaly: %v", msg), 8*time.Second)
		}

	case events.EmergencyStop, events.EmergencyResume:
		sev := "EMERGENCY STOP engaged"
		if e.Type == events.EmergencyResume {
			sev = "Emergency stop lifted"
		}
		m.Notify.Show(sev, 10*time.Second)
	}
}
