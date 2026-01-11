package model

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ztaylor/claude-follow-tui/internal/diff"
	"github.com/ztaylor/claude-follow-tui/internal/highlight"
	"github.com/ztaylor/claude-follow-tui/internal/history"
	"github.com/ztaylor/claude-follow-tui/internal/logger"
	"github.com/ztaylor/claude-follow-tui/internal/minimap"
	"github.com/ztaylor/claude-follow-tui/internal/theme"
)

// SocketMsg is sent when data is received from the socket
type SocketMsg struct {
	Payload []byte
}

// Change represents a single file change from Claude
type Change struct {
	Timestamp   time.Time
	FilePath    string
	ToolName    string
	OldString   string
	NewString   string
	FileContent string // Full file content after the change
	LineNum     int    // Line number where change starts
	LineCount   int    // Number of lines changed
	CommitSHA   string // VCS commit SHA at time of change
	CommitShort string // Short SHA for display
	VCSType     string // "git" or "jj"
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
	socketPath     string
	width          int
	height         int
	activePane     Pane
	changes        []Change
	selectedIndex  int
	diffViewport   viewport.Model
	showHelp       bool
	showHistory    bool
	showMinimap    bool // Toggle minimap visibility
	showPlan       bool // Show plan file popup
	planContent    string
	planPath       string
	planViewport   viewport.Model
	ready          bool
	theme          *theme.Theme
	highlighter    *highlight.Highlighter
	scrollX        int              // Horizontal scroll offset
	totalLines     int              // Total lines in current file (for minimap)
	minimapData    *minimap.Minimap // Cached minimap line types
	diffCache      map[int]string   // Cached rendered diffs by index
	historyStore   *history.Store   // Persistent history storage
	persistHistory bool             // Whether to save history to file
}

// Option is a functional option for configuring the Model
type Option func(*Model)

// WithTheme sets the theme for the model
func WithTheme(t *theme.Theme) Option {
	return func(m *Model) {
		m.theme = t
	}
}

// WithPersistence enables file-based history persistence
func WithPersistence(enabled bool) Option {
	return func(m *Model) {
		m.persistHistory = enabled
	}
}

// New creates a new Model with optional configuration
func New(socketPath string, opts ...Option) Model {
	t := theme.Default()
	m := Model{
		socketPath:  socketPath,
		changes:     []Change{},
		activePane:  PaneHistory,
		showHistory: true,
		showMinimap: true,
		theme:       t,
		highlighter: highlight.NewHighlighter(t),
		diffCache:   make(map[int]string),
	}

	for _, opt := range opts {
		opt(&m)
	}

	// Recreate highlighter if theme was changed via option
	if m.highlighter == nil || m.highlighter.Theme() != m.theme {
		m.highlighter = highlight.NewHighlighter(m.theme)
	}

	// Initialize history store if persistence is enabled
	if m.persistHistory {
		m.historyStore = history.NewStore(history.GetHistoryPath())
		if err := m.historyStore.Load(); err != nil {
			logger.Log("Failed to load history: %v", err)
		} else {
			// Convert history entries to changes
			for _, entry := range m.historyStore.Entries() {
				m.changes = append(m.changes, Change{
					Timestamp:   entry.Timestamp,
					FilePath:    entry.FilePath,
					ToolName:    entry.ToolName,
					OldString:   entry.OldString,
					NewString:   entry.NewString,
					LineNum:     entry.LineNum,
					LineCount:   entry.LineCount,
					CommitSHA:   entry.CommitSHA,
					CommitShort: entry.CommitShort,
					VCSType:     entry.VCSType,
				})
			}
			logger.Log("Loaded %d history entries", len(m.changes))
		}
	}

	return m
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

		// Initialize/resize viewport for diff
		headerHeight := 3
		footerHeight := 2
		if m.diffViewport.Width == 0 {
			m.diffViewport = viewport.New(m.width/2-4, m.height-headerHeight-footerHeight-2)
		}
		m.updateViewportSize()
		m.diffViewport.SetContent(m.renderDiff())

	case tea.MouseMsg:
		// Handle mouse scroll in diff pane
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.diffViewport.LineUp(3)
			case tea.MouseButtonWheelDown:
				m.diffViewport.LineDown(3)
			}
		}

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// Handle plan popup
		if m.showPlan {
			switch msg.String() {
			case "j", "down":
				m.planViewport.LineDown(1)
			case "k", "up":
				m.planViewport.LineUp(1)
			case "d":
				m.planViewport.HalfViewDown()
			case "u":
				m.planViewport.HalfViewUp()
			default:
				// Any other key closes the popup
				m.showPlan = false
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "?":
			m.showHelp = true

		case "P":
			// Show plan file popup
			m.loadPlanFile()
			if m.planContent != "" {
				m.showPlan = true
				// Initialize plan viewport if needed
				if m.planViewport.Width == 0 {
					m.planViewport = viewport.New(m.width-10, m.height-8)
				}
				m.planViewport.SetContent(m.planContent)
			}

		case "n":
			// Next change in queue
			if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
				m.selectedIndex++
				m.scrollX = 0 // Reset horizontal scroll
				m.diffViewport.SetContent(m.renderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}

		case "p":
			// Previous change in queue
			if len(m.changes) > 0 && m.selectedIndex > 0 {
				m.selectedIndex--
				m.scrollX = 0 // Reset horizontal scroll
				m.diffViewport.SetContent(m.renderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}

		case "j", "down":
			// Scroll diff pane down
			m.diffViewport.LineDown(1)

		case "k", "up":
			// Scroll diff pane up
			m.diffViewport.LineUp(1)

		case "tab":
			if m.activePane == PaneHistory {
				m.activePane = PaneDiff
			} else {
				m.activePane = PaneHistory
			}

		case "left":
			if m.scrollX > 0 {
				m.scrollX -= 4
				if m.scrollX < 0 {
					m.scrollX = 0
				}
				m.diffViewport.SetContent(m.renderDiff())
			}

		case "right":
			m.scrollX += 4
			m.diffViewport.SetContent(m.renderDiff())

		case "c":
			m.changes = []Change{}
			m.selectedIndex = 0
			m.diffViewport.SetContent("")
			m.diffCache = make(map[int]string)
			// Clear history file if persistence enabled
			if m.persistHistory && m.historyStore != nil {
				if err := m.historyStore.Clear(); err != nil {
					logger.Log("Failed to clear history file: %v", err)
				}
			}

		case "h":
			logger.Log("h key pressed, showHistory was %v", m.showHistory)
			m.showHistory = !m.showHistory
			logger.Log("showHistory now %v", m.showHistory)
			// When hiding history, switch to diff pane
			if !m.showHistory {
				m.activePane = PaneDiff
			}
			// Resize viewport for new layout and re-render content
			m.updateViewportSize()
			if len(m.changes) > 0 {
				m.diffViewport.SetContent(m.renderDiff())
			}

		case "m":
			m.showMinimap = !m.showMinimap
			m.updateViewportSize()
			if len(m.changes) > 0 {
				m.diffViewport.SetContent(m.renderDiff())
			}

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
		logger.Log("SocketMsg received, payload size: %d bytes", len(msg.Payload))

		// Extract plan_path from payload if present (sent by hook)
		var planInfo struct {
			PlanPath string `json:"plan_path"`
		}
		if json.Unmarshal(msg.Payload, &planInfo) == nil && planInfo.PlanPath != "" {
			m.planPath = planInfo.PlanPath
			logger.Log("Received planPath from hook: %s", m.planPath)
		}

		change := parsePayload(msg.Payload)
		if change != nil {
			// Get current VCS commit info
			sha, shortSHA, vcsType := history.GetCurrentCommit()
			change.CommitSHA = sha
			change.CommitShort = shortSHA
			change.VCSType = vcsType

			logger.Log("Parsed change: %s %s (line %d) commit=%s fileContent=%d bytes", change.ToolName, change.FilePath, change.LineNum, shortSHA, len(change.FileContent))
			// Append new change to end of list (queue style)
			m.changes = append(m.changes, *change)
			logger.Log("Total changes now: %d, selectedIndex: %d", len(m.changes), m.selectedIndex)

			// Save to history if persistence enabled
			if m.persistHistory && m.historyStore != nil {
				entry := history.Entry{
					Timestamp:   change.Timestamp,
					FilePath:    change.FilePath,
					ToolName:    change.ToolName,
					OldString:   change.OldString,
					NewString:   change.NewString,
					LineNum:     change.LineNum,
					LineCount:   change.LineCount,
					CommitSHA:   change.CommitSHA,
					CommitShort: change.CommitShort,
					VCSType:     change.VCSType,
				}
				if err := m.historyStore.Add(entry); err != nil {
					logger.Log("Failed to save history: %v", err)
				}
			}

			// If this is the first change, select it
			if len(m.changes) == 1 {
				m.selectedIndex = 0
				m.diffViewport.SetContent(m.renderDiff())
			}
			// Otherwise keep current position - user can press 'n' to see new items
		} else {
			logger.Log("parsePayload returned nil")
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

	if m.showPlan {
		return m.renderPlanPopup()
	}

	// Render header
	historyIndicator := ""
	if !m.showHistory {
		historyIndicator = m.theme.Dim.Render(" [h:show history]")
	}
	// Queue position indicator
	queueIndicator := ""
	if len(m.changes) > 0 {
		queueIndicator = m.theme.Selected.Render(fmt.Sprintf(" [%d/%d]", m.selectedIndex+1, len(m.changes)))
	}
	header := m.theme.Title.Render("Claude Follow") + queueIndicator + "  " +
		m.theme.Dim.Render(time.Now().Format("15:04:05")) + historyIndicator
	header = lipgloss.PlaceHorizontal(m.width, lipgloss.Left, header)

	var content string

	// Minimap/scrollbar
	minimapStr := m.renderMinimap()
	minimapWidth := 0
	if m.showMinimap {
		minimapWidth = 2
	}

	if m.showHistory {
		// Two-pane layout
		leftWidth := m.width / 3
		rightWidth := m.width - leftWidth - 3 - minimapWidth

		// Render history pane
		historyContent := m.renderHistory()
		historyBox := m.theme.Border
		if m.activePane == PaneHistory {
			historyBox = m.theme.ActiveBorder
		}
		historyPane := historyBox.
			Width(leftWidth).
			Height(m.height - 4).
			Render(historyContent)

		// Render diff pane
		diffContent := m.diffViewport.View()
		diffBox := m.theme.Border
		if m.activePane == PaneDiff {
			diffBox = m.theme.ActiveBorder
		}
		diffPane := diffBox.
			Width(rightWidth).
			Height(m.height - 4).
			Render(diffContent)

		if m.showMinimap {
			content = lipgloss.JoinHorizontal(lipgloss.Top, historyPane, diffPane, minimapStr)
		} else {
			content = lipgloss.JoinHorizontal(lipgloss.Top, historyPane, diffPane)
		}
	} else {
		// Full-width diff pane with minimap
		diffContent := m.diffViewport.View()
		diffBox := m.theme.ActiveBorder
		diffPane := diffBox.
			Width(m.width - 2 - minimapWidth).
			Height(m.height - 4).
			Render(diffContent)

		if m.showMinimap {
			content = lipgloss.JoinHorizontal(lipgloss.Top, diffPane, minimapStr)
		} else {
			content = diffPane
		}
	}

	// Render status bar
	status := m.renderStatus()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, status)
}

func (m Model) renderHistory() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("No changes yet...\nWaiting for Claude edits")
	}

	var sb strings.Builder
	sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("History (%d)\n", len(m.changes))))
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 20)) + "\n")

	// Calculate available width for path in history pane
	historyWidth := m.width / 3
	pathWidth := historyWidth - 15 // Account for timestamp, tool, prefix

	// Track current commit for grouping
	currentCommit := ""

	for i, change := range m.changes {
		// Show commit header when commit changes
		if change.CommitShort != "" && change.CommitShort != currentCommit {
			currentCommit = change.CommitShort
			vcsLabel := change.VCSType
			if vcsLabel == "" {
				vcsLabel = "commit"
			}
			commitHeader := fmt.Sprintf("── %s:%s ──", vcsLabel, change.CommitShort)
			sb.WriteString(m.theme.DiffHeader.Render(commitHeader) + "\n")
		}

		var line string
		if i == m.selectedIndex {
			// Selected: show scrollable relative path
			path := relativePath(change.FilePath)
			if m.scrollX > 0 && len(path) > m.scrollX {
				path = path[m.scrollX:]
			}
			line = fmt.Sprintf("%s %s %s",
				change.Timestamp.Format("15:04"),
				change.ToolName,
				path)
			sb.WriteString(m.theme.Selected.Render("> "+line) + "\n")
		} else {
			// Not selected: truncate path
			line = fmt.Sprintf("%s %s %s",
				change.Timestamp.Format("15:04"),
				change.ToolName,
				truncatePath(change.FilePath, pathWidth))
			sb.WriteString(m.theme.Normal.Render("  "+line) + "\n")
		}
	}

	return sb.String()
}

func (m *Model) renderDiff() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("Select a change to view diff")
	}

	// Use cache if available and no horizontal scroll
	if m.scrollX == 0 {
		if cached, ok := m.diffCache[m.selectedIndex]; ok {
			return cached
		}
	}

	change := m.changes[m.selectedIndex]

	// If FileContent is empty (e.g., loaded from history), try to read the file
	if change.FileContent == "" && change.FilePath != "" && change.ToolName != "Write" {
		if content, err := os.ReadFile(change.FilePath); err == nil {
			change.FileContent = string(content)
			// Update the stored change so we don't re-read every time
			m.changes[m.selectedIndex] = change
			logger.Log("Re-read file content for history entry: %s (%d bytes)", change.FilePath, len(change.FileContent))
		} else {
			logger.Log("Failed to re-read file for history entry: %s: %v", change.FilePath, err)
		}
	}

	var sb strings.Builder

	// Header with relative file path
	sb.WriteString(m.theme.Title.Render(relativePath(change.FilePath)))
	if change.LineNum > 0 {
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf(":%d", change.LineNum)))
	}
	sb.WriteString("\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// If we have file content, show full file with change highlighted
	if change.FileContent != "" && change.ToolName != "Write" {
		sb.WriteString(m.renderFileWithChange(change))
	} else if change.ToolName == "Write" {
		// For Write operations, show highlighted new content
		content := change.NewString
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		sb.WriteString(m.theme.DiffHeader.Render("@@ New file @@"))
		sb.WriteString("\n\n")

		lines := diff.SplitLines(content)
		for i, line := range lines {
			lineNum := fmt.Sprintf("%4d", i+1)
			highlighted := m.highlighter.HighlightLine(line, change.FilePath)
			sb.WriteString(m.theme.LineNumber.Render(lineNum))
			sb.WriteString(" ")
			sb.WriteString(m.theme.Added.Render("+ "))
			sb.WriteString(highlighted)
			sb.WriteString("\n")
		}
	} else if change.OldString != "" || change.NewString != "" {
		// Fallback: show just the diff
		opts := diff.DefaultOptions()
		diffOutput := diff.FormatDiff(change.OldString, change.NewString, m.theme, opts)
		sb.WriteString(diffOutput)
	} else {
		sb.WriteString(m.theme.Dim.Render("No diff content available"))
	}

	return sb.String()
}

// renderFileWithChange shows the full file with the changed section highlighted
func (m *Model) renderFileWithChange(change Change) string {
	var sb strings.Builder

	// Split file content into lines
	fileLines := diff.SplitLines(change.FileContent)
	oldLines := diff.SplitLines(change.OldString)
	newLines := diff.SplitLines(change.NewString)

	changeStart := change.LineNum - 1 // 0-indexed
	changeEnd := changeStart + len(oldLines)

	// Track total lines for minimap
	m.totalLines = len(fileLines) + len(newLines)

	// Build minimap data
	m.minimapData = minimap.New(m.totalLines)
	// Mark removed lines
	m.minimapData.SetRange(changeStart, changeEnd, minimap.LineRemoved)
	// Mark added lines (they appear after the removed lines in the view)
	// The added lines logically replace the removed ones at changeStart
	for i := 0; i < len(newLines); i++ {
		m.minimapData.SetLine(changeStart+i, minimap.LineAdded)
	}

	// Show diff header with stats
	sb.WriteString(m.theme.DiffHeader.Render(fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		change.LineNum, len(oldLines), change.LineNum, len(newLines))))
	sb.WriteString("  ")
	sb.WriteString(m.theme.Added.Render(fmt.Sprintf("+%d", len(newLines))))
	sb.WriteString(" ")
	sb.WriteString(m.theme.Removed.Render(fmt.Sprintf("-%d", len(oldLines))))
	sb.WriteString("\n\n")

	// Soft highlight style for changed lines
	changedBg := lipgloss.NewStyle().Background(m.theme.ChangedLineBg)

	// Render the entire file
	for i := 0; i < len(fileLines); i++ {
		lineNum := fmt.Sprintf("%4d", i+1)
		line := fileLines[i]

		// Apply horizontal scroll
		scrolledLine := line
		if m.scrollX > 0 && len(line) > m.scrollX {
			scrolledLine = line[m.scrollX:]
		} else if m.scrollX > 0 {
			scrolledLine = ""
		}

		// Check if this line is in the changed region
		if i >= changeStart && i < changeEnd {
			// This is a removed line - use diff colors (no syntax highlighting)
			lineContent := m.theme.LineNumberActive.Render(lineNum) + " " +
				m.theme.Removed.Render("- "+scrolledLine)
			sb.WriteString(changedBg.Render(lineContent))
			sb.WriteString("\n")

			// After the last removed line, insert the new lines
			if i == changeEnd-1 {
				for j, newLine := range newLines {
					// Apply horizontal scroll to new lines too
					scrolledNew := newLine
					if m.scrollX > 0 && len(newLine) > m.scrollX {
						scrolledNew = newLine[m.scrollX:]
					} else if m.scrollX > 0 {
						scrolledNew = ""
					}

					newLineNum := fmt.Sprintf("%4d", changeStart+j+1)
					lineContent := m.theme.LineNumberActive.Render(newLineNum) + " " +
						m.theme.Added.Render("+ "+scrolledNew)
					sb.WriteString(changedBg.Render(lineContent))
					sb.WriteString("\n")
				}
			}
		} else {
			// Context line - use syntax highlighting
			highlighted := m.highlighter.HighlightLine(scrolledLine, change.FilePath)
			sb.WriteString(m.theme.LineNumber.Render(lineNum))
			sb.WriteString(" ")
			sb.WriteString(m.theme.Context.Render("  "))
			sb.WriteString(highlighted)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// scrollToChange scrolls the viewport to show the current change
func (m *Model) scrollToChange() {
	if len(m.changes) == 0 {
		return
	}
	change := m.changes[m.selectedIndex]
	// Scroll to a few lines before the change so it's visible in context
	// Add 2 for the header lines in the diff view
	targetLine := change.LineNum - 3
	if targetLine < 0 {
		targetLine = 0
	}
	m.diffViewport.SetYOffset(targetLine)
}

// preloadAdjacent pre-caches rendered diffs for adjacent changes
func (m *Model) preloadAdjacent() {
	// Preload next
	if m.selectedIndex+1 < len(m.changes) {
		idx := m.selectedIndex + 1
		if _, ok := m.diffCache[idx]; !ok {
			// Store current state
			origIdx := m.selectedIndex
			origScrollX := m.scrollX
			// Render next
			m.selectedIndex = idx
			m.scrollX = 0
			m.diffCache[idx] = m.renderDiff()
			// Restore
			m.selectedIndex = origIdx
			m.scrollX = origScrollX
		}
	}
	// Preload previous
	if m.selectedIndex > 0 {
		idx := m.selectedIndex - 1
		if _, ok := m.diffCache[idx]; !ok {
			origIdx := m.selectedIndex
			origScrollX := m.scrollX
			m.selectedIndex = idx
			m.scrollX = 0
			m.diffCache[idx] = m.renderDiff()
			m.selectedIndex = origIdx
			m.scrollX = origScrollX
		}
	}
}

// updateViewportSize updates the viewport dimensions based on current layout
func (m *Model) updateViewportSize() {
	headerHeight := 2
	footerHeight := 1
	minimapWidth := 0
	if m.showMinimap {
		minimapWidth = 2
	}

	var vpWidth int
	if m.showHistory {
		leftWidth := m.width / 3
		vpWidth = m.width - leftWidth - 6 - minimapWidth
	} else {
		vpWidth = m.width - 4 - minimapWidth
	}

	m.diffViewport.Width = vpWidth
	m.diffViewport.Height = m.height - headerHeight - footerHeight - 2
}

// renderMinimap renders a visual minimap showing file structure and diff regions
func (m Model) renderMinimap() string {
	if !m.showMinimap {
		return ""
	}

	height := m.height - 4
	if height < 3 {
		return ""
	}

	// If we have minimap data, use the visual minimap
	if m.minimapData != nil && m.minimapData.TotalLines() > 0 {
		viewportStart := m.diffViewport.YOffset
		viewportEnd := viewportStart + m.diffViewport.Height
		return m.minimapData.Render(height, viewportStart, viewportEnd, m.theme)
	}

	// Fallback: simple scrollbar if no minimap data
	var sb strings.Builder

	totalLines := m.totalLines
	if totalLines < 1 {
		totalLines = 1
	}

	viewportHeight := m.diffViewport.Height
	if viewportHeight < 1 {
		viewportHeight = 1
	}

	// Thumb size proportional to viewport vs total content
	thumbSize := (viewportHeight * height) / totalLines
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > height {
		thumbSize = height
	}

	// Thumb position based on scroll offset
	scrollPos := m.diffViewport.YOffset
	maxScroll := totalLines - viewportHeight
	if maxScroll < 1 {
		maxScroll = 1
	}

	thumbPos := (scrollPos * (height - thumbSize)) / maxScroll
	if thumbPos < 0 {
		thumbPos = 0
	}
	if thumbPos+thumbSize > height {
		thumbPos = height - thumbSize
	}

	trackStyle := lipgloss.NewStyle().Foreground(m.theme.ScrollbarBg)
	thumbStyle := lipgloss.NewStyle().Foreground(m.theme.ScrollbarThumb)

	for i := 0; i < height; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(thumbStyle.Render("▐▐"))
		} else {
			sb.WriteString(trackStyle.Render("░░"))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderStatus() string {
	historyKey := "h:hide"
	if !m.showHistory {
		historyKey = "h:show"
	}
	minimapKey := "m:map"
	if m.showMinimap {
		minimapKey = "m:map✓"
	}
	keys := fmt.Sprintf("n/p:nav  ←→:scroll  %s  %s  P:plan  ^G:nvim  c:clear  ?:help  q:quit", historyKey, minimapKey)
	return m.theme.Status.Render(keys)
}

func (m Model) renderHelp() string {
	help := `
  Claude Follow TUI - Help

  Queue Navigation:
    n            Next change in queue
    p            Previous change in queue

  Scrolling:
    j / ↓        Scroll down
    k / ↑        Scroll up
    ←/→          Scroll horizontally

  View:
    h            Toggle history pane
    m            Toggle minimap
    P            Show Claude plan file
    Tab          Switch panes

  Actions:
    Ctrl+G       Open file in nvim at line
    Ctrl+O       Open file in nvim
    c            Clear history
    q            Quit

  Press any key to close help
`
	return m.theme.Help.Render(help)
}

func parsePayload(data []byte) *Change {
	logger.Log("parsePayload: raw data: %s", string(data))

	var payload HookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		logger.Log("parsePayload: JSON unmarshal error: %v", err)
		return nil
	}

	logger.Log("parsePayload: tool_name=%s", payload.ToolName)

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
	logger.Log("parsePayload: filePath=%s", filePath)
	if filePath == "" {
		logger.Log("parsePayload: filePath empty, returning nil")
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

	// Read the full file content
	var fileContent string
	var lineNum int = 1
	var lineCount int = 1

	if content, err := os.ReadFile(filePath); err == nil {
		fileContent = string(content)
		logger.Log("parsePayload: read file successfully, %d bytes", len(fileContent))

		// Find line number where the change occurs
		if oldStr != "" {
			lineNum = findLineNumber(fileContent, oldStr)
			lineCount = strings.Count(oldStr, "\n") + 1
		} else if newStr != "" {
			// For Write operations, show from beginning
			lineNum = 1
			lineCount = strings.Count(newStr, "\n") + 1
		}
	} else {
		logger.Log("parsePayload: failed to read file %s: %v", filePath, err)
	}

	return &Change{
		Timestamp:   time.Now(),
		FilePath:    filePath,
		ToolName:    payload.ToolName,
		OldString:   oldStr,
		NewString:   newStr,
		FileContent: fileContent,
		LineNum:     lineNum,
		LineCount:   lineCount,
	}
}

// findLineNumber finds the line number where searchStr first appears in content
func findLineNumber(content, searchStr string) int {
	if searchStr == "" {
		return 1
	}

	idx := strings.Index(content, searchStr)
	if idx == -1 {
		return 1
	}

	// Count newlines before the match
	return strings.Count(content[:idx], "\n") + 1
}

func truncatePath(path string, maxLen int) string {
	// First make it relative
	path = relativePath(path)
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

// relativePath converts an absolute path to relative if possible
func relativePath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		return path
	}
	// If relative path starts with many ../, just use filename
	if strings.HasPrefix(rel, "../../..") {
		return filepath.Base(path)
	}
	return rel
}

// cwdToProjectDir converts a CWD path to Claude's project directory name
// /Users/foo/bar.baz → -Users-foo-bar-baz
func cwdToProjectDir(cwd string) string {
	result := strings.ReplaceAll(cwd, "/", "-")
	result = strings.ReplaceAll(result, ".", "-")
	return result
}

// extractSlugFromJSONL reads the slug field from the last entry in a JSONL file
func extractSlugFromJSONL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Seek to end and read last chunk (avoid loading huge file)
	stat, err := f.Stat()
	if err != nil {
		return ""
	}
	size := stat.Size()
	readSize := int64(4096)
	if size < readSize {
		readSize = size
	}

	if _, err := f.Seek(-readSize, io.SeekEnd); err != nil {
		// If seek fails, try reading from start
		f.Seek(0, io.SeekStart)
	}

	buf := make([]byte, readSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return ""
	}

	// Find last complete JSON line with slug
	lines := strings.Split(string(buf[:n]), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Parse JSON and extract slug
		var entry struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal([]byte(line), &entry) == nil && entry.Slug != "" {
			return entry.Slug
		}
	}
	return ""
}

// findPlanFromSession looks up the plan file for the current session
func (m *Model) findPlanFromSession(home string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	projectDir := cwdToProjectDir(cwd)
	projectPath := filepath.Join(home, ".claude", "projects", projectDir)

	// Find most recent .jsonl in project directory
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return ""
	}

	var newestJSONL string
	var newestTime time.Time
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil || info == nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestJSONL = filepath.Join(projectPath, e.Name())
		}
	}

	if newestJSONL == "" {
		return ""
	}

	// Extract slug from JSONL
	slug := extractSlugFromJSONL(newestJSONL)
	if slug == "" {
		return ""
	}

	// Construct plan path and verify it exists
	planPath := filepath.Join(home, ".claude", "plans", slug+".md")
	if _, err := os.Stat(planPath); err == nil {
		return planPath
	}
	return ""
}

// findMostRecentPlan finds the most recently modified plan file (fallback)
func (m *Model) findMostRecentPlan(home string) string {
	plansDir := filepath.Join(home, ".claude", "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return ""
	}

	var newestPath string
	var newestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestPath = filepath.Join(plansDir, entry.Name())
		}
	}
	return newestPath
}

// loadPlanFile finds and loads the active Claude plan file
// Priority: 1) Path from hook, 2) Session-aware lookup, 3) Most recent plan
func (m *Model) loadPlanFile() {
	m.planContent = ""

	// Use path from hook if already set and valid
	planPath := m.planPath
	if planPath != "" {
		if content, err := os.ReadFile(planPath); err == nil {
			m.planContent = string(content)
			return
		}
		// Path invalid, clear it and try other methods
		m.planPath = ""
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Try session-aware lookup
	planPath = m.findPlanFromSession(home)

	// Fallback to most recent plan
	if planPath == "" {
		planPath = m.findMostRecentPlan(home)
	}

	if planPath == "" {
		m.planContent = "No plan files found in ~/.claude/plans/"
		return
	}

	// Read the plan file
	content, err := os.ReadFile(planPath)
	if err != nil {
		m.planContent = fmt.Sprintf("Error reading plan: %v", err)
		return
	}

	m.planPath = planPath
	m.planContent = string(content)
}

// renderPlanPopup renders the plan file popup
func (m Model) renderPlanPopup() string {
	// Calculate popup dimensions
	popupWidth := m.width - 8
	popupHeight := m.height - 6

	// Build header
	title := "Plan"
	if m.planPath != "" {
		title = filepath.Base(m.planPath)
	}
	header := m.theme.Title.Render(" " + title + " ")
	hint := m.theme.Dim.Render(" j/k:scroll  d/u:page  any:close ")

	// Content
	content := m.planViewport.View()

	// Build the popup box
	box := m.theme.ActiveBorder.
		Width(popupWidth).
		Height(popupHeight).
		Render(content)

	// Center the popup
	popup := lipgloss.JoinVertical(lipgloss.Left,
		header,
		box,
		hint,
	)

	// Center on screen
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		popup,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("0")),
	)
}
