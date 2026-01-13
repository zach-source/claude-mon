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
	"github.com/ztaylor/claude-follow-tui/internal/logger"
)

// Mode represents the chat operation mode
type Mode int

const (
	ModeInteractive Mode = iota // Interactive PTY-based chat
	ModeObjective               // Objective-based (auto-completes when done)
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

	// Mode and objective tracking
	mode      Mode   // Current operation mode
	objective string // The objective/prompt for objective mode

	// Channels for communication
	outputCh    chan string   // Output from Claude
	doneCh      chan struct{} // Signals chat has ended
	errCh       chan error    // Errors from the subprocess
	completedCh chan struct{} // Signals objective completed (for objective mode)
}

// New creates a new ClaudeChat instance
func New() *ClaudeChat {
	return &ClaudeChat{
		messages:    make([]Message, 0),
		outputCh:    make(chan string, 100),
		doneCh:      make(chan struct{}),
		errCh:       make(chan error, 1),
		completedCh: make(chan struct{}),
		mode:        ModeInteractive,
	}
}

// Start launches the Claude CLI in a PTY (interactive mode, isolated session)
func (c *ClaudeChat) Start(mcpConfigPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active {
		return fmt.Errorf("chat already active")
	}

	// Build command - fresh session (no --continue to avoid polluting main chat)
	args := []string{}
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
	}

	logger.Log("Starting claude CLI with args: %v", args)

	c.cmd = exec.Command("claude", args...)
	c.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Start with PTY
	var err error
	c.ptmx, err = pty.Start(c.cmd)
	if err != nil {
		logger.Log("Failed to start PTY: %v", err)
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	logger.Log("PTY started successfully, PID: %d", c.cmd.Process.Pid)

	c.active = true
	c.mode = ModeInteractive
	c.output.Reset()
	c.messages = make([]Message, 0)

	// Start goroutine to read output
	go c.readOutput()

	return nil
}

// StartWithObjective launches Claude with an initial objective (non-interactive print mode)
// Claude will run until the objective is complete, then signal completion
func (c *ClaudeChat) StartWithObjective(objective string, mcpConfigPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active {
		return fmt.Errorf("chat already active")
	}

	// Build command with -p (print mode) for non-interactive execution
	args := []string{"-p", objective}
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
	}

	logger.Log("Starting claude CLI with objective: %s, args: %v", objective[:min(50, len(objective))], args)

	c.cmd = exec.Command("claude", args...)
	c.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Start with PTY (still use PTY for output capture)
	var err error
	c.ptmx, err = pty.Start(c.cmd)
	if err != nil {
		logger.Log("Failed to start PTY for objective: %v", err)
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	logger.Log("PTY started for objective, PID: %d", c.cmd.Process.Pid)

	c.active = true
	c.mode = ModeObjective
	c.objective = objective
	c.output.Reset()
	c.messages = make([]Message, 0)

	// Record the objective as first message
	c.messages = append(c.messages, Message{
		Role:      "user",
		Content:   objective,
		Timestamp: time.Now(),
	})

	// Start goroutine to read output and detect completion
	go c.readOutputObjective()

	return nil
}

// readOutput reads from the PTY and sends output to the channel (interactive mode)
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
			logger.Log("Chat PTY read: %d bytes", n)
			c.mu.Lock()
			c.output.WriteString(chunk)
			c.mu.Unlock()

			select {
			case c.outputCh <- chunk:
				logger.Log("Chat output sent to channel")
			default:
				logger.Log("Chat output channel full, skipped")
			}
		}
	}

	logger.Log("Chat readOutput loop ended")
	close(c.doneCh)
}

// readOutputObjective reads output in objective mode and signals completion when process exits
func (c *ClaudeChat) readOutputObjective() {
	logger.Log("Objective mode: starting output reader")
	reader := bufio.NewReader(c.ptmx)
	buf := make([]byte, 4096)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			logger.Log("Objective mode: read error: %v", err)
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
			logger.Log("Objective mode PTY read: %d bytes", n)
			c.mu.Lock()
			c.output.WriteString(chunk)
			c.mu.Unlock()

			select {
			case c.outputCh <- chunk:
				logger.Log("Objective mode output sent to channel")
			default:
				logger.Log("Objective mode output channel full, skipped")
			}
		}
	}

	// In objective mode, process exit means objective complete
	c.mu.Lock()
	wasActive := c.active
	c.active = false
	c.mu.Unlock()

	logger.Log("Objective mode: process ended, wasActive=%v", wasActive)

	if wasActive {
		// Signal completion
		select {
		case c.completedCh <- struct{}{}:
			logger.Log("Objective mode: sent completion signal")
		default:
			logger.Log("Objective mode: completion channel full")
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
		logger.Log("SetSize called but ptmx is nil")
		return nil
	}

	logger.Log("Setting PTY size: %dx%d", cols, rows)
	return pty.Setsize(c.ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

// Mode returns the current chat mode
func (c *ClaudeChat) Mode() Mode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode
}

// Objective returns the objective for objective mode
func (c *ClaudeChat) Objective() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.objective
}

// CompletedChan returns channel that signals objective completion
func (c *ClaudeChat) CompletedChan() <-chan struct{} {
	return c.completedCh
}

// IsObjectiveMode returns whether chat is in objective mode
func (c *ClaudeChat) IsObjectiveMode() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mode == ModeObjective
}
