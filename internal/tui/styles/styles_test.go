package styles

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestStatusStyle(t *testing.T) {
	tests := []string{"running", "completed", "failed", "idle", "stopped", "unknown", ""}
	for _, s := range tests {
		style := StatusStyle(s)
		rendered := style.Render(s)
		if s != "" && rendered == "" {
			t.Errorf("StatusStyle(%q).Render returned empty", s)
		}
	}
}

func TestCBStyle(t *testing.T) {
	tests := []string{"CLOSED", "HALF_OPEN", "OPEN", "unknown", ""}
	for _, s := range tests {
		style := CBStyle(s)
		rendered := style.Render(s)
		if s != "" && rendered == "" {
			t.Errorf("CBStyle(%q).Render returned empty", s)
		}
	}
}

func TestProviderStyle(t *testing.T) {
	tests := []struct {
		provider string
		want     lipgloss.Style
	}{
		{"claude", ProviderClaudeStyle},
		{"gemini", ProviderGeminiStyle},
		{"codex", ProviderCodexStyle},
		{"unknown", InfoStyle},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := ProviderStyle(tt.provider)
			rendered := got.Render("test")
			if rendered == "" {
				t.Errorf("ProviderStyle(%q).Render returned empty", tt.provider)
			}
		})
	}
}

func TestAlertStyle(t *testing.T) {
	tests := []struct {
		severity string
	}{
		{"critical"},
		{"warning"},
		{"info"},
		{"unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := AlertStyle(tt.severity)
			rendered := got.Render("test")
			if rendered == "" {
				t.Errorf("AlertStyle(%q).Render returned empty", tt.severity)
			}
		})
	}
}

func TestPackageLevelStyles_RenderNonEmpty(t *testing.T) {
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"TitleStyle", TitleStyle},
		{"HeaderStyle", HeaderStyle},
		{"SelectedStyle", SelectedStyle},
		{"StatusRunning", StatusRunning},
		{"StatusCompleted", StatusCompleted},
		{"StatusFailed", StatusFailed},
		{"StatusIdle", StatusIdle},
		{"CircuitClosed", CircuitClosed},
		{"CircuitHalfOpen", CircuitHalfOpen},
		{"CircuitOpen", CircuitOpen},
		{"HelpStyle", HelpStyle},
		{"InfoStyle", InfoStyle},
		{"BreadcrumbStyle", BreadcrumbStyle},
		{"BreadcrumbSep", BreadcrumbSep},
		{"StatusBarStyle", StatusBarStyle},
		{"CommandStyle", CommandStyle},
		{"NotificationStyle", NotificationStyle},
		{"WarningStyle", WarningStyle},
		{"ProviderClaudeStyle", ProviderClaudeStyle},
		{"ProviderGeminiStyle", ProviderGeminiStyle},
		{"ProviderCodexStyle", ProviderCodexStyle},
		{"TabActive", TabActive},
		{"TabInactive", TabInactive},
		{"AlertCritical", AlertCritical},
		{"AlertWarning", AlertWarning},
		{"AlertInfo", AlertInfo},
		{"StatBox", StatBox},
		{"ModalBoxStyle", ModalBoxStyle},
		{"ModalButtonStyle", ModalButtonStyle},
		{"ModalButtonActiveStyle", ModalButtonActiveStyle},
		{"MenuStyle", MenuStyle},
		{"MenuItemStyle", MenuItemStyle},
		{"MenuItemActiveStyle", MenuItemActiveStyle},
		{"MenuItemDestructiveStyle", MenuItemDestructiveStyle},
	}
	for _, tt := range styles {
		t.Run(tt.name, func(t *testing.T) {
			rendered := tt.style.Render("test")
			if rendered == "" {
				t.Errorf("%s.Render(\"test\") returned empty", tt.name)
			}
		})
	}
}
