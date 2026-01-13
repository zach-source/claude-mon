package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all configuration options
type Config struct {
	Theme     string      `toml:"theme"`
	LeaderKey string      `toml:"leader_key"`
	Keys      KeyBindings `toml:"keys"`
}

// KeyBindings holds all configurable key bindings
type KeyBindings struct {
	// Global
	Quit           string `toml:"quit"`
	Help           string `toml:"help"`
	NextTab        string `toml:"next_tab"`
	PrevTab        string `toml:"prev_tab"`
	LeftPane       string `toml:"left_pane"`
	RightPane      string `toml:"right_pane"`
	ToggleMinimap  string `toml:"toggle_minimap"`
	ToggleChat     string `toml:"toggle_chat"`
	ToggleLeftPane string `toml:"toggle_left_pane"`

	// Navigation
	Up       string `toml:"up"`
	Down     string `toml:"down"`
	PageUp   string `toml:"page_up"`
	PageDown string `toml:"page_down"`
	Next     string `toml:"next"`
	Prev     string `toml:"prev"`

	// History mode
	ClearHistory string `toml:"clear_history"`
	OpenInNvim   string `toml:"open_in_nvim"`
	OpenNvimCwd  string `toml:"open_nvim_cwd"`
	ScrollLeft   string `toml:"scroll_left"`
	ScrollRight  string `toml:"scroll_right"`

	// Prompts mode
	NewPrompt       string `toml:"new_prompt"`
	NewGlobalPrompt string `toml:"new_global_prompt"`
	EditPrompt      string `toml:"edit_prompt"`
	RefinePrompt    string `toml:"refine_prompt"`
	DeletePrompt    string `toml:"delete_prompt"`
	YankPrompt      string `toml:"yank_prompt"`
	InjectMethod    string `toml:"inject_method"`
	SendPrompt      string `toml:"send_prompt"`
	CreateVersion   string `toml:"create_version"`
	ViewVersions    string `toml:"view_versions"`
	RevertVersion   string `toml:"revert_version"`

	// Ralph mode
	CancelRalph string `toml:"cancel_ralph"`
	Refresh     string `toml:"refresh"`

	// Plan mode
	GeneratePlan string `toml:"generate_plan"`
	EditPlan     string `toml:"edit_plan"`

	// Chat mode
	SendChat  string `toml:"send_chat"`
	CloseChat string `toml:"close_chat"`
	KillChat  string `toml:"kill_chat"`
	ClearChat string `toml:"clear_chat"`
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		Theme:     "dark",
		LeaderKey: "ctrl+g",
		Keys: KeyBindings{
			// Global
			Quit:           "q",
			Help:           "?",
			NextTab:        "tab",
			PrevTab:        "shift+tab",
			LeftPane:       "[",
			RightPane:      "]",
			ToggleMinimap:  "m",
			ToggleChat:     "c",
			ToggleLeftPane: "h",

			// Navigation
			Up:       "k",
			Down:     "j",
			PageUp:   "u",
			PageDown: "d",
			Next:     "n",
			Prev:     "p",

			// History mode
			ClearHistory: "C",
			OpenInNvim:   "ctrl+n",
			OpenNvimCwd:  "ctrl+o",
			ScrollLeft:   "left",
			ScrollRight:  "right",

			// Prompts mode
			NewPrompt:       "n",
			NewGlobalPrompt: "N",
			EditPrompt:      "e",
			RefinePrompt:    "r",
			DeletePrompt:    "ctrl+d",
			YankPrompt:      "y",
			InjectMethod:    "i",
			SendPrompt:      "enter",
			CreateVersion:   "v",
			ViewVersions:    "V",
			RevertVersion:   "r",

			// Ralph mode
			CancelRalph: "C",
			Refresh:     "r",

			// Plan mode
			GeneratePlan: "G",
			EditPlan:     "e",

			// Chat mode
			SendChat:  "enter",
			CloseChat: "esc",
			KillChat:  "ctrl+c",
			ClearChat: "ctrl+l",
		},
	}
}

// Load loads configuration from the config file, falling back to defaults
func Load() (*Config, error) {
	cfg := DefaultConfig()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil // Use defaults
	}

	configPath := filepath.Join(home, ".config", "claude-follow", "config.toml")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return cfg, nil // Use defaults
	}

	// Decode config file
	if _, err := toml.DecodeFile(configPath, cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

// Path returns the path to the config file
func Path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "claude-follow", "config.toml")
}

// EnsureDir creates the config directory if it doesn't exist
func EnsureDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configDir := filepath.Join(home, ".config", "claude-follow")
	return os.MkdirAll(configDir, 0755)
}

// WriteDefault writes a default config file with comments
func WriteDefault() error {
	if err := EnsureDir(); err != nil {
		return err
	}

	defaultConfig := `# Claude Follow TUI Configuration
# Location: ~/.config/claude-follow/config.toml

# Theme: dark, light, dracula, monokai, gruvbox, nord, catppuccin
theme = "dark"

# Leader key for which-key popup (like tmux/vim)
# Press this key to see available commands
leader_key = "ctrl+g"

[keys]
# Global shortcuts
quit = "q"
help = "?"
next_tab = "tab"
prev_tab = "shift+tab"
left_pane = "["
right_pane = "]"
toggle_minimap = "m"
toggle_chat = "c"
toggle_left_pane = "h"

# Navigation (used in multiple modes)
up = "k"
down = "j"
page_up = "u"
page_down = "d"
next = "n"
prev = "p"

# History mode
clear_history = "C"
open_in_nvim = "ctrl+n"
open_nvim_cwd = "ctrl+o"
scroll_left = "left"
scroll_right = "right"

# Prompts mode
new_prompt = "n"
new_global_prompt = "N"
edit_prompt = "e"
refine_prompt = "r"
delete_prompt = "ctrl+d"
yank_prompt = "y"
inject_method = "i"
send_prompt = "enter"
create_version = "v"
view_versions = "V"
revert_version = "r"

# Ralph mode
cancel_ralph = "C"
refresh = "r"

# Plan mode
generate_plan = "G"
edit_plan = "e"

# Chat mode
send_chat = "enter"
close_chat = "esc"
kill_chat = "ctrl+c"
clear_chat = "ctrl+l"
`

	return os.WriteFile(Path(), []byte(defaultConfig), 0644)
}
