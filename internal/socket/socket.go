package socket

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

// GetSocketPath returns the socket path for the current workspace.
// Uses the same hashing scheme as the neovim plugin for consistency.
func GetSocketPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	// Resolve to absolute path (like neovim plugin does)
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		// Fall back to whatever we have
	}

	// Resolve symlinks
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}

	// Hash the path
	hash := sha256.Sum256([]byte(cwd))
	hashStr := fmt.Sprintf("%x", hash)[:12]

	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	return fmt.Sprintf("/tmp/claude-mon-%s-%s.sock", user, hashStr)
}

// Listener handles incoming socket connections
type Listener struct {
	socketPath string
	listener   net.Listener
	messages   chan []byte
}

// NewListener creates a new socket listener
func NewListener(socketPath string) (*Listener, error) {
	// Remove existing socket file if it exists
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket: %w", err)
	}

	return &Listener{
		socketPath: socketPath,
		listener:   listener,
		messages:   make(chan []byte, 100), // Buffered channel for messages
	}, nil
}

// Listen starts accepting connections and calls handler for each payload
func (l *Listener) Listen(handler func([]byte)) {
	// Start a goroutine to process messages from the channel
	go func() {
		for data := range l.messages {
			handler(data)
		}
	}()

	// Accept connections and queue messages
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			// Listener was closed
			close(l.messages)
			return
		}

		go func(c net.Conn) {
			defer c.Close()

			// Read all data from connection until EOF
			data, err := io.ReadAll(c)
			if err != nil {
				return
			}

			if len(data) > 0 {
				// Send to buffered channel (non-blocking if buffer not full)
				select {
				case l.messages <- data:
				default:
					// Buffer full, drop message (shouldn't happen with 100 buffer)
				}
			}
		}(conn)
	}
}

// Close closes the listener and removes the socket file
func (l *Listener) Close() error {
	l.listener.Close()
	return os.Remove(l.socketPath)
}
