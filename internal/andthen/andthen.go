package andthen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Task represents a single task in the queue
type Task struct {
	Prompt   string `yaml:"prompt"`
	DoneWhen string `yaml:"done_when"`
}

// State represents the And-Then queue state from and-then-queue.local.md
type State struct {
	Active       bool      `yaml:"active"`
	CurrentIndex int       `yaml:"current_index"`
	StartedAt    time.Time `yaml:"started_at"`
	Tasks        []Task    `yaml:"tasks"`
	Path         string    `yaml:"-"` // The file path where state was found
}

// CurrentTask returns the current task, or nil if no tasks or index out of bounds
func (s *State) CurrentTask() *Task {
	if s == nil || len(s.Tasks) == 0 || s.CurrentIndex >= len(s.Tasks) {
		return nil
	}
	return &s.Tasks[s.CurrentIndex]
}

// Progress returns current/total as a string
func (s *State) Progress() string {
	if s == nil || len(s.Tasks) == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", s.CurrentIndex+1, len(s.Tasks))
}

// CompletedCount returns the number of completed tasks
func (s *State) CompletedCount() int {
	if s == nil {
		return 0
	}
	return s.CurrentIndex
}

// PendingCount returns the number of remaining tasks (including current)
func (s *State) PendingCount() int {
	if s == nil || len(s.Tasks) == 0 {
		return 0
	}
	return len(s.Tasks) - s.CurrentIndex
}

// LoadState loads the And-Then queue state from the state file.
// It checks project-local first (.claude/and-then-queue.local.md), then global (~/.claude/and-then-queue.local.md).
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
		filepath.Join(cwd, ".claude", "and-then-queue.local.md"),
		filepath.Join(home, ".claude", "and-then-queue.local.md"),
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

// parseState parses the And-Then queue state from file content with YAML frontmatter
func parseState(content, path string) (*State, error) {
	// Parse YAML frontmatter
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("invalid And-Then state file: no frontmatter")
	}

	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid And-Then state file: malformed frontmatter")
	}

	state := &State{}
	if err := yaml.Unmarshal([]byte(parts[1]), state); err != nil {
		return nil, fmt.Errorf("failed to parse And-Then frontmatter: %w", err)
	}

	state.Path = path

	return state, nil
}

// CancelQueue cancels the And-Then queue by removing the state file.
// It tries project-local first, then global.
// Returns true if a file was removed, false otherwise.
func CancelQueue() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("failed to get home dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("failed to get cwd: %w", err)
	}

	// Try to remove project-local first
	projectPath := filepath.Join(cwd, ".claude", "and-then-queue.local.md")
	if err := os.Remove(projectPath); err == nil {
		return true, nil
	}

	// Try global
	globalPath := filepath.Join(home, ".claude", "and-then-queue.local.md")
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
