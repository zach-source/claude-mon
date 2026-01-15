package ralph

import (
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ztaylor/claude-mon/internal/ralph"
	"github.com/ztaylor/claude-mon/internal/theme"
)

// KeyMap defines keybindings for ralph mode
type KeyMap struct {
	Cancel  key.Binding
	Refresh key.Binding
	Chat    key.Binding
}

// DefaultKeyMap returns default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Cancel:  key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "cancel ralph")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Chat:    key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "ralph chat")),
	}
}

// ShortHelp returns keybindings for short help
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Cancel, k.Refresh}
}

// FullHelp returns keybindings for full help
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Cancel, k.Refresh, k.Chat},
	}
}

// Model represents the ralph component state
type Model struct {
	// State
	state *ralph.State

	// Dependencies
	theme  *theme.Theme
	keyMap KeyMap

	// Layout
	width, height int

	// Ticker for auto-refresh
	refreshInterval time.Duration
}

// Option is a functional option for configuring the Model
type Option func(*Model)

// WithTheme sets the theme
func WithTheme(t *theme.Theme) Option {
	return func(m *Model) {
		m.theme = t
	}
}

// WithKeyMap sets custom keybindings
func WithKeyMap(km KeyMap) Option {
	return func(m *Model) {
		m.keyMap = km
	}
}

// WithRefreshInterval sets the auto-refresh interval
func WithRefreshInterval(d time.Duration) Option {
	return func(m *Model) {
		m.refreshInterval = d
	}
}

// New creates a new ralph component
func New(opts ...Option) Model {
	m := Model{
		keyMap:          DefaultKeyMap(),
		theme:           theme.Default(),
		refreshInterval: 5 * time.Second,
	}

	for _, opt := range opts {
		opt(&m)
	}

	// Load initial state
	m.RefreshState()

	return m
}

// RefreshState reloads the ralph state from disk
func (m *Model) RefreshState() {
	state, err := ralph.LoadState()
	if err != nil {
		m.state = nil
		return
	}
	m.state = state
}

// Cancel cancels the active ralph loop
func (m *Model) Cancel() error {
	_, err := ralph.CancelLoop()
	if err != nil {
		return err
	}
	m.state = nil
	return nil
}

// Init initializes the component
func (m Model) Init() tea.Cmd {
	return nil
}

// RefreshTickMsg is sent to trigger auto-refresh
type RefreshTickMsg struct {
	Time time.Time
}

// StartRefreshTicker returns a command that starts the refresh ticker
func (m Model) StartRefreshTicker() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return RefreshTickMsg{Time: t}
	})
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case RefreshTickMsg:
		m.RefreshState()
		return m, m.StartRefreshTicker()

	case tea.KeyMsg:
		return m.handleKeys(msg)
	}

	return m, nil
}

// handleKeys processes key events
func (m Model) handleKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.Cancel):
		if m.state != nil && m.state.Active {
			_ = m.Cancel()
		}

	case key.Matches(msg, m.keyMap.Refresh):
		m.RefreshState()
	}

	return m, nil
}

// SetSize sets the component dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// State returns the current ralph state
func (m Model) State() *ralph.State {
	return m.state
}

// IsActive returns whether there's an active ralph loop
func (m Model) IsActive() bool {
	return m.state != nil && m.state.Active
}

// KeyMap returns the current keybindings
func (m Model) KeyMap() KeyMap {
	return m.keyMap
}
