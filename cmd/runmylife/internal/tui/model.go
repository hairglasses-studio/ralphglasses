package tui

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab indices
const (
	TabToday    = 0
	TabFinances = 1
	TabWellness = 2
	TabADHD     = 3
	TabFleet    = 4
)

var allTabNames = []string{"Today", "Finances", "Wellness", "ADHD", "Fleet"}

// tickMsg triggers a data refresh.
type tickMsg time.Time

// Model is the main BubbleTea model for the runmylife TUI.
type Model struct {
	db        *sql.DB
	ralphDB   *sql.DB
	activeTab int
	width     int
	height    int
	refresh   time.Duration

	// Tab data (populated on refresh)
	todayData    todayData
	financeData  financeData
	wellnessData wellnessData
	adhdData     adhdData
	fleetData    fleetData

	// Overlay & notifications
	overlay overlay
	toasts  []toast
}

// NewModel creates the TUI model with a read-only DB connection.
// ralphDB is optional (nil if fleet monitoring not configured).
func NewModel(db *sql.DB, ralphDB *sql.DB, refresh time.Duration) Model {
	return Model{
		db:      db,
		ralphDB: ralphDB,
		refresh: refresh,
	}
}

func (m Model) maxTab() int {
	if m.ralphDB != nil {
		return TabFleet
	}
	return TabADHD
}

func (m Model) tabNames() []string {
	return allTabNames[:m.maxTab()+1]
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadData,
		tickCmd(m.refresh),
	)
}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Overlay captures all input when active
	if m.overlay.active {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			_ = keyMsg // overlay handles it
			cmd := m.overlay.update(msg)
			return m, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "left", "h":
			if m.activeTab > 0 {
				m.activeTab--
			}
		case "right", "l":
			if m.activeTab < m.maxTab() {
				m.activeTab++
			}
		case "1":
			m.activeTab = TabToday
		case "2":
			m.activeTab = TabFinances
		case "3":
			m.activeTab = TabWellness
		case "4":
			m.activeTab = TabADHD
		case "5":
			if m.ralphDB != nil {
				m.activeTab = TabFleet
			}
		case "r":
			return m, m.loadData
		case "m":
			title, form, onSubmit := openMoodForm(m.db)
			cmd := m.overlay.open(title, form, onSubmit)
			return m, cmd
		case "e":
			title, form, onSubmit := openExpenseForm(m.db)
			cmd := m.overlay.open(title, form, onSubmit)
			return m, cmd
		case "t":
			title, form, onSubmit := openTaskForm(m.db)
			cmd := m.overlay.open(title, form, onSubmit)
			return m, cmd
		case "f":
			title, form, onSubmit := openFocusForm(m.db)
			cmd := m.overlay.open(title, form, onSubmit)
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		return m, tea.Batch(m.loadData, tickCmd(m.refresh))

	case dataLoadedMsg:
		m.todayData = msg.today
		m.financeData = msg.finance
		m.wellnessData = msg.wellness
		m.adhdData = msg.adhd
		m.fleetData = msg.fleet

	case formSubmittedMsg:
		m.toasts = append(m.toasts, msg.toast)
		return m, tea.Batch(m.loadData, toastTickCmd())

	case toastExpiredMsg:
		now := time.Now()
		var active []toast
		for _, t := range m.toasts {
			if t.expiresAt.After(now) {
				active = append(active, t)
			}
		}
		m.toasts = active
		if len(m.toasts) > 0 {
			return m, toastTickCmd()
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	// Overlay takes over the full screen
	if m.overlay.active {
		return m.overlay.view(m.width, m.height)
	}

	var b strings.Builder

	// Toast at top-right
	if len(m.toasts) > 0 {
		b.WriteString(m.toasts[len(m.toasts)-1].render(m.width))
		b.WriteString("\n")
	}

	// Title
	b.WriteString(titleStyle.Render("runmylife"))
	b.WriteString("\n")

	// Tab bar
	var tabs []string
	for i, name := range m.tabNames() {
		if i == m.activeTab {
			tabs = append(tabs, tabActiveStyle.Render(name))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(name))
		}
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tabs...))
	b.WriteString("\n\n")

	// Active tab content
	contentWidth := m.width - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	switch m.activeTab {
	case TabToday:
		b.WriteString(renderToday(m.todayData, contentWidth))
	case TabFinances:
		b.WriteString(renderFinances(m.financeData, contentWidth))
	case TabWellness:
		b.WriteString(renderWellness(m.wellnessData, contentWidth))
	case TabADHD:
		b.WriteString(renderADHD(m.adhdData, contentWidth))
	case TabFleet:
		b.WriteString(renderFleet(m.fleetData, contentWidth))
	}

	// Status bar
	b.WriteString("\n")
	tabHint := "[1-4]"
	if m.ralphDB != nil {
		tabHint = "[1-5]"
	}
	b.WriteString(statusBarStyle.Render(
		fmt.Sprintf("  %s tabs  [h/l] nav  [r] refresh  [m]ood [e]xpense [t]ask [f]ocus  [q] quit  |  %s",
			tabHint, time.Now().Format("3:04 PM")),
	))

	return b.String()
}

// dataLoadedMsg carries refreshed data from the DB.
type dataLoadedMsg struct {
	today    todayData
	finance  financeData
	wellness wellnessData
	adhd     adhdData
	fleet    fleetData
}

func (m Model) loadData() tea.Msg {
	return dataLoadedMsg{
		today:    loadTodayData(m.db),
		finance:  loadFinanceData(m.db),
		wellness: loadWellnessData(m.db),
		adhd:     loadADHDData(m.db),
		fleet:    loadFleetData(m.ralphDB),
	}
}
