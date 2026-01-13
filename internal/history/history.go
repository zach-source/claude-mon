package history

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Entry represents a single file change with VCS context
type Entry struct {
	Timestamp   time.Time `json:"timestamp"`
	FilePath    string    `json:"file_path"`
	ToolName    string    `json:"tool_name"`
	OldString   string    `json:"old_string,omitempty"`
	NewString   string    `json:"new_string,omitempty"`
	LineNum     int       `json:"line_num"`
	LineCount   int       `json:"line_count"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	CommitShort string    `json:"commit_short,omitempty"` // Short SHA for display
	VCSType     string    `json:"vcs_type,omitempty"`     // "git" or "jj"
}

// Store manages persistent history storage
type Store struct {
	path    string
	entries []Entry
}

// NewStore creates a new history store at the given path
func NewStore(path string) *Store {
	return &Store{
		path:    path,
		entries: []Entry{},
	}
}

// GetHistoryPath returns the default history file path for the current workspace
func GetHistoryPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	// Use .claude-mon-history.json in the workspace root
	return filepath.Join(cwd, ".claude-mon-history.json")
}

// Load reads history from the file
func (s *Store) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.entries = []Entry{}
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.entries)
}

// Save writes history to the file
func (s *Store) Save() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Add adds an entry to the history and saves
func (s *Store) Add(entry Entry) error {
	s.entries = append(s.entries, entry)
	return s.Save()
}

// Entries returns all history entries
func (s *Store) Entries() []Entry {
	return s.entries
}

// Clear removes all history
func (s *Store) Clear() error {
	s.entries = []Entry{}
	return s.Save()
}

// GetCurrentCommit returns the current VCS commit info
func GetCurrentCommit() (sha, shortSHA, vcsType string) {
	// Try jj first (it's faster and works in git repos too via colocated mode)
	if sha, shortSHA = getJJCommit(); sha != "" {
		return sha, shortSHA, "jj"
	}

	// Fall back to git
	if sha, shortSHA = getGitCommit(); sha != "" {
		return sha, shortSHA, "git"
	}

	return "", "", ""
}

// getJJCommit gets the current jj change ID
func getJJCommit() (sha, shortSHA string) {
	// Get the current change ID (jj's equivalent of commit SHA)
	cmd := exec.Command("jj", "log", "-r", "@", "--no-graph", "-T", "change_id")
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	sha = strings.TrimSpace(string(output))
	if len(sha) > 8 {
		shortSHA = sha[:8]
	} else {
		shortSHA = sha
	}

	return sha, shortSHA
}

// getGitCommit gets the current git commit SHA
func getGitCommit() (sha, shortSHA string) {
	// Get full SHA
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	sha = strings.TrimSpace(string(output))

	// Get short SHA
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	output, err = cmd.Output()
	if err != nil {
		shortSHA = sha[:7]
	} else {
		shortSHA = strings.TrimSpace(string(output))
	}

	return sha, shortSHA
}
