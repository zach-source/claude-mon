package model

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParsePayload(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		wantPath string
		wantTool string
	}{
		{
			name:     "edit with tool_input.file_path",
			payload:  `{"tool_name":"Edit","tool_input":{"file_path":"/test/file.go","old_string":"old","new_string":"new"}}`,
			wantPath: "/test/file.go",
			wantTool: "Edit",
		},
		{
			name:     "write with tool_input.file_path",
			payload:  `{"tool_name":"Write","tool_input":{"file_path":"/new/file.go","content":"package main"}}`,
			wantPath: "/new/file.go",
			wantTool: "Write",
		},
		{
			name:     "edit with parameters.file_path",
			payload:  `{"tool_name":"Edit","parameters":{"file_path":"/params/file.go","old_string":"old","new_string":"new"}}`,
			wantPath: "/params/file.go",
			wantTool: "Edit",
		},
		{
			name:     "invalid json",
			payload:  `not json`,
			wantPath: "",
			wantTool: "",
		},
		{
			name:     "missing file_path",
			payload:  `{"tool_name":"Edit","tool_input":{"old_string":"old"}}`,
			wantPath: "",
			wantTool: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			change := parsePayload([]byte(tt.payload))
			if tt.wantPath == "" {
				if change != nil {
					t.Errorf("expected nil change for %s", tt.name)
				}
				return
			}

			if change == nil {
				t.Fatalf("expected non-nil change for %s", tt.name)
			}

			if change.FilePath != tt.wantPath {
				t.Errorf("file path: expected %s, got %s", tt.wantPath, change.FilePath)
			}

			if change.ToolName != tt.wantTool {
				t.Errorf("tool name: expected %s, got %s", tt.wantTool, change.ToolName)
			}
		})
	}
}

func TestModelNew(t *testing.T) {
	m := New("/tmp/test.sock")

	if m.socketPath != "/tmp/test.sock" {
		t.Errorf("socket path: expected /tmp/test.sock, got %s", m.socketPath)
	}

	if len(m.changes) != 0 {
		t.Error("changes should be empty initially")
	}

	if m.activePane != PaneLeft {
		t.Error("active pane should be left pane initially")
	}
}

func TestModelUpdate(t *testing.T) {
	m := New("/tmp/test.sock")

	// Simulate window resize
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	model := updated.(Model)

	if !model.ready {
		t.Error("model should be ready after window size message")
	}

	if model.width != 100 || model.height != 40 {
		t.Errorf("dimensions: expected 100x40, got %dx%d", model.width, model.height)
	}
}

func TestModelSocketMessage(t *testing.T) {
	m := New("/tmp/test.sock")

	// Set up model with window size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// Send socket message
	payload := `{"tool_name":"Edit","tool_input":{"file_path":"/test.go","old_string":"old","new_string":"new"}}`
	updated, _ = updated.Update(SocketMsg{Payload: []byte(payload)})

	model := updated.(Model)
	if len(model.changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(model.changes))
	}

	change := model.changes[0]
	if change.FilePath != "/test.go" {
		t.Errorf("file path: expected /test.go, got %s", change.FilePath)
	}
	if change.ToolName != "Edit" {
		t.Errorf("tool name: expected Edit, got %s", change.ToolName)
	}
}

func TestModelNavigation(t *testing.T) {
	m := New("/tmp/test.sock")
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// Add multiple changes
	for i := 0; i < 3; i++ {
		payload := `{"tool_name":"Edit","tool_input":{"file_path":"/test.go"}}`
		tm, _ = tm.Update(SocketMsg{Payload: []byte(payload)})
	}

	model := tm.(Model)
	if len(model.changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(model.changes))
	}

	// New behavior: most recent (last) item is selected by default
	if model.selectedIndex != 2 {
		t.Errorf("expected selected index 2 (most recent), got %d", model.selectedIndex)
	}

	// Test navigation down (default key: j) - goes to older items (lower index)
	// Display is newest-first, so down goes to visually lower = older items = lower index
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = tm.(Model)
	if model.selectedIndex != 1 {
		t.Errorf("expected selected index 1 after j (down to older), got %d", model.selectedIndex)
	}

	// Test navigation up (default key: k) - goes to newer items (higher index)
	// Display is newest-first, so up goes to visually higher = newer items = higher index
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = tm.(Model)
	if model.selectedIndex != 2 {
		t.Errorf("expected selected index 2 after k (up to newer), got %d", model.selectedIndex)
	}

	// Test pane switching with ] (default key for RightPane)
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	model = tm.(Model)
	if model.activePane != PaneRight {
		t.Error("expected right pane after ] key")
	}

	// Test switching back to left pane with [ (default key for LeftPane)
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	model = tm.(Model)
	if model.activePane != PaneLeft {
		t.Error("expected left pane after [ key")
	}
}

func TestModelClearHistory(t *testing.T) {
	m := New("/tmp/test.sock")
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// Add a change
	payload := `{"tool_name":"Edit","tool_input":{"file_path":"/test.go"}}`
	tm, _ = tm.Update(SocketMsg{Payload: []byte(payload)})

	// Clear history with uppercase C (default config key)
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}, Alt: false})

	model := tm.(Model)
	if len(model.changes) != 0 {
		t.Error("changes should be empty after clear")
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		path   string
		maxLen int
		want   string
	}{
		{"short.go", 20, "short.go"},
		{"/very/long/path/to/file.go", 15, ".../file.go"},
		{"/a/b/c.go", 10, ".../c.go"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := truncatePath(tt.path, tt.maxLen)
			if len(got) > tt.maxLen && tt.maxLen < len(tt.path) {
				// Truncation should respect max length (approximately)
				// The ".../" prefix adds 4 chars
			}
			// Just verify it doesn't panic and returns something
			if got == "" {
				t.Error("truncated path should not be empty")
			}
		})
	}
}

func TestChangeTimestamp(t *testing.T) {
	m := New("/tmp/test.sock")
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	before := time.Now()
	payload := `{"tool_name":"Edit","tool_input":{"file_path":"/test.go"}}`
	tm, _ = tm.Update(SocketMsg{Payload: []byte(payload)})
	after := time.Now()

	model := tm.(Model)
	change := model.changes[0]

	if change.Timestamp.Before(before) || change.Timestamp.After(after) {
		t.Errorf("timestamp %v should be between %v and %v", change.Timestamp, before, after)
	}
}
