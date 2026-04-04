// D1 Firstboot Wizard — interactive TUI for thin client first-boot setup.
// Runs as a standalone BubbleTea program before the main TUI.
package views

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// FirstBootConfig holds the collected configuration from the wizard.
type FirstBootConfig struct {
	Hostname    string            `json:"hostname"`
	APIKeys     map[string]string `json:"api_keys"`
	Autonomy    int               `json:"autonomy_level"`
	FleetURL    string            `json:"fleet_coordinator_url,omitempty"`
}

// FirstBootModel is a standalone BubbleTea model for first-boot setup.
type FirstBootModel struct {
	step      int
	maxSteps  int
	hostname  string
	apiKeys   map[string]string
	autonomy  int
	fleetURL  string
	confirmed bool
	done      bool
	err       error
	cursor    int // for autonomy selector
	input     string // current text input buffer
	inputKey  string // which key we're editing
	width     int
	height    int
	configDir string // where to write config (default ~/.ralphglasses)
}

// NewFirstBootModel creates the wizard model.
func NewFirstBootModel(configDir string) FirstBootModel {
	if configDir == "" {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".ralphglasses")
	}
	return FirstBootModel{
		maxSteps:  4,
		hostname:  "ralph-01",
		apiKeys:   map[string]string{"anthropic": "", "google": "", "openai": ""},
		configDir: configDir,
		input:     "ralph-01",
	}
}

func (m FirstBootModel) Init() tea.Cmd { return nil }

func (m FirstBootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.step == 0 { // only quit from first step
				return m, tea.Quit
			}
		case "esc":
			if m.step > 0 {
				m.step--
				m.loadStepInput()
			}
		case "enter":
			m.saveStepInput()
			if m.step >= m.maxSteps {
				// Final confirmation
				if err := m.writeConfig(); err != nil {
					m.err = err
				} else {
					m.done = true
				}
				return m, tea.Quit
			}
			m.step++
			m.loadStepInput()
		case "up":
			if m.step == 2 && m.cursor > 0 { // autonomy selector
				m.cursor--
				m.autonomy = m.cursor
			}
		case "down":
			if m.step == 2 && m.cursor < 3 {
				m.cursor++
				m.autonomy = m.cursor
			}
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.input += msg.String()
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m FirstBootModel) View() tea.View {
	if m.done {
		return tea.NewView("\n  Setup complete! Configuration saved.\n  Starting ralphglasses...\n\n")
	}
	if m.err != nil {
		return tea.NewView(fmt.Sprintf("\n  Error: %v\n\n  Press Ctrl+C to exit.\n", m.err))
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  ╔══════════════════════════════════════╗\n")
	sb.WriteString("  ║     ralphglasses first-boot setup    ║\n")
	sb.WriteString("  ╚══════════════════════════════════════╝\n\n")

	// Progress indicator
	for i := 0; i <= m.maxSteps; i++ {
		if i == m.step {
			sb.WriteString("  ● ")
		} else if i < m.step {
			sb.WriteString("  ✓ ")
		} else {
			sb.WriteString("  ○ ")
		}
	}
	sb.WriteString(fmt.Sprintf("  (%d/%d)\n\n", m.step+1, m.maxSteps+1))

	switch m.step {
	case 0:
		sb.WriteString("  Hostname\n")
		sb.WriteString("  ────────\n")
		sb.WriteString(fmt.Sprintf("  > %s█\n\n", m.input))
		sb.WriteString("  Enter hostname for this thin client.\n")

	case 1:
		sb.WriteString("  API Keys (optional)\n")
		sb.WriteString("  ───────────────────\n")
		keys := []string{"anthropic", "google", "openai"}
		for _, k := range keys {
			val := m.apiKeys[k]
			if k == m.inputKey {
				sb.WriteString(fmt.Sprintf("  > %s: %s█\n", k, m.input))
			} else if val != "" {
				sb.WriteString(fmt.Sprintf("    %s: %s\n", k, maskKey(val)))
			} else {
				sb.WriteString(fmt.Sprintf("    %s: (not set)\n", k))
			}
		}
		sb.WriteString("\n  Enter API keys. Press Enter to move to next.\n")

	case 2:
		sb.WriteString("  Autonomy Level\n")
		sb.WriteString("  ──────────────\n")
		levels := []struct {
			name string
			desc string
		}{
			{"0 — Observe", "Record decisions but don't execute"},
			{"1 — Auto-Recover", "Auto-restart on transient errors"},
			{"2 — Auto-Optimize", "Adjust budgets, providers, rate limits"},
			{"3 — Full Autonomy", "Self-modify config, launch from roadmap"},
		}
		for i, l := range levels {
			marker := "  "
			if i == m.cursor {
				marker = "> "
			}
			sb.WriteString(fmt.Sprintf("  %s%s\n", marker, l.name))
			sb.WriteString(fmt.Sprintf("      %s\n", l.desc))
		}
		sb.WriteString("\n  Use ↑/↓ to select, Enter to confirm.\n")

	case 3:
		sb.WriteString("  Fleet Coordinator (optional)\n")
		sb.WriteString("  ────────────────────────────\n")
		sb.WriteString(fmt.Sprintf("  > %s█\n\n", m.input))
		sb.WriteString("  Enter coordinator URL to join a fleet (e.g. 10.0.0.5:9473).\n")
		sb.WriteString("  Leave empty for standalone mode.\n")

	case 4:
		sb.WriteString("  Summary\n")
		sb.WriteString("  ───────\n")
		sb.WriteString(fmt.Sprintf("  Hostname:  %s\n", m.hostname))
		for k, v := range m.apiKeys {
			if v != "" {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", k, maskKey(v)))
			}
		}
		sb.WriteString(fmt.Sprintf("  Autonomy:  Level %d\n", m.autonomy))
		if m.fleetURL != "" {
			sb.WriteString(fmt.Sprintf("  Fleet:     %s\n", m.fleetURL))
		}
		sb.WriteString("\n  Press Enter to save and start.\n")
	}

	sb.WriteString("\n  [Enter] next  [Esc] back  [Ctrl+C] quit\n")
	return tea.NewView(sb.String())
}

func (m *FirstBootModel) saveStepInput() {
	switch m.step {
	case 0:
		m.hostname = m.input
	case 1:
		if m.inputKey != "" {
			m.apiKeys[m.inputKey] = m.input
		}
	case 3:
		m.fleetURL = m.input
	}
}

func (m *FirstBootModel) loadStepInput() {
	switch m.step {
	case 0:
		m.input = m.hostname
	case 1:
		// Cycle through API keys
		keys := []string{"anthropic", "google", "openai"}
		m.inputKey = keys[0]
		m.input = m.apiKeys[m.inputKey]
	case 2:
		m.cursor = m.autonomy
	case 3:
		m.input = m.fleetURL
	case 4:
		m.input = ""
	}
}

func (m *FirstBootModel) writeConfig() error {
	cfg := FirstBootConfig{
		Hostname: m.hostname,
		APIKeys:  m.apiKeys,
		Autonomy: m.autonomy,
		FleetURL: m.fleetURL,
	}

	if err := os.MkdirAll(m.configDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(m.configDir, "config.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return err
	}

	// Touch marker file
	markerPath := filepath.Join(m.configDir, ".firstboot-done")
	return os.WriteFile(markerPath, []byte(m.hostname+"\n"), 0644)
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
