package daemon

import (
	"database/sql"
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

// Daemon manages the daemon server
type Daemon struct {
	db            *database.DB
	socketPath    string
	queryPath     string
	listener      net.Listener
	queryListener net.Listener
	wg            sync.WaitGroup
	shutdown      chan struct{}
}

// Config holds daemon configuration
type Config struct {
	SocketPath string
	QueryPath  string
	DBConfig   *database.Config
}

// DefaultConfig returns default daemon configuration
func DefaultConfig() (*Config, error) {
	dbCfg, err := database.DefaultConfig()
	if err != nil {
		return nil, err
	}

	return &Config{
		SocketPath: DefaultSocketPath,
		QueryPath:  DefaultQuerySocketPath,
		DBConfig:   dbCfg,
	}, nil
}

// New creates a new daemon
func New(cfg *Config) (*Daemon, error) {
	db, err := database.Open(cfg.DBConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Daemon{
		db:         db,
		socketPath: cfg.SocketPath,
		queryPath:  cfg.QueryPath,
		shutdown:   make(chan struct{}),
	}, nil
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
	SessionID     int64    `json:"session_id"`
	Workspace     string   `json:"workspace"`
	WorkspaceName string   `json:"workspace_name"`
	Branch        string   `json:"branch"`
	CommitSHA     string   `json:"commit_sha"`
	ToolName      string   `json:"tool_name"`
	FilePath      string   `json:"file_path"`
	OldString     string   `json:"old_string"`
	NewString     string   `json:"new_string"`
	LineNum       int      `json:"line_num"`
	LineCount     int      `json:"line_count"`
	Type          string   `json:"type"` // "edit" or "prompt"
	PromptName    string   `json:"prompt_name,omitempty"`
	PromptDesc    string   `json:"prompt_description,omitempty"`
	PromptTags    []string `json:"prompt_tags,omitempty"`
}

// processPayload processes incoming hook data
func (d *Daemon) processPayload(payload *HookPayload) error {
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
		}
		if err := d.db.RecordEdit(edit); err != nil {
			return fmt.Errorf("failed to record edit: %w", err)
		}
		logger.Log("Recorded edit: %s to %s", payload.ToolName, payload.FilePath)

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

// sqlInt64 converts int64 to sql.NullInt64
func sqlInt64(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

// Query represents a database query
type Query struct {
	Type     string `json:"type"` // "recent", "file", "prompts", "sessions"
	FilePath string `json:"file_path,omitempty"`
	Name     string `json:"name,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// QueryResult represents query results
type QueryResult struct {
	Type     string              `json:"type"`
	Edits    []*database.Edit    `json:"edits,omitempty"`
	Prompts  []*database.Prompt  `json:"prompts,omitempty"`
	Sessions []*database.Session `json:"sessions,omitempty"`
}

// executeQuery executes a database query
func (d *Daemon) executeQuery(query *Query) (*QueryResult, error) {
	result := &QueryResult{Type: query.Type}

	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}

	switch query.Type {
	case "recent":
		edits, err := d.db.GetRecentEdits(limit)
		if err != nil {
			return nil, err
		}
		result.Edits = edits

	case "file":
		if query.FilePath == "" {
			return nil, fmt.Errorf("file_path required for file queries")
		}
		edits, err := d.db.GetEditsByFile(query.FilePath, limit)
		if err != nil {
			return nil, err
		}
		result.Edits = edits

	case "prompts":
		name := query.Name
		if name == "" {
			name = "%"
		}
		prompts, err := d.db.GetPrompts(name, limit)
		if err != nil {
			return nil, err
		}
		result.Prompts = prompts

	case "sessions":
		sessions, err := d.db.GetSessions(limit)
		if err != nil {
			return nil, err
		}
		result.Sessions = sessions

	default:
		return nil, fmt.Errorf("unknown query type: %s", query.Type)
	}

	return result, nil
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
