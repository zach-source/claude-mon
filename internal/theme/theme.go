package theme

import "github.com/charmbracelet/lipgloss"

// Theme defines all colors used throughout the TUI
type Theme struct {
	Name string

	// UI Chrome
	Title        lipgloss.Style
	Border       lipgloss.Style
	ActiveBorder lipgloss.Style
	Selected     lipgloss.Style
	Normal       lipgloss.Style
	Dim          lipgloss.Style
	Status       lipgloss.Style
	Help         lipgloss.Style

	// Diff Colors
	Added            lipgloss.Style
	Removed          lipgloss.Style
	Modified         lipgloss.Style // For word-level changes within a line
	Context          lipgloss.Style // Unchanged context lines
	DiffHeader       lipgloss.Style // @@ -1,3 +1,5 @@
	LineNumber       lipgloss.Style
	LineNumberActive lipgloss.Style

	// Syntax Highlighting (maps to Chroma token types)
	Keyword     lipgloss.Style
	String      lipgloss.Style
	Number      lipgloss.Style
	Comment     lipgloss.Style
	Function    lipgloss.Style
	Type        lipgloss.Style
	Operator    lipgloss.Style
	Punctuation lipgloss.Style

	// Background colors for diff + syntax layering
	AddedBg       lipgloss.Color
	RemovedBg     lipgloss.Color
	ChangedLineBg lipgloss.Color // Soft highlight for changed lines

	// Scrollbar/minimap colors
	ScrollbarBg     lipgloss.Color
	ScrollbarThumb  lipgloss.Color
	ScrollbarActive lipgloss.Color

	// Chroma style name for advanced highlighting
	ChromaStyle string
}

// Default returns the default dark theme
func Default() *Theme {
	return Dark()
}

// Get retrieves a theme by name
func Get(name string) *Theme {
	switch name {
	case "light":
		return Light()
	case "dracula":
		return Dracula()
	case "monokai":
		return Monokai()
	case "gruvbox":
		return Gruvbox()
	case "nord":
		return Nord()
	case "catppuccin":
		return Catppuccin()
	case "dark":
		return Dark()
	default:
		return Dark()
	}
}

// Available returns list of available theme names
func Available() []string {
	return []string{"dark", "light", "dracula", "monokai", "gruvbox", "nord", "catppuccin"}
}
