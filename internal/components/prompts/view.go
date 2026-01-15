package prompts

import (
	"fmt"
	"os"
	"strings"

	"github.com/ztaylor/claude-mon/internal/prompt"
)

// View renders the prompts list for the left pane
func (m Model) View() string {
	return m.RenderList()
}

// RenderList renders the prompts list
func (m Model) RenderList() string {
	listWidth := m.width / 3

	// Show refine input overlay when refining
	if m.refining {
		return m.renderRefineView(listWidth)
	}

	// Show versions if in version view mode
	if m.showVersions {
		return m.renderVersionView(listWidth)
	}

	// Use bubbles/list rendering with custom styling
	return m.list.View()
}

// renderRefineView renders the refine mode overlay
func (m Model) renderRefineView(width int) string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("Refine Prompt") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", width-4)) + "\n\n")

	if !m.refineDone {
		// Input phase or running
		sb.WriteString(m.theme.Normal.Render("What do you want to improve?\n\n"))
		sb.WriteString(m.refineInput.View() + "\n\n")
		sb.WriteString(m.theme.Dim.Render("Enter:submit  Esc:cancel\n\n"))

		// Show available template variables
		sb.WriteString(m.theme.Title.Render("Available Variables:") + "\n")
		sb.WriteString(m.theme.Dim.Render("{{plan}}       Plan content\n"))
		sb.WriteString(m.theme.Dim.Render("{{plan_name}}  Plan file name\n"))
		sb.WriteString(m.theme.Dim.Render("{{file}}       Current file path\n"))
		sb.WriteString(m.theme.Dim.Render("{{file_name}}  Current file name\n"))
		sb.WriteString(m.theme.Dim.Render("{{project}}    Project name\n"))
		sb.WriteString(m.theme.Dim.Render("{{cwd}}        Working directory\n"))
	} else {
		// Refinement complete
		sb.WriteString(m.theme.Title.Render("✓ Refinement complete!\n\n"))
		sb.WriteString(m.theme.Dim.Render("Reviewing changes...\n\n"))
	}

	return sb.String()
}

// renderVersionView renders the version list
func (m Model) renderVersionView(width int) string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("Versions") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", width-4)) + "\n")

	// Show current prompt name
	if p, ok := m.SelectedPrompt(); ok {
		sb.WriteString(m.theme.Dim.Render(p.Name) + "\n\n")
	}

	if len(m.versions) == 0 {
		sb.WriteString(m.theme.Dim.Render("No versions found"))
	} else {
		for i, v := range m.versions {
			prefix := "  "
			if i == m.versionSelected {
				prefix = "> "
			}
			line := fmt.Sprintf("%sv%d", prefix, v.Version)
			if i == m.versionSelected {
				sb.WriteString(m.theme.Selected.Render(line) + "\n")
			} else {
				sb.WriteString(m.theme.Normal.Render(line) + "\n")
			}
		}
	}

	return sb.String()
}

// RenderPreview renders the prompt preview for the right pane
func (m Model) RenderPreview(renderMarkdown func(string, int) (string, error), vpWidth int) string {
	var sb strings.Builder

	if m.showVersions {
		// Version preview mode
		return m.renderVersionPreview(renderMarkdown, vpWidth)
	}

	// Normal prompt preview
	p, ok := m.SelectedPrompt()
	if !ok {
		return m.theme.Dim.Render("No prompts yet.\n\nPress 'n' to create a new prompt.\nPress 'o' to switch back to History mode.")
	}

	// Header
	sb.WriteString(m.theme.Title.Render(p.Name) + "\n")
	if p.Description != "" && p.Description != "Describe what this prompt does" {
		sb.WriteString(m.theme.Dim.Render(p.Description) + "\n")
	}
	sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("v%d | %s | %s", p.Version, p.Updated.Format("2006-01-02"), prompt.MethodName(m.injectMethod))) + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// Render content as markdown
	rendered, err := renderMarkdown(p.Content, vpWidth-4)
	if err != nil {
		sb.WriteString(p.Content)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}

// renderVersionPreview renders the version preview
func (m Model) renderVersionPreview(renderMarkdown func(string, int) (string, error), vpWidth int) string {
	if len(m.versions) == 0 {
		return m.theme.Dim.Render("No versions available")
	}

	var sb strings.Builder
	v := m.versions[m.versionSelected]

	sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Version %d", v.Version)) + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// Load and render version content from file
	content, err := os.ReadFile(v.Path)
	if err != nil {
		sb.WriteString(m.theme.Dim.Render("Failed to load version: " + err.Error()))
		return sb.String()
	}

	// Parse to extract just the content
	p, err := prompt.Parse(string(content))
	if err != nil {
		sb.WriteString(string(content))
		return sb.String()
	}

	rendered, err := renderMarkdown(p.Content, vpWidth-4)
	if err != nil {
		sb.WriteString(p.Content)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}
