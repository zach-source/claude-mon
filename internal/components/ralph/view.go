package ralph

import (
	"fmt"
	"strings"
	"time"

	"github.com/ztaylor/claude-mon/internal/andthen"
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

	// Render Ralph Loop section
	sb.WriteString(m.renderRalphStatus(listWidth))

	// Render And-Then Queue section
	sb.WriteString(m.renderAndThenStatus(listWidth))

	return sb.String()
}

// renderRalphStatus renders the ralph loop status section
func (m Model) renderRalphStatus(listWidth int) string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("Ralph Loop") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

	if m.state == nil || !m.state.Active {
		sb.WriteString(m.theme.Dim.Render("No active Ralph loop\n\n"))
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

// renderAndThenStatus renders the and-then queue status section
func (m Model) renderAndThenStatus(listWidth int) string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("And-Then Queue") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", listWidth-4)) + "\n\n")

	if m.andThenState == nil || !m.andThenState.Active {
		sb.WriteString(m.theme.Dim.Render("No active queue\n\n"))
		return sb.String()
	}

	// Active queue status
	sb.WriteString(m.theme.Selected.Render("📋 Active") + "\n\n")

	// Progress
	progress := fmt.Sprintf("Task: %s", m.andThenState.Progress())
	sb.WriteString(m.theme.Normal.Render(progress) + "\n\n")

	// Current task
	if task := m.andThenState.CurrentTask(); task != nil {
		sb.WriteString(m.theme.Dim.Render("Current:\n"))
		prompt := task.Prompt
		if len(prompt) > listWidth-6 {
			prompt = prompt[:listWidth-9] + "..."
		}
		sb.WriteString(m.theme.Normal.Render(prompt) + "\n\n")

		sb.WriteString(m.theme.Dim.Render("Done when:\n"))
		doneWhen := task.DoneWhen
		if len(doneWhen) > listWidth-6 {
			doneWhen = doneWhen[:listWidth-9] + "..."
		}
		sb.WriteString(m.theme.Normal.Render(doneWhen) + "\n\n")
	}

	// Duration
	if !m.andThenState.StartedAt.IsZero() {
		durationStr := andthen.FormatDuration(time.Since(m.andThenState.StartedAt))
		sb.WriteString(m.theme.Dim.Render("Duration: "+durationStr) + "\n\n")
	}

	return sb.String()
}

// RenderFull renders a combined full-width view (status + prompt)
func (m Model) RenderFull(renderMarkdown func(string, int) (string, error)) string {
	var sb strings.Builder

	hasRalph := m.state != nil && m.state.Active
	hasAndThen := m.andThenState != nil && m.andThenState.Active

	// If neither is active, show inactive state
	if !hasRalph && !hasAndThen {
		sb.WriteString(m.theme.Title.Render("Task Automation") + "\n")
		sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")
		sb.WriteString(m.theme.Dim.Render("No active automation\n\n"))
		sb.WriteString(m.theme.Dim.Render("Start a Ralph loop with:\n"))
		sb.WriteString(m.theme.Normal.Render("  /ralph-loop\n\n"))
		sb.WriteString(m.theme.Dim.Render("Start an And-Then queue with:\n"))
		sb.WriteString(m.theme.Normal.Render("  /and-then\n\n"))
		return sb.String()
	}

	// Render Ralph Loop if active
	if hasRalph {
		sb.WriteString(m.renderRalphFull(renderMarkdown))
	}

	// Render And-Then Queue if active
	if hasAndThen {
		if hasRalph {
			sb.WriteString("\n")
		}
		sb.WriteString(m.renderAndThenFull(renderMarkdown))
	}

	return sb.String()
}

// renderRalphFull renders the full ralph loop view
func (m Model) renderRalphFull(renderMarkdown func(string, int) (string, error)) string {
	var sb strings.Builder

	// Header
	sb.WriteString(m.theme.Title.Render("Ralph Loop Status") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

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

// renderAndThenFull renders the full and-then queue view
func (m Model) renderAndThenFull(renderMarkdown func(string, int) (string, error)) string {
	var sb strings.Builder

	// Header
	sb.WriteString(m.theme.Title.Render("And-Then Queue Status") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	sb.WriteString(m.theme.Selected.Render("📋 Active") + "\n\n")

	// Progress
	progress := fmt.Sprintf("Task: %s", m.andThenState.Progress())
	sb.WriteString(m.theme.Normal.Render(progress) + "\n\n")

	// Duration
	if !m.andThenState.StartedAt.IsZero() {
		durationStr := andthen.FormatDuration(time.Since(m.andThenState.StartedAt))
		sb.WriteString(m.theme.Dim.Render("Duration: ") + m.theme.Normal.Render(durationStr) + "\n\n")
	}

	// State file path
	if m.andThenState.Path != "" {
		sb.WriteString(m.theme.Dim.Render("State: ") + m.theme.Normal.Render(m.andThenState.Path) + "\n\n")
	}

	// Task list
	sb.WriteString(m.theme.Title.Render("Tasks") + "\n")
	sb.WriteString(m.theme.Dim.Render(strings.Repeat("─", 40)) + "\n\n")

	for i, task := range m.andThenState.Tasks {
		var status string
		var taskLine string

		if i < m.andThenState.CurrentIndex {
			status = "✓"
			taskLine = fmt.Sprintf("%s %d. %s", status, i+1, task.Prompt)
			sb.WriteString(m.theme.Dim.Render(taskLine) + "\n")
		} else if i == m.andThenState.CurrentIndex {
			status = "→"
			taskLine = fmt.Sprintf("%s %d. %s", status, i+1, task.Prompt)
			sb.WriteString(m.theme.Selected.Render(taskLine) + "\n")
			sb.WriteString(m.theme.Dim.Render(fmt.Sprintf("     Done when: %s", task.DoneWhen)) + "\n")
		} else {
			status = "○"
			taskLine = fmt.Sprintf("%s %d. %s", status, i+1, task.Prompt)
			sb.WriteString(m.theme.Dim.Render(taskLine) + "\n")
		}
	}

	return sb.String()
}
