#!/usr/bin/env bash
# claude-mon-daemon-hook.sh
# Hook script for sending Claude Code activity to claude-mon daemon
#
# Usage: Add to your Claude Code hooks (PostToolUse, etc.)
#
# Example in .claude/hooks/PostToolUse:
#   #!/bin/bash
#   /path/to/claude-mon-daemon-hook.sh edit "$TOOL_NAME" "$TOOL_INPUT"
#
# Environment variables automatically available:
#   WORKSPACE_ID - Unique workspace identifier
#   WORKSPACE_PATH - Full path to workspace
#   WORKSPACE_NAME - Name of the workspace/project

set -euo pipefail

# Configuration
DAEMON_SOCKET="${CLAUDE_MON_DAEMON_SOCKET:-/tmp/claude-mon-daemon.sock}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Get workspace info
WORKSPACE_PATH="${WORKSPACE_PATH:-$(pwd)}"
WORKSPACE_NAME="${WORKSPACE_NAME:-$(basename "$WORKSPACE_PATH")}"

# Get git info (only if not already set)
: "${BRANCH:=}"
: "${COMMIT_SHA:=}"

if [[ -z "$BRANCH" ]] || [[ -z "$COMMIT_SHA" ]]; then
	if git rev-parse --git-dir > /dev/null 2>&1; then
		: "${BRANCH:=$(git branch --show-current 2>/dev/null || echo '')}"
		: "${COMMIT_SHA:=$(git rev-parse HEAD 2>/dev/null || echo '')}"
	fi
fi

# Function to send data to daemon
send_to_daemon() {
	local payload="$1"

	# Check if daemon is running
	if [[ ! -S "$DAEMON_SOCKET" ]]; then
		# Daemon not running, silently skip
		exit 0
	fi

	# Send payload
	echo "$payload" | nc -U "$DAEMON_SOCKET" || true
}

# Parse command
COMMAND="${1:-}"
shift || true

case "$COMMAND" in
	edit)
		# PostToolUse hook - record file edit
		TOOL_NAME="${1:-}"
		TOOL_INPUT="${2:-}"

		if [[ -z "$TOOL_INPUT" ]]; then
			exit 0
		fi

		# Parse tool input to extract file path and strings
		# Using jq if available, otherwise basic string parsing
		if command -v jq >/dev/null 2>&1; then
			FILE_PATH=$(echo "$TOOL_INPUT" | jq -r '.tool_input.file_path // empty')
			OLD_STRING=$(echo "$TOOL_INPUT" | jq -r '.tool_input.old_string // empty' | head -c 1000)
			NEW_STRING=$(echo "$TOOL_INPUT" | jq -r '.tool_input.new_string // empty' | head -c 1000)
			LINE_NUM=$(echo "$TOOL_INPUT" | jq -r '.tool_input.line_num // 0')
			LINE_COUNT=$(echo "$TOOL_INPUT" | jq -r '.tool_input.line_count // 0')
		else
			# Fallback: basic grep parsing (less reliable)
			FILE_PATH=$(echo "$TOOL_INPUT" | grep -o '"file_path":"[^"]*"' | cut -d'"' -f4)
			LINE_NUM=0
			LINE_COUNT=0
			OLD_STRING=""
			NEW_STRING=""
		fi

		if [[ -n "$FILE_PATH" ]]; then
			# Calculate line count from new string
			if [[ -n "$NEW_STRING" ]]; then
				LINE_COUNT=$(echo "$NEW_STRING" | wc -l)
			fi

			# Create JSON payload
			PAYLOAD=$(cat <<EOF
{
	"type": "edit",
	"workspace": $(echo "$WORKSPACE_PATH" | jq -Rs .),
	"workspace_name": $(echo "$WORKSPACE_NAME" | jq -Rs .),
	"branch": $(echo "$BRANCH" | jq -Rs .),
	"commit_sha": $(echo "$COMMIT_SHA" | jq -Rs .),
	"tool_name": $(echo "$TOOL_NAME" | jq -Rs .),
	"file_path": $(echo "$FILE_PATH" | jq -Rs .),
	"old_string": $(echo "$OLD_STRING" | jq -Rs .),
	"new_string": $(echo "$NEW_STRING" | jq -Rs .),
	"line_num": $LINE_NUM,
	"line_count": $LINE_COUNT
}
EOF
			)
			send_to_daemon "$PAYLOAD"
		fi
		;;

	prompt)
		# Record prompt creation/update
		PROMPT_NAME="${1:-}"
		PROMPT_DESC="${2:-}"
		PROMPT_CONTENT="${3:-}"
		PROMPT_TAGS="${4:-}"

		if [[ -z "$PROMPT_NAME" ]] || [[ -z "$PROMPT_CONTENT" ]]; then
			echo "Error: prompt name and content required" >&2
			exit 1
		fi

		# Create JSON payload
		PAYLOAD=$(cat <<EOF
{
	"type": "prompt",
	"workspace": "$WORKSPACE_PATH",
	"workspace_name": "$WORKSPACE_NAME",
	"branch": "$BRANCH",
	"commit_sha": "$COMMIT_SHA",
	"prompt_name": $(echo "$PROMPT_NAME" | jq -Rs .),
	"prompt_description": $(echo "$PROMPT_DESC" | jq -Rs .),
	"new_string": $(echo "$PROMPT_CONTENT" | jq -Rs .),
	"prompt_tags": $(echo "$PROMPT_TAGS" | jq -R '. | split(" ")' 2>/dev/null || echo "[]")
}
EOF
		)
		send_to_daemon "$PAYLOAD"
		;;

	*)
		echo "Unknown command: $COMMAND" >&2
		echo "Usage: $0 {edit|prompt} [args...]" >&2
		exit 1
		;;
esac

exit 0
