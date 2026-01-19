package daemon

import (
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ztaylor/claude-mon/internal/database"
)

// TestDaemonHookE2E tests the full flow from hook payload to database storage
func TestDaemonHookE2E(t *testing.T) {
	// Create temporary directory for test
	tmpDir, err := os.MkdirTemp("", "daemon-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &Config{
		Directory: DirectoryConfig{
			DataDir: tmpDir,
		},
		Database: DatabaseConfig{
			Path:               "test.db",
			MaxDBSizeMB:        100,
			WALCheckpointPages: 1000,
		},
		Sockets: SocketsConfig{
			DaemonSocket: filepath.Join(tmpDir, "daemon.sock"),
			QuerySocket:  filepath.Join(tmpDir, "query.sock"),
			BufferSize:   8192,
		},
		Retention: RetentionConfig{
			RetentionDays:      30,
			MaxEditsPerSession: 10000,
			CleanupIntervalHrs: 0, // Disable for testing
			AutoVacuum:         false,
		},
		Backup: BackupConfig{
			Enabled: false, // Disable for testing
			Format:  "sqlite",
		},
		Query: QueryConfig{
			DefaultLimit: 100,
			MaxLimit:     1000,
			TimeoutSecs:  30,
		},
		Workspaces: WorkspacesConfig{
			Tracked: []string{}, // Track all
			Ignored: []string{},
		},
		Hooks: HooksConfig{
			TimeoutSecs:   30,
			RetryAttempts: 3,
			AsyncMode:     false,
		},
		Logging: LoggingConfig{
			Path:       "test.log",
			Level:      "debug",
			MaxSizeMB:  10,
			MaxBackups: 1,
			Compress:   false,
		},
		Performance: PerformanceConfig{
			MaxConnections: 10,
			PoolSize:       5,
			CacheEnabled:   false,
			CacheTTLSecs:   0,
		},
	}

	// Create daemon
	daemon, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}

	// Start daemon in goroutine
	daemonErr := make(chan error, 1)
	go func() {
		daemonErr <- daemon.Start()
	}()

	// Wait for daemon to start
	time.Sleep(100 * time.Millisecond)

	// Ensure daemon stops when test completes
	defer func() {
		daemon.Stop()
		select {
		case err := <-daemonErr:
			if err != nil {
				t.Logf("daemon stopped with error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Log("timeout waiting for daemon to stop")
		}
	}()

	// Connect to daemon socket
	conn, err := net.Dial("unix", cfg.Sockets.DaemonSocket)
	if err != nil {
		t.Fatalf("failed to connect to daemon: %v", err)
	}
	defer conn.Close()

	t.Run("BasicEditCapture", func(t *testing.T) {
		testBasicEditCapture(t, conn, cfg.Sockets.QuerySocket)
	})

	t.Run("FileSnapshotCapture", func(t *testing.T) {
		// New connection for this test
		conn2, err := net.Dial("unix", cfg.Sockets.DaemonSocket)
		if err != nil {
			t.Fatalf("failed to connect to daemon: %v", err)
		}
		defer conn2.Close()
		testFileSnapshotCapture(t, conn2, cfg.Sockets.QuerySocket, tmpDir)
	})

	t.Run("VCSMetadataCapture", func(t *testing.T) {
		conn3, err := net.Dial("unix", cfg.Sockets.DaemonSocket)
		if err != nil {
			t.Fatalf("failed to connect to daemon: %v", err)
		}
		defer conn3.Close()
		testVCSMetadataCapture(t, conn3, cfg.Sockets.QuerySocket)
	})

	t.Run("OldNewStringCapture", func(t *testing.T) {
		conn4, err := net.Dial("unix", cfg.Sockets.DaemonSocket)
		if err != nil {
			t.Fatalf("failed to connect to daemon: %v", err)
		}
		defer conn4.Close()
		testOldNewStringCapture(t, conn4, cfg.Sockets.QuerySocket)
	})

	t.Run("LargeContentTruncation", func(t *testing.T) {
		conn5, err := net.Dial("unix", cfg.Sockets.DaemonSocket)
		if err != nil {
			t.Fatalf("failed to connect to daemon: %v", err)
		}
		defer conn5.Close()
		testLargeContentHandling(t, conn5, cfg.Sockets.QuerySocket)
	})
}

func sendPayloadAndWaitForResponse(t *testing.T, conn net.Conn, payload *HookPayload) {
	t.Helper()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(payload); err != nil {
		t.Fatalf("failed to send payload: %v", err)
	}

	var response map[string]string
	if err := decoder.Decode(&response); err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if status, ok := response["status"]; !ok || status != "ok" {
		t.Fatalf("unexpected response: %v", response)
	}
}

func queryRecentEdits(t *testing.T, querySocket string, limit int) []*database.Edit {
	t.Helper()

	conn, err := net.Dial("unix", querySocket)
	if err != nil {
		t.Fatalf("failed to connect to query socket: %v", err)
	}
	defer conn.Close()

	query := Query{
		Type:  "recent",
		Limit: limit,
	}

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(query); err != nil {
		t.Fatalf("failed to send query: %v", err)
	}

	var result QueryResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("failed to decode query result: %v", err)
	}

	return result.Edits
}

func testBasicEditCapture(t *testing.T, conn net.Conn, querySocket string) {
	payload := &HookPayload{
		Type:          "edit",
		Workspace:     "/test/workspace",
		WorkspaceName: "test-workspace",
		ToolName:      "Edit",
		FilePath:      "/test/workspace/main.go",
		OldString:     "old content",
		NewString:     "new content",
		LineNum:       10,
		LineCount:     5,
	}

	sendPayloadAndWaitForResponse(t, conn, payload)

	// Query and verify
	edits := queryRecentEdits(t, querySocket, 10)
	if len(edits) == 0 {
		t.Fatal("expected at least one edit, got none")
	}

	// Find our edit
	var found *database.Edit
	for _, e := range edits {
		if e.FilePath == "/test/workspace/main.go" && e.ToolName == "Edit" {
			found = e
			break
		}
	}

	if found == nil {
		t.Fatal("edit not found in results")
	}

	// Verify fields
	if found.OldString != "old content" {
		t.Errorf("old_string: expected 'old content', got '%s'", found.OldString)
	}
	if found.NewString != "new content" {
		t.Errorf("new_string: expected 'new content', got '%s'", found.NewString)
	}
	if found.LineNum != 10 {
		t.Errorf("line_num: expected 10, got %d", found.LineNum)
	}
	if found.LineCount != 5 {
		t.Errorf("line_count: expected 5, got %d", found.LineCount)
	}
}

func testFileSnapshotCapture(t *testing.T, conn net.Conn, querySocket string, tmpDir string) {
	// Create a test file to capture
	testFile := filepath.Join(tmpDir, "snapshot-test.go")
	testContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Base64 encode the content
	contentB64 := base64.StdEncoding.EncodeToString([]byte(testContent))

	payload := &HookPayload{
		Type:           "edit",
		Workspace:      tmpDir,
		WorkspaceName:  "snapshot-test",
		ToolName:       "Edit",
		FilePath:       testFile,
		OldString:      "old",
		NewString:      "new",
		FileContentB64: contentB64,
	}

	sendPayloadAndWaitForResponse(t, conn, payload)

	// Query and verify file content was stored and decompressed correctly
	edits := queryRecentEdits(t, querySocket, 10)

	var found *database.Edit
	for _, e := range edits {
		if e.FilePath == testFile {
			found = e
			break
		}
	}

	if found == nil {
		t.Fatal("edit with file snapshot not found")
	}

	// Verify file content was captured and decompressed
	if found.FileContent == "" {
		t.Error("file_content is empty, snapshot was not captured or decompressed")
	} else if found.FileContent != testContent {
		t.Errorf("file_content mismatch:\nexpected: %q\ngot: %q", testContent, found.FileContent)
	}
}

func testVCSMetadataCapture(t *testing.T, conn net.Conn, querySocket string) {
	// Test Git VCS metadata
	payload := &HookPayload{
		Type:          "edit",
		Workspace:     "/test/git-workspace",
		WorkspaceName: "git-test",
		ToolName:      "Edit",
		FilePath:      "/test/git-workspace/file.go",
		OldString:     "",
		NewString:     "content",
		Branch:        "feature/test-branch",
		CommitSHA:     "abc123def456",
		VCSType:       "git",
	}

	sendPayloadAndWaitForResponse(t, conn, payload)

	edits := queryRecentEdits(t, querySocket, 10)

	var found *database.Edit
	for _, e := range edits {
		if e.FilePath == "/test/git-workspace/file.go" {
			found = e
			break
		}
	}

	if found == nil {
		t.Fatal("git edit not found")
	}

	if found.CommitSHA != "abc123def456" {
		t.Errorf("commit_sha: expected 'abc123def456', got '%s'", found.CommitSHA)
	}
	if found.VCSType != "git" {
		t.Errorf("vcs_type: expected 'git', got '%s'", found.VCSType)
	}

	// Test Jujutsu VCS metadata
	payloadJJ := &HookPayload{
		Type:          "edit",
		Workspace:     "/test/jj-workspace",
		WorkspaceName: "jj-test",
		ToolName:      "Write",
		FilePath:      "/test/jj-workspace/new-file.go",
		NewString:     "new file content",
		CommitSHA:     "zxpqwrst", // jj uses shorter change IDs
		VCSType:       "jj",
	}

	sendPayloadAndWaitForResponse(t, conn, payloadJJ)

	edits = queryRecentEdits(t, querySocket, 10)

	var foundJJ *database.Edit
	for _, e := range edits {
		if e.FilePath == "/test/jj-workspace/new-file.go" {
			foundJJ = e
			break
		}
	}

	if foundJJ == nil {
		t.Fatal("jj edit not found")
	}

	if foundJJ.VCSType != "jj" {
		t.Errorf("vcs_type: expected 'jj', got '%s'", foundJJ.VCSType)
	}
}

func testOldNewStringCapture(t *testing.T, conn net.Conn, querySocket string) {
	// Test with multi-line content to verify line counting
	oldContent := `func old() {
	return 1
}`
	newContent := `func new() {
	x := 1
	y := 2
	return x + y
}`

	payload := &HookPayload{
		Type:          "edit",
		Workspace:     "/test/multiline",
		WorkspaceName: "multiline-test",
		ToolName:      "Edit",
		FilePath:      "/test/multiline/functions.go",
		OldString:     oldContent,
		NewString:     newContent,
		LineNum:       20,
		LineCount:     4, // New content has 4 lines
	}

	sendPayloadAndWaitForResponse(t, conn, payload)

	edits := queryRecentEdits(t, querySocket, 10)

	var found *database.Edit
	for _, e := range edits {
		if e.FilePath == "/test/multiline/functions.go" {
			found = e
			break
		}
	}

	if found == nil {
		t.Fatal("multiline edit not found")
	}

	// Verify complete old/new strings were captured
	if found.OldString != oldContent {
		t.Errorf("old_string not captured correctly:\nexpected: %q\ngot: %q", oldContent, found.OldString)
	}
	if found.NewString != newContent {
		t.Errorf("new_string not captured correctly:\nexpected: %q\ngot: %q", newContent, found.NewString)
	}
}

func testLargeContentHandling(t *testing.T, conn net.Conn, querySocket string) {
	// Test with content near the 10KB truncation limit
	// The hook script truncates to 10KB, but the daemon should handle whatever it receives

	// Create content just under 10KB
	largeContent := strings.Repeat("x", 9000) + "\nend"

	payload := &HookPayload{
		Type:          "edit",
		Workspace:     "/test/large",
		WorkspaceName: "large-test",
		ToolName:      "Write",
		FilePath:      "/test/large/bigfile.txt",
		NewString:     largeContent,
	}

	sendPayloadAndWaitForResponse(t, conn, payload)

	edits := queryRecentEdits(t, querySocket, 10)

	var found *database.Edit
	for _, e := range edits {
		if e.FilePath == "/test/large/bigfile.txt" {
			found = e
			break
		}
	}

	if found == nil {
		t.Fatal("large content edit not found")
	}

	// Verify content was stored completely
	if len(found.NewString) != len(largeContent) {
		t.Errorf("large content length: expected %d, got %d", len(largeContent), len(found.NewString))
	}
}

// TestDaemonQueryStatus tests the daemon status query
func TestDaemonQueryStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "daemon-status-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		Directory: DirectoryConfig{
			DataDir: tmpDir,
		},
		Database: DatabaseConfig{
			Path:               "test.db",
			MaxDBSizeMB:        100,
			WALCheckpointPages: 1000,
		},
		Sockets: SocketsConfig{
			DaemonSocket: filepath.Join(tmpDir, "daemon.sock"),
			QuerySocket:  filepath.Join(tmpDir, "query.sock"),
			BufferSize:   8192,
		},
		Retention: RetentionConfig{
			RetentionDays:      30,
			MaxEditsPerSession: 10000,
			CleanupIntervalHrs: 0,
			AutoVacuum:         false,
		},
		Backup: BackupConfig{
			Enabled: false,
			Format:  "sqlite",
		},
		Query: QueryConfig{
			DefaultLimit: 100,
			MaxLimit:     1000,
			TimeoutSecs:  30,
		},
		Workspaces: WorkspacesConfig{
			Tracked: []string{},
			Ignored: []string{},
		},
		Hooks: HooksConfig{
			TimeoutSecs:   30,
			RetryAttempts: 3,
			AsyncMode:     false,
		},
		Logging: LoggingConfig{
			Path:       "test.log",
			Level:      "info",
			MaxSizeMB:  10,
			MaxBackups: 1,
			Compress:   false,
		},
		Performance: PerformanceConfig{
			MaxConnections: 10,
			PoolSize:       5,
			CacheEnabled:   false,
			CacheTTLSecs:   0,
		},
	}

	daemon, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create daemon: %v", err)
	}

	daemonErr := make(chan error, 1)
	go func() {
		daemonErr <- daemon.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	defer func() {
		daemon.Stop()
		select {
		case <-daemonErr:
		case <-time.After(5 * time.Second):
		}
	}()

	// Query status
	conn, err := net.Dial("unix", cfg.Sockets.QuerySocket)
	if err != nil {
		t.Fatalf("failed to connect to query socket: %v", err)
	}
	defer conn.Close()

	query := Query{Type: "status"}
	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	if err := encoder.Encode(query); err != nil {
		t.Fatalf("failed to send status query: %v", err)
	}

	var result QueryResult
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("failed to decode status result: %v", err)
	}

	if result.Status == nil {
		t.Fatal("status is nil")
	}

	if !result.Status.Running {
		t.Error("daemon should report as running")
	}

	if result.Status.Uptime <= 0 {
		t.Error("uptime should be positive")
	}
}

// TestCompressionDecompression tests that file content compression works correctly
func TestCompressionDecompression(t *testing.T) {
	original := `package main

import (
	"fmt"
)

func main() {
	fmt.Println("Hello, this is a test file with multiple lines")
	fmt.Println("Line 2")
	fmt.Println("Line 3")
}
`
	// Simulate the compression that happens in processPayload
	encoded := base64.StdEncoding.EncodeToString([]byte(original))
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	// Compress
	var compressed []byte
	{
		var buf strings.Builder
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(decoded); err != nil {
			t.Fatalf("gzip write failed: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("gzip close failed: %v", err)
		}
		compressed = []byte(buf.String())
	}

	// Decompress (simulating what GetRecentEdits does)
	r, err := gzip.NewReader(strings.NewReader(string(compressed)))
	if err != nil {
		t.Fatalf("gzip reader failed: %v", err)
	}
	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("gzip read failed: %v", err)
	}
	r.Close()

	if string(decompressed) != original {
		t.Errorf("compression/decompression mismatch:\noriginal: %q\nresult: %q", original, string(decompressed))
	}
}
