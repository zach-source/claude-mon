package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ztaylor/claude-mon/internal/logger"
)

// CreateVersion creates a versioned backup of a prompt
// e.g., code-review.prompt.md -> code-review.v1.prompt.md
func (s *Store) CreateVersion(p *Prompt) error {
	logger.Log("CreateVersion called: path=%s", p.Path)
	if p.Path == "" {
		return fmt.Errorf("prompt has no path")
	}

	// Find next version number
	versions, err := s.ListVersions(p.Path)
	if err != nil {
		logger.Log("ListVersions error: %v", err)
		return err
	}
	logger.Log("Found %d existing versions", len(versions))

	nextVersion := 1
	if len(versions) > 0 {
		// Get highest version number
		nextVersion = versions[len(versions)-1].Version + 1
	}

	// Build version path
	dir := filepath.Dir(p.Path)
	base := filepath.Base(p.Path)
	name := strings.TrimSuffix(base, ".prompt.md")
	versionPath := filepath.Join(dir, fmt.Sprintf("%s.v%d.prompt.md", name, nextVersion))
	logger.Log("Version path: %s", versionPath)

	// Copy current file to version
	content, err := os.ReadFile(p.Path)
	if err != nil {
		logger.Log("ReadFile error: %v", err)
		return err
	}
	logger.Log("Read %d bytes from source", len(content))

	if err := os.WriteFile(versionPath, content, 0644); err != nil {
		logger.Log("WriteFile error: %v", err)
		return err
	}
	logger.Log("Wrote version file successfully")

	// Increment version in original
	p.Version++

	return nil
}

// PromptVersion represents a versioned backup
type PromptVersion struct {
	Version int
	Path    string
}

// ListVersions returns all version backups for a prompt
func (s *Store) ListVersions(promptPath string) ([]PromptVersion, error) {
	dir := filepath.Dir(promptPath)
	base := filepath.Base(promptPath)
	name := strings.TrimSuffix(base, ".prompt.md")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var versions []PromptVersion
	pattern := regexp.MustCompile(fmt.Sprintf(`^%s\.v(\d+)\.prompt\.md$`, regexp.QuoteMeta(name)))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := pattern.FindStringSubmatch(entry.Name())
		if len(matches) == 2 {
			version, _ := strconv.Atoi(matches[1])
			versions = append(versions, PromptVersion{
				Version: version,
				Path:    filepath.Join(dir, entry.Name()),
			})
		}
	}

	// Sort by version number
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})

	return versions, nil
}

// RestoreVersion replaces the current prompt with a version backup
func (s *Store) RestoreVersion(promptPath string, version int) error {
	dir := filepath.Dir(promptPath)
	base := filepath.Base(promptPath)
	name := strings.TrimSuffix(base, ".prompt.md")
	versionPath := filepath.Join(dir, fmt.Sprintf("%s.v%d.prompt.md", name, version))

	content, err := os.ReadFile(versionPath)
	if err != nil {
		return fmt.Errorf("version %d not found: %w", version, err)
	}

	return os.WriteFile(promptPath, content, 0644)
}
