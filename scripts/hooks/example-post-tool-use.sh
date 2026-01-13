#!/bin/bash
# Claude Code PostToolUse Hook for claude-mon daemon
# This script sends all Claude Code edits to the daemon for persistent storage

# Get tool input from Claude Code
TOOL_INPUT="${1:-}"

# Get workspace info
WORKSPACE_PATH="$(pwd)"
WORKSPACE_NAME="$(basename "$WORKSPACE_PATH")"

# Call the daemon hook script
"${SCRIPT_DIR}/claude-mon-daemon-hook.sh" edit "Edit" "$TOOL_INPUT"
