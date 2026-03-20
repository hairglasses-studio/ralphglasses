package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// View modes (navigation stack).
type ViewMode int

const (
	ViewOverview ViewMode = iota
	ViewRepoDetail
	ViewLogs
	ViewConfigEditor
	ViewHelp
)

// InputMode tracks the current input capture mode.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeFilter
)

type tickMsg time.Time

// RefreshErrorMsg is sent when RefreshRepo encounters parse errors.
type RefreshErrorMsg struct {
	RepoPath string
	Errors   []error
}

// Model is the root Bubble Tea model.
type Model struct {
	// Config
	ScanPath string

	// Data
	Repos []*model.Repo

	// Navigation
	CurrentView ViewMode
	ViewStack  []ViewMode
	Breadcrumb components.Breadcrumb

	// Components
	Table      *components.Table
	StatusBar  components.StatusBar
	Notify     components.NotificationManager
	LogView    *views.LogView
	ConfigEdit *views.ConfigEditor

	// State
	Width       int
	Height      int
	InputMode   InputMode
	CommandBuf  string
	Filter      FilterState
	SelectedIdx int // index into Repos for detail/log views
	LogOffset   int64
	LastRefresh time.Time

	// Process management
	ProcMgr *process.Manager
}

// NewModel creates the root model.
func NewModel(scanPath string) Model {
	table := views.NewOverviewTable()
	return Model{
		ScanPath:  scanPath,
		CurrentView: ViewOverview,
		Table:     table,
		LogView:   views.NewLogView(),
		ProcMgr:   process.NewManager(),
		Breadcrumb: components.Breadcrumb{Parts: []string{"Overview"}},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.scanRepos(),
		m.tickCmd(),
	)
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) scanRepos() tea.Cmd {
	path := m.ScanPath
	return func() tea.Msg {
		repos, err := discovery.Scan(path)
		if err != nil {
			return scanResultMsg{err: err}
		}
		return scanResultMsg{repos: repos}
	}
}

type scanResultMsg struct {
	repos []*model.Repo
	err   error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Table.Width = msg.Width
		m.Table.Height = msg.Height
		m.LogView.Width = msg.Width
		m.LogView.Height = msg.Height
		m.StatusBar.Width = msg.Width
		return m, nil

	case tickMsg:
		m.refreshAllRepos()
		m.updateTable()
		m.LastRefresh = time.Now()
		var cmds []tea.Cmd
		cmds = append(cmds, m.tickCmd())
		// If viewing logs, tail the log
		if m.CurrentView == ViewLogs && m.SelectedIdx < len(m.Repos) {
			cmds = append(cmds, process.TailLog(m.Repos[m.SelectedIdx].Path, &m.LogOffset))
		}
		return m, tea.Batch(cmds...)

	case RefreshErrorMsg:
		m.Notify.Show(fmt.Sprintf("⚠ %s: parse errors", filepath.Base(msg.RepoPath)), 5*time.Second)
		return m, nil

	case scanResultMsg:
		if msg.err != nil {
			m.Notify.Show(fmt.Sprintf("Scan error: %v", msg.err), 5*time.Second)
			return m, nil
		}
		m.Repos = msg.repos
		m.updateTable()
		m.LastRefresh = time.Now()
		// Start watching status files
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		return m, process.WatchStatusFiles(paths)

	case process.FileChangedMsg:
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

	case process.LogLinesMsg:
		if len(msg.Lines) > 0 {
			m.LogView.AppendLines(msg.Lines)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Command mode input
	if m.InputMode == ModeCommand {
		return m.handleCommandInput(key, msg)
	}

	// Filter mode input
	if m.InputMode == ModeFilter {
		return m.handleFilterInput(key, msg)
	}

	// Config editor in edit mode
	if m.CurrentView == ViewConfigEditor && m.ConfigEdit != nil && m.ConfigEdit.Editing {
		return m.handleConfigEditInput(key, msg)
	}

	// Global keys
	switch key {
	case KeyQ, KeyCtrlC:
		m.ProcMgr.StopAll()
		return m, tea.Quit
	case KeyColon:
		m.InputMode = ModeCommand
		m.CommandBuf = ""
		return m, nil
	case KeySlash:
		m.InputMode = ModeFilter
		m.Filter.Active = true
		m.Filter.Text = ""
		return m, nil
	case KeyQMark:
		if m.CurrentView == ViewHelp {
			return m.popView()
		}
		m.pushView(ViewHelp, "Help")
		return m, nil
	case KeyEsc:
		return m.popView()
	case KeyR:
		return m, m.scanRepos()
	}

	// View-specific keys
	switch m.CurrentView {
	case ViewOverview:
		return m.handleOverviewKey(key)
	case ViewRepoDetail:
		return m.handleDetailKey(key)
	case ViewLogs:
		return m.handleLogKey(key)
	case ViewConfigEditor:
		return m.handleConfigKey(key)
	}

	return m, nil
}

func (m Model) handleOverviewKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case KeyJ:
		m.Table.MoveDown()
	case KeyK:
		m.Table.MoveUp()
	case KeyS:
		m.Table.CycleSort()
	case KeyEnter:
		row := m.Table.SelectedRow()
		if row != nil {
			m.SelectedIdx = m.findRepoByName(row[0])
			if m.SelectedIdx >= 0 {
				m.pushView(ViewRepoDetail, m.Repos[m.SelectedIdx].Name)
			}
		}
	case KeyShiftS:
		return m.startSelectedLoop()
	case KeyShiftX:
		return m.stopSelectedLoop()
	case KeyShiftP:
		return m.togglePauseSelected()
	}
	return m, nil
}

func (m Model) handleDetailKey(key string) (tea.Model, tea.Cmd) {
	if m.SelectedIdx < 0 || m.SelectedIdx >= len(m.Repos) {
		return m, nil
	}
	switch key {
	case KeyEnter:
		m.LogOffset = 0
		m.LogView = views.NewLogView()
		m.LogView.Width = m.Width
		m.LogView.Height = m.Height
		lines, _ := process.ReadFullLog(m.Repos[m.SelectedIdx].Path)
		m.LogView.SetLines(lines)
		m.pushView(ViewLogs, "Logs")
		return m, nil
	case KeyE:
		repo := m.Repos[m.SelectedIdx]
		if repo.Config != nil {
			m.ConfigEdit = views.NewConfigEditor(repo.Config)
			m.ConfigEdit.Height = m.Height
			m.pushView(ViewConfigEditor, "Config")
		} else {
			m.Notify.Show("No .ralphrc found", 3*time.Second)
		}
		return m, nil
	case KeyShiftS:
		return m.startLoop(m.SelectedIdx)
	case KeyShiftX:
		return m.stopLoop(m.SelectedIdx)
	case KeyShiftP:
		return m.togglePause(m.SelectedIdx)
	}
	return m, nil
}

func (m Model) handleLogKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case KeyJ:
		m.LogView.ScrollDown()
	case KeyK:
		m.LogView.ScrollUp()
	case KeyShiftG:
		m.LogView.ScrollToEnd()
	case KeyG:
		m.LogView.ScrollToStart()
	case KeyF:
		m.LogView.ToggleFollow()
	case KeyCtrlU:
		m.LogView.PageUp()
	case KeyCtrlD:
		m.LogView.PageDown()
	}
	return m, nil
}

func (m Model) handleConfigKey(key string) (tea.Model, tea.Cmd) {
	if m.ConfigEdit == nil {
		return m, nil
	}
	switch key {
	case KeyJ:
		m.ConfigEdit.MoveDown()
	case KeyK:
		m.ConfigEdit.MoveUp()
	case KeyEnter:
		m.ConfigEdit.StartEdit()
	case KeyW:
		if err := m.ConfigEdit.Save(); err != nil {
			m.Notify.Show(fmt.Sprintf("Save error: %v", err), 3*time.Second)
		} else {
			m.Notify.Show("Config saved", 2*time.Second)
		}
	}
	return m, nil
}

func (m Model) handleConfigEditInput(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case KeyEnter:
		m.ConfigEdit.ConfirmEdit()
	case KeyEsc:
		m.ConfigEdit.CancelEdit()
	case "backspace":
		m.ConfigEdit.Backspace()
	default:
		if len(msg.Runes) == 1 {
			m.ConfigEdit.TypeChar(msg.Runes[0])
		}
	}
	return m, nil
}

func (m Model) handleCommandInput(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case KeyEnter:
		cmd := ParseCommand(m.CommandBuf)
		m.InputMode = ModeNormal
		m.CommandBuf = ""
		return m.execCommand(cmd)
	case KeyEsc:
		m.InputMode = ModeNormal
		m.CommandBuf = ""
		return m, nil
	case "backspace":
		if len(m.CommandBuf) > 0 {
			m.CommandBuf = m.CommandBuf[:len(m.CommandBuf)-1]
		}
		return m, nil
	default:
		if len(msg.Runes) == 1 {
			m.CommandBuf += string(msg.Runes[0])
		}
		return m, nil
	}
}

func (m Model) handleFilterInput(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case KeyEnter, KeyEsc:
		m.InputMode = ModeNormal
		if key == KeyEsc {
			m.Filter.Clear()
			m.Table.SetFilter("")
		}
		return m, nil
	case "backspace":
		m.Filter.Backspace()
		m.Table.SetFilter(m.Filter.Text)
		return m, nil
	default:
		if len(msg.Runes) == 1 {
			m.Filter.Type(msg.Runes[0])
			m.Table.SetFilter(m.Filter.Text)
		}
		return m, nil
	}
}

func (m Model) execCommand(cmd Command) (tea.Model, tea.Cmd) {
	switch cmd.Name {
	case "quit", "q":
		m.ProcMgr.StopAll()
		return m, tea.Quit
	case "scan":
		return m, m.scanRepos()
	case "start":
		if len(cmd.Args) > 0 {
			idx := m.findRepoByName(cmd.Args[0])
			if idx >= 0 {
				return m.startLoop(idx)
			}
			m.Notify.Show(fmt.Sprintf("Repo not found: %s", cmd.Args[0]), 3*time.Second)
		}
	case "stop":
		if len(cmd.Args) > 0 {
			idx := m.findRepoByName(cmd.Args[0])
			if idx >= 0 {
				return m.stopLoop(idx)
			}
			m.Notify.Show(fmt.Sprintf("Repo not found: %s", cmd.Args[0]), 3*time.Second)
		}
	case "stopall":
		m.ProcMgr.StopAll()
		m.Notify.Show("All loops stopped", 3*time.Second)
	default:
		m.Notify.Show(fmt.Sprintf("Unknown command: %s", cmd.Name), 3*time.Second)
	}
	return m, nil
}

// Process management helpers

func (m Model) startSelectedLoop() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 {
		return m.startLoop(idx)
	}
	return m, nil
}

func (m Model) stopSelectedLoop() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 {
		return m.stopLoop(idx)
	}
	return m, nil
}

func (m Model) togglePauseSelected() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 {
		return m.togglePause(idx)
	}
	return m, nil
}

func (m Model) startLoop(idx int) (tea.Model, tea.Cmd) {
	repo := m.Repos[idx]
	if err := m.ProcMgr.Start(repo.Path); err != nil {
		m.Notify.Show(fmt.Sprintf("Start error: %v", err), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Started loop: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

func (m Model) stopLoop(idx int) (tea.Model, tea.Cmd) {
	repo := m.Repos[idx]
	if err := m.ProcMgr.Stop(repo.Path); err != nil {
		m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Stopped loop: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

func (m Model) togglePause(idx int) (tea.Model, tea.Cmd) {
	repo := m.Repos[idx]
	paused, err := m.ProcMgr.TogglePause(repo.Path)
	if err != nil {
		m.Notify.Show(fmt.Sprintf("Pause error: %v", err), 3*time.Second)
	} else if paused {
		m.Notify.Show(fmt.Sprintf("Paused: %s", repo.Name), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Resumed: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

// Navigation helpers

func (m *Model) pushView(v ViewMode, name string) {
	m.ViewStack = append(m.ViewStack, m.CurrentView)
	m.CurrentView = v
	m.Breadcrumb.Push(name)
}

func (m Model) popView() (tea.Model, tea.Cmd) {
	if len(m.ViewStack) == 0 {
		// At root — quit on Esc
		return m, nil
	}
	m.CurrentView = m.ViewStack[len(m.ViewStack)-1]
	m.ViewStack = m.ViewStack[:len(m.ViewStack)-1]
	m.Breadcrumb.Pop()
	return m, nil
}

func (m *Model) refreshAllRepos() {
	for _, r := range m.Repos {
		model.RefreshRepo(r)
	}
}

func (m *Model) updateTable() {
	rows := views.ReposToRows(m.Repos)
	m.Table.SetRows(rows)
	m.StatusBar.RepoCount = len(m.Repos)
	m.StatusBar.RunningCount = len(m.ProcMgr.RunningPaths())
	m.StatusBar.LastRefresh = m.LastRefresh
}

func (m Model) findRepoByName(name string) int {
	for i, r := range m.Repos {
		if r.Name == name || filepath.Base(r.Path) == name {
			return i
		}
	}
	return -1
}

// View renders the TUI.
func (m Model) View() string {
	var b strings.Builder

	// Title bar
	b.WriteString(styles.TitleStyle.Render(" 👓 ralphglasses "))
	b.WriteString("  ")
	b.WriteString(m.Breadcrumb.View())
	b.WriteString("\n\n")

	// Main content
	switch m.CurrentView {
	case ViewOverview:
		b.WriteString(m.Table.View())
	case ViewRepoDetail:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			b.WriteString(views.RenderRepoDetail(m.Repos[m.SelectedIdx], m.Width))
		}
	case ViewLogs:
		b.WriteString(m.LogView.View())
	case ViewConfigEditor:
		if m.ConfigEdit != nil {
			b.WriteString(m.ConfigEdit.View())
		}
	case ViewHelp:
		b.WriteString(views.RenderHelp(m.Width, m.Height))
	}

	// Notification overlay
	if notif := m.Notify.View(); notif != "" {
		b.WriteString("\n")
		b.WriteString(notif)
	}

	b.WriteString("\n")

	// Input line
	switch m.InputMode {
	case ModeCommand:
		b.WriteString(styles.CommandStyle.Render(":"))
		b.WriteString(m.CommandBuf)
		b.WriteString(styles.CommandStyle.Render("█"))
	case ModeFilter:
		b.WriteString(styles.CommandStyle.Render("/"))
		b.WriteString(m.Filter.Text)
		b.WriteString(styles.CommandStyle.Render("█"))
	default:
		// Mode indicator in status bar
		m.StatusBar.Mode = "NORMAL"
		m.StatusBar.Filter = m.Filter.Text
		b.WriteString(m.StatusBar.View())
	}

	return b.String()
}
