package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Prompt represents a stored prompt with metadata
type Prompt struct {
	Name         string    `yaml:"name"`
	Description  string    `yaml:"description,omitempty"`
	Version      int       `yaml:"version"`
	Created      time.Time `yaml:"created"`
	Updated      time.Time `yaml:"updated"`
	Tags         []string  `yaml:"tags,omitempty"`
	Content      string    `yaml:"-"` // The actual prompt text (not in frontmatter)
	Path         string    `yaml:"-"` // File path
	IsGlobal     bool      `yaml:"-"` // Global vs project-local
	VersionCount int       `yaml:"-"` // Number of version backups
}

// Store manages prompt storage in global and project directories
type Store struct {
	globalDir  string // ~/.claude/prompts/
	projectDir string // .claude/prompts/
}

// NewStore creates a new prompt store
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home dir: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get cwd: %w", err)
	}

	return &Store{
		globalDir:  filepath.Join(home, ".claude", "prompts"),
		projectDir: filepath.Join(cwd, ".claude", "prompts"),
	}, nil
}

// List returns all prompts from both global and project directories
func (s *Store) List() ([]Prompt, error) {
	var prompts []Prompt

	// Load global prompts
	globalPrompts, err := s.loadFromDir(s.globalDir, true)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load global prompts: %w", err)
	}
	prompts = append(prompts, globalPrompts...)

	// Load project prompts
	projectPrompts, err := s.loadFromDir(s.projectDir, false)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load project prompts: %w", err)
	}
	prompts = append(prompts, projectPrompts...)

	// Sort by updated time (newest first)
	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].Updated.After(prompts[j].Updated)
	})

	return prompts, nil
}

// loadFromDir loads all prompts from a directory
func (s *Store) loadFromDir(dir string, isGlobal bool) ([]Prompt, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// First pass: count versions for each prompt (single directory scan)
	versionCounts := make(map[string]int) // base name -> count
	versionPattern := regexp.MustCompile(`^(.+)\.v\d+\.prompt\.md$`)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if matches := versionPattern.FindStringSubmatch(entry.Name()); len(matches) == 2 {
			baseName := matches[1]
			versionCounts[baseName]++
		}
	}

	// Second pass: load prompts
	var prompts []Prompt
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Only load .prompt.md files, skip version backups
		if !strings.HasSuffix(name, ".prompt.md") {
			continue
		}
		// Skip version backups (e.g., name.v1.prompt.md)
		if versionPattern.MatchString(name) {
			continue
		}

		path := filepath.Join(dir, name)
		prompt, err := s.Load(path)
		if err != nil {
			continue // Skip invalid files
		}
		prompt.IsGlobal = isGlobal

		// Get version count from pre-computed map
		baseName := strings.TrimSuffix(name, ".prompt.md")
		prompt.VersionCount = versionCounts[baseName]

		prompts = append(prompts, *prompt)
	}

	return prompts, nil
}

// Load reads a prompt from a file
func (s *Store) Load(path string) (*Prompt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	prompt, err := Parse(string(data))
	if err != nil {
		return nil, err
	}

	prompt.Path = path

	// If no name, derive from filename
	if prompt.Name == "" {
		base := filepath.Base(path)
		prompt.Name = strings.TrimSuffix(base, ".prompt.md")
	}

	// Get file info for timestamps if not set
	info, err := os.Stat(path)
	if err == nil {
		if prompt.Created.IsZero() {
			prompt.Created = info.ModTime()
		}
		if prompt.Updated.IsZero() {
			prompt.Updated = info.ModTime()
		}
	}

	return prompt, nil
}

// Parse parses a prompt from its string content (with optional frontmatter)
func Parse(content string) (*Prompt, error) {
	prompt := &Prompt{
		Version: 1,
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Check for YAML frontmatter
	if strings.HasPrefix(content, "---\n") {
		parts := strings.SplitN(content, "---\n", 3)
		if len(parts) >= 3 {
			// Parse frontmatter
			if err := yaml.Unmarshal([]byte(parts[1]), prompt); err != nil {
				// If frontmatter is invalid, treat entire content as prompt
				prompt.Content = content
				return prompt, nil
			}
			prompt.Content = strings.TrimSpace(parts[2])
			return prompt, nil
		}
	}

	// No frontmatter, entire content is the prompt
	prompt.Content = strings.TrimSpace(content)
	return prompt, nil
}

// Save writes a prompt to disk
func (s *Store) Save(p *Prompt) error {
	// Determine target directory
	dir := s.projectDir
	if p.IsGlobal {
		dir = s.globalDir
	}

	// Create directory if needed
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create prompts dir: %w", err)
	}

	// Update timestamp
	p.Updated = time.Now()

	// Build file content
	content := p.Format()

	// Determine path
	path := p.Path
	if path == "" {
		// Generate filename from name
		safeName := strings.ReplaceAll(strings.ToLower(p.Name), " ", "-")
		safeName = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(safeName, "")
		path = filepath.Join(dir, safeName+".prompt.md")
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}

	p.Path = path
	return nil
}

// Format returns the prompt as a string with frontmatter
func (p *Prompt) Format() string {
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	frontmatter := struct {
		Name        string    `yaml:"name"`
		Description string    `yaml:"description,omitempty"`
		Version     int       `yaml:"version"`
		Created     time.Time `yaml:"created"`
		Updated     time.Time `yaml:"updated"`
		Tags        []string  `yaml:"tags,omitempty,flow"`
	}{
		Name:        p.Name,
		Description: p.Description,
		Version:     p.Version,
		Created:     p.Created,
		Updated:     p.Updated,
		Tags:        p.Tags,
	}

	data, _ := yaml.Marshal(frontmatter)
	sb.Write(data)
	sb.WriteString("---\n\n")
	sb.WriteString(p.Content)
	sb.WriteString("\n")

	return sb.String()
}

// Delete removes a prompt file
func (s *Store) Delete(path string) error {
	return os.Remove(path)
}

// GlobalDir returns the global prompts directory
func (s *Store) GlobalDir() string {
	return s.globalDir
}

// ProjectDir returns the project prompts directory
func (s *Store) ProjectDir() string {
	return s.projectDir
}

// NewPromptTemplate returns a template for a new prompt
func NewPromptTemplate(name string) string {
	p := &Prompt{
		Name:        name,
		Description: "Describe what this prompt does",
		Version:     1,
		Created:     time.Now(),
		Updated:     time.Now(),
		Content:     "Your prompt content here.\n\nBe specific about:\n- The task to perform\n- Expected output format\n- Any constraints or guidelines",
	}
	return p.Format()
}
