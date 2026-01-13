package database

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaFS embed.FS

// DB wraps SQLite database operations
type DB struct {
	db *sql.DB
}

// Config holds database configuration
type Config struct {
	Path string // Path to SQLite database file
}

// DefaultConfig returns default database config
func DefaultConfig() (*Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	dataDir := filepath.Join(homeDir, ".claude-mon")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	return &Config{
		Path: filepath.Join(dataDir, "claude-mon.db"),
	}, nil
}

// Open opens or creates the database
func Open(cfg *Config) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite3", cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Initialize schema
	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

func initSchema(db *sql.DB) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema: %w", err)
	}

	_, err = db.Exec(string(schema))
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	return nil
}

// Session represents a Claude session
type Session struct {
	ID            int64
	WorkspacePath string
	WorkspaceName string
	Branch        string
	CommitSHA     string
	StartedAt     time.Time
	LastActivity  time.Time
}

// UpsertSession creates or updates a session
func (d *DB) UpsertSession(workspacePath, workspaceName, branch, commitSHA string) (int64, error) {
	query := `
		INSERT INTO sessions (workspace_path, workspace_name, branch, commit_sha, last_activity)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(workspace_path, branch) DO UPDATE SET
			last_activity = CURRENT_TIMESTAMP,
			commit_sha = excluded.commit_sha
		RETURNING id
	`

	var id int64
	err := d.db.QueryRow(query, workspacePath, workspaceName, branch, commitSHA).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to upsert session: %w", err)
	}

	return id, nil
}

// GetSession retrieves a session by ID
func (d *DB) GetSession(id int64) (*Session, error) {
	query := `
		SELECT id, workspace_path, workspace_name, branch, commit_sha, started_at, last_activity
		FROM sessions WHERE id = ?
	`

	var s Session
	err := d.db.QueryRow(query, id).Scan(
		&s.ID, &s.WorkspacePath, &s.WorkspaceName, &s.Branch,
		&s.CommitSHA, &s.StartedAt, &s.LastActivity,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &s, nil
}

// Edit represents a file edit
type Edit struct {
	ID        int64
	SessionID int64
	ToolName  string
	FilePath  string
	OldString string
	NewString string
	LineNum   int
	LineCount int
	Timestamp time.Time
}

// RecordEdit records a file edit
func (d *DB) RecordEdit(edit *Edit) error {
	query := `
		INSERT INTO edits (session_id, tool_name, file_path, old_string, new_string, line_num, line_count)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.Exec(query, edit.SessionID, edit.ToolName, edit.FilePath,
		edit.OldString, edit.NewString, edit.LineNum, edit.LineCount)
	if err != nil {
		return fmt.Errorf("failed to record edit: %w", err)
	}

	return nil
}

// Prompt represents a prompt
type Prompt struct {
	ID          int64
	SessionID   sql.NullInt64
	Name        string
	Description string
	Content     string
	Tags        []string
	Version     int
	IsGlobal    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RecordPrompt records or updates a prompt
func (d *DB) RecordPrompt(prompt *Prompt) (int64, error) {
	tagsJSON, err := json.Marshal(prompt.Tags)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal tags: %w", err)
	}

	query := `
		INSERT INTO prompts (session_id, name, description, content, tags, version, is_global)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name, session_id) DO UPDATE SET
			content = excluded.content,
			description = excluded.description,
			tags = excluded.tags,
			version = version + 1,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`

	var id int64
	var sessionID interface{}
	if prompt.SessionID.Valid {
		sessionID = prompt.SessionID.Int64
	} else {
		sessionID = nil
	}

	err = d.db.QueryRow(query, sessionID, prompt.Name, prompt.Description,
		prompt.Content, string(tagsJSON), prompt.Version, prompt.IsGlobal).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to record prompt: %w", err)
	}

	return id, nil
}

// GetPrompts retrieves prompts matching filters
func (d *DB) GetPrompts(namePattern string, limit int) ([]*Prompt, error) {
	query := `
		SELECT id, session_id, name, description, content, tags, version, is_global, created_at, updated_at
		FROM prompts
		WHERE name LIKE ?
		ORDER BY updated_at DESC
		LIMIT ?
	`

	rows, err := d.db.Query(query, "%"+namePattern+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompts: %w", err)
	}
	defer rows.Close()

	var prompts []*Prompt
	for rows.Next() {
		var p Prompt
		var tagsJSON string

		err := rows.Scan(
			&p.ID, &p.SessionID, &p.Name, &p.Description, &p.Content,
			&tagsJSON, &p.Version, &p.IsGlobal, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan prompt: %w", err)
		}

		if err := json.Unmarshal([]byte(tagsJSON), &p.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}

		prompts = append(prompts, &p)
	}

	return prompts, nil
}

// GetRecentEdits retrieves recent edits
func (d *DB) GetRecentEdits(limit int) ([]*Edit, error) {
	query := `
		SELECT e.id, e.session_id, e.tool_name, e.file_path,
		       e.old_string, e.new_string, e.line_num, e.line_count, e.timestamp
		FROM edits e
		ORDER BY e.timestamp DESC
		LIMIT ?
	`

	rows, err := d.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent edits: %w", err)
	}
	defer rows.Close()

	var edits []*Edit
	for rows.Next() {
		var e Edit
		err := rows.Scan(
			&e.ID, &e.SessionID, &e.ToolName, &e.FilePath,
			&e.OldString, &e.NewString, &e.LineNum, &e.LineCount, &e.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan edit: %w", err)
		}

		edits = append(edits, &e)
	}

	return edits, nil
}

// GetEditsByFile retrieves edits for a specific file
func (d *DB) GetEditsByFile(filePath string, limit int) ([]*Edit, error) {
	query := `
		SELECT id, session_id, tool_name, file_path,
		       old_string, new_string, line_num, line_count, timestamp
		FROM edits
		WHERE file_path = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := d.db.Query(query, filePath, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get edits by file: %w", err)
	}
	defer rows.Close()

	var edits []*Edit
	for rows.Next() {
		var e Edit
		err := rows.Scan(
			&e.ID, &e.SessionID, &e.ToolName, &e.FilePath,
			&e.OldString, &e.NewString, &e.LineNum, &e.LineCount, &e.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan edit: %w", err)
		}

		edits = append(edits, &e)
	}

	return edits, nil
}

// GetSessions retrieves all sessions
func (d *DB) GetSessions(limit int) ([]*Session, error) {
	query := `
		SELECT id, workspace_path, workspace_name, branch, commit_sha, started_at, last_activity
		FROM sessions
		ORDER BY last_activity DESC
		LIMIT ?
	`

	rows, err := d.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		var s Session
		err := rows.Scan(
			&s.ID, &s.WorkspacePath, &s.WorkspaceName, &s.Branch,
			&s.CommitSHA, &s.StartedAt, &s.LastActivity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		sessions = append(sessions, &s)
	}

	return sessions, nil
}

// DeleteOldEdits deletes edits older than the specified date
func (d *DB) DeleteOldEdits(beforeDate time.Time) (int64, error) {
	result, err := d.db.Exec("DELETE FROM edits WHERE timestamp < ?", beforeDate.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("failed to delete old edits: %w", err)
	}

	return result.RowsAffected()
}

// CapEditsPerSession caps the number of edits for a specific session
func (d *DB) CapEditsPerSession(sessionID int64, maxEdits int) (int64, error) {
	// First, count the edits
	var count int
	err := d.db.QueryRow("SELECT COUNT(*) FROM edits WHERE session_id = ?", sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count edits: %w", err)
	}

	if count <= maxEdits {
		return 0, nil // No need to cap
	}

	// Delete oldest edits beyond the limit
	query := `
		DELETE FROM edits
		WHERE session_id = ?
		AND id NOT IN (
			SELECT id FROM edits
			WHERE session_id = ?
			ORDER BY timestamp DESC
			LIMIT ?
		)
	`

	result, err := d.db.Exec(query, sessionID, sessionID, maxEdits)
	if err != nil {
		return 0, fmt.Errorf("failed to cap edits: %w", err)
	}

	return result.RowsAffected()
}

// GetDatabaseSize returns the size of the database file in bytes
func (d *DB) GetDatabaseSize() (int64, error) {
	var dbPath string
	err := d.db.QueryRow("PRAGMA database_list").Scan(&dbPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get database path: %w", err)
	}

	// For SQLite, the path is usually embedded in the result
	// We need to extract just the file path
	// For now, use a simpler approach - check the main database file
	// This is a bit of a workaround but works for our use case
	return getDatabaseSizeFromPath(d.db)
}

// getDatabaseSizeFromPath gets the database file size from PRAGMA
func getDatabaseSizeFromPath(db *sql.DB) (int64, error) {
	// Use page count and page size to calculate size
	var pageSize, pageCount int
	err := db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	if err != nil {
		return 0, err
	}

	err = db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	if err != nil {
		return 0, err
	}

	return int64(pageSize * pageCount), nil
}

// Vacuum runs VACUUM to reclaim disk space
func (d *DB) Vacuum() error {
	_, err := d.db.Exec("VACUUM")
	if err != nil {
		return fmt.Errorf("failed to vacuum: %w", err)
	}
	return nil
}
