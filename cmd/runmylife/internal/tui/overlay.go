package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type formSubmittedMsg struct {
	toast toast
}

type overlay struct {
	active   bool
	title    string
	form     *huh.Form
	onSubmit func() tea.Msg
}

func (o *overlay) open(title string, form *huh.Form, onSubmit func() tea.Msg) tea.Cmd {
	o.active = true
	o.title = title
	o.form = form
	o.onSubmit = onSubmit
	return o.form.Init()
}

func (o *overlay) close() {
	o.active = false
	o.form = nil
	o.title = ""
	o.onSubmit = nil
}

func (o *overlay) update(msg tea.Msg) tea.Cmd {
	if !o.active || o.form == nil {
		return nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" {
			o.close()
			return nil
		}
	}

	model, cmd := o.form.Update(msg)
	if f, ok := model.(*huh.Form); ok {
		o.form = f
	}

	if o.form.State == huh.StateCompleted {
		var submitCmd tea.Cmd
		if o.onSubmit != nil {
			fn := o.onSubmit
			submitCmd = func() tea.Msg { return fn() }
		}
		o.close()
		return tea.Batch(cmd, submitCmd)
	}

	return cmd
}

func (o *overlay) view(width, height int) string {
	if !o.active || o.form == nil {
		return ""
	}

	formView := o.form.View()
	boxWidth := width / 2
	if boxWidth < 44 {
		boxWidth = 44
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Width(boxWidth).
		Render(subtitleStyle.Render(o.title) + "\n\n" + formView + "\n\n" + mutedStyle.Render("esc to cancel"))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
