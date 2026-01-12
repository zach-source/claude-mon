package plan

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Adjectives for memorable slug generation
var adjectives = []string{
	"dancing", "buzzing", "cryptic", "delightful", "indexed",
	"prancing", "singing", "whirling", "gleaming", "sparkling",
	"humming", "floating", "drifting", "glowing", "shimmering",
	"bouncing", "twirling", "swaying", "beaming", "blazing",
	"dazzling", "flashing", "glinting", "radiant", "vibrant",
	"mystic", "cosmic", "stellar", "lunar", "solar",
	"golden", "silver", "crimson", "azure", "emerald",
}

// Nouns for memorable slug generation
var nouns = []string{
	"nova", "teacup", "dawn", "bonbon", "codd", "wadler",
	"phoenix", "aurora", "nebula", "quasar", "pulsar",
	"meadow", "brook", "willow", "cypress", "maple",
	"falcon", "raven", "sparrow", "finch", "crane",
	"crystal", "prism", "diamond", "opal", "jade",
	"breeze", "zephyr", "tempest", "cascade", "glacier",
	"echo", "whisper", "harmony", "melody", "sonata",
}

// MCP config for plan generation - only graphiti and context7
const planMCPConfig = `{
  "mcpServers": {
    "graphiti": {
      "command": "uvx",
      "args": ["graphiti-mcp-server"],
      "env": {
        "NEO4J_URI": "bolt://localhost:7687",
        "NEO4J_USER": "neo4j",
        "NEO4J_PASSWORD": "password"
      }
    },
    "context7": {
      "command": "npx",
      "args": ["-y", "@context7/mcp-server"]
    }
  }
}`

// Meta-prompt template for plan generation
const planMetaPrompt = `You are a technical planning assistant. Create a detailed implementation plan for:

%s

Output a markdown document with these sections:
# Plan: [Descriptive Title]

## Overview
[2-3 sentence summary]

## Architecture
[Technical approach, components involved]

## Implementation Phases
### Phase 1: [Name]
**Goal**: [Specific deliverable]
**Files**: [Key files to modify]
**Tasks**:
- [ ] Task 1
- [ ] Task 2

### Phase 2: [Name]
[Continue for each phase]

## Verification
[How to test that the implementation is complete]

## Edge Cases
[Potential issues and how to handle them]

Output ONLY the markdown plan, no preamble.`

func init() {
	rand.Seed(time.Now().UnixNano())
}

// GenerateSlug creates a memorable slug like "dancing-singing-bonbon"
func GenerateSlug() string {
	adj1 := adjectives[rand.Intn(len(adjectives))]
	adj2 := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	return fmt.Sprintf("%s-%s-%s", adj1, adj2, noun)
}

// WriteMCPConfig writes the MCP config for plan generation to a temp file.
// Returns the path to the config file. Caller should clean up with os.Remove.
func WriteMCPConfig() (string, error) {
	tmpDir := os.TempDir()
	mcpPath := filepath.Join(tmpDir, "claude-follow-plan-mcp.json")
	if err := os.WriteFile(mcpPath, []byte(planMCPConfig), 0644); err != nil {
		return "", fmt.Errorf("failed to write MCP config: %w", err)
	}
	return mcpPath, nil
}

// Generate creates a new plan using Claude CLI with the given description.
// Returns the path to the generated plan file.
func Generate(description string) (string, error) {
	// Write MCP config
	mcpConfigPath, err := WriteMCPConfig()
	if err != nil {
		return "", err
	}
	defer os.Remove(mcpConfigPath) // Clean up temp file

	// Build the prompt
	prompt := fmt.Sprintf(planMetaPrompt, description)

	// Run Claude CLI with MCP servers
	cmd := exec.Command("claude", "-p", prompt, "--mcp-config", mcpConfigPath)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude CLI failed: %s", string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to run claude CLI: %w", err)
	}

	// Generate unique slug and path
	slug := GenerateSlug()
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %w", err)
	}

	// Ensure plans directory exists
	plansDir := filepath.Join(home, ".claude", "plans")
	if err := os.MkdirAll(plansDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create plans dir: %w", err)
	}

	planPath := filepath.Join(plansDir, slug+".md")

	// Check for collision and regenerate slug if needed
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(planPath); os.IsNotExist(err) {
			break
		}
		slug = GenerateSlug()
		planPath = filepath.Join(plansDir, slug+".md")
	}

	// Write plan file
	if err := os.WriteFile(planPath, output, 0644); err != nil {
		return "", fmt.Errorf("failed to write plan file: %w", err)
	}

	return planPath, nil
}

// GetPlansDir returns the directory where plans are stored
func GetPlansDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "plans"), nil
}

// ListPlans returns all plan files in the plans directory
func ListPlans() ([]string, error) {
	plansDir, err := GetPlansDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(plansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No plans directory yet
		}
		return nil, err
	}

	var plans []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			plans = append(plans, filepath.Join(plansDir, entry.Name()))
		}
	}

	return plans, nil
}

// DeletePlan removes a plan file
func DeletePlan(path string) error {
	return os.Remove(path)
}
