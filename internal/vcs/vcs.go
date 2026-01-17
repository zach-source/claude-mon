// Package vcs provides version control system operations
package vcs

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetFileAtCommit retrieves file content at a specific commit/change ID
// workspacePath is the root of the VCS repository
// filePath is the path to the file (can be absolute or relative to workspace)
// commitSHA is the commit hash (git) or change ID (jj)
// vcsType is "git" or "jj"
func GetFileAtCommit(workspacePath, filePath, commitSHA, vcsType string) (string, error) {
	if commitSHA == "" {
		return "", fmt.Errorf("no commit SHA provided")
	}

	// Make file path relative to workspace if it's absolute
	relPath := filePath
	if filepath.IsAbs(filePath) && workspacePath != "" {
		var err error
		relPath, err = filepath.Rel(workspacePath, filePath)
		if err != nil {
			relPath = filePath // Fall back to original
		}
	}

	switch vcsType {
	case "jj":
		return getFileFromJJ(workspacePath, relPath, commitSHA)
	case "git":
		return getFileFromGit(workspacePath, relPath, commitSHA)
	default:
		// Try jj first (auto-detection), then git
		content, err := getFileFromJJ(workspacePath, relPath, commitSHA)
		if err == nil {
			return content, nil
		}
		return getFileFromGit(workspacePath, relPath, commitSHA)
	}
}

// getFileFromJJ retrieves file content from jj at a specific change ID
func getFileFromJJ(workspacePath, filePath, changeID string) (string, error) {
	// jj file show <file> -r <revision>
	cmd := exec.Command("jj", "file", "show", filePath, "-r", changeID)
	cmd.Dir = workspacePath
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("jj file show failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("jj file show failed: %w", err)
	}
	return string(output), nil
}

// getFileFromGit retrieves file content from git at a specific commit
func getFileFromGit(workspacePath, filePath, commitSHA string) (string, error) {
	// git show <commit>:<file>
	// Note: git needs the path relative to repo root
	cmd := exec.Command("git", "show", fmt.Sprintf("%s:%s", commitSHA, filePath))
	cmd.Dir = workspacePath
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git show failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("git show failed: %w", err)
	}
	return string(output), nil
}

// DetectVCSType detects the VCS type for a given directory
func DetectVCSType(dir string) string {
	// Check for jj first
	cmd := exec.Command("jj", "root")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		return "jj"
	}

	// Check for git
	cmd = exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		return "git"
	}

	return ""
}

// GetCurrentCommit gets the current commit/change ID
func GetCurrentCommit(dir, vcsType string) (string, error) {
	switch vcsType {
	case "jj":
		cmd := exec.Command("jj", "log", "-r", "@", "--no-graph", "-T", "change_id.short()")
		cmd.Dir = dir
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("jj log failed: %w", err)
		}
		return strings.TrimSpace(string(output)), nil

	case "git":
		cmd := exec.Command("git", "rev-parse", "HEAD")
		cmd.Dir = dir
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git rev-parse failed: %w", err)
		}
		return strings.TrimSpace(string(output)), nil

	default:
		// Auto-detect
		vcsType = DetectVCSType(dir)
		if vcsType != "" {
			return GetCurrentCommit(dir, vcsType)
		}
		return "", fmt.Errorf("no VCS detected")
	}
}

// GetWorkspaceRoot returns the root directory of the VCS workspace
func GetWorkspaceRoot(dir, vcsType string) (string, error) {
	switch vcsType {
	case "jj":
		cmd := exec.Command("jj", "root")
		cmd.Dir = dir
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("jj root failed: %w", err)
		}
		return strings.TrimSpace(string(output)), nil

	case "git":
		cmd := exec.Command("git", "rev-parse", "--show-toplevel")
		cmd.Dir = dir
		output, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git rev-parse failed: %w", err)
		}
		return strings.TrimSpace(string(output)), nil

	default:
		// Auto-detect
		vcsType = DetectVCSType(dir)
		if vcsType != "" {
			return GetWorkspaceRoot(dir, vcsType)
		}
		return "", fmt.Errorf("no VCS detected")
	}
}
