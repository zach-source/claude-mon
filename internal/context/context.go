package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ContextsDir is where context files are stored
var ContextsDir = filepath.Join(os.Getenv("HOME"), ".claude", "contexts")

// Context represents working context for a project
type Context struct {
	Version     int                    `json:"version"`
	ProjectID   string                 `json:"project_id"`
	ProjectRoot string                 `json:"project_root"`
	Updated     string                 `json:"updated"`
	Context     map[string]interface{} `json:"context"`
}

// KubernetesContext represents Kubernetes-specific context
type KubernetesContext struct {
	Context    string `json:"context,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
}

// AWSContext represents AWS-specific context
type AWSContext struct {
	Profile string `json:"profile,omitempty"`
	Region  string `json:"region,omitempty"`
}

// GitContext represents git-specific context
type GitContext struct {
	Branch string `json:"branch,omitempty"`
	Repo   string `json:"repo,omitempty"`
}

// New creates an empty context for the current project
func New() *Context {
	projectRoot := getProjectRoot()
	projectID := getProjectID(projectRoot)

	return &Context{
		Version:     2,
		ProjectID:   projectID,
		ProjectRoot: projectRoot,
		Updated:     "",
		Context:     make(map[string]interface{}),
	}
}

// Load loads existing context or returns new empty context
func Load() (*Context, error) {
	projectRoot := getProjectRoot()
	projectID := getProjectID(projectRoot)
	contextFile := filepath.Join(ContextsDir, projectID+".json")

	// Ensure contexts directory exists
	if err := os.MkdirAll(ContextsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create contexts directory: %w", err)
	}

	// Try to load existing context
	data, err := os.ReadFile(contextFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return new empty context
			return New(), nil
		}
		return nil, fmt.Errorf("failed to read context file: %w", err)
	}

	// Parse JSON
	var ctx Context
	if err := json.Unmarshal(data, &ctx); err != nil {
		// Invalid JSON, return new context
		return New(), nil
	}

	// Update metadata
	ctx.ProjectID = projectID
	ctx.ProjectRoot = projectRoot

	return &ctx, nil
}

// Save saves the context with an updated timestamp
func (c *Context) Save() error {
	c.Updated = time.Now().UTC().Format(time.RFC3339)

	// Ensure directory exists
	if err := os.MkdirAll(ContextsDir, 0755); err != nil {
		return fmt.Errorf("failed to create contexts directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	// Write to file
	contextFile := filepath.Join(ContextsDir, c.ProjectID+".json")
	if err := os.WriteFile(contextFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write context file: %w", err)
	}

	return nil
}

// SetKubernetes sets Kubernetes context
func (c *Context) SetKubernetes(context, namespace, kubeconfig string) {
	k8s := KubernetesContext{
		Context:    context,
		Namespace:  namespace,
		Kubeconfig: kubeconfig,
	}
	c.Context["kubernetes"] = k8s
}

// GetKubernetes gets Kubernetes context
func (c *Context) GetKubernetes() *KubernetesContext {
	if val, ok := c.Context["kubernetes"]; ok {
		// Handle both map and struct cases
		switch v := val.(type) {
		case map[string]interface{}:
			return &KubernetesContext{
				Context:    getString(v, "context"),
				Namespace:  getString(v, "namespace"),
				Kubeconfig: getString(v, "kubeconfig"),
			}
		case KubernetesContext:
			return &v
		}
	}
	return nil
}

// SetAWS sets AWS context
func (c *Context) SetAWS(profile, region string) {
	aws := AWSContext{
		Profile: profile,
		Region:  region,
	}
	c.Context["aws"] = aws
}

// GetAWS gets AWS context
func (c *Context) GetAWS() *AWSContext {
	if val, ok := c.Context["aws"]; ok {
		switch v := val.(type) {
		case map[string]interface{}:
			return &AWSContext{
				Profile: getString(v, "profile"),
				Region:  getString(v, "region"),
			}
		case AWSContext:
			return &v
		}
	}
	return nil
}

// SetGit sets git context (auto-detects if empty)
func (c *Context) SetGit(branch, repo string) {
	// Auto-detect branch if not provided
	if branch == "" {
		branch = runGitCommand("rev-parse", "--abbrev-ref", "HEAD")
	}

	// Auto-detect repo if not provided
	if repo == "" {
		// Try to get repo name from remote URL
		if url := runGitCommand("remote", "get-url", "origin"); url != "" {
			// Extract repo name from URL
			parts := strings.Split(url, "/")
			if len(parts) > 0 {
				repo = strings.TrimSuffix(parts[len(parts)-1], ".git")
			}
		}
	}

	if branch != "" || repo != "" {
		git := GitContext{
			Branch: branch,
			Repo:   repo,
		}
		c.Context["git"] = git
	}
}

// GetGit gets git context
func (c *Context) GetGit() *GitContext {
	if val, ok := c.Context["git"]; ok {
		switch v := val.(type) {
		case map[string]interface{}:
			return &GitContext{
				Branch: getString(v, "branch"),
				Repo:   getString(v, "repo"),
			}
		case GitContext:
			return &v
		}
	}
	return nil
}

// SetEnv sets environment variables
func (c *Context) SetEnv(vars map[string]string) {
	env := make(map[string]string)
	for k, v := range vars {
		env[k] = v
	}
	c.Context["env"] = env
}

// GetEnv gets environment variables
func (c *Context) GetEnv() map[string]string {
	if val, ok := c.Context["env"]; ok {
		switch v := val.(type) {
		case map[string]interface{}:
			result := make(map[string]string)
			for k, val := range v {
				if str, ok := val.(string); ok {
					result[k] = str
				}
			}
			return result
		case map[string]string:
			return v
		}
	}
	return nil
}

// SetCustom sets custom key-value pairs
func (c *Context) SetCustom(custom map[string]string) {
	customMap := make(map[string]string)
	for k, v := range custom {
		customMap[k] = v
	}
	c.Context["custom"] = customMap
}

// GetCustom gets custom key-value pairs
func (c *Context) GetCustom() map[string]string {
	if val, ok := c.Context["custom"]; ok {
		switch v := val.(type) {
		case map[string]interface{}:
			result := make(map[string]string)
			for k, val := range v {
				if str, ok := val.(string); ok {
					result[k] = str
				}
			}
			return result
		case map[string]string:
			return v
		}
	}
	return nil
}

// Clear removes a section from context
func (c *Context) Clear(section string) {
	sectionMap := map[string]string{
		"k8s":        "kubernetes",
		"kubernetes": "kubernetes",
		"aws":        "aws",
		"env":        "env",
		"git":        "git",
		"custom":     "custom",
	}

	if section == "all" {
		c.Context = make(map[string]interface{})
		return
	}

	if key, ok := sectionMap[section]; ok {
		delete(c.Context, key)
	}
}

// Format formats the context for display
func (c *Context) Format() string {
	var lines []string

	lines = append(lines, fmt.Sprintf("  Project: %s", c.ProjectRoot))
	lines = append(lines, "")

	if len(c.Context) == 0 {
		lines = append(lines, "  No context configured for this project.")
		return strings.Join(lines, "\n")
	}

	// Kubernetes
	if k8s := c.GetKubernetes(); k8s != nil {
		k8sStr := k8s.Context
		if k8s.Namespace != "" {
			k8sStr += fmt.Sprintf(" / %s", k8s.Namespace)
		}
		if k8s.Kubeconfig != "" {
			k8sStr += fmt.Sprintf(" (kubeconfig: %s)", k8s.Kubeconfig)
		}
		lines = append(lines, fmt.Sprintf("  Kubernetes: %s", k8sStr))
	}

	// AWS
	if aws := c.GetAWS(); aws != nil {
		awsStr := aws.Profile
		if aws.Region != "" {
			awsStr += fmt.Sprintf(" (%s)", aws.Region)
		}
		lines = append(lines, fmt.Sprintf("  AWS: %s", awsStr))
	}

	// Git
	if git := c.GetGit(); git != nil {
		gitStr := git.Branch
		if git.Repo != "" {
			if gitStr != "" {
				gitStr = fmt.Sprintf("%s @ %s", gitStr, git.Repo)
			} else {
				gitStr = git.Repo
			}
		}
		if gitStr != "" {
			lines = append(lines, fmt.Sprintf("  Git: %s", gitStr))
		}
	}

	// Env
	if env := c.GetEnv(); env != nil && len(env) > 0 {
		var envParts []string
		for k, v := range env {
			envParts = append(envParts, fmt.Sprintf("%s=%s", k, v))
		}
		lines = append(lines, fmt.Sprintf("  Env: %s", strings.Join(envParts, ", ")))
	}

	// Custom
	if custom := c.GetCustom(); custom != nil && len(custom) > 0 {
		for k, v := range custom {
			lines = append(lines, fmt.Sprintf("  %s: %s", k, v))
		}
	}

	// Updated time
	if c.Updated != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  Updated: %s", c.Updated))
	}

	return strings.Join(lines, "\n")
}

// GetAge returns a human-readable age string
func (c *Context) GetAge() string {
	if c.Updated == "" {
		return "never"
	}

	updated, err := time.Parse(time.RFC3339, c.Updated)
	if err != nil {
		return "unknown"
	}

	delta := time.Since(updated)

	minutes := int(delta.Minutes())
	hours := int(delta.Hours())
	days := int(delta.Hours() / 24)

	if minutes < 60 {
		return fmt.Sprintf("%dm ago", minutes)
	} else if hours < 24 {
		return fmt.Sprintf("%dh ago", hours)
	} else {
		return fmt.Sprintf("%dd ago", days)
	}
}

// IsStale returns true if context is older than 24 hours
func (c *Context) IsStale() bool {
	if c.Updated == "" {
		return true
	}

	updated, err := time.Parse(time.RFC3339, c.Updated)
	if err != nil {
		return true
	}

	return time.Since(updated).Hours() > 24
}

// ListAll returns all project contexts
func ListAll() ([]*Context, error) {
	if err := os.MkdirAll(ContextsDir, 0755); err != nil {
		return nil, err
	}

	files, err := filepath.Glob(filepath.Join(ContextsDir, "*.json"))
	if err != nil {
		return nil, err
	}

	var contexts []*Context
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var ctx Context
		if err := json.Unmarshal(data, &ctx); err != nil {
			continue
		}

		contexts = append(contexts, &ctx)
	}

	return contexts, nil
}

// Helper functions

func getProjectRoot() string {
	// Try to get git root
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output))
	}

	// Fallback to current directory
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func getProjectID(projectRoot string) string {
	// Generate short hash of path
	hash := sha256.Sum256([]byte(projectRoot))
	hashStr := hex.EncodeToString(hash[:])[:12]

	// Get readable project name
	projectName := strings.ToLower(strings.ReplaceAll(filepath.Base(projectRoot), " ", "-"))
	if len(projectName) > 20 {
		projectName = projectName[:20]
	}

	return fmt.Sprintf("%s-%s", projectName, hashStr)
}

func runGitCommand(args ...string) string {
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
