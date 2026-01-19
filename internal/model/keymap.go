package model

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/ztaylor/claude-mon/internal/config"
)

// KeyMap holds all keybindings with help text for bubbles/help integration.
// Implements help.KeyMap interface via ShortHelp() and FullHelp() methods.
type KeyMap struct {
	// Global
	Quit           key.Binding
	Help           key.Binding
	NextTab        key.Binding
	PrevTab        key.Binding
	LeftPane       key.Binding
	RightPane      key.Binding
	ToggleMinimap  key.Binding
	ToggleLeftPane key.Binding

	// Navigation
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Next     key.Binding
	Prev     key.Binding

	// History mode
	ClearHistory key.Binding
	OpenInNvim   key.Binding
	OpenNvimCwd  key.Binding
	ScrollLeft   key.Binding
	ScrollRight  key.Binding

	// Prompts mode
	NewPrompt       key.Binding
	NewGlobalPrompt key.Binding
	EditPrompt      key.Binding
	DeletePrompt    key.Binding
	YankPrompt      key.Binding
	InjectMethod    key.Binding
	SendPrompt      key.Binding
	CreateVersion   key.Binding
	ViewVersions    key.Binding
	RevertVersion   key.Binding
	FilterPrompts   key.Binding // fzf fuzzy filter
	FilterScope     key.Binding // cycle all/project/global

	// Ralph mode
	CancelRalph key.Binding
	Refresh     key.Binding

	// Plan mode
	GeneratePlan key.Binding
	EditPlan     key.Binding
}

// NewKeyMap creates a KeyMap with default bindings
func NewKeyMap() KeyMap {
	return KeyMap{
		// Global
		Quit:           key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Help:           key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		NextTab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
		PrevTab:        key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev tab")),
		LeftPane:       key.NewBinding(key.WithKeys("["), key.WithHelp("[", "left pane")),
		RightPane:      key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "right pane")),
		ToggleMinimap:  key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "minimap")),
		ToggleLeftPane: key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "toggle left")),

		// Navigation
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		PageUp:   key.NewBinding(key.WithKeys("u", "pgup"), key.WithHelp("u", "page up")),
		PageDown: key.NewBinding(key.WithKeys("d", "pgdown"), key.WithHelp("d", "page down")),
		Next:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
		Prev:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev")),

		// History mode
		ClearHistory: key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "clear history")),
		OpenInNvim:   key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("C-n", "open in nvim")),
		OpenNvimCwd:  key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("C-o", "nvim cwd")),
		ScrollLeft:   key.NewBinding(key.WithKeys("left"), key.WithHelp("←", "scroll left")),
		ScrollRight:  key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "scroll right")),

		// Prompts mode
		NewPrompt:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new prompt")),
		NewGlobalPrompt: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "new global")),
		EditPrompt:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		DeletePrompt:    key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("C-d", "delete")),
		YankPrompt:      key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yank")),
		InjectMethod:    key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "inject")),
		SendPrompt:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "send")),
		CreateVersion:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "version")),
		ViewVersions:    key.NewBinding(key.WithKeys("V"), key.WithHelp("V", "view versions")),
		RevertVersion:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "revert")),
		FilterPrompts:   key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "fzf filter")),
		FilterScope:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "scope")),

		// Ralph mode
		CancelRalph: key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "cancel ralph")),
		Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),

		// Plan mode
		GeneratePlan: key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "generate plan")),
		EditPlan:     key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit plan")),
	}
}

// FromConfig creates a KeyMap from user configuration, preserving backwards compatibility
func FromConfig(cfg *config.Config) KeyMap {
	km := NewKeyMap()

	// Global
	if cfg.Keys.Quit != "" {
		km.Quit = key.NewBinding(key.WithKeys(cfg.Keys.Quit), key.WithHelp(cfg.Keys.Quit, "quit"))
	}
	if cfg.Keys.Help != "" {
		km.Help = key.NewBinding(key.WithKeys(cfg.Keys.Help), key.WithHelp(cfg.Keys.Help, "help"))
	}
	if cfg.Keys.NextTab != "" {
		km.NextTab = key.NewBinding(key.WithKeys(cfg.Keys.NextTab), key.WithHelp(cfg.Keys.NextTab, "next tab"))
	}
	if cfg.Keys.PrevTab != "" {
		km.PrevTab = key.NewBinding(key.WithKeys(cfg.Keys.PrevTab), key.WithHelp(cfg.Keys.PrevTab, "prev tab"))
	}
	if cfg.Keys.LeftPane != "" {
		km.LeftPane = key.NewBinding(key.WithKeys(cfg.Keys.LeftPane), key.WithHelp(cfg.Keys.LeftPane, "left pane"))
	}
	if cfg.Keys.RightPane != "" {
		km.RightPane = key.NewBinding(key.WithKeys(cfg.Keys.RightPane), key.WithHelp(cfg.Keys.RightPane, "right pane"))
	}
	if cfg.Keys.ToggleMinimap != "" {
		km.ToggleMinimap = key.NewBinding(key.WithKeys(cfg.Keys.ToggleMinimap), key.WithHelp(cfg.Keys.ToggleMinimap, "minimap"))
	}
	if cfg.Keys.ToggleLeftPane != "" {
		km.ToggleLeftPane = key.NewBinding(key.WithKeys(cfg.Keys.ToggleLeftPane), key.WithHelp(cfg.Keys.ToggleLeftPane, "toggle left"))
	}

	// Navigation
	if cfg.Keys.Up != "" {
		km.Up = key.NewBinding(key.WithKeys(cfg.Keys.Up, "up"), key.WithHelp(cfg.Keys.Up, "up"))
	}
	if cfg.Keys.Down != "" {
		km.Down = key.NewBinding(key.WithKeys(cfg.Keys.Down, "down"), key.WithHelp(cfg.Keys.Down, "down"))
	}
	if cfg.Keys.PageUp != "" {
		km.PageUp = key.NewBinding(key.WithKeys(cfg.Keys.PageUp), key.WithHelp(cfg.Keys.PageUp, "page up"))
	}
	if cfg.Keys.PageDown != "" {
		km.PageDown = key.NewBinding(key.WithKeys(cfg.Keys.PageDown), key.WithHelp(cfg.Keys.PageDown, "page down"))
	}
	if cfg.Keys.Next != "" {
		km.Next = key.NewBinding(key.WithKeys(cfg.Keys.Next), key.WithHelp(cfg.Keys.Next, "next"))
	}
	if cfg.Keys.Prev != "" {
		km.Prev = key.NewBinding(key.WithKeys(cfg.Keys.Prev), key.WithHelp(cfg.Keys.Prev, "prev"))
	}

	// History mode
	if cfg.Keys.ClearHistory != "" {
		km.ClearHistory = key.NewBinding(key.WithKeys(cfg.Keys.ClearHistory), key.WithHelp(cfg.Keys.ClearHistory, "clear history"))
	}
	if cfg.Keys.OpenInNvim != "" {
		km.OpenInNvim = key.NewBinding(key.WithKeys(cfg.Keys.OpenInNvim), key.WithHelp(cfg.Keys.OpenInNvim, "open in nvim"))
	}
	if cfg.Keys.OpenNvimCwd != "" {
		km.OpenNvimCwd = key.NewBinding(key.WithKeys(cfg.Keys.OpenNvimCwd), key.WithHelp(cfg.Keys.OpenNvimCwd, "nvim cwd"))
	}
	if cfg.Keys.ScrollLeft != "" {
		km.ScrollLeft = key.NewBinding(key.WithKeys(cfg.Keys.ScrollLeft), key.WithHelp(cfg.Keys.ScrollLeft, "scroll left"))
	}
	if cfg.Keys.ScrollRight != "" {
		km.ScrollRight = key.NewBinding(key.WithKeys(cfg.Keys.ScrollRight), key.WithHelp(cfg.Keys.ScrollRight, "scroll right"))
	}

	// Prompts mode
	if cfg.Keys.NewPrompt != "" {
		km.NewPrompt = key.NewBinding(key.WithKeys(cfg.Keys.NewPrompt), key.WithHelp(cfg.Keys.NewPrompt, "new prompt"))
	}
	if cfg.Keys.NewGlobalPrompt != "" {
		km.NewGlobalPrompt = key.NewBinding(key.WithKeys(cfg.Keys.NewGlobalPrompt), key.WithHelp(cfg.Keys.NewGlobalPrompt, "new global"))
	}
	if cfg.Keys.EditPrompt != "" {
		km.EditPrompt = key.NewBinding(key.WithKeys(cfg.Keys.EditPrompt), key.WithHelp(cfg.Keys.EditPrompt, "edit"))
	}
	if cfg.Keys.DeletePrompt != "" {
		km.DeletePrompt = key.NewBinding(key.WithKeys(cfg.Keys.DeletePrompt), key.WithHelp(cfg.Keys.DeletePrompt, "delete"))
	}
	if cfg.Keys.YankPrompt != "" {
		km.YankPrompt = key.NewBinding(key.WithKeys(cfg.Keys.YankPrompt), key.WithHelp(cfg.Keys.YankPrompt, "yank"))
	}
	if cfg.Keys.InjectMethod != "" {
		km.InjectMethod = key.NewBinding(key.WithKeys(cfg.Keys.InjectMethod), key.WithHelp(cfg.Keys.InjectMethod, "inject"))
	}
	if cfg.Keys.SendPrompt != "" {
		km.SendPrompt = key.NewBinding(key.WithKeys(cfg.Keys.SendPrompt), key.WithHelp(cfg.Keys.SendPrompt, "send"))
	}
	if cfg.Keys.CreateVersion != "" {
		km.CreateVersion = key.NewBinding(key.WithKeys(cfg.Keys.CreateVersion), key.WithHelp(cfg.Keys.CreateVersion, "version"))
	}
	if cfg.Keys.ViewVersions != "" {
		km.ViewVersions = key.NewBinding(key.WithKeys(cfg.Keys.ViewVersions), key.WithHelp(cfg.Keys.ViewVersions, "view versions"))
	}
	if cfg.Keys.RevertVersion != "" {
		km.RevertVersion = key.NewBinding(key.WithKeys(cfg.Keys.RevertVersion), key.WithHelp(cfg.Keys.RevertVersion, "revert"))
	}

	// Ralph mode
	if cfg.Keys.CancelRalph != "" {
		km.CancelRalph = key.NewBinding(key.WithKeys(cfg.Keys.CancelRalph), key.WithHelp(cfg.Keys.CancelRalph, "cancel ralph"))
	}
	if cfg.Keys.Refresh != "" {
		km.Refresh = key.NewBinding(key.WithKeys(cfg.Keys.Refresh), key.WithHelp(cfg.Keys.Refresh, "refresh"))
	}

	// Plan mode
	if cfg.Keys.GeneratePlan != "" {
		km.GeneratePlan = key.NewBinding(key.WithKeys(cfg.Keys.GeneratePlan), key.WithHelp(cfg.Keys.GeneratePlan, "generate plan"))
	}
	if cfg.Keys.EditPlan != "" {
		km.EditPlan = key.NewBinding(key.WithKeys(cfg.Keys.EditPlan), key.WithHelp(cfg.Keys.EditPlan, "edit plan"))
	}

	return km
}

// ShortHelp returns key bindings for the short help view (implements help.KeyMap)
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit, k.NextTab, k.Up, k.Down}
}

// FullHelp returns key bindings for the full help view (implements help.KeyMap)
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		// Row 1: Global navigation
		{k.NextTab, k.PrevTab, k.LeftPane, k.RightPane, k.ToggleLeftPane},
		// Row 2: Movement
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Next, k.Prev},
		// Row 3: Actions
		{k.ToggleMinimap, k.Help, k.Quit},
	}
}

// HistoryHelp returns keybindings relevant to history mode
func (k KeyMap) HistoryHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.ScrollLeft, k.ScrollRight, k.OpenInNvim, k.OpenNvimCwd},
		{k.ClearHistory, k.Next, k.Prev},
	}
}

// PromptsHelp returns keybindings relevant to prompts mode
func (k KeyMap) PromptsHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.SendPrompt, k.EditPrompt},
		{k.NewPrompt, k.NewGlobalPrompt, k.DeletePrompt},
		{k.YankPrompt, k.InjectMethod},
		{k.CreateVersion, k.ViewVersions, k.RevertVersion},
		{k.FilterPrompts, k.FilterScope},
	}
}

// RalphHelp returns keybindings relevant to ralph mode
func (k KeyMap) RalphHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.CancelRalph, k.Refresh},
	}
}

// PlanHelp returns keybindings relevant to plan mode
func (k KeyMap) PlanHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown},
		{k.GeneratePlan, k.EditPlan},
	}
}

// ContextHelp returns keybindings relevant to context mode
func (k KeyMap) ContextHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Refresh},
	}
}

// ModeKeyMap wraps KeyMap to provide mode-specific help
type ModeKeyMap struct {
	KeyMap
	mode string
}

// NewModeKeyMap creates a ModeKeyMap for a specific mode
func NewModeKeyMap(km KeyMap, mode string) ModeKeyMap {
	return ModeKeyMap{KeyMap: km, mode: mode}
}

// FullHelp returns mode-specific keybindings (implements help.KeyMap)
func (m ModeKeyMap) FullHelp() [][]key.Binding {
	switch m.mode {
	case "history":
		return m.KeyMap.HistoryHelp()
	case "prompts":
		return m.KeyMap.PromptsHelp()
	case "ralph":
		return m.KeyMap.RalphHelp()
	case "plan":
		return m.KeyMap.PlanHelp()
	case "context":
		return m.KeyMap.ContextHelp()
	default:
		return m.KeyMap.FullHelp()
	}
}
