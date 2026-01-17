package contextview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// wrapText wraps text at the given width, breaking on path separators when possible
func wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}

	var lines []string
	remaining := text

	for len(remaining) > width {
		// Find a good break point (prefer / for paths)
		breakAt := width
		for i := width - 1; i > width/2; i-- {
			if remaining[i] == '/' {
				breakAt = i + 1
				break
			}
		}
		lines = append(lines, remaining[:breakAt])
		remaining = remaining[breakAt:]
	}
	if len(remaining) > 0 {
		lines = append(lines, remaining)
	}

	return lines
}

// View renders the context display
func (m Model) View() string {
	if m.editMode {
		return m.renderEditView()
	}

	if m.showList {
		return m.renderAllContexts()
	}

	return m.renderCurrentContext()
}

// renderCurrentContext renders the current project context
func (m Model) renderCurrentContext() string {
	var sb strings.Builder

	// Title
	sb.WriteString(m.theme.Title.Render("⚙️ Working Context\n\n"))

	if m.current == nil {
		sb.WriteString(m.theme.Dim.Render("No context available"))
		return sb.String()
	}

	// Project info - wrap long paths across multiple lines
	sb.WriteString(m.theme.Selected.Render("📁 Location:") + "\n")
	// Wrap the path with a reasonable width
	pathWidth := m.width - 4
	if pathWidth < 40 {
		pathWidth = 40
	}
	wrappedPath := wrapText(m.current.ProjectRoot, pathWidth)
	for _, line := range wrappedPath {
		sb.WriteString("   " + m.theme.Normal.Render(line) + "\n")
	}
	sb.WriteString("\n")

	// Build table rows for context details
	var rows [][]string

	// Kubernetes
	if k8s := m.current.GetKubernetes(); k8s != nil {
		k8sInfo := k8s.Context
		if k8s.Namespace != "" {
			k8sInfo += " / " + k8s.Namespace
		}
		rows = append(rows, []string{"⚙️ Kubernetes", k8sInfo})
	}

	// AWS
	if aws := m.current.GetAWS(); aws != nil {
		awsInfo := aws.Profile
		if aws.Region != "" {
			awsInfo += " (" + aws.Region + ")"
		}
		rows = append(rows, []string{"⛅️ AWS", awsInfo})
	}

	// Git
	if git := m.current.GetGit(); git != nil {
		gitInfo := ""
		if git.Branch != "" {
			gitInfo = git.Branch
			if git.Repo != "" {
				gitInfo += " @ " + git.Repo
			}
		}
		if gitInfo != "" {
			rows = append(rows, []string{"🌿 Git", gitInfo})
		}
	}

	// Environment variables
	if env := m.current.GetEnv(); env != nil && len(env) > 0 {
		var envPairs []string
		for k, v := range env {
			envPairs = append(envPairs, k+"="+v)
		}
		if len(envPairs) > 3 {
			envPairs = append(envPairs[:3], fmt.Sprintf("(+%d more)", len(envPairs)-3))
		}
		rows = append(rows, []string{"📦 Env", strings.Join(envPairs, ", ")})
	}

	// Custom values
	if custom := m.current.GetCustom(); custom != nil && len(custom) > 0 {
		for k, v := range custom {
			rows = append(rows, []string{"🔧 " + k, v})
		}
	}

	// Render table if we have rows
	if len(rows) > 0 {
		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(m.theme.Dim).
			Headers("Section", "Value").
			Rows(rows...).
			StyleFunc(func(row, col int) lipgloss.Style {
				if row == 0 {
					return m.theme.Title
				}
				if col == 0 {
					return m.theme.Dim
				}
				return m.theme.Normal
			})

		sb.WriteString(t.String())
		sb.WriteString("\n")
	}

	// Stale warning
	if m.current.IsStale() {
		sb.WriteString("\n")
		sb.WriteString(m.theme.Status.Render("⚠️ Context is stale (>24h)"))
		sb.WriteString("\n")
	}

	// Help
	sb.WriteString("\n")
	sb.WriteString(m.theme.Dim.Render("Press 'a' to show all contexts"))

	return sb.String()
}

// renderAllContexts renders the list of all contexts
func (m Model) renderAllContexts() string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("All Project Contexts"))
	sb.WriteString("\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)))
	sb.WriteString("\n\n")

	if len(m.all) == 0 {
		sb.WriteString(m.theme.Dim.Render("No contexts found."))
		return sb.String()
	}

	for i, ctx := range m.all {
		// Selection indicator
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}

		// Project path with selection styling
		projectLine := prefix + "📁 " + ctx.ProjectRoot
		if i == m.selected {
			sb.WriteString(m.theme.Selected.Render(projectLine))
		} else {
			sb.WriteString(m.theme.Normal.Render(projectLine))
		}
		sb.WriteString("\n")

		// Kubernetes
		if k8s := ctx.GetKubernetes(); k8s != nil {
			k8sInfo := k8s.Context
			if k8s.Namespace != "" {
				k8sInfo += " / " + k8s.Namespace
			}
			sb.WriteString(m.theme.Dim.Render("    ⚙️ K8s: ") + m.theme.Normal.Render(k8sInfo))
			sb.WriteString("\n")
		}

		// AWS
		if aws := ctx.GetAWS(); aws != nil {
			awsInfo := aws.Profile
			if aws.Region != "" {
				awsInfo += " (" + aws.Region + ")"
			}
			sb.WriteString(m.theme.Dim.Render("    ⛅️ AWS: ") + m.theme.Normal.Render(awsInfo))
			sb.WriteString("\n")
		}

		// Git
		if git := ctx.GetGit(); git != nil {
			gitInfo := ""
			if git.Branch != "" {
				gitInfo = git.Branch
			}
			if gitInfo != "" {
				sb.WriteString(m.theme.Dim.Render("    🌿 Git: ") + m.theme.Normal.Render(gitInfo))
				sb.WriteString("\n")
			}
		}

		sb.WriteString("\n")
	}

	// Help
	sb.WriteString(m.theme.Dim.Render("Press 'a' to return to current context"))

	return sb.String()
}

// renderEditView renders the edit mode overlay
func (m Model) renderEditView() string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("Edit Context Value") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	sb.WriteString(m.theme.Dim.Render("Field: ") + m.theme.Normal.Render(string(m.editField)) + "\n\n")
	sb.WriteString(m.editInput.View() + "\n\n")

	// Show format hint for key=value fields
	if m.editField == EditEnvVar || m.editField == EditCustom {
		sb.WriteString(m.theme.Dim.Render("Format: KEY=value or KEY=\"sentence with spaces\"") + "\n")
		sb.WriteString(m.theme.Dim.Render("        Quotes: \" ' or ` for multi-word values") + "\n\n")
	}

	sb.WriteString(m.theme.Dim.Render("Enter:save  Esc:cancel"))

	return sb.String()
}
