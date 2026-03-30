package views

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// CoordinationNode represents a session node in the dependency graph.
type CoordinationNode struct {
	ID           string
	Name         string
	Provider     string
	Status       string // running, waiting, error, completed
	Cost         float64
	RepoPath     string
	Task         string
	DependsOn    []string // IDs of nodes this depends on
	ClaimedTasks []string // tasks claimed by this session
}

// CoordinationRefreshMsg signals the coordination view to refresh.
type CoordinationRefreshMsg struct{}

// CoordinationSelectMsg signals that a node was selected.
type CoordinationSelectMsg struct {
	NodeID string
}

// CoordinationModel implements tea.Model for the cross-session dependency graph.
type CoordinationModel struct {
	nodes  []CoordinationNode
	cursor int
	width  int
	height int
}

// NewCoordination creates a new CoordinationModel.
func NewCoordination() CoordinationModel {
	return CoordinationModel{}
}

// SetNodes updates the node list displayed in the graph.
func (m *CoordinationModel) SetNodes(nodes []CoordinationNode) {
	m.nodes = nodes
	if m.cursor >= len(nodes) {
		m.cursor = max(0, len(nodes)-1)
	}
}

// Init implements tea.Model.
func (m CoordinationModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m CoordinationModel) Update(msg tea.Msg) (CoordinationModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, func() tea.Msg { return CoordinationRefreshMsg{} }
		case "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.nodes) {
				nid := m.nodes[m.cursor].ID
				return m, func() tea.Msg { return CoordinationSelectMsg{NodeID: nid} }
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View implements tea.Model.
func (m CoordinationModel) View() tea.View {
	var b strings.Builder

	// Title
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Coordination Dashboard", styles.IconFleet)))
	b.WriteString("\n\n")

	// Utilization summary
	running, waiting, errored, completed := m.countByStatus()
	totalCost := m.totalCost()
	contentions := m.findContentions()

	statBoxes := []string{
		styles.StatBox.Render(fmt.Sprintf("%s NODES\n  %d total", styles.IconSession, len(m.nodes))),
		styles.StatBox.Render(fmt.Sprintf("%s RUNNING\n  %d", styles.IconRunning, running)),
		styles.StatBox.Render(fmt.Sprintf("%s WAITING\n  %d", styles.IconIdle, waiting)),
		styles.StatBox.Render(fmt.Sprintf("%s ERRORS\n  %d", styles.IconErrored, errored)),
		styles.StatBox.Render(fmt.Sprintf("%s DONE\n  %d", styles.IconCompleted, completed)),
		styles.StatBox.Render(fmt.Sprintf("%s COST\n  $%.2f", styles.IconBudget, totalCost)),
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, statBoxes...))
	b.WriteString("\n\n")

	// Resource contention warnings
	if len(contentions) > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Resource Contention", styles.IconAlert)))
		b.WriteString("\n")
		for repo, ids := range contentions {
			b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("  %s %s: %s", styles.IconWarning, repo, strings.Join(ids, ", "))))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Dependency graph
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Dependency Graph", styles.IconFleet)))
	b.WriteString("\n")

	if len(m.nodes) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No sessions"))
		b.WriteString("\n")
	} else {
		graph := m.renderGraph()
		b.WriteString(graph)
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  r:refresh  q:back  j/k:move  Enter:select"))

	return tea.NewView(b.String())
}

// renderGraph builds an ASCII dependency graph with box-drawing characters.
func (m CoordinationModel) renderGraph() string {
	var b strings.Builder

	// Build lookup for which nodes have dependents (children pointing to them).
	nodeIndex := make(map[string]int, len(m.nodes))
	for i, n := range m.nodes {
		nodeIndex[n.ID] = i
	}

	// Determine root nodes (no dependencies) and dependent nodes.
	roots := make([]int, 0)
	children := make(map[string][]int) // parentID -> child indices
	for i, n := range m.nodes {
		if len(n.DependsOn) == 0 {
			roots = append(roots, i)
		}
		for _, dep := range n.DependsOn {
			children[dep] = append(children[dep], i)
		}
	}

	// If no clear root structure, show all as flat list with dependency arrows.
	rendered := make(map[int]bool)

	// Render roots first, then their children with indentation.
	for _, ri := range roots {
		m.renderNode(&b, ri, 0, nodeIndex, children, rendered)
	}

	// Render any remaining unvisited nodes (cycles or orphans).
	for i := range m.nodes {
		if !rendered[i] {
			m.renderNode(&b, i, 0, nodeIndex, children, rendered)
		}
	}

	return b.String()
}

// renderNode renders a single node and its dependents recursively.
func (m CoordinationModel) renderNode(b *strings.Builder, idx, depth int, nodeIndex map[string]int, children map[string][]int, rendered map[int]bool) {
	if rendered[idx] {
		return
	}
	rendered[idx] = true

	n := m.nodes[idx]

	// Build indentation with box-drawing connectors.
	indent := ""
	if depth > 0 {
		indent = strings.Repeat("│   ", depth-1) + "├── "
	}

	// Selection marker.
	marker := "  "
	if idx == m.cursor {
		marker = styles.SelectedStyle.Render("> ")
	}

	// Status styling.
	statusStr := m.styledStatus(n.Status)

	// Provider styling.
	providerStr := styles.ProviderStyle(n.Provider).Render(n.Provider)

	// Node name (or ID if no name).
	name := n.Name
	if name == "" {
		name = n.ID
	}
	if len(name) > 20 {
		name = name[:17] + "..."
	}

	// Cost.
	costStr := fmt.Sprintf("$%.4f", n.Cost)

	// Task (truncated).
	task := n.Task
	if len(task) > 30 {
		task = task[:27] + "..."
	}

	line := fmt.Sprintf("%s%s%s %s %s %s", marker, indent, name, providerStr, statusStr, costStr)
	if task != "" {
		line += fmt.Sprintf("  %s", task)
	}
	b.WriteString(line)
	b.WriteString("\n")

	// Render dependency arrows for this node's claimed tasks.
	if len(n.ClaimedTasks) > 0 && depth == 0 {
		taskIndent := "  " + strings.Repeat("│   ", depth)
		b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("%s└─ claims: %s", taskIndent, strings.Join(n.ClaimedTasks, ", "))))
		b.WriteString("\n")
	}

	// Render children.
	kids := children[n.ID]
	for _, ci := range kids {
		m.renderNode(b, ci, depth+1, nodeIndex, children, rendered)
	}
}

// styledStatus returns a status string with appropriate color.
func (m CoordinationModel) styledStatus(status string) string {
	switch status {
	case "running":
		return styles.StatusRunning.Render("running")
	case "waiting":
		return lipgloss.NewStyle().Foreground(styles.ColorYellow).Render("waiting")
	case "error":
		return styles.StatusFailed.Render("error")
	case "completed":
		return styles.StatusCompleted.Render("completed")
	default:
		return styles.InfoStyle.Render(status)
	}
}

// countByStatus returns running, waiting, errored, and completed counts.
func (m CoordinationModel) countByStatus() (running, waiting, errored, completed int) {
	for _, n := range m.nodes {
		switch n.Status {
		case "running":
			running++
		case "waiting":
			waiting++
		case "error":
			errored++
		case "completed":
			completed++
		}
	}
	return
}

// totalCost sums Cost across all nodes.
func (m CoordinationModel) totalCost() float64 {
	var total float64
	for _, n := range m.nodes {
		total += n.Cost
	}
	return total
}

// findContentions identifies repos with multiple active sessions competing for them.
func (m CoordinationModel) findContentions() map[string][]string {
	repoSessions := make(map[string][]string)
	for _, n := range m.nodes {
		if n.RepoPath == "" {
			continue
		}
		if n.Status == "running" || n.Status == "waiting" {
			repoSessions[n.RepoPath] = append(repoSessions[n.RepoPath], n.ID)
		}
	}

	contentions := make(map[string][]string)
	// Use sorted keys for deterministic output.
	repos := make([]string, 0, len(repoSessions))
	for repo := range repoSessions {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	for _, repo := range repos {
		ids := repoSessions[repo]
		if len(ids) > 1 {
			contentions[repo] = ids
		}
	}
	return contentions
}

// LayoutGraph computes a simple topological ordering for the nodes,
// returning indices grouped by depth level. This is exported for testing.
func LayoutGraph(nodes []CoordinationNode) [][]int {
	nodeIndex := make(map[string]int, len(nodes))
	for i, n := range nodes {
		nodeIndex[n.ID] = i
	}

	// Compute in-degree for topological sort.
	inDegree := make(map[int]int, len(nodes))
	children := make(map[int][]int)
	for i, n := range nodes {
		for _, dep := range n.DependsOn {
			if pi, ok := nodeIndex[dep]; ok {
				children[pi] = append(children[pi], i)
				inDegree[i]++
			}
		}
	}

	// BFS-based level assignment.
	var levels [][]int
	visited := make(map[int]bool)

	// Start with roots (in-degree 0).
	queue := make([]int, 0)
	for i := range nodes {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	for len(queue) > 0 {
		levels = append(levels, queue)
		var next []int
		for _, idx := range queue {
			visited[idx] = true
			for _, ci := range children[idx] {
				inDegree[ci]--
				if inDegree[ci] == 0 {
					next = append(next, ci)
				}
			}
		}
		queue = next
	}

	// Add any unvisited nodes (cycles) as a final level.
	var remaining []int
	for i := range nodes {
		if !visited[i] {
			remaining = append(remaining, i)
		}
	}
	if len(remaining) > 0 {
		levels = append(levels, remaining)
	}

	return levels
}
