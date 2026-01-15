package model

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// LayoutConfig holds dimensions for layout calculations.
// Used by View() to calculate pane sizes and overlay positioning.
type LayoutConfig struct {
	Width        int
	Height       int
	HeaderHeight int
	FooterHeight int
	MinLeftWidth int
	MaxLeftRatio float64
	MinimapWidth int
	ShowLeftPane bool
	ShowMinimap  bool
}

// DefaultLayoutConfig returns sensible defaults for layout
func DefaultLayoutConfig(width, height int) LayoutConfig {
	return LayoutConfig{
		Width:        width,
		Height:       height,
		HeaderHeight: 1,
		FooterHeight: 1,
		MinLeftWidth: 25,
		MaxLeftRatio: 0.5,
		MinimapWidth: 2,
		ShowLeftPane: true,
		ShowMinimap:  true,
	}
}

// ContentHeight returns available height for content
func (c LayoutConfig) ContentHeight() int {
	return c.Height - c.HeaderHeight - c.FooterHeight - 2
}

// CalculatePaneWidths returns left and right pane widths
func (c LayoutConfig) CalculatePaneWidths(leftContentWidth int) (leftWidth, rightWidth int) {
	minimapWidth := 0
	if c.ShowMinimap {
		minimapWidth = c.MinimapWidth
	}

	if !c.ShowLeftPane {
		return 0, c.Width - 2 - minimapWidth
	}

	maxWidth := int(float64(c.Width) * c.MaxLeftRatio)

	if leftContentWidth < c.MinLeftWidth {
		leftWidth = c.MinLeftWidth
	} else if leftContentWidth > maxWidth {
		leftWidth = maxWidth
	} else {
		leftWidth = leftContentWidth
	}

	rightWidth = c.Width - leftWidth - 3 - minimapWidth
	return leftWidth, rightWidth
}

// CenterOverlay places content centered over a base view
func CenterOverlay(base, overlay string, width, height int) string {
	overlayWidth := lipgloss.Width(overlay)
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	// Center vertically
	startLineIdx := (len(baseLines) - len(overlayLines)) / 2
	if startLineIdx < 0 {
		startLineIdx = 0
	}

	// Center horizontally
	targetPos := (width - overlayWidth) / 2
	if targetPos < 0 {
		targetPos = 0
	}

	// Replace lines with centered overlay content
	for i, overlayLine := range overlayLines {
		lineIdx := startLineIdx + i
		if lineIdx >= 0 && lineIdx < len(baseLines) {
			padding := strings.Repeat(" ", targetPos)
			baseLines[lineIdx] = padding + overlayLine
		}
	}

	return strings.Join(baseLines, "\n")
}

// BottomOverlay places content centered at the bottom of a base view
func BottomOverlay(base, overlay string, width int, offsetFromBottom int) string {
	overlayWidth := lipgloss.Width(overlay)
	overlayLines := strings.Split(overlay, "\n")
	baseLines := strings.Split(base, "\n")

	// Position from bottom
	startLineIdx := len(baseLines) - offsetFromBottom - len(overlayLines)
	if startLineIdx < 0 {
		startLineIdx = 0
	}

	// Center horizontally
	targetPos := (width - overlayWidth) / 2
	if targetPos < 0 {
		targetPos = 0
	}

	// Replace lines with centered overlay content
	for i, overlayLine := range overlayLines {
		lineIdx := startLineIdx + i
		if lineIdx >= 0 && lineIdx < len(baseLines) {
			padding := strings.Repeat(" ", targetPos)
			baseLines[lineIdx] = padding + overlayLine
		}
	}

	return strings.Join(baseLines, "\n")
}

// TwoPaneLayout creates a two-pane horizontal layout with optional minimap
func TwoPaneLayout(left, right, minimap string, leftStyle, rightStyle lipgloss.Style, cfg LayoutConfig) string {
	leftWidth := lipgloss.Width(left)
	leftPaneWidth, rightPaneWidth := cfg.CalculatePaneWidths(leftWidth)

	contentHeight := cfg.ContentHeight()

	leftPane := leftStyle.
		Width(leftPaneWidth).
		Height(contentHeight).
		Render(left)

	rightPane := rightStyle.
		Width(rightPaneWidth).
		Height(contentHeight).
		Render(right)

	if cfg.ShowMinimap && minimap != "" {
		return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane, minimap)
	}

	if cfg.ShowLeftPane {
		return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	}

	return rightPane
}

// FullWidthLayout creates a single full-width pane layout
func FullWidthLayout(content, minimap string, style lipgloss.Style, cfg LayoutConfig) string {
	minimapWidth := 0
	if cfg.ShowMinimap {
		minimapWidth = cfg.MinimapWidth
	}

	pane := style.
		Width(cfg.Width - 2 - minimapWidth).
		Height(cfg.ContentHeight()).
		Render(content)

	if cfg.ShowMinimap && minimap != "" {
		return lipgloss.JoinHorizontal(lipgloss.Top, pane, minimap)
	}

	return pane
}
