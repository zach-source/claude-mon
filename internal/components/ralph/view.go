package ralph

import (
	"fmt"
	"strings"
	"time"

	"github.com/ztaylor/claude-mon/internal/ralph"
)

// View renders the ralph component
func (m Model) View() string {
	return m.RenderStatus()
}

// RenderStatus renders the ralph status for the left pane
func (m Model) RenderStatus() string {
	var sb strings.Builder
	listWidth := m.width / 3

	sb.WriteString(m.theme.Title.Render("Ralph Loop") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

	if m.state == nil || !m.state.Active {
		sb.WriteString(m.theme.Dim.Render("No active Ralph loop\n\n"))
		sb.WriteString(m.theme.Dim.Render("Start a Ralph loop with:\n"))
		sb.WriteString(m.theme.Dim.Render("/ralph-loop\n\n"))
		return sb.String()
	}

	// Active Ralph loop status
	sb.WriteString(m.theme.Selected.Render("🔄 Active") + "\n\n")

	// Iteration progress
	progress := fmt.Sprintf("Iteration: %d / %d", m.state.Iteration, m.state.MaxIterations)
	sb.WriteString(m.theme.Normal.Render(progress) + "\n\n")

	// Promise
	if m.state.Promise != "" {
		sb.WriteString(m.theme.Dim.Render("Promise:\n"))
		promise := m.state.Promise
		if len(promise) > listWidth-6 {
			promise = promise[:listWidth-9] + "..."
		}
		sb.WriteString(m.theme.Normal.Render(promise) + "\n\n")
	}

	// Duration
	if !m.state.StartedAt.IsZero() {
		durationStr := ralph.FormatDuration(time.Since(m.state.StartedAt))
		sb.WriteString(m.theme.Dim.Render("Duration: "+durationStr) + "\n\n")
	}

	return sb.String()
}

// RenderFull renders a combined full-width view (status + prompt)
func (m Model) RenderFull(renderMarkdown func(string, int) (string, error)) string {
	var sb strings.Builder

	if m.state == nil || !m.state.Active {
		sb.WriteString(m.theme.Title.Render("Ralph Loop") + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")
		sb.WriteString(m.theme.Dim.Render("No active Ralph loop\n\n"))
		sb.WriteString(m.theme.Dim.Render("Start a Ralph loop with:\n"))
		sb.WriteString(m.theme.Normal.Render("  /ralph-loop\n\n"))
		return sb.String()
	}

	// Header
	sb.WriteString(m.theme.Title.Render("Ralph Loop Status") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	if m.state.Active {
		sb.WriteString(m.theme.Selected.Render("🔄 Active") + "\n\n")

		// Iteration
		progress := fmt.Sprintf("Iteration: %d/%d", m.state.Iteration, m.state.MaxIterations)
		sb.WriteString(m.theme.Normal.Render(progress) + "\n\n")

		// Promise
		if m.state.Promise != "" {
			sb.WriteString(m.theme.Dim.Render("Promise: ") + m.theme.Normal.Render("\""+m.state.Promise+"\"") + "\n\n")
		}

		// Duration
		if !m.state.StartedAt.IsZero() {
			durationStr := ralph.FormatDuration(time.Since(m.state.StartedAt))
			sb.WriteString(m.theme.Dim.Render("Duration: ") + m.theme.Normal.Render(durationStr) + "\n\n")
		}

		// State file path
		if m.state.Path != "" {
			sb.WriteString(m.theme.Dim.Render("State: ") + m.theme.Normal.Render(m.state.Path) + "\n\n")
		}
	}

	// Separator before prompt
	sb.WriteString(m.theme.Title.Render("Current Prompt") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	if m.state.Prompt == "" {
		sb.WriteString(m.theme.Dim.Render("No prompt loaded"))
		return sb.String()
	}

	// Render prompt as markdown
	rendered, err := renderMarkdown(m.state.Prompt, m.width-4)
	if err != nil {
		sb.WriteString(m.state.Prompt)
	} else {
		sb.WriteString(rendered)
	}

	return sb.String()
}
