package daemon

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ztaylor/claude-mon/internal/database"
	"github.com/ztaylor/claude-mon/internal/logger"
)

const (
	// DefaultSocketPath is the default path for the daemon socket
	DefaultSocketPath = "/tmp/claude-mon-daemon.sock"
	// DefaultQuerySocketPath is the default path for query socket
	DefaultQuerySocketPath = "/tmp/claude-mon-query.sock"
)

// WorkspaceActivity tracks activity for a workspace
type WorkspaceActivity struct {
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	LastActivity time.Time `json:"last_activity"`
	EditCount    int       `json:"edit_count"`
}

// Daemon manages the daemon server
type Daemon struct {
	cfg            *Config
	db             *database.DB
	cleanupManager *CleanupManager
	backupManager  *BackupManager
	socketPath     string
	queryPath      string
	listener       net.Listener
	queryListener  net.Listener
	wg             sync.WaitGroup
	shutdown       chan struct{}

	// Activity tracking
	workspacesMu sync.RWMutex
	workspaces   map[string]*WorkspaceActivity
	startedAt    time.Time
}

// DefaultConfig returns default daemon configuration
// Deprecated: Use LoadConfig() instead for full configuration support
func DefaultConfig() (*Config, error) {
	return LoadConfig("")
}

// New creates a new daemon
func New(cfg *Config) (*Daemon, error) {
	dbCfg, err := cfg.ToDBConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get database config: %w", err)
	}

	db, err := database.Open(dbCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	d := &Daemon{
		cfg:        cfg,
		db:         db,
		socketPath: cfg.Sockets.DaemonSocket,
		queryPath:  cfg.Sockets.QuerySocket,
		shutdown:   make(chan struct{}),
		workspaces: make(map[string]*WorkspaceActivity),
		startedAt:  time.Now(),
	}

	// Initialize cleanup manager
	d.cleanupManager = NewCleanupManager(cfg, db)

	// Initialize backup manager
	d.backupManager = NewBackupManager(cfg)

	return d, nil
}

// Start starts the daemon server
func (d *Daemon) Start() error {
	// Remove existing socket if present
	os.Remove(d.socketPath)
	os.Remove(d.queryPath)

	// Create data socket listener
	listener, err := net.Listen("unix", d.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", d.socketPath, err)
	}
	d.listener = listener

	// Create query socket listener
	queryListener, err := net.Listen("unix", d.queryPath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", d.queryPath, err)
	}
	d.queryListener = queryListener

	logger.Log("Daemon started on %s (query: %s)", d.socketPath, d.queryPath)

	// Start cleanup manager
	d.cleanupManager.Start()

	// Start backup manager
	d.backupManager.Start()

	// Start accept goroutines
	d.wg.Add(2)
	go d.acceptConnections()
	go d.acceptQueries()

	// Wait for shutdown signal
	return d.waitForShutdown()
}

// acceptConnections accepts data connections from hooks
func (d *Daemon) acceptConnections() {
	defer d.wg.Done()

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.shutdown:
				return
			default:
				logger.Log("Accept error: %v", err)
				continue
			}
		}

		d.wg.Add(1)
		go d.handleConnection(conn)
	}
}

// acceptQueries accepts query connections from CLI
func (d *Daemon) acceptQueries() {
	defer d.wg.Done()

	for {
		conn, err := d.queryListener.Accept()
		if err != nil {
			select {
			case <-d.shutdown:
				return
			default:
				logger.Log("Query accept error: %v", err)
				continue
			}
		}

		d.wg.Add(1)
		go d.handleQuery(conn)
	}
}

// handleConnection handles a data connection from a hook
func (d *Daemon) handleConnection(conn net.Conn) {
	defer d.wg.Done()
	defer conn.Close()

	logger.Log("New data connection from %s", conn.RemoteAddr())

	decoder := json.NewDecoder(conn)
	for {
		var payload HookPayload
		if err := decoder.Decode(&payload); err != nil {
			if err != io.EOF {
				logger.Log("Decode error: %v", err)
			}
			break
		}

		if err := d.processPayload(&payload); err != nil {
			logger.Log("Process payload error: %v", err)
			// Send error back
			json.NewEncoder(conn).Encode(map[string]string{"error": err.Error()})
		} else {
			// Send success
			json.NewEncoder(conn).Encode(map[string]string{"status": "ok"})
		}
	}
}

// handleQuery handles a query connection from CLI
func (d *Daemon) handleQuery(conn net.Conn) {
	defer d.wg.Done()
	defer conn.Close()

	logger.Log("New query connection from %s", conn.RemoteAddr())

	decoder := json.NewDecoder(conn)
	var query Query
	if err := decoder.Decode(&query); err != nil {
		logger.Log("Query decode error: %v", err)
		return
	}

	// Execute query
	result, err := d.executeQuery(&query)
	if err != nil {
		logger.Log("Query execution error: %v", err)
		json.NewEncoder(conn).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Send result
	if err := json.NewEncoder(conn).Encode(result); err != nil {
		logger.Log("Query response error: %v", err)
	}
}

// HookPayload represents data from Claude hooks
type HookPayload struct {
	SessionID      int64    `json:"session_id"`
	Workspace      string   `json:"workspace"`
	WorkspaceName  string   `json:"workspace_name"`
	Branch         string   `json:"branch"`
	CommitSHA      string   `json:"commit_sha"`
	VCSType        string   `json:"vcs_type"` // "git" or "jj"
	ToolName       string   `json:"tool_name"`
	FilePath       string   `json:"file_path"`
	OldString      string   `json:"old_string"`
	NewString      string   `json:"new_string"`
	FileContentB64 string   `json:"file_content_b64"` // base64-encoded file content
	LineNum        int      `json:"line_num"`
	LineCount      int      `json:"line_count"`
	Type           string   `json:"type"` // "edit" or "prompt"
	PromptName     string   `json:"prompt_name,omitempty"`
	PromptDesc     string   `json:"prompt_description,omitempty"`
	PromptTags     []string `json:"prompt_tags,omitempty"`
}

// processPayload processes incoming hook data
func (d *Daemon) processPayload(payload *HookPayload) error {
	// Check if workspace should be tracked
	if !d.cfg.ShouldTrackWorkspace(payload.Workspace) {
		logger.Log("Workspace %s is being ignored", payload.Workspace)
		return nil
	}

	// Track workspace activity
	d.trackWorkspaceActivity(payload.Workspace, payload.WorkspaceName, payload.Type == "edit")

	// Ensure session exists
	sessionID, err := d.db.UpsertSession(
		payload.Workspace,
		payload.WorkspaceName,
		payload.Branch,
		payload.CommitSHA,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert session: %w", err)
	}

	switch payload.Type {
	case "edit":
		edit := &database.Edit{
			SessionID: sessionID,
			ToolName:  payload.ToolName,
			FilePath:  payload.FilePath,
			OldString: payload.OldString,
			NewString: payload.NewString,
			LineNum:   payload.LineNum,
			LineCount: payload.LineCount,
			CommitSHA: payload.CommitSHA,
			VCSType:   payload.VCSType,
		}

		// Decode and compress file content if provided
		if payload.FileContentB64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(payload.FileContentB64)
			if err != nil {
				logger.Log("Warning: failed to decode file content: %v", err)
			} else {
				// Compress the file content with gzip
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				if _, err := w.Write(decoded); err != nil {
					logger.Log("Warning: failed to compress file content: %v", err)
				} else if err := w.Close(); err != nil {
					logger.Log("Warning: failed to finalize compression: %v", err)
				} else {
					edit.FileSnapshot = buf.Bytes()
					logger.Log("Compressed file snapshot: %d bytes -> %d bytes", len(decoded), len(edit.FileSnapshot))
				}
			}
		} else {
			logger.Log("No file_content_b64 provided for %s (file: %s)", payload.ToolName, payload.FilePath)
		}

		if err := d.db.RecordEdit(edit); err != nil {
			return fmt.Errorf("failed to record edit: %w", err)
		}
		logger.Log("Recorded edit: %s to %s (vcs=%s, sha=%s)", payload.ToolName, payload.FilePath, payload.VCSType, payload.CommitSHA)

	case "prompt":
		prompt := &database.Prompt{
			SessionID:   sqlInt64(sessionID),
			Name:        payload.PromptName,
			Description: payload.PromptDesc,
			Content:     payload.NewString, // For prompts, content is in new_string
			Tags:        payload.PromptTags,
			IsGlobal:    false,
		}
		if _, err := d.db.RecordPrompt(prompt); err != nil {
			return fmt.Errorf("failed to record prompt: %w", err)
		}
		logger.Log("Recorded prompt: %s", payload.PromptName)

	default:
		return fmt.Errorf("unknown payload type: %s", payload.Type)
	}

	return nil
}

// trackWorkspaceActivity updates the activity tracker for a workspace
func (d *Daemon) trackWorkspaceActivity(path, name string, isEdit bool) {
	d.workspacesMu.Lock()
	defer d.workspacesMu.Unlock()

	activity, exists := d.workspaces[path]
	if !exists {
		activity = &WorkspaceActivity{
			Path: path,
			Name: name,
		}
		d.workspaces[path] = activity
	}

	activity.LastActivity = time.Now()
	if isEdit {
		activity.EditCount++
	}
}

// sqlInt64 converts int64 to sql.NullInt64
func sqlInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

// Query represents a database query
type Query struct {
	Type          string `json:"type"` // "recent", "workspace", "file", "prompts", "sessions", "status"
	WorkspacePath string `json:"workspace_path,omitempty"`
	FilePath      string `json:"file_path,omitempty"`
	Name          string `json:"name,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

// StatusResult represents daemon status
type StatusResult struct {
	Running         bool                          `json:"running"`
	Uptime          time.Duration                 `json:"uptime"`
	UptimeStr       string                        `json:"uptime_str"`
	ActiveWorkspace *WorkspaceActivity            `json:"active_workspace,omitempty"`
	Workspaces      map[string]*WorkspaceActivity `json:"workspaces"`
}

// QueryResult represents query results
type QueryResult struct {
	Type     string              `json:"type"`
	Edits    []*database.Edit    `json:"edits,omitempty"`
	Prompts  []*database.Prompt  `json:"prompts,omitempty"`
	Sessions []*database.Session `json:"sessions,omitempty"`
	Status   *StatusResult       `json:"status,omitempty"`
}

// executeQuery executes a database query
func (d *Daemon) executeQuery(query *Query) (*QueryResult, error) {
	result := &QueryResult{
		Type:     query.Type,
		Edits:    []*database.Edit{},
		Prompts:  []*database.Prompt{},
		Sessions: []*database.Session{},
	}

	limit := query.Limit
	if limit <= 0 {
		limit = d.cfg.Query.DefaultLimit
	}

	// Enforce max limit
	if limit > d.cfg.Query.MaxLimit {
		limit = d.cfg.Query.MaxLimit
	}

	switch query.Type {
	case "recent":
		edits, err := d.db.GetRecentEdits(limit)
		if err != nil {
			return nil, err
		}
		if edits != nil {
			result.Edits = edits
		}

	case "workspace":
		if query.WorkspacePath == "" {
			return nil, fmt.Errorf("workspace_path required for workspace queries")
		}
		edits, err := d.db.GetEditsByWorkspace(query.WorkspacePath, limit)
		if err != nil {
			return nil, err
		}
		if edits != nil {
			result.Edits = edits
		}

	case "file":
		if query.FilePath == "" {
			return nil, fmt.Errorf("file_path required for file queries")
		}
		edits, err := d.db.GetEditsByFile(query.FilePath, limit)
		if err != nil {
			return nil, err
		}
		if edits != nil {
			result.Edits = edits
		}

	case "prompts":
		name := query.Name
		if name == "" {
			name = "%"
		}
		prompts, err := d.db.GetPrompts(name, limit)
		if err != nil {
			return nil, err
		}
		if prompts != nil {
			result.Prompts = prompts
		}

	case "sessions":
		sessions, err := d.db.GetSessions(limit)
		if err != nil {
			return nil, err
		}
		if sessions != nil {
			result.Sessions = sessions
		}

	case "status":
		result.Status = d.getStatus(query.WorkspacePath)

	default:
		return nil, fmt.Errorf("unknown query type: %s", query.Type)
	}

	return result, nil
}

// getStatus returns the daemon status, optionally checking for a specific workspace
func (d *Daemon) getStatus(workspacePath string) *StatusResult {
	uptime := time.Since(d.startedAt)

	// Format uptime string
	var uptimeStr string
	if uptime < time.Minute {
		uptimeStr = fmt.Sprintf("%ds", int(uptime.Seconds()))
	} else if uptime < time.Hour {
		uptimeStr = fmt.Sprintf("%dm", int(uptime.Minutes()))
	} else if uptime < 24*time.Hour {
		hours := int(uptime.Hours())
		mins := int(uptime.Minutes()) % 60
		uptimeStr = fmt.Sprintf("%dh %dm", hours, mins)
	} else {
		days := int(uptime.Hours() / 24)
		hours := int(uptime.Hours()) % 24
		uptimeStr = fmt.Sprintf("%dd %dh", days, hours)
	}

	d.workspacesMu.RLock()
	defer d.workspacesMu.RUnlock()

	// Copy workspaces map
	workspaces := make(map[string]*WorkspaceActivity, len(d.workspaces))
	for k, v := range d.workspaces {
		workspaces[k] = v
	}

	status := &StatusResult{
		Running:    true,
		Uptime:     uptime,
		UptimeStr:  uptimeStr,
		Workspaces: workspaces,
	}

	// Check if specific workspace is active
	if workspacePath != "" {
		if activity, exists := d.workspaces[workspacePath]; exists {
			status.ActiveWorkspace = activity
		}
	}

	return status
}

// waitForShutdown waits for shutdown signal
func (d *Daemon) waitForShutdown() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	select {
	case sig := <-sigChan:
		logger.Log("Received signal: %v", sig)
		return d.Stop()
	case <-d.shutdown:
		return nil
	}
}

// Stop stops the daemon
func (d *Daemon) Stop() error {
	logger.Log("Shutting down daemon...")

	close(d.shutdown)

	// Stop cleanup manager
	d.cleanupManager.Stop()

	// Stop backup manager
	d.backupManager.Stop()

	// Close listeners
	if d.listener != nil {
		d.listener.Close()
	}
	if d.queryListener != nil {
		d.queryListener.Close()
	}

	// Wait for connections to finish
	done := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Log("All connections closed")
	case <-time.After(10 * time.Second):
		logger.Log("Timeout waiting for connections")
	}

	// Close database
	if err := d.db.Close(); err != nil {
		logger.Log("Database close error: %v", err)
	}

	// Remove socket files
	os.Remove(d.socketPath)
	os.Remove(d.queryPath)

	logger.Log("Daemon stopped")
	return nil
}

// Run starts the daemon and blocks until stopped
func (d *Daemon) Run() error {
	return d.Start()
}
