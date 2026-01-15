package history

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/ztaylor/claude-mon/internal/config"
)

// FromConfig creates a KeyMap from user configuration
func FromConfig(cfg *config.Config) KeyMap {
	km := DefaultKeyMap()

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
		km.Next = key.NewBinding(key.WithKeys(cfg.Keys.Next), key.WithHelp(cfg.Keys.Next, "next change"))
	}
	if cfg.Keys.Prev != "" {
		km.Prev = key.NewBinding(key.WithKeys(cfg.Keys.Prev), key.WithHelp(cfg.Keys.Prev, "prev change"))
	}

	// History-specific
	if cfg.Keys.ScrollLeft != "" {
		km.ScrollLeft = key.NewBinding(key.WithKeys(cfg.Keys.ScrollLeft), key.WithHelp(cfg.Keys.ScrollLeft, "scroll left"))
	}
	if cfg.Keys.ScrollRight != "" {
		km.ScrollRight = key.NewBinding(key.WithKeys(cfg.Keys.ScrollRight), key.WithHelp(cfg.Keys.ScrollRight, "scroll right"))
	}
	if cfg.Keys.ClearHistory != "" {
		km.ClearHistory = key.NewBinding(key.WithKeys(cfg.Keys.ClearHistory), key.WithHelp(cfg.Keys.ClearHistory, "clear history"))
	}
	if cfg.Keys.OpenInNvim != "" {
		km.OpenInNvim = key.NewBinding(key.WithKeys(cfg.Keys.OpenInNvim), key.WithHelp(cfg.Keys.OpenInNvim, "open in nvim"))
	}
	if cfg.Keys.OpenNvimCwd != "" {
		km.OpenNvimCwd = key.NewBinding(key.WithKeys(cfg.Keys.OpenNvimCwd), key.WithHelp(cfg.Keys.OpenNvimCwd, "nvim cwd"))
	}

	return km
}
