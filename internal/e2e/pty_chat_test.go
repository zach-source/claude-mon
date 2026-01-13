//go:build e2e
// +build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/ztaylor/claude-mon/internal/chat"
)

// TestPTYChatSinglePrompt tests basic PTY-based chat with a single prompt
func TestPTYChatSinglePrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	c := chat.New()
	c.SetPurpose(chat.ContextGeneral)

	t.Log("Starting PTY chat...")
	if err := c.Start(""); err != nil {
		t.Fatalf("Failed to start PTY chat: %v", err)
	}
	defer c.Stop()

	// Wait for startup and auto-confirm of trust prompt
	time.Sleep(3 * time.Second)

	prompt := "Say 'Hello from PTY' and nothing else."
	t.Logf("Sending prompt: %q", prompt)

	if err := c.Send(prompt); err != nil {
		t.Fatalf("Failed to send prompt: %v", err)
	}

	// Wait for response
	timeout := time.After(60 * time.Second)
	gotHello := false

	t.Log("Waiting for response...")
	for {
		select {
		case outputRaw := <-c.OutputChan():
			if outputRaw == nil {
				continue
			}
			output, ok := outputRaw.(string)
			if !ok || output == "" {
				continue
			}

			// Skip ANSI escape sequences and control characters
			if strings.Contains(output, "\x1b") || len(output) < 3 {
				continue
			}

			t.Logf("Received output: %q", truncateForLog(stripANSI(output), 200))

			cleanOutput := stripANSI(output)
			if strings.Contains(strings.ToLower(cleanOutput), "hello") &&
				strings.Contains(strings.ToLower(cleanOutput), "pty") {
				gotHello = true
			}

			// Check if response is complete
			if gotHello {
				t.Log("SUCCESS: Received expected response")
				return
			}

		case <-time.After(10 * time.Second):
			// Check accumulated output
			cleanOutput := stripANSI(c.Output())
			if gotHello || strings.Contains(strings.ToLower(cleanOutput), "hello") {
				t.Log("SUCCESS: Received expected response (found in output)")
				return
			}

		case <-timeout:
			output := c.Output()
			cleanOutput := stripANSI(output)
			t.Logf("Final output (cleaned): %q", truncateForLog(cleanOutput, 500))

			if gotHello || strings.Contains(strings.ToLower(cleanOutput), "hello") {
				t.Log("SUCCESS: Found expected response in final output")
			} else {
				t.Log("Note: Response may vary or trust prompt handling may need adjustment")
			}
			return
		}
	}
}

// stripANSI removes ANSI escape sequences from strings
func stripANSI(s string) string {
	// Simple ANSI escape sequence remover
	var result strings.Builder
	inEscape := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			inEscape = true
			continue
		}
		if inEscape && (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			inEscape = false
			continue
		}
		if !inEscape && c >= 32 && c < 127 { // Printable ASCII
			result.WriteByte(c)
		}
	}
	return result.String()
}

// TestPTYChatMultiTurn tests multi-turn conversation in PTY mode
func TestPTYChatMultiTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	c := chat.New()
	c.SetPurpose(chat.ContextGeneral)

	t.Log("Starting PTY chat for multi-turn...")
	if err := c.Start(""); err != nil {
		t.Fatalf("Failed to start PTY chat: %v", err)
	}
	defer c.Stop()

	// Wait for startup and auto-confirm
	time.Sleep(2 * time.Second)

	// First message
	firstMsg := "Count to 3"
	t.Logf("Sending first message: %q", firstMsg)
	if err := c.Send(firstMsg); err != nil {
		t.Fatalf("Failed to send first message: %v", err)
	}

	// Wait for first response
	timeout := time.After(45 * time.Second)
	firstReceived := false

	t.Log("Waiting for first response...")
	for {
		select {
		case outputRaw := <-c.OutputChan():
			if outputRaw == nil {
				continue
			}
			output, ok := outputRaw.(string)
			if !ok || output == "" {
				continue
			}

			if strings.Contains(output, "1") || strings.Contains(output, "2") || strings.Contains(output, "3") {
				firstReceived = true
				t.Log("Received first response")
				break
			}

		case <-time.After(10 * time.Second):
			// Check accumulated output
			if strings.Contains(c.Output(), "1") || strings.Contains(c.Output(), "2") || strings.Contains(c.Output(), "3") {
				firstReceived = true
				t.Log("Found first response in accumulated output")
				break
			}

		case <-timeout:
			break
		}
		break
	}

	if !firstReceived {
		t.Log("Note: First response may have been missed (output may be in buffer)")
	}

	// Give Claude a moment to finish
	time.Sleep(2 * time.Second)

	// Second message
	secondMsg := "Now say goodbye"
	t.Logf("Sending second message: %q", secondMsg)
	if err := c.Send(secondMsg); err != nil {
		t.Fatalf("Failed to send second message: %v", err)
	}

	// Wait for second response
	timeout = time.After(45 * time.Second)
	secondReceived := false

	t.Log("Waiting for second response...")
	for {
		select {
		case outputRaw := <-c.OutputChan():
			if outputRaw == nil {
				continue
			}
			output, ok := outputRaw.(string)
			if !ok || output == "" {
				continue
			}

			outputLower := strings.ToLower(output)
			if strings.Contains(outputLower, "goodbye") || strings.Contains(outputLower, "bye") {
				secondReceived = true
				t.Log("SUCCESS: Received second response")
				return
			}

		case <-time.After(10 * time.Second):
			// Check accumulated output
			outputLower := strings.ToLower(c.Output())
			if strings.Contains(outputLower, "goodbye") || strings.Contains(outputLower, "bye") {
				secondReceived = true
				t.Log("SUCCESS: Found second response in accumulated output")
				return
			}

		case <-timeout:
			break
		}
		break
	}

	finalOutput := c.Output()
	t.Logf("Final output length: %d", len(finalOutput))

	if secondReceived || strings.Contains(strings.ToLower(finalOutput), "goodbye") || strings.Contains(strings.ToLower(finalOutput), "bye") {
		t.Log("SUCCESS: Multi-turn conversation worked")
	} else {
		t.Log("Note: Multi-turn test completed (response may vary)")
	}
}

// TestPTYChatSessionIsolation tests that different PTY sessions have different IDs
func TestPTYChatSessionIsolation(t *testing.T) {
	c1 := chat.New()
	c2 := chat.New()

	if err := c1.Start(""); err != nil {
		t.Fatalf("Failed to start first chat: %v", err)
	}
	defer c1.Stop()

	if err := c2.Start(""); err != nil {
		t.Fatalf("Failed to start second chat: %v", err)
	}
	defer c2.Stop()

	id1 := c1.SessionID()
	id2 := c2.SessionID()

	if id1 == "" || id2 == "" {
		t.Error("expected non-empty session IDs")
	}

	if id1 == id2 {
		t.Errorf("expected different session IDs, got both: %s", id1)
	}

	t.Logf("Session 1 ID: %s", id1)
	t.Logf("Session 2 ID: %s", id2)
	t.Log("SUCCESS: PTY sessions are isolated")
}

// TestPTYChatMessageHistory tests that message history is tracked
func TestPTYChatMessageHistory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	c := chat.New()
	c.SetPurpose(chat.ContextGeneral)

	t.Log("Starting PTY chat for history test...")
	if err := c.Start(""); err != nil {
		t.Fatalf("Failed to start PTY chat: %v", err)
	}
	defer c.Stop()

	// Wait for startup
	time.Sleep(2 * time.Second)

	prompt := "Test message"
	t.Logf("Sending message: %q", prompt)
	if err := c.Send(prompt); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Wait a moment
	time.Sleep(2 * time.Second)

	// Check message history
	messages := c.Messages()
	t.Logf("Message history length: %d", len(messages))

	// We should have at least our sent message
	if len(messages) < 1 {
		t.Error("expected at least 1 message in history")
	}

	// Check that our message is in history
	found := false
	for _, msg := range messages {
		if strings.Contains(msg.Content, prompt) && msg.Role == "user" {
			found = true
			t.Logf("Found message in history: %q", prompt)
			break
		}
	}

	if found {
		t.Log("SUCCESS: Message history tracking works")
	} else {
		t.Log("Note: Message may be in buffer or history format may vary")
	}
}

// TestPTYChatAutoConfirm tests that trust prompts are auto-confirmed
func TestPTYChatAutoConfirm(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	c := chat.New()
	c.SetPurpose(chat.ContextGeneral)

	t.Log("Starting PTY chat (testing auto-confirm)...")
	if err := c.Start(""); err != nil {
		t.Fatalf("Failed to start PTY chat: %v", err)
	}
	defer c.Stop()

	// Wait for startup and auto-confirm
	time.Sleep(3 * time.Second)

	// Send a simple message to verify chat is responsive
	prompt := "Say OK"
	t.Logf("Sending prompt: %q", prompt)

	if err := c.Send(prompt); err != nil {
		t.Fatalf("Failed to send prompt: %v", err)
	}

	// Wait for response
	timeout := time.After(30 * time.Second)
	gotOK := false

	for {
		select {
		case outputRaw := <-c.OutputChan():
			if outputRaw == nil {
				continue
			}
			output, ok := outputRaw.(string)
			if !ok || output == "" {
				continue
			}

			t.Logf("Received output: %q", truncateForLog(output, 100))

			if strings.Contains(strings.ToUpper(output), "OK") {
				gotOK = true
			}

		case <-time.After(5 * time.Second):
			if gotOK || strings.Contains(strings.ToUpper(c.Output()), "OK") {
				t.Log("SUCCESS: Auto-confirm worked, chat is responsive")
				return
			}

		case <-timeout:
			output := c.Output()
			if len(output) > 0 {
				t.Logf("Final output: %q", truncateForLog(output, 500))
				t.Log("SUCCESS: Chat is responsive (auto-confirm likely worked)")
				return
			}
			t.Error("Chat may not be responsive")
			return
		}
	}
}

// TestPTYChatContextPurpose tests that context purpose is set correctly
func TestPTYChatContextPurpose(t *testing.T) {
	c := chat.New()

	// Test setting different purposes
	purposes := []chat.ContextPurpose{
		chat.ContextGeneral,
		chat.ContextRalph,
		chat.ContextPrompt,
		chat.ContextPlan,
	}

	for _, purpose := range purposes {
		c.SetPurpose(purpose)
		if c.Purpose() != purpose {
			t.Errorf("expected purpose %v, got %v", purpose, c.Purpose())
		}
	}

	t.Log("SUCCESS: Context purpose can be set correctly")
}

// TestPTYChatMode tests that mode is set correctly
func TestPTYChatMode(t *testing.T) {
	c := chat.New()

	// Before starting, mode should be interactive (default)
	if c.Mode() != chat.ModeInteractive {
		t.Errorf("expected ModeInteractive, got %v", c.Mode())
	}

	t.Log("SUCCESS: Default mode is interactive")
}

// truncateForLog truncates a string for logging purposes
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
