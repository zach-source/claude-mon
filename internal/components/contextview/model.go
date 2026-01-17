package contextview

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ztaylor/claude-mon/internal/context"
	"github.com/ztaylor/claude-mon/internal/theme"
)

// KeyMap defines key bindings for context navigation
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Edit     key.Binding
	Save     key.Binding
	Cancel   key.Binding
	ShowAll  key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		PageUp:   key.NewBinding(key.WithKeys("u", "pgup"), key.WithHelp("u", "page up")),
		PageDown: key.NewBinding(key.WithKeys("d", "pgdown"), key.WithHelp("d", "page down")),
		Edit:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Save:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "save")),
		Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		ShowAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "show all")),
	}
}

// ShortHelp returns key bindings for short help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Edit}
}

// FullHelp returns key bindings for full help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.Edit, k.Save, k.Cancel, k.ShowAll},
	}
}

// EditField represents which field is being edited
type EditField string

const (
	EditNone       EditField = ""
	EditK8sContext EditField = "kubernetes.context"
	EditK8sNS      EditField = "kubernetes.namespace"
	EditAWSProfile EditField = "aws.profile"
	EditAWSRegion  EditField = "aws.region"
	EditGitBranch  EditField = "git.branch"
	EditEnvVar     EditField = "env"
	EditCustom     EditField = "custom"
)

// Model represents the context component state
type Model struct {
	// Data
	current *context.Context
	all     []*context.Context

	// Selection state
	selected int
	showList bool // Show all contexts vs current only

	// Edit mode
	editMode  bool
	editField EditField
	editInput textinput.Model

	// Dependencies
	theme    *theme.Theme
	keyMap   KeyMap
	viewport viewport.Model

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

// WithKeyMap sets custom key bindings
func WithKeyMap(km KeyMap) Option {
	return func(m *Model) {
		m.keyMap = km
	}
}

// New creates a new context component
func New(opts ...Option) Model {
	m := Model{
		keyMap: DefaultKeyMap(),
		theme:  theme.Default(),
	}

	// Apply options
	for _, opt := range opts {
		opt(&m)
	}

	// Initialize edit input
	ti := textinput.New()
	ti.Placeholder = "Enter value..."
	ti.CharLimit = 256
	ti.Width = 40
	m.editInput = ti

	// Load current context
	m.RefreshContext()

	return m
}

// RefreshContext reloads the current context
func (m *Model) RefreshContext() {
	ctx, err := context.Load()
	if err == nil {
		m.current = ctx
	}
}

// RefreshAll reloads all contexts
func (m *Model) RefreshAll() {
	contexts, err := context.ListAll()
	if err == nil {
		m.all = contexts
	}
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
		if m.editMode {
			return m.handleEditKeys(msg)
		}
		return m.handleKeys(msg)
	}

	return m, nil
}

// handleKeys processes key events in normal mode
func (m Model) handleKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.ShowAll):
		m.showList = !m.showList
		if m.showList {
			m.RefreshAll()
		}

	case key.Matches(msg, m.keyMap.Edit):
		// Start edit mode - would need to know which field is selected
		m.editMode = true
		m.editInput.Focus()

	case key.Matches(msg, m.keyMap.Down):
		if m.showList && m.selected < len(m.all)-1 {
			m.selected++
		}

	case key.Matches(msg, m.keyMap.Up):
		if m.showList && m.selected > 0 {
			m.selected--
		}
	}

	return m, nil
}

// handleEditKeys processes key events in edit mode
func (m Model) handleEditKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.Cancel):
		m.editMode = false
		m.editField = EditNone
		m.editInput.SetValue("")

	case key.Matches(msg, m.keyMap.Save):
		// Save the edited value
		m.saveEdit()
		m.editMode = false
		m.editField = EditNone
		m.editInput.SetValue("")
	}

	// Update the text input
	var cmd tea.Cmd
	m.editInput, cmd = m.editInput.Update(msg)
	return m, cmd
}

// parseKeyValue parses a KEY=VALUE string where VALUE can be quoted with ", ', or `
// Examples:
//   - "foo=bar" -> ("foo", "bar")
//   - "foo='hello world'" -> ("foo", "hello world")
//   - `foo="my sentence"` -> ("foo", "my sentence")
//   - "foo=`backtick quoted`" -> ("foo", "backtick quoted")
func parseKeyValue(input string) (key, value string, ok bool) {
	// Find the first = sign
	eqIdx := -1
	for i, c := range input {
		if c == '=' {
			eqIdx = i
			break
		}
	}

	if eqIdx <= 0 {
		return "", "", false
	}

	key = input[:eqIdx]
	remainder := input[eqIdx+1:]

	// Check if value is quoted
	if len(remainder) >= 2 {
		first := remainder[0]
		if first == '"' || first == '\'' || first == '`' {
			// Find matching closing quote
			for i := len(remainder) - 1; i > 0; i-- {
				if remainder[i] == first {
					value = remainder[1:i]
					return key, value, true
				}
			}
		}
	}

	// Not quoted, use the whole remainder
	value = remainder
	return key, value, true
}

// saveEdit saves the current edit to the context
func (m *Model) saveEdit() {
	if m.current == nil {
		return
	}

	value := m.editInput.Value()

	switch m.editField {
	case EditK8sContext:
		if k8s := m.current.GetKubernetes(); k8s != nil {
			k8s.Context = value
		}
	case EditK8sNS:
		if k8s := m.current.GetKubernetes(); k8s != nil {
			k8s.Namespace = value
		}
	case EditAWSProfile:
		if aws := m.current.GetAWS(); aws != nil {
			aws.Profile = value
		}
	case EditAWSRegion:
		if aws := m.current.GetAWS(); aws != nil {
			aws.Region = value
		}
	case EditGitBranch:
		if git := m.current.GetGit(); git != nil {
			git.Branch = value
		}
	case EditEnvVar:
		// Parse KEY=VALUE with quote support and merge into existing env
		if k, v, ok := parseKeyValue(value); ok {
			env := m.current.GetEnv()
			if env == nil {
				env = make(map[string]string)
			}
			env[k] = v
			m.current.SetEnv(env)
		}
	case EditCustom:
		// Parse KEY=VALUE with quote support and merge into existing custom
		if k, v, ok := parseKeyValue(value); ok {
			custom := m.current.GetCustom()
			if custom == nil {
				custom = make(map[string]string)
			}
			custom[k] = v
			m.current.SetCustom(custom)
		}
	}

	// Save context
	_ = m.current.Save()
}

// SetSize sets the component dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Current returns the current context
func (m Model) Current() *context.Context {
	return m.current
}

// All returns all loaded contexts
func (m Model) All() []*context.Context {
	return m.all
}

// IsEditing returns whether we're in edit mode
func (m Model) IsEditing() bool {
	return m.editMode
}

// ShowingList returns whether we're showing all contexts
func (m Model) ShowingList() bool {
	return m.showList
}
