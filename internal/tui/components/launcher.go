package components

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LaunchField identifies a field in the session launcher.
type LaunchField int

const (
	FieldProvider LaunchField = iota
	FieldPrompt
	FieldModel
	FieldBudget
	FieldAgent
	launchFieldCount
)

// LaunchResultMsg is sent when the launcher form is submitted.
type LaunchResultMsg struct {
	Provider string
	Prompt   string
	Model    string
	Budget   string
	Agent    string
	RepoPath string
	RepoName string
}

// SessionLauncher is a modal form for launching a new session.
type SessionLauncher struct {
	Active   bool
	RepoPath string
	RepoName string
	Cursor   LaunchField
	Fields   [launchFieldCount]string
	Labels   [launchFieldCount]string
	Editing  bool
	EditBuf  string
	Width    int
}

// NewSessionLauncher creates a launcher pre-configured with defaults.
func NewSessionLauncher(repoPath, repoName string) *SessionLauncher {
	l := &SessionLauncher{
		Active:   true,
		RepoPath: repoPath,
		RepoName: repoName,
	}
	l.Labels = [launchFieldCount]string{
		"Provider", "Prompt", "Model", "Budget", "Agent",
	}
	l.Fields[FieldProvider] = "claude"
	return l
}

// CycleProvider cycles through claude → gemini → codex.
func (l *SessionLauncher) CycleProvider() {
	switch l.Fields[FieldProvider] {
	case "claude":
		l.Fields[FieldProvider] = "gemini"
	case "gemini":
		l.Fields[FieldProvider] = "codex"
	default:
		l.Fields[FieldProvider] = "claude"
	}
}

// HandleKey processes a key in the launcher. Returns a launch result and true on submit.
func (l *SessionLauncher) HandleKey(keyType string, r rune) (LaunchResultMsg, bool) {
	if l.Editing {
		switch keyType {
		case "enter":
			l.Fields[l.Cursor] = l.EditBuf
			l.Editing = false
			return LaunchResultMsg{}, false
		case "esc":
			l.Editing = false
			return LaunchResultMsg{}, false
		case "backspace":
			if len(l.EditBuf) > 0 {
				l.EditBuf = l.EditBuf[:len(l.EditBuf)-1]
			}
			return LaunchResultMsg{}, false
		case "rune":
			l.EditBuf += string(r)
			return LaunchResultMsg{}, false
		}
		return LaunchResultMsg{}, false
	}

	switch keyType {
	case "up":
		if l.Cursor > 0 {
			l.Cursor--
		}
	case "down":
		if l.Cursor < launchFieldCount-1 {
			l.Cursor++
		}
	case "tab":
		if l.Cursor == FieldProvider {
			l.CycleProvider()
		} else {
			l.Editing = true
			l.EditBuf = l.Fields[l.Cursor]
		}
	case "enter":
		if l.Cursor == FieldProvider {
			l.CycleProvider()
		} else {
			// If prompt is non-empty, submit. Otherwise start editing.
			if l.Fields[FieldPrompt] != "" {
				return l.Submit(), true
			}
			l.Editing = true
			l.EditBuf = l.Fields[l.Cursor]
		}
	case "esc":
		l.Active = false
		return LaunchResultMsg{}, false
	case "rune":
		// Start editing current field on any printable char
		l.Editing = true
		l.EditBuf = l.Fields[l.Cursor] + string(r)
	}

	return LaunchResultMsg{}, false
}

// Submit returns the launch result from current field values.
func (l *SessionLauncher) Submit() LaunchResultMsg {
	l.Active = false
	return LaunchResultMsg{
		Provider: l.Fields[FieldProvider],
		Prompt:   l.Fields[FieldPrompt],
		Model:    l.Fields[FieldModel],
		Budget:   l.Fields[FieldBudget],
		Agent:    l.Fields[FieldAgent],
		RepoPath: l.RepoPath,
		RepoName: l.RepoName,
	}
}

// View renders the launcher form.
func (l *SessionLauncher) View() string {
	if !l.Active {
		return ""
	}

	width := l.Width
	if width <= 0 {
		width = 60
	}

	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" Launch Session — %s ", l.RepoName)))
	b.WriteString("\n\n")

	for i := LaunchField(0); i < launchFieldCount; i++ {
		prefix := "  "
		if i == l.Cursor {
			prefix = "> "
		}

		label := fmt.Sprintf("%-10s", l.Labels[i])
		value := l.Fields[i]

		if i == l.Cursor && l.Editing {
			value = l.EditBuf + "█"
		}

		if value == "" {
			value = styles.InfoStyle.Render("(empty)")
		}

		line := fmt.Sprintf("%s%s: %s", prefix, label, value)
		if i == l.Cursor {
			b.WriteString(styles.SelectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  Tab: cycle/edit  Enter: submit  Esc: cancel"))

	return styles.StatBox.Width(width - 4).Render(b.String())
}
