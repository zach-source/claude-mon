package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/ztaylor/claude-mon/internal/daemon"
	"github.com/ztaylor/claude-mon/internal/logger"
	"github.com/ztaylor/claude-mon/internal/model"
	"github.com/ztaylor/claude-mon/internal/socket"
	"github.com/ztaylor/claude-mon/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	selectedTheme = "dark"
	debugMode     = false
	persistMode   = false
	configPath    = ""
)

func main() {
	// Handle daemon and query commands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "daemon":
			if err := handleDaemonCommand(); err != nil {
				fmt.Fprintf(os.Stderr, "Daemon error: %v\n", err)
				os.Exit(1)
			}
			return
		case "query":
			if err := handleQueryCommand(); err != nil {
				fmt.Fprintf(os.Stderr, "Query error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	// Parse flags first
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--theme", "-t":
			if i+1 < len(args) {
				selectedTheme = args[i+1]
				i++ // skip next arg
			}
		case "--debug", "-d":
			debugMode = true
		case "--persist", "-p":
			persistMode = true
		case "--config":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++ // skip next arg
			}
		case "--list-themes":
			fmt.Println("Available themes:")
			for _, name := range theme.Available() {
				if name == "dark" {
					fmt.Printf("  %s (default)\n", name)
				} else {
					fmt.Printf("  %s\n", name)
				}
			}
			return
		case "send":
			if err := sendToSocket(); err != nil {
				// Fail silently - TUI might not be running
				os.Exit(0)
			}
			return
		case "--help", "-h", "help":
			printHelp()
			return
		case "--version", "-v", "version":
			fmt.Println("claude-mon v0.1.0")
			return
		case "write-config":
			// Get path from next argument if available
			writePath := ""
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				writePath = args[i+1]
			}
			if err := writeDefaultConfig(writePath); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	// Validate theme
	validTheme := false
	for _, name := range theme.Available() {
		if name == selectedTheme {
			validTheme = true
			break
		}
	}
	if !validTheme {
		fmt.Fprintf(os.Stderr, "Unknown theme: %s\nAvailable: %s\n",
			selectedTheme, strings.Join(theme.Available(), ", "))
		os.Exit(1)
	}

	// Run TUI
	if err := runTUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI() error {
	// Initialize logger (only logs to file when debug mode enabled)
	if err := logger.Init("/tmp/claude-mon.log", debugMode); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not init logger: %v\n", err)
	}
	defer logger.Close()
	logger.Log("Starting TUI, debug=%v, persist=%v", debugMode, persistMode)

	// Create socket listener
	socketPath := socket.GetSocketPath()
	listener, err := socket.NewListener(socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	defer listener.Close()

	// Create the Bubbletea program with theme and options
	t := theme.Get(selectedTheme)
	m := model.New(socketPath, model.WithTheme(t), model.WithPersistence(persistMode))
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start socket listener in goroutine, sending messages to program
	go listener.Listen(func(payload []byte) {
		p.Send(model.SocketMsg{Payload: payload})
	})

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running program: %w", err)
	}

	return nil
}

func sendToSocket() error {
	socketPath := socket.GetSocketPath()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		// Socket doesn't exist or TUI not running
		return err
	}
	defer conn.Close()

	// Copy stdin to socket
	_, err = io.Copy(conn, os.Stdin)
	return err
}

func printHelp() {
	fmt.Print(`claude-mon (clmon) - Watch Claude Code edits in real-time

Usage:
  claude-mon, clmon              Run the TUI
  claude-mon send, clmon send    Send JSON to running TUI (for hooks)
  claude-mon help, clmon help    Show this help

Flags:
  --theme, -t <name>   Set color theme (default: dark)
  --list-themes        List available themes
  --persist, -p        Persist history to file (.claude-mon-history.json)
  --debug, -d          Enable debug logging
  --config <path>      Path to daemon config file (default: ~/.config/claude-mon/daemon.toml)

Config Commands:
  write-config                 Write default configuration to file
  write-config <path>          Write configuration to custom path

Available themes: dark, light, dracula, monokai, gruvbox, nord, catppuccin

Keybindings:
  n/p          Navigate changes in queue
  j/k          Scroll diff up/down
  ←/→          Scroll horizontally
  Tab          Switch panes
  Ctrl+G       Open file in nvim at line
  Ctrl+O       Open file in nvim
  h            Toggle history pane
  m            Toggle minimap
  c            Clear history
  q            Quit
  ?            Show help

History:
  When --persist is enabled, changes are saved to .claude-mon-history.json
  in the workspace root. History includes git/jj commit SHAs for context.

Mouse:
  Scroll       Scroll diff viewport

Daemon Commands:
  claude-mon daemon start       Start the background daemon
  claude-mon daemon stop        Stop the background daemon
  claude-mon daemon status      Check daemon status

Query Commands:
  claude-mon query recent       Show recent activity (all sessions)
  claude-mon query file <path>  Show edits for specific file
  claude-mon query prompts      List all prompts
  claude-mon query sessions     List all sessions
`)
}

// handleDaemonCommand handles daemon subcommands
func handleDaemonCommand() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: claude-mon daemon {start|stop|status}")
	}

	cmd := os.Args[2]
	switch cmd {
	case "start":
		return startDaemon()
	case "stop":
		return stopDaemon()
	case "status":
		return daemonStatus()
	default:
		return fmt.Errorf("unknown daemon command: %s", cmd)
	}
}

// startDaemon starts the daemon in foreground
func startDaemon() error {
	cfg, err := daemon.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	d, err := daemon.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	fmt.Println("Starting claude-mon daemon...")
	fmt.Printf("Data socket: %s\n", cfg.Sockets.DaemonSocket)
	fmt.Printf("Query socket: %s\n", cfg.Sockets.QuerySocket)
	fmt.Printf("Database: %s\n", cfg.GetDBPath())
	fmt.Println("Press Ctrl+C to stop")

	return d.Run()
}

// stopDaemon stops the running daemon
func stopDaemon() error {
	conn, err := net.Dial("unix", daemon.DefaultSocketPath)
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}
	defer conn.Close()

	// Send shutdown signal
	fmt.Println("Stopping daemon...")
	conn.Close()

	// Wait a bit for graceful shutdown
	// In production, we'd use PID file or systemd
	fmt.Println("Daemon stopped")
	return nil
}

// daemonStatus checks if daemon is running
func daemonStatus() error {
	conn, err := net.Dial("unix", daemon.DefaultSocketPath)
	if err != nil {
		fmt.Println("Daemon: not running")
		return nil
	}
	defer conn.Close()

	fmt.Println("Daemon: running")
	return nil
}

// handleQueryCommand handles query commands
func handleQueryCommand() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: claude-mon query {recent|file|prompts|sessions} [args]")
	}

	queryType := os.Args[2]
	query := &daemon.Query{Type: queryType}

	switch queryType {
	case "recent":
		// Optional limit
		if len(os.Args) > 3 {
			fmt.Sscanf(os.Args[3], "%d", &query.Limit)
		}
	case "file":
		if len(os.Args) < 4 {
			return fmt.Errorf("usage: claude-mon query file <path> [limit]")
		}
		query.FilePath = os.Args[3]
		if len(os.Args) > 4 {
			fmt.Sscanf(os.Args[4], "%d", &query.Limit)
		}
	case "prompts":
		if len(os.Args) > 3 {
			query.Name = os.Args[3]
		}
		if len(os.Args) > 4 {
			fmt.Sscanf(os.Args[4], "%d", &query.Limit)
		}
	case "sessions":
		if len(os.Args) > 3 {
			fmt.Sscanf(os.Args[3], "%d", &query.Limit)
		}
	default:
		return fmt.Errorf("unknown query type: %s", queryType)
	}

	return executeQuery(query)
}

// executeQuery sends query to daemon and prints results
func executeQuery(query *daemon.Query) error {
	conn, err := net.Dial("unix", daemon.DefaultQuerySocketPath)
	if err != nil {
		return fmt.Errorf("daemon not running: %w", err)
	}
	defer conn.Close()

	// Send query
	if err := json.NewEncoder(conn).Encode(query); err != nil {
		return fmt.Errorf("failed to send query: %w", err)
	}

	// Read response
	var result daemon.QueryResult
	if err := json.NewDecoder(conn).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Print results
	switch result.Type {
	case "recent", "file":
		if len(result.Edits) == 0 {
			fmt.Println("No edits found")
			return nil
		}
		for _, edit := range result.Edits {
			fmt.Printf("[%s] %s:%d\n", edit.ToolName, edit.FilePath, edit.LineNum)
			fmt.Printf("  Timestamp: %s\n", edit.Timestamp.Format("2006-01-02 15:04:05"))
		}
	case "prompts":
		if len(result.Prompts) == 0 {
			fmt.Println("No prompts found")
			return nil
		}
		for _, prompt := range result.Prompts {
			fmt.Printf("Name: %s (v%d)\n", prompt.Name, prompt.Version)
			if prompt.Description != "" {
				fmt.Printf("  Description: %s\n", prompt.Description)
			}
			fmt.Printf("  Tags: %v\n", prompt.Tags)
			fmt.Printf("  Updated: %s\n\n", prompt.UpdatedAt.Format("2006-01-02 15:04:05"))
		}
	case "sessions":
		if len(result.Sessions) == 0 {
			fmt.Println("No sessions found")
			return nil
		}
		for _, session := range result.Sessions {
			fmt.Printf("Workspace: %s\n", session.WorkspaceName)
			fmt.Printf("  Path: %s\n", session.WorkspacePath)
			fmt.Printf("  Branch: %s\n", session.Branch)
			fmt.Printf("  Last Activity: %s\n\n", session.LastActivity.Format("2006-01-02 15:04:05"))
		}
	}

	return nil
}

// writeDefaultConfig writes the default configuration to a file
func writeDefaultConfig(path string) error {
	// Use default path if not provided
	if path == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, ".config", "claude-mon", "daemon.toml")
	}

	// Write config
	if err := daemon.WriteDefaultConfig(path); err != nil {
		return err
	}

	fmt.Printf("Default configuration written to: %s\n", path)
	fmt.Println("Edit this file to customize your daemon settings.")
	return nil
}
