package vcs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectVCSType(t *testing.T) {
	// Get current directory (should be in a git repo)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	vcsType := DetectVCSType(cwd)
	// Skip if no VCS detected (e.g., in nix build sandbox)
	if vcsType == "" {
		t.Skip("No VCS detected (may be running in sandboxed environment)")
	}
	// The test repo is a git repo
	if vcsType != "git" && vcsType != "jj" {
		t.Errorf("Expected git or jj, got: %s", vcsType)
	}
	t.Logf("Detected VCS type: %s", vcsType)
}

func TestGetCurrentCommit(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	vcsType := DetectVCSType(cwd)
	if vcsType == "" {
		t.Skip("No VCS detected")
	}

	commit, err := GetCurrentCommit(cwd, vcsType)
	if err != nil {
		t.Fatalf("Failed to get current commit: %v", err)
	}

	if commit == "" {
		t.Error("Expected non-empty commit")
	}
	t.Logf("Current commit: %s", commit)
}

func TestGetFileAtCommit(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	// Navigate up to repo root
	repoRoot := cwd
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		repoRoot = filepath.Dir(repoRoot)
	}

	vcsType := DetectVCSType(repoRoot)
	if vcsType == "" {
		t.Skip("No VCS detected")
	}

	commit, err := GetCurrentCommit(repoRoot, vcsType)
	if err != nil {
		t.Fatalf("Failed to get current commit: %v", err)
	}

	// Test with a file that should exist
	filePath := "go.mod"
	content, err := GetFileAtCommit(repoRoot, filePath, commit, vcsType)
	if err != nil {
		t.Fatalf("Failed to get file at commit: %v", err)
	}

	if !strings.Contains(content, "module") {
		t.Error("Expected go.mod to contain 'module'")
	}
	t.Logf("File content length: %d bytes", len(content))
}

func TestGetWorkspaceRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	vcsType := DetectVCSType(cwd)
	if vcsType == "" {
		t.Skip("No VCS detected")
	}

	root, err := GetWorkspaceRoot(cwd, vcsType)
	if err != nil {
		t.Fatalf("Failed to get workspace root: %v", err)
	}

	if root == "" {
		t.Error("Expected non-empty workspace root")
	}
	t.Logf("Workspace root: %s", root)
}
