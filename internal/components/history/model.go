package history

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ztaylor/claude-mon/internal/diff"
	"github.com/ztaylor/claude-mon/internal/highlight"
	"github.com/ztaylor/claude-mon/internal/history"
	"github.com/ztaylor/claude-mon/internal/logger"
	"github.com/ztaylor/claude-mon/internal/minimap"
	"github.com/ztaylor/claude-mon/internal/theme"
)

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
	CommitSHA   string // Git/jj commit SHA if available
	CommitShort string // Short commit hash
	VCSType     string // "git" or "jj"
}

// KeyMap defines key bindings for history navigation
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	Next         key.Binding
	Prev         key.Binding
	ScrollLeft   key.Binding
	ScrollRight  key.Binding
	ClearHistory key.Binding
	OpenInNvim   key.Binding
	OpenNvimCwd  key.Binding
}

// DefaultKeyMap returns the default key bindings for history
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:         key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		PageUp:       key.NewBinding(key.WithKeys("u", "pgup"), key.WithHelp("u", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("d", "pgdown"), key.WithHelp("d", "page down")),
		Next:         key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next change")),
		Prev:         key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev change")),
		ScrollLeft:   key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "scroll left")),
		ScrollRight:  key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "scroll right")),
		ClearHistory: key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "clear history")),
		OpenInNvim:   key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("C-n", "open in nvim")),
		OpenNvimCwd:  key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("C-o", "nvim cwd")),
	}
}

// ShortHelp returns key bindings for short help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Next, k.Prev}
}

// FullHelp returns key bindings for full help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.ScrollLeft, k.ScrollRight, k.OpenInNvim, k.OpenNvimCwd},
		{k.ClearHistory, k.Next, k.Prev},
	}
}

// Model represents the history component state
type Model struct {
	// Data
	changes       []Change
	selectedIndex int

	// Display state
	viewport         viewport.Model
	scrollX          int // Horizontal scroll offset for selected item path
	listScrollOffset int // Vertical scroll offset for history list
	totalLines       int // Total lines in current file (for minimap)
	minimapData      *minimap.Minimap
	diffCache        map[int]string // Cached rendered diffs by index

	// Storage
	store      *history.Store
	persistent bool // Whether to save history to file

	// Dependencies
	theme       *theme.Theme
	highlighter *highlight.Highlighter
	keyMap      KeyMap

	// Layout
	width, height int
	focusLeft     bool // Whether left pane is focused
}

// Option is a functional option for configuring the Model
type Option func(*Model)

// WithTheme sets the theme
func WithTheme(t *theme.Theme) Option {
	return func(m *Model) {
		m.theme = t
		m.highlighter = highlight.NewHighlighter(t)
	}
}

// WithKeyMap sets custom key bindings
func WithKeyMap(km KeyMap) Option {
	return func(m *Model) {
		m.keyMap = km
	}
}

// WithPersistence enables file-based history persistence
func WithPersistence(enabled bool) Option {
	return func(m *Model) {
		m.persistent = enabled
	}
}

// New creates a new history component
func New(opts ...Option) Model {
	m := Model{
		changes:   []Change{},
		diffCache: make(map[int]string),
		keyMap:    DefaultKeyMap(),
		theme:     theme.Default(),
		focusLeft: true,
	}

	// Apply options
	for _, opt := range opts {
		opt(&m)
	}

	// Initialize highlighter if not set
	if m.highlighter == nil {
		m.highlighter = highlight.NewHighlighter(m.theme)
	}

	// Initialize history store if persistence is enabled
	if m.persistent {
		m.store = history.NewStore(history.GetHistoryPath())
		if err := m.store.Load(); err != nil {
			logger.Log("Failed to load history: %v", err)
		} else {
			// Convert history entries to changes
			for _, entry := range m.store.Entries() {
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
			// Select most recent (last) item
			if len(m.changes) > 0 {
				m.selectedIndex = len(m.changes) - 1
			}
		}
	}

	return m
}

// Init initializes the component
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeys(msg)
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// handleKeys processes key events
func (m Model) handleKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.Down):
		if m.focusLeft {
			// Navigate history (list is reversed, so down = decrement index)
			if len(m.changes) > 0 && m.selectedIndex > 0 {
				m.selectedIndex--
				m.scrollX = 0
				m.ensureSelectedVisible()
				m.viewport.SetContent(m.RenderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}
		} else {
			m.viewport.LineDown(1)
		}

	case key.Matches(msg, m.keyMap.Up):
		if m.focusLeft {
			// Navigate history (list is reversed, so up = increment index)
			if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
				m.selectedIndex++
				m.scrollX = 0
				m.ensureSelectedVisible()
				m.viewport.SetContent(m.RenderDiff())
				m.scrollToChange()
				m.preloadAdjacent()
			}
		} else {
			m.viewport.LineUp(1)
		}

	case key.Matches(msg, m.keyMap.PageDown):
		if m.focusLeft {
			// Page down in history list
			visibleItems := m.listVisibleItems()
			for i := 0; i < visibleItems && m.selectedIndex > 0; i++ {
				m.selectedIndex--
			}
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.viewport.SetContent(m.RenderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		} else {
			m.viewport.ViewDown()
		}

	case key.Matches(msg, m.keyMap.PageUp):
		if m.focusLeft {
			// Page up in history list
			visibleItems := m.listVisibleItems()
			for i := 0; i < visibleItems && m.selectedIndex < len(m.changes)-1; i++ {
				m.selectedIndex++
			}
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.viewport.SetContent(m.RenderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		} else {
			m.viewport.ViewUp()
		}

	case key.Matches(msg, m.keyMap.Next):
		// Next change in queue
		if len(m.changes) > 0 && m.selectedIndex < len(m.changes)-1 {
			m.selectedIndex++
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.viewport.SetContent(m.RenderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		}

	case key.Matches(msg, m.keyMap.Prev):
		// Previous change in queue
		if len(m.changes) > 0 && m.selectedIndex > 0 {
			m.selectedIndex--
			m.scrollX = 0
			m.ensureSelectedVisible()
			m.viewport.SetContent(m.RenderDiff())
			m.scrollToChange()
			m.preloadAdjacent()
		}

	case key.Matches(msg, m.keyMap.ScrollLeft):
		if m.scrollX > 0 {
			m.scrollX -= 4
			if m.scrollX < 0 {
				m.scrollX = 0
			}
			m.viewport.SetContent(m.RenderDiff())
		}

	case key.Matches(msg, m.keyMap.ScrollRight):
		m.scrollX += 4
		m.viewport.SetContent(m.RenderDiff())

	case key.Matches(msg, m.keyMap.ClearHistory):
		m.changes = []Change{}
		m.selectedIndex = 0
		m.viewport.SetContent("")
		m.diffCache = make(map[int]string)
		if m.persistent && m.store != nil {
			if err := m.store.Clear(); err != nil {
				logger.Log("Failed to clear history file: %v", err)
			}
		}

	case key.Matches(msg, m.keyMap.OpenInNvim):
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", fmt.Sprintf("+%d", change.LineNum), change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}

	case key.Matches(msg, m.keyMap.OpenNvimCwd):
		if len(m.changes) > 0 {
			change := m.changes[m.selectedIndex]
			cmd := exec.Command("nvim", change.FilePath)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
				return nil
			})
		}
	}

	return m, nil
}

// SetSize sets the component dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width
	m.viewport.Height = height - 4 // Account for header/footer
}

// SetFocusLeft sets whether the left pane is focused
func (m *Model) SetFocusLeft(focused bool) {
	m.focusLeft = focused
}

// AddChange adds a new change to history
func (m *Model) AddChange(change Change) {
	m.changes = append(m.changes, change)
	m.selectedIndex = len(m.changes) - 1

	// When 40+ items, start at top (newest) and scroll to show selected
	if len(m.changes) >= 40 {
		m.listScrollOffset = 0 // Start at top showing newest items
	}
	m.ensureSelectedVisible()

	// Persist if enabled
	if m.persistent && m.store != nil {
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
		if err := m.store.Add(entry); err != nil {
			logger.Log("Failed to persist history entry: %v", err)
		}
	}

	// Invalidate cache
	delete(m.diffCache, m.selectedIndex)
}

// Changes returns all changes
func (m Model) Changes() []Change {
	return m.changes
}

// SelectedChange returns the currently selected change
func (m Model) SelectedChange() (Change, bool) {
	if len(m.changes) == 0 || m.selectedIndex >= len(m.changes) {
		return Change{}, false
	}
	return m.changes[m.selectedIndex], true
}

// SelectedIndex returns the current selection index
func (m Model) SelectedIndex() int {
	return m.selectedIndex
}

// MinimapData returns the minimap data for the current view
func (m Model) MinimapData() *minimap.Minimap {
	return m.minimapData
}

// TotalLines returns total lines in the current file
func (m Model) TotalLines() int {
	return m.totalLines
}

// Viewport returns the viewport model for external updates
func (m Model) Viewport() viewport.Model {
	return m.viewport
}

// SetViewport updates the viewport model
func (m *Model) SetViewport(vp viewport.Model) {
	m.viewport = vp
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
	m.viewport.SetYOffset(targetLine)
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
			m.diffCache[idx] = m.RenderDiff()
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
			m.diffCache[idx] = m.RenderDiff()
			m.selectedIndex = origIdx
			m.scrollX = origScrollX
		}
	}
}

// UpdateDiffContent refreshes the viewport with current diff content
func (m *Model) UpdateDiffContent() {
	m.viewport.SetContent(m.RenderDiff())
}

// utility function to get relative path
func relativePath(p string) string {
	// Use diff package's relativePath if available, or implement here
	return diff.RelativePath(p)
}

// listVisibleItems returns the number of items that can fit in the list view
func (m Model) listVisibleItems() int {
	// Account for header (2 lines: title + separator)
	availableHeight := m.height - 2
	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// ensureSelectedVisible adjusts listScrollOffset to keep selected item visible
func (m *Model) ensureSelectedVisible() {
	if len(m.changes) == 0 {
		return
	}

	// Convert selectedIndex to visual position (list is reversed)
	// Visual position 0 = changes[len-1], visual position N = changes[len-1-N]
	visualPos := len(m.changes) - 1 - m.selectedIndex

	visibleItems := m.listVisibleItems()

	// If selected is above visible area, scroll up
	if visualPos < m.listScrollOffset {
		m.listScrollOffset = visualPos
	}

	// If selected is below visible area, scroll down
	if visualPos >= m.listScrollOffset+visibleItems {
		m.listScrollOffset = visualPos - visibleItems + 1
	}

	// Clamp scroll offset
	maxOffset := len(m.changes) - visibleItems
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.listScrollOffset > maxOffset {
		m.listScrollOffset = maxOffset
	}
	if m.listScrollOffset < 0 {
		m.listScrollOffset = 0
	}
}

// ListScrollOffset returns the current list scroll offset
func (m Model) ListScrollOffset() int {
	return m.listScrollOffset
}

// SetListScrollOffset sets the list scroll offset
func (m *Model) SetListScrollOffset(offset int) {
	m.listScrollOffset = offset
}
