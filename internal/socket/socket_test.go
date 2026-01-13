package socket

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGetSocketPath(t *testing.T) {
	path := GetSocketPath()

	// Should start with /tmp/
	if !strings.HasPrefix(path, "/tmp/claude-mon-") {
		t.Errorf("socket path should start with /tmp/claude-mon-, got: %s", path)
	}

	// Should end with .sock
	if !strings.HasSuffix(path, ".sock") {
		t.Errorf("socket path should end with .sock, got: %s", path)
	}

	// Should contain username
	user := os.Getenv("USER")
	if user != "" && !strings.Contains(path, user) {
		t.Errorf("socket path should contain username %s, got: %s", user, path)
	}
}

func TestListenerCreateAndClose(t *testing.T) {
	// Use a unique test socket
	socketPath := "/tmp/claude-mon-test.sock"
	defer os.Remove(socketPath)

	listener, err := NewListener(socketPath)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	// Socket file should exist
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("socket file should exist after listener creation")
	}

	// Close should work (ignore "no such file" errors since socket cleanup is best-effort)
	err = listener.Close()
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("failed to close listener: %v", err)
	}

	// Socket file should be removed after close
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be removed after close")
	}
}

func TestListenerReceiveMessage(t *testing.T) {
	socketPath := "/tmp/claude-mon-test-msg.sock"
	defer os.Remove(socketPath)

	listener, err := NewListener(socketPath)
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer listener.Close()

	testPayload := `{"tool_name":"Edit","tool_input":{"file_path":"test.go"}}`
	received := make(chan []byte, 1)

	// Start listening in goroutine
	go listener.Listen(func(payload []byte) {
		received <- payload
	})

	// Give listener time to start
	time.Sleep(50 * time.Millisecond)

	// Connect and send message
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to connect to socket: %v", err)
	}

	_, err = conn.Write([]byte(testPayload))
	if err != nil {
		t.Fatalf("failed to write to socket: %v", err)
	}
	conn.Close()

	// Wait for message
	select {
	case payload := <-received:
		if string(payload) != testPayload {
			t.Errorf("payload mismatch: expected %s, got %s", testPayload, string(payload))
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for message")
	}
}
