package planview

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// View renders the plan component
func (m Model) View() string {
	return m.RenderList()
}

// RenderList renders the plan info for the left pane
func (m Model) RenderList() string {
	var sb strings.Builder
	listWidth := m.width / 3

	sb.WriteString(m.theme.Title.Render("Plan") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

	// Show plan input if active
	if m.inputActive {
		sb.WriteString(m.theme.Normal.Render("New Plan\n\n"))
		sb.WriteString(m.theme.Dim.Render("Describe what to build:\n\n"))
		sb.WriteString(m.input.View() + "\n\n")
		sb.WriteString(m.theme.Dim.Render("Enter:submit  Esc:cancel"))
		return sb.String()
	}

	// Show generating status
	if m.generating {
		sb.WriteString(m.theme.Selected.Render("⏳ Generating...") + "\n\n")
		sb.WriteString(m.theme.Dim.Render("Claude is creating your plan.\n"))
		sb.WriteString(m.theme.Dim.Render("This may take a moment."))
		return sb.String()
	}

	if m.path == "" {
		sb.WriteString(m.theme.Dim.Render("No active plan\n\n"))
		sb.WriteString(m.theme.Dim.Render("Press 'G' to generate a new\n"))
		sb.WriteString(m.theme.Dim.Render("plan with Claude.\n\n"))
		sb.WriteString(m.theme.Dim.Render("Or press 'r' to refresh if\n"))
		sb.WriteString(m.theme.Dim.Render("Claude created one."))
		return sb.String()
	}

	// Show current plan info
	planName := strings.TrimSuffix(filepath.Base(m.path), ".md")
	sb.WriteString(m.theme.Selected.Render("📋 "+planName) + "\n\n")

	// Plan file location
	sb.WriteString(m.theme.Dim.Render("Location:") + "\n")
	location := m.path
	if len(location) > listWidth-6 {
		location = "..." + location[len(location)-listWidth+9:]
	}
	sb.WriteString(m.theme.Normal.Render(location) + "\n\n")

	// File info
	if info, err := os.Stat(m.path); err == nil {
		sb.WriteString(m.theme.Dim.Render("Modified: "+info.ModTime().Format("2006-01-02 15:04")) + "\n")
		sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("Size: %d bytes", info.Size())) + "\n\n")
	}

	sb.WriteString(m.theme.Dim.Render("G:new  e:edit  r:refresh"))

	return sb.String()
}

// RenderContent renders the plan content for the right pane
func (m Model) RenderContent(renderMarkdown func(string, int) (string, error), vpWidth int) string {
	var sb strings.Builder

	if m.path == "" || m.content == "" {
		return m.theme.Dim.Render("No active plan.\n\nPlans are created when Claude enters plan mode.")
	}

	planName := strings.TrimSuffix(filepath.Base(m.path), ".md")
	sb.WriteString(m.theme.Title.Render(planName) + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	// Render plan as markdown
	rendered, err := renderMarkdown(m.content, vpWidth-4)
	if err != nil {
		sb.WriteString(m.content)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}
