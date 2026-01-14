package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ztaylor/claude-mon/internal/context"
)

func main() {
	// Execute the context injection
	if err := context.InjectForHook(); err != nil {
		// Log error to stderr, but continue without injection
		fmt.Fprintf(os.Stderr, "inject-context error: %v\n", err)
		result := context.HookResult{Continue: true}
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode result: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}
