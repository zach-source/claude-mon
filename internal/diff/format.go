package diff

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/ztaylor/claude-mon/internal/theme"
)

// DiffLine represents a single line in the formatted diff
type DiffLine struct {
	Type       DiffType
	OldLineNum int    // 0 if not applicable
	NewLineNum int    // 0 if not applicable
	Content    string // The line content
}

// DiffStats contains statistics about the diff
type DiffStats struct {
	Additions int
	Deletions int
}

// FormatOptions configures diff formatting
type FormatOptions struct {
	ContextLines int  // Number of context lines to show
	ShowStats    bool // Show addition/deletion stats
}

// DefaultOptions returns sensible default options
func DefaultOptions() FormatOptions {
	return FormatOptions{
		ContextLines: 3,
		ShowStats:    true,
	}
}

// FormatDiff formats a diff with line numbers and styling
func FormatDiff(oldText, newText string, t *theme.Theme, opts FormatOptions) string {
	if oldText == "" && newText == "" {
		return t.Dim.Render("No content to diff")
	}

	// Handle new file case
	if oldText == "" {
		return formatNewFile(newText, t)
	}

	// Handle deleted file case
	if newText == "" {
		return formatDeletedFile(oldText, t)
	}

	// Check if inputs are single-line (no newlines)
	// For single-line edits, show simple old/new format
	oldHasNewline := strings.Contains(oldText, "\n")
	newHasNewline := strings.Contains(newText, "\n")

	if !oldHasNewline && !newHasNewline {
		return formatSimpleDiff(oldText, newText, t)
	}

	// Ensure texts end with newlines for proper line-mode diff
	if !strings.HasSuffix(oldText, "\n") {
		oldText += "\n"
	}
	if !strings.HasSuffix(newText, "\n") {
		newText += "\n"
	}

	// Use line-mode diff for better results
	dmp := diffmatchpatch.New()

	// Convert to line-based diff
	a, b, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Convert to our line format
	lines := convertToLines(diffs)
	stats := computeStats(lines)

	var sb strings.Builder

	// Write header
	if opts.ShowStats {
		sb.WriteString(formatHeader(stats, t))
		sb.WriteString("\n")
	}

	// Write diff lines
	for _, line := range lines {
		sb.WriteString(formatLine(line, t))
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatSimpleDiff handles single-line changes with a clean display
func formatSimpleDiff(oldText, newText string, t *theme.Theme) string {
	var sb strings.Builder

	sb.WriteString(t.DiffHeader.Render("@@ -1 +1 @@"))
	sb.WriteString("\n")
	sb.WriteString(t.LineNumber.Render("   1     "))
	sb.WriteString(" ")
	sb.WriteString(t.Removed.Render("- " + oldText))
	sb.WriteString("\n")
	sb.WriteString(t.LineNumber.Render("      1  "))
	sb.WriteString(" ")
	sb.WriteString(t.Added.Render("+ " + newText))
	sb.WriteString("\n")

	return sb.String()
}

func formatNewFile(content string, t *theme.Theme) string {
	var sb strings.Builder
	sb.WriteString(t.DiffHeader.Render("@@ New file @@"))
	sb.WriteString("\n")

	lines := SplitLines(content)
	for i, line := range lines {
		lineNum := fmt.Sprintf("%4d", i+1)
		sb.WriteString(t.LineNumber.Render(lineNum))
		sb.WriteString(" ")
		sb.WriteString(t.Added.Render("+ " + line))
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatDeletedFile(content string, t *theme.Theme) string {
	var sb strings.Builder
	sb.WriteString(t.DiffHeader.Render("@@ Deleted file @@"))
	sb.WriteString("\n")

	lines := SplitLines(content)
	for i, line := range lines {
		lineNum := fmt.Sprintf("%4d", i+1)
		sb.WriteString(t.LineNumber.Render(lineNum))
		sb.WriteString(" ")
		sb.WriteString(t.Removed.Render("- " + line))
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatHeader(stats DiffStats, t *theme.Theme) string {
	header := fmt.Sprintf("@@ -%d +%d @@", stats.Deletions, stats.Additions)
	statsText := fmt.Sprintf("  %s, %s",
		t.Added.Render(fmt.Sprintf("+%d", stats.Additions)),
		t.Removed.Render(fmt.Sprintf("-%d", stats.Deletions)))

	return t.DiffHeader.Render(header) + statsText
}

func formatLine(line DiffLine, t *theme.Theme) string {
	// Format line numbers
	var lineNumStr string
	if line.OldLineNum > 0 && line.NewLineNum > 0 {
		lineNumStr = fmt.Sprintf("%4d %4d", line.OldLineNum, line.NewLineNum)
	} else if line.OldLineNum > 0 {
		lineNumStr = fmt.Sprintf("%4d     ", line.OldLineNum)
	} else if line.NewLineNum > 0 {
		lineNumStr = fmt.Sprintf("     %4d", line.NewLineNum)
	} else {
		lineNumStr = "         "
	}

	var prefix string
	var style lipgloss.Style

	switch line.Type {
	case DiffInsert:
		prefix = "+"
		style = t.Added
	case DiffDelete:
		prefix = "-"
		style = t.Removed
	case DiffEqual:
		prefix = " "
		style = t.Context
	}

	return t.LineNumber.Render(lineNumStr) + " " + style.Render(prefix+" "+line.Content)
}

func convertToLines(diffs []diffmatchpatch.Diff) []DiffLine {
	var result []DiffLine
	oldLineNum := 1
	newLineNum := 1

	for _, d := range diffs {
		// Split diff text into individual lines
		text := d.Text
		// Remove trailing newline for cleaner splitting
		text = strings.TrimSuffix(text, "\n")
		lines := strings.Split(text, "\n")

		for _, line := range lines {
			switch d.Type {
			case diffmatchpatch.DiffEqual:
				result = append(result, DiffLine{
					Type:       DiffEqual,
					OldLineNum: oldLineNum,
					NewLineNum: newLineNum,
					Content:    line,
				})
				oldLineNum++
				newLineNum++
			case diffmatchpatch.DiffInsert:
				result = append(result, DiffLine{
					Type:       DiffInsert,
					NewLineNum: newLineNum,
					Content:    line,
				})
				newLineNum++
			case diffmatchpatch.DiffDelete:
				result = append(result, DiffLine{
					Type:       DiffDelete,
					OldLineNum: oldLineNum,
					Content:    line,
				})
				oldLineNum++
			}
		}
	}

	return result
}

func computeStats(lines []DiffLine) DiffStats {
	var stats DiffStats
	for _, line := range lines {
		switch line.Type {
		case DiffInsert:
			stats.Additions++
		case DiffDelete:
			stats.Deletions++
		}
	}
	return stats
}
