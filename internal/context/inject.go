package context

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// HookPayload represents the UserPromptSubmit hook payload
type HookPayload struct {
	Prompt string `json:"prompt"`
	// Other fields may be present but we only need prompt
}

// HookResult represents the result returned to the hook system
type HookResult struct {
	Continue bool   `json:"continue"`
	Message  string `json:"message,omitempty"`
}

// InjectForHook loads context and formats it for Claude hook injection.
// This reads JSON from stdin and writes result to stdout.
// Returns error if injection fails, but allows continuing without error.
func InjectForHook() error {
	// Read the input from stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to read stdin: %w", err)
	}

	// Parse the JSON payload
	var payload HookPayload
	if err := json.Unmarshal(input, &payload); err != nil {
		// Invalid JSON, pass through unchanged
		result := HookResult{Continue: true}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	// Check if context is already present in prompt
	if strings.Contains(payload.Prompt, "<working-context>") {
		result := HookResult{Continue: true}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	// Load the working context
	ctx, err := Load()
	if err != nil || ctx == nil {
		// No context data, pass through
		result := HookResult{Continue: true}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	// Format the context block
	contextBlock := ctx.FormatForInjection()
	if contextBlock == "" {
		// Empty context, pass through
		result := HookResult{Continue: true}
		return json.NewEncoder(os.Stdout).Encode(result)
	}

	// Return the context as a message to be added
	result := HookResult{
		Continue: true,
		Message:  "\n" + contextBlock + "\n",
	}

	return json.NewEncoder(os.Stdout).Encode(result)
}

// FormatForInjection formats the context as a <working-context> block for prompt injection.
// This is similar to Format() but uses the specific XML-like format expected by Claude.
func (c *Context) FormatForInjection() string {
	if len(c.Context) == 0 {
		return ""
	}

	var lines []string

	// Kubernetes
	if k8s := c.GetKubernetes(); k8s != nil {
		k8sStr := k8s.Context
		if k8sStr == "" {
			k8sStr = "default"
		}
		if k8s.Namespace != "" {
			k8sStr += fmt.Sprintf(" / %s", k8s.Namespace)
		}
		if k8s.Kubeconfig != "" {
			k8sStr += fmt.Sprintf(" (kubeconfig: %s)", k8s.Kubeconfig)
		}
		lines = append(lines, fmt.Sprintf("Kubernetes: %s", k8sStr))
	}

	// AWS
	if aws := c.GetAWS(); aws != nil {
		awsStr := aws.Profile
		if awsStr == "" {
			awsStr = "default"
		}
		if aws.Region != "" {
			awsStr += fmt.Sprintf(" (%s)", aws.Region)
		}
		lines = append(lines, fmt.Sprintf("AWS Profile: %s", awsStr))
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
			lines = append(lines, fmt.Sprintf("Git: %s", gitStr))
		}
	}

	// Environment variables
	if env := c.GetEnv(); env != nil && len(env) > 0 {
		var envParts []string
		for k, v := range env {
			envParts = append(envParts, fmt.Sprintf("%s=%s", k, v))
		}
		lines = append(lines, fmt.Sprintf("Env: %s", strings.Join(envParts, ", ")))
	}

	// Custom values
	if custom := c.GetCustom(); custom != nil && len(custom) > 0 {
		var customParts []string
		for k, v := range custom {
			customParts = append(customParts, fmt.Sprintf("%s=%s", k, v))
		}
		lines = append(lines, fmt.Sprintf("Custom: %s", strings.Join(customParts, ", ")))
	}

	if len(lines) == 0 {
		return ""
	}

	// Build the context block
	var block strings.Builder
	block.WriteString("<working-context>\n")
	for _, line := range lines {
		block.WriteString(fmt.Sprintf("  %s\n", line))
	}

	// Add age with stale warning
	if c.Updated != "" {
		age := c.GetAge()
		staleWarning := ""
		if c.IsStale() {
			staleWarning = " (STALE - consider updating)"
		}
		block.WriteString(fmt.Sprintf("  Updated: %s%s\n", age, staleWarning))
	}

	block.WriteString("</working-context>")
	return block.String()
}
