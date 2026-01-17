package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ztaylor/claude-mon/internal/diff"
	"github.com/ztaylor/claude-mon/internal/minimap"
	"github.com/ztaylor/claude-mon/internal/vcs"
)

// View renders the history list for the left pane
func (m Model) View() string {
	return m.RenderList()
}

// RenderList renders the history list
func (m Model) RenderList() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("No changes yet...\nWaiting for Claude edits")
	}

	var sb strings.Builder
	sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("History (%d)\n", len(m.changes))))
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 20)) + "\n")

	// Calculate available width for path in history pane
	historyWidth := m.width / 3
	pathWidth := historyWidth - 15 // Account for timestamp, tool, prefix

	// Track current commit for grouping
	currentCommit := ""

	// Iterate in reverse to show newest on top
	for i := len(m.changes) - 1; i >= 0; i-- {
		change := m.changes[i]
		// Show commit header when commit changes
		if change.CommitShort != "" && change.CommitShort != currentCommit {
			currentCommit = change.CommitShort
			vcsLabel := change.VCSType
			if vcsLabel == "" {
				vcsLabel = "commit"
			}
			commitHeader := fmt.Sprintf("── %s:%s ──", vcsLabel, change.CommitShort)
			sb.WriteString(m.theme.DiffHeader.Render(commitHeader) + "\n")
		}

		var line string
		if i == m.selectedIndex {
			// Selected: show scrollable relative path
			path := relativePath(change.FilePath)
			if m.scrollX > 0 && len(path) > m.scrollX {
				path = path[m.scrollX:]
			}
			line = fmt.Sprintf("%s %s %s",
				change.Timestamp.Format("15:04"),
				change.ToolName,
				path)
			sb.WriteString(m.theme.Selected.Render("> "+line) + "\n")
		} else {
			// Not selected: truncate path
			line = fmt.Sprintf("%s %s %s",
				change.Timestamp.Format("15:04"),
				change.ToolName,
				truncatePath(change.FilePath, pathWidth))
			sb.WriteString(m.theme.Normal.Render("  "+line) + "\n")
		}
	}

	return sb.String()
}

// RenderDiff renders the diff view for the right pane
func (m *Model) RenderDiff() string {
	if len(m.changes) == 0 {
		return m.theme.Dim.Render("Select a change to view diff")
	}

	// Use cache if available and no horizontal scroll
	if m.scrollX == 0 {
		if cached, ok := m.diffCache[m.selectedIndex]; ok {
			return cached
		}
	}

	change := m.changes[m.selectedIndex]

	// If FileContent is empty (e.g., loaded from history), try to retrieve it
	if change.FileContent == "" && change.FilePath != "" && change.ToolName != "Write" {
		var fileContent string

		// Try VCS-based retrieval if we have commit info
		if change.CommitSHA != "" && change.VCSType != "" {
			dir := filepath.Dir(change.FilePath)
			if workspaceRoot, err := vcs.GetWorkspaceRoot(dir, change.VCSType); err == nil {
				if content, err := vcs.GetFileAtCommit(workspaceRoot, change.FilePath, change.CommitSHA, change.VCSType); err == nil {
					fileContent = content
				}
			}
		}

		// Fall back to reading current file if VCS retrieval failed
		if fileContent == "" {
			if content, err := os.ReadFile(change.FilePath); err == nil {
				fileContent = string(content)
			}
		}

		if fileContent != "" {
			change.FileContent = fileContent
			// Update the stored change so we don't re-read every time
			m.changes[m.selectedIndex] = change
		}
	}

	var sb strings.Builder

	// Header with relative file path
	sb.WriteString(m.theme.Title.Render(relativePath(change.FilePath)))
	if change.LineNum > 0 {
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf(":%d", change.LineNum)))
	}
	sb.WriteString("\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// If we have file content, show full file with change highlighted
	if change.FileContent != "" && change.ToolName != "Write" {
		sb.WriteString(m.renderFileWithChange(change))
	} else if change.ToolName == "Write" {
		// For Write operations, show highlighted new content
		content := change.NewString
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		sb.WriteString(m.theme.DiffHeader.Render("@@ New file @@"))
		sb.WriteString("\n\n")

		lines := diff.SplitLines(content)
		for i, line := range lines {
			lineNum := fmt.Sprintf("%4d", i+1)
			highlighted := m.highlighter.HighlightLine(line, change.FilePath)
			sb.WriteString(m.theme.LineNumber.Render(lineNum))
			sb.WriteString(" ")
			sb.WriteString(m.theme.Added.Render("+ "))
			sb.WriteString(highlighted)
			sb.WriteString("\n")
		}
	} else if change.OldString != "" || change.NewString != "" {
		// Fallback: show just the diff
		opts := diff.DefaultOptions()
		diffOutput := diff.FormatDiff(change.OldString, change.NewString, m.theme, opts)
		sb.WriteString(diffOutput)
	} else {
		sb.WriteString(m.theme.Dim.Render("No diff content available"))
	}

	return sb.String()
}

// renderFileWithChange shows the full file with the changed section highlighted
func (m *Model) renderFileWithChange(change Change) string {
	var sb strings.Builder

	// Split file content into lines
	fileLines := diff.SplitLines(change.FileContent)
	oldLines := diff.SplitLines(change.OldString)
	newLines := diff.SplitLines(change.NewString)

	changeStart := change.LineNum - 1 // 0-indexed
	changeEnd := changeStart + len(oldLines)

	// Track total lines for minimap
	m.totalLines = len(fileLines) + len(newLines)

	// Build minimap data
	m.minimapData = minimap.New(m.totalLines)
	// Mark removed lines
	m.minimapData.SetRange(changeStart, changeEnd, minimap.LineRemoved)
	// Mark added lines (they appear after the removed lines in the view)
	for i := 0; i < len(newLines); i++ {
		m.minimapData.SetLine(changeStart+i, minimap.LineAdded)
	}

	// Show diff header with stats
	sb.WriteString(m.theme.DiffHeader.Render(fmt.Sprintf("@@ -%d,%d +%d,%d @@",
		change.LineNum, len(oldLines), change.LineNum, len(newLines))))
	sb.WriteString("  ")
	sb.WriteString(m.theme.Added.Render(fmt.Sprintf("+%d", len(newLines))))
	sb.WriteString(" ")
	sb.WriteString(m.theme.Removed.Render(fmt.Sprintf("-%d", len(oldLines))))
	sb.WriteString("\n\n")

	// Soft highlight style for changed lines
	changedBg := lipgloss.NewStyle().Background(m.theme.ChangedLineBg)

	// Render the entire file
	for i := 0; i < len(fileLines); i++ {
		lineNum := fmt.Sprintf("%4d", i+1)
		line := fileLines[i]

		// Apply horizontal scroll
		scrolledLine := line
		if m.scrollX > 0 && len(line) > m.scrollX {
			scrolledLine = line[m.scrollX:]
		} else if m.scrollX > 0 {
			scrolledLine = ""
		}

		// Check if this line is in the changed region
		if i >= changeStart && i < changeEnd {
			// This is a removed line - use diff colors (no syntax highlighting)
			lineContent := m.theme.LineNumberActive.Render(lineNum) + " " +
				m.theme.Removed.Render("- "+scrolledLine)
			sb.WriteString(changedBg.Render(lineContent))
			sb.WriteString("\n")

			// After the last removed line, insert the new lines
			if i == changeEnd-1 {
				for j, newLine := range newLines {
					// Apply horizontal scroll to new lines too
					scrolledNew := newLine
					if m.scrollX > 0 && len(newLine) > m.scrollX {
						scrolledNew = newLine[m.scrollX:]
					} else if m.scrollX > 0 {
						scrolledNew = ""
					}

					newLineNum := fmt.Sprintf("%4d", changeStart+j+1)
					lineContent := m.theme.LineNumberActive.Render(newLineNum) + " " +
						m.theme.Added.Render("+ "+scrolledNew)
					sb.WriteString(changedBg.Render(lineContent))
					sb.WriteString("\n")
				}
			}
		} else {
			// Context line - use syntax highlighting
			highlighted := m.highlighter.HighlightLine(scrolledLine, change.FilePath)
			sb.WriteString(m.theme.LineNumber.Render(lineNum))
			sb.WriteString(" ")
			sb.WriteString(m.theme.Context.Render("  "))
			sb.WriteString(highlighted)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// truncatePath truncates a path to fit in maxLen characters
func truncatePath(p string, maxLen int) string {
	rel := relativePath(p)
	if len(rel) <= maxLen {
		return rel
	}
	if maxLen < 4 {
		return rel[:maxLen]
	}
	return rel[:maxLen-3] + "..."
}
