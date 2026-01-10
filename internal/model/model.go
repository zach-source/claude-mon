package model

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// SocketMsg is sent when data is received from the socket
type SocketMsg struct {
	Payload []byte
}

// Change represents a single file change from Claude
type Change struct {
	Timestamp time.Time
	FilePath  string
	ToolName  string
	OldString string
	NewString string
	LineNum   int
	LineCount int
}

// HookPayload matches the JSON structure from the Claude hook
type HookPayload struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		Content   string `json:"content"`
	} `json:"tool_input"`
	Parameters struct {
		FilePath  string `json:"file_path"`
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"parameters"`
}

// Pane represents which pane is active
type Pane int

const (
	PaneHistory Pane = iota
	PaneDiff
)

// Model is the Bubbletea model
type Model struct {
	socketPath    string
	width         int
	height        int
	activePane    Pane
	changes       []Change
	selectedIndex int
	diffViewport  viewport.Model
	showHelp      bool
	ready         bool
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57"))

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	addedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	removedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))

	activeBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// New creates a new Model
func New(socketPath string) Model {
	return Model{
		socketPath: socketPath,
		changes:    []Change{},
		activePane: PaneHistory,
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Initialize viewport for diff
		headerHeight := 3
		footerHeight := 2
		m.diffViewport = viewport.New(m.width/2-4, m.height-headerHeight-footerHeight-2)
		m.diffViewport.SetContent(m.renderDiff())

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "?":
			m.showHelp = true

		case "j", "down":
			if m.activePane == PaneHistory && len(m.changes) > 0 {
				if m.selectedIndex < len(m.changes)-1 {
					m.selectedIndex++
					m.diffViewport.SetContent(m.renderDiff())
				}
			} else if m.activePane == PaneDiff {
				m.diffViewport.LineDown(1)
			}

		case "k", "up":
			if m.activePane == PaneHistory && len(m.changes) > 0 {
				if m.selectedIndex > 0 {
					m.selectedIndex--
					m.diffViewport.SetContent(m.renderDiff())
				}
			} else if m.activePane == PaneDiff {
				m.diffViewport.LineUp(1)
			}

		case "tab":
			if m.activePane == PaneHistory {
				m.activePane = PaneDiff
			} else {
				m.activePane = PaneHistory
			}

		case "c":
			m.changes = []Change{}
			m.selectedIndex = 0
			m.diffViewport.SetContent("")

		case "ctrl+g":
			if len(m.changes) > 0 {
				change := m.changes[m.selectedIndex]
				cmd := exec.Command("nvim", fmt.Sprintf("+%d", change.LineNum), change.FilePath)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return nil
				})
			}

		case "ctrl+o":
			if len(m.changes) > 0 {
				change := m.changes[m.selectedIndex]
				cmd := exec.Command("nvim", change.FilePath)
				return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return nil
				})
			}
		}

	case SocketMsg:
		change := parsePayload(msg.Payload)
		if change != nil {
			// Prepend new change to list
			m.changes = append([]Change{*change}, m.changes...)
			m.selectedIndex = 0
			m.diffViewport.SetContent(m.renderDiff())
		}
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	if m.showHelp {
		return m.renderHelp()
	}

	// Calculate pane widths
	leftWidth := m.width / 3
	rightWidth := m.width - leftWidth - 3

	// Render header
	header := titleStyle.Render("Claude Follow") + "  " +
		dimStyle.Render(time.Now().Format("15:04:05"))
	header = lipgloss.PlaceHorizontal(m.width, lipgloss.Left, header)

	// Render history pane
	historyContent := m.renderHistory()
	historyBox := borderStyle
	if m.activePane == PaneHistory {
		historyBox = activeBorderStyle
	}
	historyPane := historyBox.
		Width(leftWidth).
		Height(m.height - 5).
		Render(historyContent)

	// Render diff pane
	diffContent := m.diffViewport.View()
	diffBox := borderStyle
	if m.activePane == PaneDiff {
		diffBox = activeBorderStyle
	}
	diffPane := diffBox.
		Width(rightWidth).
		Height(m.height - 5).
		Render(diffContent)

	// Combine panes
	content := lipgloss.JoinHorizontal(lipgloss.Top, historyPane, diffPane)

	// Render status bar
	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
}

func (m Model) renderHistory() string {
	if len(m.changes) == 0 {
		return dimStyle.Render("No changes yet...\nWaiting for Claude edits")
	}

	var sb strings.Builder
	sb.WriteString(dimStyle.Render(fmt.Sprintf("History (%d)\n", len(m.changes))))
	sb.WriteString(dimStyle.Render(strings.Repeat("─", 20)) + "\n")

	for i, change := range m.changes {
		line := fmt.Sprintf("%s %s %s",
			change.Timestamp.Format("15:04"),
			change.ToolName,
			truncatePath(change.FilePath, 20))

		if i == m.selectedIndex {
			sb.WriteString(selectedStyle.Render("> "+line) + "\n")
		} else {
			sb.WriteString(normalStyle.Render("  "+line) + "\n")
		}
	}

	return sb.String()
}

func (m Model) renderDiff() string {
	if len(m.changes) == 0 {
		return dimStyle.Render("Select a change to view diff")
	}

	change := m.changes[m.selectedIndex]
	var sb strings.Builder

	// Header
	sb.WriteString(titleStyle.Render(change.FilePath))
	if change.LineNum > 0 {
		sb.WriteString(dimStyle.Render(fmt.Sprintf(":%d", change.LineNum)))
	}
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(strings.Repeat("─", 30)) + "\n\n")

	// Generate diff
	if change.ToolName == "Write" {
		// For Write operations, show the new content (or truncated version)
		content := change.NewString
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		sb.WriteString(addedStyle.Render("+ New file created\n\n"))
		sb.WriteString(normalStyle.Render(content))
	} else if change.OldString != "" || change.NewString != "" {
		// For Edit operations, show diff
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(change.OldString, change.NewString, false)

		for _, diff := range diffs {
			text := diff.Text
			switch diff.Type {
			case diffmatchpatch.DiffInsert:
				for _, line := range strings.Split(text, "\n") {
					if line != "" {
						sb.WriteString(addedStyle.Render("+ "+line) + "\n")
					}
				}
			case diffmatchpatch.DiffDelete:
				for _, line := range strings.Split(text, "\n") {
					if line != "" {
						sb.WriteString(removedStyle.Render("- "+line) + "\n")
					}
				}
			case diffmatchpatch.DiffEqual:
				// Show context (first and last few lines)
				lines := strings.Split(text, "\n")
				if len(lines) > 6 {
					for _, line := range lines[:3] {
						sb.WriteString(dimStyle.Render("  "+line) + "\n")
					}
					sb.WriteString(dimStyle.Render("  ...") + "\n")
					for _, line := range lines[len(lines)-3:] {
						sb.WriteString(dimStyle.Render("  "+line) + "\n")
					}
				} else {
					for _, line := range lines {
						sb.WriteString(dimStyle.Render("  "+line) + "\n")
					}
				}
			}
		}
	} else {
		sb.WriteString(dimStyle.Render("No diff content available"))
	}

	return sb.String()
}

func (m Model) renderStatus() string {
	keys := "j/k:move  Tab:switch  Ctrl+G:nvim  c:clear  ?:help  q:quit"
	return statusStyle.Render(keys)
}

func (m Model) renderHelp() string {
	help := `
  Claude Follow TUI - Help

  Navigation:
    j/k, ↑/↓     Move through history
    Tab          Switch between panes
    Enter        Expand/collapse

  Actions:
    Ctrl+G       Open file in nvim at line
    Ctrl+O       Open file in nvim
    c            Clear history
    q            Quit

  Press any key to close help
`
	return helpStyle.Render(help)
}

func parsePayload(data []byte) *Change {
	var payload HookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}

	// Extract file path (try multiple locations)
	filePath := payload.ToolInput.FilePath
	if filePath == "" {
		filePath = payload.ToolInput.Path
	}
	if filePath == "" {
		filePath = payload.Parameters.FilePath
	}
	if filePath == "" {
		filePath = payload.Parameters.Path
	}
	if filePath == "" {
		return nil
	}

	// Extract old/new strings
	oldStr := payload.ToolInput.OldString
	if oldStr == "" {
		oldStr = payload.Parameters.OldString
	}

	newStr := payload.ToolInput.NewString
	if newStr == "" {
		newStr = payload.Parameters.NewString
	}
	if newStr == "" {
		newStr = payload.ToolInput.Content
	}

	return &Change{
		Timestamp: time.Now(),
		FilePath:  filePath,
		ToolName:  payload.ToolName,
		OldString: oldStr,
		NewString: newStr,
		LineNum:   1, // TODO: extract from payload
		LineCount: 1,
	}
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Show last part of path
	parts := strings.Split(path, "/")
	result := parts[len(parts)-1]
	if len(result) > maxLen {
		return "..." + result[len(result)-maxLen+3:]
	}
	return ".../" + result
}
