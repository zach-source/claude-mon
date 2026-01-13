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
	"github.com/google/uuid"
	"github.com/ztaylor/claude-mon/internal/logger"
)

// Mode represents the chat operation mode
type Mode int

const (
	ModeInteractive Mode = iota // Interactive PTY-based chat
	ModeObjective               // Objective-based (auto-completes when done)
	// ModeJSONStream              // JSON streaming mode (disabled - use PTY mode instead)
)

// ContextPurpose defines what the chat session is being used for
type ContextPurpose string

const (
	ContextGeneral ContextPurpose = "general" // General chat
	ContextRalph   ContextPurpose = "ralph"   // Ralph loop interaction
	ContextPrompt  ContextPurpose = "prompt"  // Prompt modification
	ContextPlan    ContextPurpose = "plan"    // Plan update
)

// Message represents a chat message
type Message struct {
	Role      string    // "user" or "assistant"
	Content   string    // Message content
	Timestamp time.Time // When the message was sent/received
}

// JSON streaming types - DISABLED: JSON streaming mode is disabled, use PTY mode instead
// The following types are kept for compatibility but are no longer actively used
/*
type JSONEventType string

const (
	EventTypeText          JSONEventType = "text"           // Regular text output
	EventTypeThinking      JSONEventType = "thinking"       // Thinking/reasoning
	EventTypeToolUse       JSONEventType = "tool_use"       // Tool being used
	EventTypeToolResult    JSONEventType = "tool_result"    // Tool execution result
	EventTypeInputRequired JSONEventType = "input_required" // Waiting for user input
	EventTypeError         JSONEventType = "error"          // Error occurred
	EventTypePartial       JSONEventType = "partial"        // Partial message chunk
)

// JSONEvent represents a streaming JSON event from Claude CLI
type JSONEvent struct {
	Type         string          `json:"type"`
	Subtype      string          `json:"subtype,omitempty"`
	StreamedText string          `json:"streamed_text,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`
	ToolUse      *ToolUseInfo    `json:"tool_use,omitempty"`
	ToolResult   *ToolResultInfo `json:"tool_result,omitempty"`
	Error        string          `json:"error,omitempty"`
	MessageIndex int             `json:"message_index,omitempty"`
	Message      *JSONMessage    `json:"message,omitempty"` // Nested message for assistant type
	Result       string          `json:"result,omitempty"`  // Result content
}

// JSONMessage represents the nested message structure in assistant events
type JSONMessage struct {
	ID      string             `json:"id,omitempty"`
	Type    string             `json:"type,omitempty"`
	Role    string             `json:"role,omitempty"`
	Content []JSONContentBlock `json:"content,omitempty"`
}

// JSONContentBlock represents a content block within a message
type JSONContentBlock struct {
	Type     string       `json:"type,omitempty"`
	Text     string       `json:"text,omitempty"`
	ToolUse  *ToolUseInfo `json:"tool_use,omitempty"`
	Thinking string       `json:"thinking,omitempty"`
}

// ToolUseInfo contains information about a tool being used
type ToolUseInfo struct {
	Name      string                 `json:"name"`
	Input     map[string]interface{} `json:"input"`
	Recipient string                 `json:"recipient,omitempty"`
}

// ToolResultInfo contains the result of a tool execution
type ToolResultInfo struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// UserMessageJSON is sent to stdin for multi-turn conversations
type UserMessageJSON struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}
*/

// ClaudeChat manages a Claude CLI subprocess (PTY-based)
type ClaudeChat struct {
	ptmx *os.File // PTY master
	// stdin    io.WriteCloser  // stdin for JSON mode (DISABLED)
	// stdout   io.ReadCloser   // stdout for JSON mode (DISABLED)
	// stderr   io.ReadCloser   // stderr for JSON mode (DISABLED)
	cmd      *exec.Cmd       // Claude CLI process
	output   strings.Builder // Accumulated output
	messages []Message       // Chat history
	active   bool            // Whether chat is active
	mu       sync.Mutex      // Protects shared state

	// Session identification
	sessionID string         // Unique session ID for isolation
	purpose   ContextPurpose // What this chat session is for

	// Mode and objective tracking
	mode      Mode   // Current operation mode
	objective string // The objective/prompt for objective mode

	// JSON streaming state (DISABLED)
	// currentMessage  *strings.Builder // Current message being built
	// currentThinking *strings.Builder // Current thinking content
	// awaitingInput   bool             // Waiting for user input in JSON mode

	// Channels for communication
	outputCh    chan interface{} // Output from Claude (string)
	doneCh      chan struct{}    // Signals chat has ended
	errCh       chan error       // Errors from the subprocess
	completedCh chan struct{}    // Signals objective completed (for objective mode)
}

// New creates a new ClaudeChat instance
func New() *ClaudeChat {
	return &ClaudeChat{
		messages:    make([]Message, 0),
		outputCh:    make(chan interface{}, 100),
		doneCh:      make(chan struct{}),
		errCh:       make(chan error, 1),
		completedCh: make(chan struct{}),
		mode:        ModeInteractive,
		// currentMessage:  &strings.Builder{}, // DISABLED: JSON streaming mode
		// currentThinking: &strings.Builder{}, // DISABLED: JSON streaming mode
	}
}

// Start launches the Claude CLI in a PTY (interactive mode, isolated session)
func (c *ClaudeChat) Start(mcpConfigPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active {
		return fmt.Errorf("chat already active")
	}

	// Generate unique session ID for isolation
	if c.sessionID == "" {
		c.sessionID = uuid.New().String()
	}

	// Build command with session ID for isolation
	args := []string{"--session-id", c.sessionID}
	if mcpConfigPath != "" {
		args = append(args, "--mcp-config", mcpConfigPath)
	}

	logger.Log("Starting claude CLI with session ID %s, args: %v", c.sessionID, args)

	c.cmd = exec.Command("claude", args...)
	c.cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Start with PTY
	var err error
	c.ptmx, err = pty.Start(c.cmd)
	if err != nil {
		logger.Log("Failed to start PTY: %v", err)
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	logger.Log("PTY started successfully, PID: %d, session: %s", c.cmd.Process.Pid, c.sessionID)

	c.active = true
	c.mode = ModeInteractive
	c.output.Reset()
	c.messages = make([]Message, 0)

	// Start goroutine to read output (also handles auto-confirmation of prompts)
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

	// Start goroutine to read output and detect completion (also handles auto-confirmation of prompts)
	go c.readOutputObjective()

	return nil
}

// StartJSON launches Claude with JSON streaming for structured input/output
// DISABLED: JSON streaming mode is disabled - use PTY mode (Start) instead
func (c *ClaudeChat) StartJSON(initialPrompt string, mcpConfigPath string) error {
	return fmt.Errorf("JSON streaming mode is disabled; use PTY mode (Start) instead")
}

// readJSONOutput reads and parses JSON events from stdout
// DISABLED: JSON streaming mode is disabled, use PTY mode instead
/*
func (c *ClaudeChat) readJSONOutput() {
	logger.Log("JSON stream: starting output reader")
	scanner := bufio.NewScanner(c.stdout)
	// Increase buffer size for long JSON lines (up to 1MB)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		logger.Log("JSON stream: received line: %s", string(line))

		var event JSONEvent
		if err := json.Unmarshal(line, &event); err != nil {
			logger.Log("JSON stream: failed to parse JSON: %v, line: %s", err, string(line))
			// Send as raw text if JSON parsing fails
			select {
			case c.outputCh <- string(line):
			default:
			}
			continue
		}

		// Handle different event types from Claude CLI
		switch event.Type {
		case "assistant":
			// Assistant message - extract text from nested content blocks
			if event.Message != nil {
				for _, block := range event.Message.Content {
					if block.Type == "text" && block.Text != "" {
						c.mu.Lock()
						c.currentMessage.WriteString(block.Text)
						c.output.WriteString(block.Text)
						c.mu.Unlock()

						// Send as partial text event for streaming
						outEvent := event
						outEvent.StreamedText = block.Text
						select {
						case c.outputCh <- outEvent:
						default:
						}
					} else if block.Type == "tool_use" && block.ToolUse != nil {
						// Tool being used
						toolLine := fmt.Sprintf("\n[Using: %s]\n", block.ToolUse.Name)
						c.mu.Lock()
						c.output.WriteString(toolLine)
						c.mu.Unlock()
					}
				}
			}

		case "result":
			// Final result event - contains the complete response text
			if event.Result != "" {
				// Result is already captured from assistant events, but log it
				logger.Log("JSON stream: received result, subtype=%s", event.Subtype)

				// Signal completion
				c.mu.Lock()
				c.awaitingInput = true
				wasActive := c.active
				c.mu.Unlock()

				// Finalize current message
				c.mu.Lock()
				if c.currentMessage.Len() > 0 {
					content := c.currentMessage.String()
					c.messages = append(c.messages, Message{
						Role:      "assistant",
						Content:   content,
						Timestamp: time.Now(),
						EventType: EventTypeText,
					})
					c.currentMessage.Reset()
				}
				c.mu.Unlock()

				// Send completion signal for non-success results
				if event.Subtype != "success" {
					select {
					case c.outputCh <- event:
					default:
					}
				}

				// If process ended, signal completion
				if wasActive && (event.Subtype == "success" || event.Subtype == "error") {
					select {
					case c.completedCh <- struct{}{}:
						logger.Log("JSON stream: sent completion signal from result")
					default:
						logger.Log("JSON stream: completion channel full")
					}
				}
			}

		case "system":
			// System initialization event - log but don't display
			logger.Log("JSON stream: system event, subtype=%s", event.Subtype)

		case "error":
			if event.Error != "" {
				c.mu.Lock()
				c.output.WriteString(fmt.Sprintf("Error: %s\n", event.Error))
				c.mu.Unlock()
			}
			select {
			case c.outputCh <- event:
			default:
			}

		default:
			// Unknown event type, log and pass through for debugging
			logger.Log("JSON stream: unknown event type: %s", event.Type)
			select {
			case c.outputCh <- event:
			default:
			}
		}
	}

	if err := scanner.Err(); err != nil {
		logger.Log("JSON stream: scanner error: %v", err)
		select {
		case c.errCh <- err:
		default:
		}
	}

	// Process ended
	c.mu.Lock()
	wasActive := c.active
	c.active = false
	c.awaitingInput = false
	c.mu.Unlock()

	logger.Log("JSON stream: process ended, wasActive=%v", wasActive)

	// Finalize any pending message
	c.mu.Lock()
	if c.currentMessage.Len() > 0 {
		content := c.currentMessage.String()
		c.messages = append(c.messages, Message{
			Role:      "assistant",
			Content:   content,
			Timestamp: time.Now(),
			EventType: EventTypeText,
		})
		c.currentMessage.Reset()
	}
	c.mu.Unlock()

	if wasActive {
		select {
		case c.completedCh <- struct{}{}:
			logger.Log("JSON stream: sent completion signal")
		default:
			logger.Log("JSON stream: completion channel full")
		}
	}

	close(c.doneCh)
}

// SendJSONMessage sends a user message in JSON format for multi-turn chat
// DISABLED: JSON streaming mode is disabled, use PTY mode instead
func (c *ClaudeChat) SendJSONMessage(content string) error {
	return fmt.Errorf("JSON streaming mode is disabled; use PTY mode (Send) instead")
}

// AwaitingInput returns whether Claude is waiting for user input (JSON mode only)
// DISABLED: JSON streaming mode is disabled, always returns false
func (c *ClaudeChat) AwaitingInput() bool {
	return false
}

// JSONEventsChan returns the channel for receiving JSON events
// DISABLED: JSON streaming mode is disabled, returns regular output channel
func (c *ClaudeChat) JSONEventsChan() <-chan interface{} {
	return c.outputCh
}

// Thinking returns the current thinking content (JSON mode only)
// DISABLED: JSON streaming mode is disabled, always returns empty string
func (c *ClaudeChat) Thinking() string {
	return ""
}

// ClearThinking clears the thinking buffer
// DISABLED: JSON streaming mode is disabled, no-op
func (c *ClaudeChat) ClearThinking() {
	// No-op: JSON streaming mode is disabled
}
*/
// readOutput reads from the PTY and sends output to the channel (interactive mode)
func (c *ClaudeChat) readOutput() {
	reader := bufio.NewReader(c.ptmx)
	buf := make([]byte, 4096)
	sentConfirm := false

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

			// Check for trust prompts in output (check continuously, not just first chunk)
			// Only respond once per session
			if !sentConfirm && len(chunk) > 0 {
				if containsIgnoreCase(chunk, "trust") || containsIgnoreCase(chunk, "confirm") {
					logger.Log("Detected trust/confirm prompt, sending 'y'")
					c.ptmx.Write([]byte("y\n"))
					sentConfirm = true
				} else if containsIgnoreCase(chunk, "continue") && (containsIgnoreCase(chunk, "press") || containsIgnoreCase(chunk, "enter")) {
					logger.Log("Detected continue prompt, sending enter")
					c.ptmx.Write([]byte("\n"))
					sentConfirm = true
				}
			}

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

// containsIgnoreCase checks if a string contains a substring (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// readOutputObjective reads output in objective mode and signals completion when process exits
func (c *ClaudeChat) readOutputObjective() {
	logger.Log("Objective mode: starting output reader")
	reader := bufio.NewReader(c.ptmx)
	buf := make([]byte, 4096)
	sentConfirm := false

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

			// Check for trust prompts in output (check continuously, not just first chunk)
			if !sentConfirm && len(chunk) > 0 {
				if containsIgnoreCase(chunk, "trust") || containsIgnoreCase(chunk, "confirm") {
					logger.Log("Objective mode: detected trust/confirm prompt, sending 'y'")
					c.ptmx.Write([]byte("y\n"))
					sentConfirm = true
				} else if containsIgnoreCase(chunk, "continue") && (containsIgnoreCase(chunk, "press") || containsIgnoreCase(chunk, "enter")) {
					logger.Log("Objective mode: detected continue prompt, sending enter")
					c.ptmx.Write([]byte("\n"))
					sentConfirm = true
				}
			}

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

// OutputChan returns the channel for receiving output (may contain string or JSONEvent)
func (c *ClaudeChat) OutputChan() <-chan interface{} {
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

// SetPurpose sets the purpose/context for this chat session
// Must be called before Start()
func (c *ClaudeChat) SetPurpose(purpose ContextPurpose) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purpose = purpose
}

// Purpose returns the purpose of this chat session
func (c *ClaudeChat) Purpose() ContextPurpose {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.purpose
}

// SessionID returns the unique session ID for this chat
func (c *ClaudeChat) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// SetSessionID sets a specific session ID (useful for resuming)
// Must be called before Start()
func (c *ClaudeChat) SetSessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sessionID
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncateString truncates a string to max length, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// For multi-byte strings, use rune counting
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}
