package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewPromptEditor_Defaults(t *testing.T) {
	m := NewPromptEditor("hello world", []string{"claude", "gemini"})
	if m.original != "hello world" {
		t.Fatalf("expected original %q, got %q", "hello world", m.original)
	}
	if m.ActivePane() != PaneOriginal {
		t.Fatal("expected initial pane to be PaneOriginal")
	}
	if m.SelectedProvider() != "claude" {
		t.Fatalf("expected initial provider 'claude', got %q", m.SelectedProvider())
	}
}

func TestNewPromptEditor_EmptyProviders(t *testing.T) {
	m := NewPromptEditor("test", nil)
	if m.SelectedProvider() != "claude" {
		t.Fatalf("expected fallback provider 'claude', got %q", m.SelectedProvider())
	}
}

func TestPromptEditor_TabSwitchesPane(t *testing.T) {
	m := NewPromptEditor("orig", []string{"claude"})
	if m.ActivePane() != PaneOriginal {
		t.Fatal("should start on original pane")
	}

	msg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.Update(msg)
	m2 := result.(PromptEditorModel)
	if m2.ActivePane() != PaneEnhanced {
		t.Fatal("tab should switch to enhanced pane")
	}

	result, _ = m2.Update(msg)
	m3 := result.(PromptEditorModel)
	if m3.ActivePane() != PaneOriginal {
		t.Fatal("tab should wrap back to original pane")
	}
}

func TestPromptEditor_ShiftTabSwitchesPane(t *testing.T) {
	m := NewPromptEditor("orig", []string{"claude"})

	msg := tea.KeyMsg{Type: tea.KeyShiftTab}
	result, _ := m.Update(msg)
	m2 := result.(PromptEditorModel)
	if m2.ActivePane() != PaneEnhanced {
		t.Fatal("shift+tab from original should go to enhanced")
	}
}

func TestPromptEditor_EnterAcceptsOriginal(t *testing.T) {
	m := NewPromptEditor("my prompt", []string{"claude"})

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}

	result := cmd()
	accepted, ok := result.(PromptAcceptedMsg)
	if !ok {
		t.Fatalf("expected PromptAcceptedMsg, got %T", result)
	}
	if accepted.Text != "my prompt" {
		t.Fatalf("expected accepted text %q, got %q", "my prompt", accepted.Text)
	}
	if accepted.Provider != "claude" {
		t.Fatalf("expected provider 'claude', got %q", accepted.Provider)
	}
}

func TestPromptEditor_EnterAcceptsEnhanced(t *testing.T) {
	m := NewPromptEditor("orig", []string{"gemini"})
	m.SetEnhanced("better prompt")

	// Switch to enhanced pane
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.Update(tabMsg)
	m2 := result.(PromptEditorModel)

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m2.Update(enterMsg)
	if cmd == nil {
		t.Fatal("enter should produce a command")
	}

	out := cmd()
	accepted := out.(PromptAcceptedMsg)
	if accepted.Text != "better prompt" {
		t.Fatalf("expected enhanced text, got %q", accepted.Text)
	}
}

func TestPromptEditor_EnterOnEmptyEnhancedFallsBackToOriginal(t *testing.T) {
	m := NewPromptEditor("orig", []string{"claude"})
	// No enhanced text set; switch to enhanced pane
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.Update(tabMsg)
	m2 := result.(PromptEditorModel)

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m2.Update(enterMsg)
	out := cmd()
	accepted := out.(PromptAcceptedMsg)
	if accepted.Text != "orig" {
		t.Fatalf("empty enhanced should fall back to original, got %q", accepted.Text)
	}
}

func TestPromptEditor_ProviderCycling(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude", "gemini", "openai"})
	if m.SelectedProvider() != "claude" {
		t.Fatal("should start on claude")
	}

	// Press 'p' to cycle forward
	pMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}
	result, _ := m.Update(pMsg)
	m2 := result.(PromptEditorModel)
	if m2.SelectedProvider() != "gemini" {
		t.Fatalf("expected gemini, got %q", m2.SelectedProvider())
	}

	result, _ = m2.Update(pMsg)
	m3 := result.(PromptEditorModel)
	if m3.SelectedProvider() != "openai" {
		t.Fatalf("expected openai, got %q", m3.SelectedProvider())
	}

	result, _ = m3.Update(pMsg)
	m4 := result.(PromptEditorModel)
	if m4.SelectedProvider() != "claude" {
		t.Fatalf("expected wrap to claude, got %q", m4.SelectedProvider())
	}
}

func TestPromptEditor_ProviderCycleBackward(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude", "gemini"})
	bigP := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}}
	result, _ := m.Update(bigP)
	m2 := result.(PromptEditorModel)
	if m2.SelectedProvider() != "gemini" {
		t.Fatalf("expected gemini, got %q", m2.SelectedProvider())
	}
}

func TestPromptEditor_ScrollKeys(t *testing.T) {
	m := NewPromptEditor("line1\nline2\nline3\nline4\nline5", []string{"claude"})

	downMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ := m.Update(downMsg)
	m2 := result.(PromptEditorModel)
	if m2.scrollLeft != 1 {
		t.Fatalf("expected scrollLeft=1, got %d", m2.scrollLeft)
	}

	upMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ = m2.Update(upMsg)
	m3 := result.(PromptEditorModel)
	if m3.scrollLeft != 0 {
		t.Fatalf("expected scrollLeft=0, got %d", m3.scrollLeft)
	}
}

func TestPromptEditor_ScrollUpDoesNotGoNegative(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude"})
	upMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	result, _ := m.Update(upMsg)
	m2 := result.(PromptEditorModel)
	if m2.scrollLeft != 0 {
		t.Fatal("scroll should not go negative")
	}
}

func TestPromptEditor_ViewContainsTitle(t *testing.T) {
	m := NewPromptEditor("hello", []string{"claude"})
	m.width = 100
	m.height = 30
	output := m.View()
	if !strings.Contains(output, "Prompt A/B Editor") {
		t.Fatal("view should contain title")
	}
}

func TestPromptEditor_ViewContainsPaneLabels(t *testing.T) {
	m := NewPromptEditor("my prompt", []string{"claude"})
	m.width = 100
	m.height = 30
	m.SetEnhanced("improved prompt")
	output := m.View()
	if !strings.Contains(output, "Original") {
		t.Fatal("view should contain 'Original' label")
	}
	if !strings.Contains(output, "Enhanced") {
		t.Fatal("view should contain 'Enhanced' label")
	}
}

func TestPromptEditor_ViewContainsPromptContent(t *testing.T) {
	m := NewPromptEditor("alpha bravo", []string{"claude"})
	m.width = 100
	m.height = 30
	m.SetEnhanced("charlie delta")
	output := m.View()
	if !strings.Contains(output, "alpha bravo") {
		t.Fatal("view should contain original prompt text")
	}
	if !strings.Contains(output, "charlie delta") {
		t.Fatal("view should contain enhanced prompt text")
	}
}

func TestPromptEditor_ScoreRendering(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude"})
	m.width = 120
	m.height = 30
	m.SetScore(&QualityScore{
		Overall: 82,
		Grade:   "B",
		Dimensions: []ScoreDimension{
			{Name: "clarity", Score: 90, Grade: "A"},
			{Name: "structure", Score: 75, Grade: "C"},
		},
	})
	output := m.View()
	if !strings.Contains(output, "82/100") {
		t.Fatal("view should contain overall score")
	}
	if !strings.Contains(output, "B") {
		t.Fatal("view should contain overall grade")
	}
	if !strings.Contains(output, "clarity") {
		t.Fatal("view should contain dimension name")
	}
}

func TestPromptEditor_ViewContainsHelpBar(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude"})
	m.width = 100
	m.height = 30
	output := m.View()
	if !strings.Contains(output, "tab:switch pane") {
		t.Fatal("view should contain help text")
	}
	if !strings.Contains(output, "enter:accept") {
		t.Fatal("view should contain enter help")
	}
}

func TestPromptEditor_ViewShowsProviders(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude", "gemini"})
	m.width = 100
	m.height = 30
	output := m.View()
	if !strings.Contains(output, "claude") {
		t.Fatal("view should show claude provider")
	}
	if !strings.Contains(output, "gemini") {
		t.Fatal("view should show gemini provider")
	}
}

func TestPromptEditor_WindowSizeMsg(t *testing.T) {
	m := NewPromptEditor("test", []string{"claude"})
	msg := tea.WindowSizeMsg{Width: 120, Height: 40}
	result, _ := m.Update(msg)
	m2 := result.(PromptEditorModel)
	if m2.width != 120 || m2.height != 40 {
		t.Fatalf("expected 120x40, got %dx%d", m2.width, m2.height)
	}
}

func TestPromptEditor_InitReturnsNil(t *testing.T) {
	m := NewPromptEditor("test", nil)
	cmd := m.Init()
	if cmd != nil {
		t.Fatal("Init should return nil")
	}
}

func TestPromptEditor_EmptyContent(t *testing.T) {
	m := NewPromptEditor("", []string{"claude"})
	m.width = 80
	m.height = 24
	output := m.View()
	if !strings.Contains(output, "(empty)") {
		t.Fatal("empty content should show placeholder")
	}
}

func TestPromptEditorPane_String(t *testing.T) {
	if PaneOriginal.String() != "Original" {
		t.Fatal("PaneOriginal string mismatch")
	}
	if PaneEnhanced.String() != "Enhanced" {
		t.Fatal("PaneEnhanced string mismatch")
	}
	if PromptEditorPane(99).String() != "?" {
		t.Fatal("unknown pane should return '?'")
	}
}

func TestPromptEditor_ScrollRightPane(t *testing.T) {
	m := NewPromptEditor("orig", []string{"claude"})
	m.SetEnhanced("line1\nline2\nline3")

	// Switch to enhanced pane
	tabMsg := tea.KeyMsg{Type: tea.KeyTab}
	result, _ := m.Update(tabMsg)
	m2 := result.(PromptEditorModel)

	// Scroll down in right pane
	downMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	result, _ = m2.Update(downMsg)
	m3 := result.(PromptEditorModel)
	if m3.scrollRight != 1 {
		t.Fatalf("expected scrollRight=1, got %d", m3.scrollRight)
	}
	// Left pane scroll should be unchanged
	if m3.scrollLeft != 0 {
		t.Fatalf("expected scrollLeft=0, got %d", m3.scrollLeft)
	}
}
