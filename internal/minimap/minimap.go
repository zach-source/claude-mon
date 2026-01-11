package minimap

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ztaylor/claude-follow-tui/internal/theme"
)

// LineType represents the type of a line for minimap coloring
type LineType int

const (
	LineContext LineType = iota
	LineAdded
	LineRemoved
)

// Block characters for rendering
const (
	BlockFull   = "█" // Full block for added lines
	BlockDark   = "▓" // Dark shade for removed lines
	BlockLight  = "░" // Light shade for context
	BlockMedium = "▒" // Medium shade for mixed
)

// Minimap represents a compressed view of file content with diff info
type Minimap struct {
	lines      []LineType // Type for each line in the file
	totalLines int
}

// New creates a new Minimap with the given total lines
func New(totalLines int) *Minimap {
	if totalLines < 0 {
		totalLines = 0
	}
	return &Minimap{
		lines:      make([]LineType, totalLines),
		totalLines: totalLines,
	}
}

// SetLine sets the type for a specific line (0-indexed)
func (m *Minimap) SetLine(lineNum int, lineType LineType) {
	if lineNum >= 0 && lineNum < len(m.lines) {
		m.lines[lineNum] = lineType
	}
}

// SetRange sets the type for a range of lines [start, end) (0-indexed)
func (m *Minimap) SetRange(start, end int, lineType LineType) {
	if start < 0 {
		start = 0
	}
	if end > len(m.lines) {
		end = len(m.lines)
	}
	for i := start; i < end; i++ {
		m.lines[i] = lineType
	}
}

// TotalLines returns the total number of lines
func (m *Minimap) TotalLines() int {
	return m.totalLines
}

// getDominantType returns the most common line type in a range
func (m *Minimap) getDominantType(start, end int) LineType {
	if start < 0 {
		start = 0
	}
	if end > len(m.lines) {
		end = len(m.lines)
	}
	if start >= end {
		return LineContext
	}

	counts := make(map[LineType]int)
	for i := start; i < end; i++ {
		counts[m.lines[i]]++
	}

	// Priority: Added > Removed > Context
	// This ensures diff regions are visible even when compressed
	if counts[LineAdded] > 0 {
		return LineAdded
	}
	if counts[LineRemoved] > 0 {
		return LineRemoved
	}
	return LineContext
}

// Render generates the minimap string with colors
// height: display height in rows
// viewportStart: first visible line in the viewport (0-indexed)
// viewportEnd: last visible line in the viewport (0-indexed)
// t: theme for colors
func (m *Minimap) Render(height int, viewportStart, viewportEnd int, t *theme.Theme) string {
	if height < 1 || m.totalLines < 1 {
		return ""
	}

	var sb strings.Builder

	// Calculate lines per display row
	linesPerRow := float64(m.totalLines) / float64(height)
	if linesPerRow < 1 {
		linesPerRow = 1
	}

	// Calculate viewport position in minimap coordinates
	vpStartRow := int(float64(viewportStart) / linesPerRow)
	vpEndRow := int(float64(viewportEnd) / linesPerRow)
	if vpEndRow >= height {
		vpEndRow = height - 1
	}

	// Styles for different line types
	addedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1"))   // Green
	removedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")) // Red
	contextStyle := lipgloss.NewStyle().Foreground(t.ScrollbarBg)

	// Brighter versions for viewport indicator
	addedVpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Background(t.ScrollbarThumb)
	removedVpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#f38ba8")).Background(t.ScrollbarThumb)
	contextVpStyle := lipgloss.NewStyle().Foreground(t.ScrollbarActive).Background(t.ScrollbarThumb)

	for row := 0; row < height; row++ {
		// Calculate which file lines this row represents
		startLine := int(float64(row) * linesPerRow)
		endLine := int(float64(row+1) * linesPerRow)
		if endLine > m.totalLines {
			endLine = m.totalLines
		}

		// Get dominant type for this range
		lineType := m.getDominantType(startLine, endLine)

		// Check if this row is within the viewport
		inViewport := row >= vpStartRow && row <= vpEndRow

		// Select character and style based on type and viewport
		var char string
		var style lipgloss.Style

		switch lineType {
		case LineAdded:
			char = BlockFull
			if inViewport {
				style = addedVpStyle
			} else {
				style = addedStyle
			}
		case LineRemoved:
			char = BlockDark
			if inViewport {
				style = removedVpStyle
			} else {
				style = removedStyle
			}
		default: // LineContext
			char = BlockLight
			if inViewport {
				style = contextVpStyle
			} else {
				style = contextStyle
			}
		}

		// Render two characters wide for visibility
		sb.WriteString(style.Render(char + char))
		sb.WriteString("\n")
	}

	return sb.String()
}
