package prompts

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ztaylor/claude-mon/internal/prompt"
	"github.com/ztaylor/claude-mon/internal/theme"
)

// PromptItem implements list.Item for bubbles/list integration
type PromptItem struct {
	prompt.Prompt
}

// FilterValue implements list.Item - used for fuzzy filtering
func (i PromptItem) FilterValue() string {
	return i.Name
}

// Title implements list.DefaultItem
func (i PromptItem) Title() string {
	return i.Name
}

// Description implements list.DefaultItem
func (i PromptItem) Description() string {
	if i.Prompt.Description != "" {
		return i.Prompt.Description
	}
	// Show scope indicator
	if i.IsGlobal {
		return "[Global]"
	}
	return "[Project]"
}

// KeyMap defines key bindings for prompts navigation
type KeyMap struct {
	Up              key.Binding
	Down            key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	NewPrompt       key.Binding
	NewGlobalPrompt key.Binding
	EditPrompt      key.Binding
	RefinePrompt    key.Binding
	DeletePrompt    key.Binding
	YankPrompt      key.Binding
	InjectMethod    key.Binding
	SendPrompt      key.Binding
	CreateVersion   key.Binding
	ViewVersions    key.Binding
	RevertVersion   key.Binding
}

// DefaultKeyMap returns the default key bindings for prompts
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:              key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:            key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		PageUp:          key.NewBinding(key.WithKeys("u", "pgup"), key.WithHelp("u", "page up")),
		PageDown:        key.NewBinding(key.WithKeys("d", "pgdown"), key.WithHelp("d", "page down")),
		NewPrompt:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new prompt")),
		NewGlobalPrompt: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new global")),
		EditPrompt:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		RefinePrompt:    key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refine")),
		DeletePrompt:    key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("C-d", "delete")),
		YankPrompt:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		InjectMethod:    key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "inject method")),
		SendPrompt:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "send")),
		CreateVersion:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "version")),
		ViewVersions:    key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "view versions")),
		RevertVersion:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "revert")),
	}
}

// ShortHelp returns key bindings for short help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.SendPrompt, k.EditPrompt, k.NewPrompt}
}

// FullHelp returns key bindings for full help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.SendPrompt, k.EditPrompt},
		{k.NewPrompt, k.NewGlobalPrompt, k.DeletePrompt},
		{k.RefinePrompt, k.YankPrompt, k.InjectMethod},
		{k.CreateVersion, k.ViewVersions, k.RevertVersion},
	}
}

// Model represents the prompts component state
type Model struct {
	// Core data
	store    *prompt.Store
	list     list.Model // bubbles/list for fuzzy filtering
	selected int

	// Version view mode
	showVersions    bool
	versions        []prompt.PromptVersion
	versionSelected int

	// Refine mode
	refining       bool
	refineInput    textinput.Model
	refiningPrompt *prompt.Prompt
	refineOutput   string
	refineDone     bool

	// Injection
	injectMethod prompt.InjectionMethod

	// Dependencies
	theme    *theme.Theme
	keyMap   KeyMap
	viewport viewport.Model

	// Layout
	width, height int
	focusLeft     bool
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

// New creates a new prompts component
func New(opts ...Option) Model {
	m := Model{
		keyMap:       DefaultKeyMap(),
		theme:        theme.Default(),
		injectMethod: prompt.DetectBestMethod(),
		focusLeft:    true,
	}

	// Apply options
	for _, opt := range opts {
		opt(&m)
	}

	// Initialize prompt store
	if store, err := prompt.NewStore(); err == nil {
		m.store = store
	}

	// Initialize refine input
	ti := textinput.New()
	ti.Placeholder = "What do you want to improve?"
	ti.CharLimit = 500
	ti.Width = 50
	m.refineInput = ti

	// Initialize bubbles/list
	m.initList()

	return m
}

// initList initializes the bubbles/list with prompts
func (m *Model) initList() {
	// Create delegate with custom styling
	delegate := list.NewDefaultDelegate()

	// Create list
	m.list = list.New([]list.Item{}, delegate, m.width/3, m.height-4)
	m.list.Title = "Prompts"
	m.list.SetShowStatusBar(false)
	m.list.SetFilteringEnabled(true)
	m.list.SetShowHelp(false) // We handle our own help

	// Load prompts
	m.RefreshList()
}

// RefreshList reloads prompts from storage
func (m *Model) RefreshList() {
	if m.store == nil {
		return
	}

	prompts, err := m.store.List()
	if err != nil {
		return
	}

	// Convert to list items
	items := make([]list.Item, len(prompts))
	for i, p := range prompts {
		items[i] = PromptItem{Prompt: p}
	}

	m.list.SetItems(items)
}

// Init initializes the component
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		// Handle our keys first, then pass to list
		if !m.refining {
			return m.handleKeys(msg)
		}
		// In refine mode, handle refine-specific keys
		return m.handleRefineKeys(msg)
	}

	// Update the list
	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)
	cmds = append(cmds, listCmd)

	return m, tea.Batch(cmds...)
}

// handleKeys processes key events in normal mode
func (m Model) handleKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	// If showing versions, delegate to version keys
	if m.showVersions {
		return m.handleVersionKeys(msg)
	}

	// Let bubbles/list handle navigation when filtering
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, m.keyMap.ViewVersions):
		// Enter version view mode
		if m.store != nil && len(m.list.Items()) > 0 {
			m.loadVersionList()
			if len(m.versions) > 0 {
				m.showVersions = true
				m.versionSelected = 0
			}
		}
		return m, nil

	case key.Matches(msg, m.keyMap.RefinePrompt):
		if len(m.list.Items()) > 0 && !m.refining {
			m.refining = true
			m.refineInput.Focus()
			if item, ok := m.list.SelectedItem().(PromptItem); ok {
				m.refiningPrompt = &item.Prompt
			}
		}
		return m, nil
	}

	// Let list handle everything else
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// handleVersionKeys processes key events in version view mode
func (m Model) handleVersionKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keyMap.ViewVersions):
		// Exit version view
		m.showVersions = false
		m.versionSelected = 0
	case key.Matches(msg, m.keyMap.Down):
		if m.versionSelected < len(m.versions)-1 {
			m.versionSelected++
		}
	case key.Matches(msg, m.keyMap.Up):
		if m.versionSelected > 0 {
			m.versionSelected--
		}
	}
	return m, nil
}

// handleRefineKeys processes key events in refine mode
func (m Model) handleRefineKeys(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.refining = false
		m.refineDone = false
		m.refineOutput = ""
		m.refiningPrompt = nil
		return m, nil
	case "enter":
		if !m.refineDone && m.refineInput.Value() != "" {
			// Start refinement - this would typically trigger a command
			// For now, just mark as done
			m.refineDone = true
		}
		return m, nil
	}

	// Update the text input
	var cmd tea.Cmd
	m.refineInput, cmd = m.refineInput.Update(msg)
	return m, cmd
}

// loadVersionList loads versions for the selected prompt
func (m *Model) loadVersionList() {
	if m.store == nil || len(m.list.Items()) == 0 {
		return
	}

	item, ok := m.list.SelectedItem().(PromptItem)
	if !ok {
		return
	}

	versions, err := m.store.ListVersions(item.Path)
	if err != nil {
		m.versions = nil
		return
	}
	m.versions = versions
}

// SetSize sets the component dimensions
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetSize(width/3, height-4)
}

// SetFocusLeft sets whether the left pane is focused
func (m *Model) SetFocusLeft(focused bool) {
	m.focusLeft = focused
}

// SelectedPrompt returns the currently selected prompt
func (m Model) SelectedPrompt() (prompt.Prompt, bool) {
	if item, ok := m.list.SelectedItem().(PromptItem); ok {
		return item.Prompt, true
	}
	return prompt.Prompt{}, false
}

// InjectMethod returns the current injection method
func (m Model) InjectMethod() prompt.InjectionMethod {
	return m.injectMethod
}

// SetInjectMethod sets the injection method
func (m *Model) SetInjectMethod(method prompt.InjectionMethod) {
	m.injectMethod = method
}

// IsRefining returns whether we're in refine mode
func (m Model) IsRefining() bool {
	return m.refining
}

// ShowingVersions returns whether we're in version view mode
func (m Model) ShowingVersions() bool {
	return m.showVersions
}

// Store returns the prompt store
func (m Model) Store() *prompt.Store {
	return m.store
}

// List returns the bubbles/list model
func (m Model) List() list.Model {
	return m.list
}
