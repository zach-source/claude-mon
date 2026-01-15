package theme

import "github.com/charmbracelet/lipgloss"

// Dark is the default dark theme (monokai-inspired)
func Dark() *Theme {
	return &Theme{
		Name:        "dark",
		ChromaStyle: "monokai",

		// UI Chrome
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0),
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205")).Padding(0),
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")),
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("241")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),

		// Syntax (monokai-style)
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("197")),
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("186")),
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("141")),
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("59")).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("197")),
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("252")),

		AddedBg:         lipgloss.Color("22"),
		RemovedBg:       lipgloss.Color("52"),
		ChangedLineBg:   lipgloss.Color("236"),
		ScrollbarBg:     lipgloss.Color("235"),
		ScrollbarThumb:  lipgloss.Color("240"),
		ScrollbarActive: lipgloss.Color("205"),
	}
}

// Light is a light theme for bright terminals
func Light() *Theme {
	return &Theme{
		Name:        "light",
		ChromaStyle: "github",

		// UI Chrome
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("91")),
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("250")).Padding(0),
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("91")).Padding(0),
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("25")),
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("235")),
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("245")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("28")),
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("124")),
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("130")).Bold(true),
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("25")).Bold(true),
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("130")),

		// Syntax (github-style)
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("127")),
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("22")),
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("21")),
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("130")),
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("25")),
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("235")),
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("235")),

		AddedBg:         lipgloss.Color("194"),
		RemovedBg:       lipgloss.Color("224"),
		ChangedLineBg:   lipgloss.Color("254"),
		ScrollbarBg:     lipgloss.Color("253"),
		ScrollbarThumb:  lipgloss.Color("248"),
		ScrollbarActive: lipgloss.Color("91"),
	}
}

// Dracula is the popular Dracula theme
func Dracula() *Theme {
	return &Theme{
		Name:        "dracula",
		ChromaStyle: "dracula",

		// UI Chrome - Dracula palette
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff79c6")),
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#6272a4")).Padding(0),
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#bd93f9")).Padding(0),
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2")).Background(lipgloss.Color("#44475a")),
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2")),
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5555")),
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb86c")).Bold(true),
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")),
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")).Bold(true),
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("#44475a")),
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffb86c")),

		// Syntax
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("#ff79c6")),
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("#f1fa8c")),
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("#bd93f9")),
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6272a4")).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("#50fa7b")),
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd")),
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#ff79c6")),
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2")),

		AddedBg:         lipgloss.Color("#1e3a1e"),
		RemovedBg:       lipgloss.Color("#3a1e1e"),
		ChangedLineBg:   lipgloss.Color("#343746"),
		ScrollbarBg:     lipgloss.Color("#282a36"),
		ScrollbarThumb:  lipgloss.Color("#44475a"),
		ScrollbarActive: lipgloss.Color("#bd93f9"),
	}
}

// Monokai is the classic Monokai theme
func Monokai() *Theme {
	return &Theme{
		Name:        "monokai",
		ChromaStyle: "monokai",

		// UI Chrome
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#f92672")),
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#75715e")).Padding(0),
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#a6e22e")).Padding(0),
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2")).Background(lipgloss.Color("#49483e")),
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2")),
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("#75715e")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#75715e")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("#75715e")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e22e")),
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("#f92672")),
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("#fd971f")).Bold(true),
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("#75715e")),
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("#66d9ef")).Bold(true),
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("#49483e")),
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#fd971f")),

		// Syntax
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("#f92672")),
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("#e6db74")),
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("#ae81ff")),
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("#75715e")).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e22e")),
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("#66d9ef")),
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#f92672")),
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("#f8f8f2")),

		AddedBg:         lipgloss.Color("#1e3a1e"),
		RemovedBg:       lipgloss.Color("#3a1e1e"),
		ChangedLineBg:   lipgloss.Color("#3c3d37"),
		ScrollbarBg:     lipgloss.Color("#272822"),
		ScrollbarThumb:  lipgloss.Color("#49483e"),
		ScrollbarActive: lipgloss.Color("#a6e22e"),
	}
}

// Gruvbox is the Gruvbox dark theme
func Gruvbox() *Theme {
	return &Theme{
		Name:        "gruvbox",
		ChromaStyle: "gruvbox",

		// UI Chrome - Gruvbox palette
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#fe8019")),
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#665c54")).Padding(0),
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#fabd2f")).Padding(0),
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("#ebdbb2")).Background(lipgloss.Color("#504945")),
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("#ebdbb2")),
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("#b8bb26")),
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("#fb4934")),
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("#fabd2f")).Bold(true),
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")),
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("#83a598")).Bold(true),
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("#504945")),
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#fabd2f")),

		// Syntax
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("#fb4934")),
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("#b8bb26")),
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("#d3869b")),
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("#928374")).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("#b8bb26")),
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("#fabd2f")),
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#fe8019")),
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("#ebdbb2")),

		AddedBg:         lipgloss.Color("#1d2021"),
		RemovedBg:       lipgloss.Color("#3c1f1e"),
		ChangedLineBg:   lipgloss.Color("#3c3836"),
		ScrollbarBg:     lipgloss.Color("#1d2021"),
		ScrollbarThumb:  lipgloss.Color("#504945"),
		ScrollbarActive: lipgloss.Color("#fabd2f"),
	}
}

// Nord is the Nord theme
func Nord() *Theme {
	return &Theme{
		Name:        "nord",
		ChromaStyle: "nord",

		// UI Chrome - Nord palette
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#88c0d0")),
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#4c566a")).Padding(0),
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#88c0d0")).Padding(0),
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("#eceff4")).Background(lipgloss.Color("#434c5e")),
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("#eceff4")),
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")),
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("#a3be8c")),
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("#bf616a")),
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("#ebcb8b")).Bold(true),
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("#4c566a")),
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("#81a1c1")).Bold(true),
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("#3b4252")),
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#ebcb8b")),

		// Syntax
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("#81a1c1")),
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("#a3be8c")),
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("#b48ead")),
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("#616e88")).Italic(true),
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("#88c0d0")),
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("#8fbcbb")),
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#81a1c1")),
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("#eceff4")),

		AddedBg:         lipgloss.Color("#2e3440"),
		RemovedBg:       lipgloss.Color("#3b2b2b"),
		ChangedLineBg:   lipgloss.Color("#3b4252"),
		ScrollbarBg:     lipgloss.Color("#2e3440"),
		ScrollbarThumb:  lipgloss.Color("#4c566a"),
		ScrollbarActive: lipgloss.Color("#88c0d0"),
	}
}

// Catppuccin is the Catppuccin Mocha theme
func Catppuccin() *Theme {
	return &Theme{
		Name:        "catppuccin",
		ChromaStyle: "catppuccin-mocha",

		// UI Chrome - Catppuccin Mocha palette
		Title:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#cba6f7")),                                        // Mauve
		Border:       lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#585b70")).Padding(0), // Surface2
		ActiveBorder: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#89b4fa")).Padding(0), // Blue
		Selected:     lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")).Background(lipgloss.Color("#45475a")),             // Text on Surface1
		Normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")),                                                   // Text
		Dim:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")),                                                   // Overlay1
		Status:       lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")),
		Help:         lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")),

		// Diff Colors
		Added:            lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")),            // Green
		Removed:          lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")),            // Red
		Modified:         lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")).Bold(true), // Peach
		Context:          lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")),            // Overlay1
		DiffHeader:       lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")).Bold(true), // Blue
		LineNumber:       lipgloss.NewStyle().Foreground(lipgloss.Color("#45475a")),            // Surface1
		LineNumberActive: lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")),            // Peach

		// Syntax - Catppuccin style
		Keyword:     lipgloss.NewStyle().Foreground(lipgloss.Color("#cba6f7")),              // Mauve
		String:      lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")),              // Green
		Number:      lipgloss.NewStyle().Foreground(lipgloss.Color("#fab387")),              // Peach
		Comment:     lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Italic(true), // Overlay1
		Function:    lipgloss.NewStyle().Foreground(lipgloss.Color("#89b4fa")),              // Blue
		Type:        lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")),              // Yellow
		Operator:    lipgloss.NewStyle().Foreground(lipgloss.Color("#89dceb")),              // Sky
		Punctuation: lipgloss.NewStyle().Foreground(lipgloss.Color("#cdd6f4")),              // Text

		AddedBg:         lipgloss.Color("#1e3a29"),
		RemovedBg:       lipgloss.Color("#3a1e2a"),
		ChangedLineBg:   lipgloss.Color("#313244"), // Surface0
		ScrollbarBg:     lipgloss.Color("#1e1e2e"), // Base
		ScrollbarThumb:  lipgloss.Color("#45475a"), // Surface1
		ScrollbarActive: lipgloss.Color("#cba6f7"), // Mauve
	}
}
