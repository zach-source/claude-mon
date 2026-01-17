#!/bin/bash
# claude-mon PostToolUse hook
# Sends tool edits to both the TUI and daemon for real-time display and persistence

# Get the current directory and resolve to absolute path
CWD="$(cd "$(pwd)" && pwd)"

# Resolve symlinks (macOS compatible)
if command -v realpath &>/dev/null; then
    CWD="$(realpath "$CWD")"
elif [[ "$(uname)" == "Darwin" ]]; then
    # macOS: use perl to resolve symlinks
    CWD="$(perl -MCwd -e 'print Cwd::realpath($ARGV[0])' "$CWD")"
else
    CWD="$(readlink -f "$CWD")"
fi

# Hash the path (matching Go's sha256.Sum256[:12])
HASH="$(echo -n "$CWD" | sha256sum | cut -c1-12)"

# Get username
USER="${USER:-unknown}"

# Socket paths
TUI_SOCKET="/tmp/claude-mon-${USER}-${HASH}.sock"
DAEMON_SOCKET="/tmp/claude-mon-daemon.sock"

# Send to TUI if socket exists (raw TOOL_INPUT)
if [[ -S "$TUI_SOCKET" ]]; then
    echo "$TOOL_INPUT" | nc -U "$TUI_SOCKET" &
fi

# Send to daemon if socket exists (formatted payload)
if [[ -S "$DAEMON_SOCKET" ]] && command -v jq &>/dev/null; then
    # Parse tool input
    TOOL_NAME="${TOOL_NAME:-unknown}"
    FILE_PATH=$(echo "$TOOL_INPUT" | jq -r '.file_path // .path // empty' 2>/dev/null)
    OLD_STRING=$(echo "$TOOL_INPUT" | jq -r '.old_string // empty' 2>/dev/null | head -c 10000)
    NEW_STRING=$(echo "$TOOL_INPUT" | jq -r '.new_string // .content // empty' 2>/dev/null | head -c 10000)

    if [[ -n "$FILE_PATH" ]]; then
        # Get VCS info (jj or git)
        BRANCH=""
        COMMIT_SHA=""
        VCS_TYPE=""

        # Check for jj first (it auto-commits every change)
        if jj root &>/dev/null 2>&1; then
            VCS_TYPE="jj"
            # Get current change ID (short form)
            COMMIT_SHA=$(jj log -r @ --no-graph -T 'change_id.short()' 2>/dev/null || echo "")
            # jj doesn't have branches in the same way, use bookmark or description
            BRANCH=$(jj log -r @ --no-graph -T 'bookmarks' 2>/dev/null | head -1 || echo "")
        elif git rev-parse --git-dir &>/dev/null; then
            VCS_TYPE="git"
            BRANCH=$(git branch --show-current 2>/dev/null || echo "")
            COMMIT_SHA=$(git rev-parse HEAD 2>/dev/null || echo "")
        fi

        # Calculate line count
        LINE_COUNT=0
        if [[ -n "$NEW_STRING" ]]; then
            LINE_COUNT=$(echo "$NEW_STRING" | wc -l | tr -d ' ')
        fi

        # Read and base64-encode file content (max 500KB to avoid huge payloads)
        FILE_CONTENT_B64=""
        ABSOLUTE_PATH="$FILE_PATH"
        if [[ ! "$FILE_PATH" = /* ]]; then
            ABSOLUTE_PATH="$CWD/$FILE_PATH"
        fi
        if [[ -f "$ABSOLUTE_PATH" ]] && [[ $(stat -f%z "$ABSOLUTE_PATH" 2>/dev/null || stat -c%s "$ABSOLUTE_PATH" 2>/dev/null) -lt 512000 ]]; then
            FILE_CONTENT_B64=$(base64 < "$ABSOLUTE_PATH" 2>/dev/null | tr -d '\n' || echo "")
        fi

        # Create daemon payload
        PAYLOAD=$(jq -n \
            --arg type "edit" \
            --arg workspace "$CWD" \
            --arg workspace_name "$(basename "$CWD")" \
            --arg branch "$BRANCH" \
            --arg commit_sha "$COMMIT_SHA" \
            --arg vcs_type "$VCS_TYPE" \
            --arg tool_name "$TOOL_NAME" \
            --arg file_path "$FILE_PATH" \
            --arg old_string "$OLD_STRING" \
            --arg new_string "$NEW_STRING" \
            --arg file_content_b64 "$FILE_CONTENT_B64" \
            --argjson line_num 0 \
            --argjson line_count "$LINE_COUNT" \
            '{
                type: $type,
                workspace: $workspace,
                workspace_name: $workspace_name,
                branch: $branch,
                commit_sha: $commit_sha,
                vcs_type: $vcs_type,
                tool_name: $tool_name,
                file_path: $file_path,
                old_string: $old_string,
                new_string: $new_string,
                file_content_b64: $file_content_b64,
                line_num: $line_num,
                line_count: $line_count
            }')

        echo "$PAYLOAD" | nc -U "$DAEMON_SOCKET" &
    fi
fi

# Wait for background jobs to complete before exiting
wait
