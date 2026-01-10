package main

import (
	"fmt"
	"io"
	"net"
	"os"

	"github.com/ztaylor/claude-follow-tui/internal/model"
	"github.com/ztaylor/claude-follow-tui/internal/socket"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
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

	// Run TUI
	if err := runTUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI() error {
	// Create socket listener
	socketPath := socket.GetSocketPath()
	listener, err := socket.NewListener(socketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	defer listener.Close()

	// Create the Bubbletea program
	m := model.New(socketPath)
	p := tea.NewProgram(m, tea.WithAltScreen())

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
  --persist    Persist history to SQLite (not yet implemented)
  --debug      Enable debug logging

Keybindings:
  j/k          Navigate history
  Tab          Switch panes
  Enter        Expand/collapse diff
  Ctrl+G       Open file in nvim at line
  Ctrl+O       Open file in nvim
  c            Clear history
  q            Quit
  ?            Show help
`)
}
