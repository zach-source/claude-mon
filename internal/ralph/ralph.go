package ralph

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// State represents the Ralph Loop state from ralph-loop.local.md
type State struct {
	Active        bool      `yaml:"active"`
	Iteration     int       `yaml:"iteration"`
	MaxIterations int       `yaml:"max_iterations"`
	Promise       string    `yaml:"completion_promise"`
	StartedAt     time.Time `yaml:"started_at"`
	Prompt        string    `yaml:"-"` // The prompt content (not in frontmatter)
	Path          string    `yaml:"-"` // The file path where state was found
}

// LoadState loads the Ralph Loop state from the state file.
// It checks project-local first (.claude/ralph-loop.local.md), then global (~/.claude/ralph-loop.local.md).
// Returns nil if no state file is found.
func LoadState() (*State, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get cwd: %w", err)
	}

	// Try project-local first, then global
	paths := []string{
		filepath.Join(cwd, ".claude", "ralph-loop.local.md"),
		filepath.Join(home, ".claude", "ralph-loop.local.md"),
	}

	var content []byte
	var foundPath string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			content = data
			foundPath = path
			break
		}
	}

	if content == nil {
		return nil, nil // No state file found, return nil (not an error)
	}

	return parseState(string(content), foundPath)
}

// parseState parses the Ralph Loop state from file content with YAML frontmatter
func parseState(content, path string) (*State, error) {
	// Parse YAML frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("invalid Ralph state file: no frontmatter")
	}

	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid Ralph state file: malformed frontmatter")
	}

	state := &State{}
	if err := yaml.Unmarshal([]byte(parts[1]), state); err != nil {
		return nil, fmt.Errorf("failed to parse Ralph frontmatter: %w", err)
	}

	state.Prompt = strings.TrimSpace(parts[2])
	state.Path = path

	return state, nil
}

// CancelLoop cancels the Ralph Loop by removing the state file.
// It tries project-local first, then global.
// Returns true if a file was removed, false otherwise.
func CancelLoop() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("failed to get home dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("failed to get cwd: %w", err)
	}

	// Try to remove project-local first
	projectPath := filepath.Join(cwd, ".claude", "ralph-loop.local.md")
	if err := os.Remove(projectPath); err == nil {
		return true, nil
	}

	// Try global
	globalPath := filepath.Join(home, ".claude", "ralph-loop.local.md")
	if err := os.Remove(globalPath); err == nil {
		return true, nil
	}

	return false, nil // No file found to remove
}

// FormatDuration formats the elapsed time in a human-readable way
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("%dh %dm ago", hours, mins)
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd ago", days)
}
