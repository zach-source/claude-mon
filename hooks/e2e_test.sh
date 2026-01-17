#!/bin/bash
# E2E Hook Reliability Test
# Tests the claude-mon hook under various conditions
# Usage: ./hooks/e2e_test.sh [count]

set -e

COUNT=${1:-100}
TIMEOUT_MS=${2:-100}  # milliseconds
TIMEOUT_S=$(echo "scale=3; $TIMEOUT_MS / 1000" | bc)

cd "$(dirname "$0")/.."

echo "=============================================="
echo "Claude-Mon Hook E2E Test"
echo "=============================================="
echo "Records per test: $COUNT"
echo "Hook timeout: ${TIMEOUT_MS}ms"
echo ""

# Verify daemon is running
if ! claude-mon query recent 1 &>/dev/null; then
    echo "ERROR: Daemon not running. Start with: claude-mon daemon start"
    exit 1
fi

# Create test hooks
TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

# Old hook (no wait)
cat > "$TEMP_DIR/old-hook.sh" << 'HOOK'
#!/bin/bash
CWD="$(pwd)"
DAEMON_SOCKET="/tmp/claude-mon-daemon.sock"
if [[ -S "$DAEMON_SOCKET" ]] && command -v jq &>/dev/null; then
    FILE_PATH=$(echo "$TOOL_INPUT" | jq -r '.file_path // empty' 2>/dev/null)
    NEW_STRING=$(echo "$TOOL_INPUT" | jq -r '.new_string // empty' 2>/dev/null)
    sleep 0.01  # Simulate processing time
    PAYLOAD=$(jq -n --arg type "edit" --arg workspace "$CWD" --arg workspace_name "test-old" \
        --arg tool_name "$TOOL_NAME" --arg file_path "$FILE_PATH" --arg new_string "$NEW_STRING" \
        --arg old_string "" --arg branch "" --arg commit_sha "" --argjson line_num 0 --argjson line_count 1 \
        '{type:$type,workspace:$workspace,workspace_name:$workspace_name,tool_name:$tool_name,file_path:$file_path,old_string:$old_string,new_string:$new_string,branch:$branch,commit_sha:$commit_sha,line_num:$line_num,line_count:$line_count}')
    echo "$PAYLOAD" | nc -U "$DAEMON_SOCKET" &
fi
HOOK
chmod +x "$TEMP_DIR/old-hook.sh"

# New hook (with wait)
cat > "$TEMP_DIR/new-hook.sh" << 'HOOK'
#!/bin/bash
CWD="$(pwd)"
DAEMON_SOCKET="/tmp/claude-mon-daemon.sock"
if [[ -S "$DAEMON_SOCKET" ]] && command -v jq &>/dev/null; then
    FILE_PATH=$(echo "$TOOL_INPUT" | jq -r '.file_path // empty' 2>/dev/null)
    NEW_STRING=$(echo "$TOOL_INPUT" | jq -r '.new_string // empty' 2>/dev/null)
    sleep 0.01  # Simulate processing time
    PAYLOAD=$(jq -n --arg type "edit" --arg workspace "$CWD" --arg workspace_name "test-new" \
        --arg tool_name "$TOOL_NAME" --arg file_path "$FILE_PATH" --arg new_string "$NEW_STRING" \
        --arg old_string "" --arg branch "" --arg commit_sha "" --argjson line_num 0 --argjson line_count 1 \
        '{type:$type,workspace:$workspace,workspace_name:$workspace_name,tool_name:$tool_name,file_path:$file_path,old_string:$old_string,new_string:$new_string,branch:$branch,commit_sha:$commit_sha,line_num:$line_num,line_count:$line_count}')
    echo "$PAYLOAD" | nc -U "$DAEMON_SOCKET" &
fi
wait  # KEY FIX: Wait for background nc to complete
HOOK
chmod +x "$TEMP_DIR/new-hook.sh"

# Generate unique test ID
TEST_ID=$(date +%s)

echo ">>> Test 1: OLD hook (no wait) with ${TIMEOUT_MS}ms timeout..."
OLD_SUCCESS=0
for i in $(seq 1 $COUNT); do
    export TOOL_NAME="Edit"
    export TOOL_INPUT='{"file_path":"/tmp/e2e-'$TEST_ID'-old-'$i'.txt","new_string":"old hook '$i'"}'
    timeout $TIMEOUT_S "$TEMP_DIR/old-hook.sh" 2>/dev/null && ((OLD_SUCCESS++)) || true
done
sleep 2
OLD_COUNT=$(claude-mon query recent $((COUNT * 3)) 2>/dev/null | grep -c "e2e-${TEST_ID}-old-" || true)
OLD_COUNT=${OLD_COUNT:-0}
echo "  Hooks completed: $OLD_SUCCESS / $COUNT"
echo "  Records in DB:   $OLD_COUNT / $COUNT"
OLD_LOSS=$((COUNT - OLD_COUNT))
echo ""

echo ">>> Test 2: NEW hook (with wait) with ${TIMEOUT_MS}ms timeout..."
NEW_SUCCESS=0
for i in $(seq 1 $COUNT); do
    export TOOL_NAME="Edit"
    export TOOL_INPUT='{"file_path":"/tmp/e2e-'$TEST_ID'-new-'$i'.txt","new_string":"new hook '$i'"}'
    timeout $TIMEOUT_S "$TEMP_DIR/new-hook.sh" 2>/dev/null && ((NEW_SUCCESS++)) || true
done
sleep 2
NEW_COUNT=$(claude-mon query recent $((COUNT * 3)) 2>/dev/null | grep -c "e2e-${TEST_ID}-new-" || true)
NEW_COUNT=${NEW_COUNT:-0}
echo "  Hooks completed: $NEW_SUCCESS / $COUNT"
echo "  Records in DB:   $NEW_COUNT / $COUNT"
NEW_LOSS=$((COUNT - NEW_COUNT))
echo ""

echo "=============================================="
echo "RESULTS"
echo "=============================================="
printf "| %-20s | %-12s | %-12s | %-12s |\n" "Hook Version" "Completed" "Recorded" "Data Loss"
printf "| %-20s | %-12s | %-12s | %-12s |\n" "--------------------" "------------" "------------" "------------"
printf "| %-20s | %-12s | %-12s | %-12s |\n" "Old (no wait)" "$OLD_SUCCESS/$COUNT" "$OLD_COUNT/$COUNT" "$OLD_LOSS"
printf "| %-20s | %-12s | %-12s | %-12s |\n" "New (with wait)" "$NEW_SUCCESS/$COUNT" "$NEW_COUNT/$COUNT" "$NEW_LOSS"
echo ""

if [[ $NEW_LOSS -eq 0 ]] && [[ $OLD_LOSS -gt 0 || $NEW_COUNT -ge $OLD_COUNT ]]; then
    echo "✅ PASS: New hook (with wait) is more reliable"
    exit 0
elif [[ $NEW_LOSS -eq 0 ]] && [[ $OLD_LOSS -eq 0 ]]; then
    echo "✅ PASS: Both hooks succeeded (try lower timeout to stress test)"
    exit 0
else
    echo "❌ FAIL: New hook lost data"
    exit 1
fi
