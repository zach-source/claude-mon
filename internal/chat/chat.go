package chat

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

// Message represents a chat message
type Message struct {
	Role      string    // "user" or "assistant"
	Content   string    // Message content
	Timestamp time.Time // When the message was sent/received
}

// ClaudeChat manages a PTY-based Claude CLI subprocess
type ClaudeChat struct {
	ptmx     *os.File        // PTY master
	cmd      *exec.Cmd       // Claude CLI process
	output   strings.Builder // Accumulated output
	messages []Message       // Chat history
	active   bool            // Whether chat is active
	mu       sync.Mutex      // Protects shared state

	// Channels for communication
	outputCh chan string   // Output from Claude
	doneCh   chan struct{} // Signals chat has ended
	errCh    chan error    // Errors from the subprocess
}

// New creates a new ClaudeChat instance
func New() *ClaudeChat {
	return &ClaudeChat{
		messages: make([]Message, 0),
		outputCh: make(chan string, 100),
		doneCh:   make(chan struct{}),
		errCh:    make(chan error, 1),
	}
}

// Start launches the Claude CLI in a PTY
func (c *ClaudeChat) Start(mcpConfigPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active {
		return fmt.Errorf("chat already active")
	}

	// Build command with optional MCP config
	args := []string{}
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
	}

	c.cmd = exec.Command("claude", args...)
	c.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Start with PTY
	var err error
	c.ptmx, err = pty.Start(c.cmd)
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	c.active = true
	c.output.Reset()
	c.messages = make([]Message, 0)

	// Start goroutine to read output
	go c.readOutput()

	return nil
}

// readOutput reads from the PTY and sends output to the channel
func (c *ClaudeChat) readOutput() {
	reader := bufio.NewReader(c.ptmx)
	buf := make([]byte, 4096)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err != io.EOF {
				select {
				case c.errCh <- err:
				default:
				}
			}
			break
		}

		if n > 0 {
			chunk := string(buf[:n])
			c.mu.Lock()
			c.output.WriteString(chunk)
			c.mu.Unlock()

			select {
			case c.outputCh <- chunk:
			default:
				// Channel full, skip
			}
		}
	}

	close(c.doneCh)
}

// Send sends a message to Claude
func (c *ClaudeChat) Send(input string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.active || c.ptmx == nil {
		return fmt.Errorf("chat not active")
	}

	// Record user message
	c.messages = append(c.messages, Message{
		Role:      "user",
		Content:   input,
		Timestamp: time.Now(),
	})

	// Send to PTY with newline
	_, err := c.ptmx.Write([]byte(input + "\n"))
	return err
}

// Stop terminates the Claude CLI process
func (c *ClaudeChat) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.active {
		return nil
	}

	c.active = false

	// Close PTY
	if c.ptmx != nil {
		c.ptmx.Close()
	}

	// Kill process
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}

	return nil
}

// IsActive returns whether the chat is currently active
func (c *ClaudeChat) IsActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Output returns the accumulated output
func (c *ClaudeChat) Output() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}

// Messages returns the chat message history
func (c *ClaudeChat) Messages() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]Message, len(c.messages))
	copy(result, c.messages)
	return result
}

// OutputChan returns the channel for receiving output
func (c *ClaudeChat) OutputChan() <-chan string {
	return c.outputCh
}

// DoneChan returns the channel that signals chat has ended
func (c *ClaudeChat) DoneChan() <-chan struct{} {
	return c.doneCh
}

// ErrorChan returns the channel for receiving errors
func (c *ClaudeChat) ErrorChan() <-chan error {
	return c.errCh
}

// ClearOutput clears the accumulated output
func (c *ClaudeChat) ClearOutput() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.output.Reset()
}

// SetSize sets the PTY window size
func (c *ClaudeChat) SetSize(rows, cols int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ptmx == nil {
		return nil
	}

	return pty.Setsize(c.ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}
