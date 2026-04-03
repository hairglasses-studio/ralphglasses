package session

import "log/slog"

// SaveBanditState persists the cascade router's bandit state to disk.
// No-op if no cascade router or bandit router is configured.
func (m *Manager) SaveBanditState(dir string) {
	if m.cascade == nil {
		return
	}
	br := m.cascade.BanditRouter()
	if br == nil {
		return
	}
	if err := br.SaveBanditState(dir); err != nil {
		slog.Warn("manager: failed to save bandit state", "error", err)
	}
}

// RestoreBanditState loads bandit state from disk and applies it to the
// cascade router. No-op if no state file exists or no cascade router is set.
func (m *Manager) RestoreBanditState(dir string) {
	if m.cascade == nil {
		return
	}
	br := m.cascade.BanditRouter()
	if br == nil {
		return
	}
	state, err := LoadBanditState(dir)
	if err != nil {
		slog.Warn("manager: failed to load bandit state", "error", err)
		return
	}
	if state != nil {
		br.RestoreFromState(state)
	}
}
