package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/ztaylor/claude-follow-tui/internal/logger"
	"github.com/ztaylor/claude-follow-tui/internal/model"
	"github.com/ztaylor/claude-follow-tui/internal/socket"
	"github.com/ztaylor/claude-follow-tui/internal/theme"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	selectedTheme = "dark"
	debugMode     = false
	persistMode   = false
)

func main() {
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
			fmt.Println("claude-follow-tui v0.1.0")
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
	if err := logger.Init("/tmp/claude-follow-tui.log", debugMode); err != nil {
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
	fmt.Print(`claude-follow-tui - Watch Claude Code edits in real-time

Usage:
  claude-follow-tui              Run the TUI
  claude-follow-tui send         Send JSON to running TUI (for hooks)
  claude-follow-tui help         Show this help

Flags:
  --theme, -t <name>   Set color theme (default: dark)
  --list-themes        List available themes
  --persist, -p        Persist history to file (.claude-follow-history.json)
  --debug, -d          Enable debug logging

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
  When --persist is enabled, changes are saved to .claude-follow-history.json
  in the workspace root. History includes git/jj commit SHAs for context.

Mouse:
  Scroll       Scroll diff viewport
`)
}
