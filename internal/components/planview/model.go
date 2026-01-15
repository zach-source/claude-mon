package planview

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ztaylor/claude-mon/internal/theme"
)

// KeyMap defines keybindings for plan mode
type KeyMap struct {
	Generate key.Binding
	Edit     key.Binding
	Refresh  key.Binding
	Submit   key.Binding
	Cancel   key.Binding
}

// DefaultKeyMap returns default keybindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Generate: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "generate plan")),
		Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit plan")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Submit:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "submit")),
		Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

// ShortHelp returns keybindings for short help
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Generate, k.Edit, k.Refresh}
}

// FullHelp returns keybindings for full help
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Generate, k.Edit, k.Refresh},
		{k.Submit, k.Cancel},
	}
}

// Model represents the plan component state
type Model struct {
	// Plan data
	path    string
	content string

	// Input state
	inputActive bool
	input       textinput.Model
	generating  bool

	// Viewport for scrolling
	viewport viewport.Model

	// Dependencies
	theme  *theme.Theme
	keyMap KeyMap

	// Layout
	width, height int
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

// New creates a new plan component
func New(opts ...Option) Model {
	m := Model{
		keyMap: DefaultKeyMap(),
		theme:  theme.Default(),
	}

	for _, opt := range opts {
		opt(&m)
	}

	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = "Describe what to build..."
	ti.CharLimit = 500
	ti.Width = 50
	m.input = ti

	return m
}

// SetPath sets the plan file path and loads content
func (m *Model) SetPath(path string) {
	m.path = path
	m.LoadContent()
}

// LoadContent loads the plan content from disk
func (m *Model) LoadContent() {
	if m.path == "" {
		m.content = ""
		return
	}

	data, err := os.ReadFile(m.path)
	if err != nil {
		m.content = ""
		return
	}
	m.content = string(data)
}

// Init initializes the component
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)

	case tea.KeyMsg:
		if m.inputActive {
			return m.handleInputKeys(msg)
		}
		return m.handleKeys(msg)
	}

	return m, nil
}

// handleKeys processes key events in normal mode
func (m Model) handleKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.Generate):
		if !m.generating {
			m.inputActive = true
			m.input.Focus()
		}

	case key.Matches(msg, m.keyMap.Refresh):
		m.LoadContent()
	}

	return m, nil
}

// handleInputKeys processes key events in input mode
func (m Model) handleInputKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.Submit):
		description := m.input.Value()
		if description != "" {
			m.inputActive = false
			m.generating = true
			m.input.Reset()
			// Return a command to generate the plan
			return m, func() tea.Msg {
				return GeneratePlanMsg{Description: description}
			}
		}

	case key.Matches(msg, m.keyMap.Cancel):
		m.inputActive = false
		m.input.Reset()
		return m, nil
	}

	// Update the text input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// SetSize sets the component dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width
	m.viewport.Height = height - 4 // Account for header
}

// Path returns the current plan path
func (m Model) Path() string {
	return m.path
}

// Content returns the plan content
func (m Model) Content() string {
	return m.content
}

// IsInputActive returns whether input mode is active
func (m Model) IsInputActive() bool {
	return m.inputActive
}

// IsGenerating returns whether plan generation is in progress
func (m Model) IsGenerating() bool {
	return m.generating
}

// SetGenerating sets the generating state
func (m *Model) SetGenerating(generating bool) {
	m.generating = generating
}

// Name returns the plan name (filename without extension)
func (m Model) Name() string {
	if m.path == "" {
		return ""
	}
	return filepath.Base(m.path[:len(m.path)-len(filepath.Ext(m.path))])
}

// GeneratePlanMsg is sent when the user submits a plan description
type GeneratePlanMsg struct {
	Description string
}

// PlanGeneratedMsg is sent when plan generation completes
type PlanGeneratedMsg struct {
	Path string
	Slug string
}

// PlanErrorMsg is sent when plan generation fails
type PlanErrorMsg struct {
	Err error
}

// PlanEditedMsg is sent when the plan file is edited externally
type PlanEditedMsg struct{}
