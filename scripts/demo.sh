#!/usr/bin/env bash
# Demo script to simulate Claude Code edits to the TUI
# Usage: Run the TUI first, then run this script in another terminal

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TUI_BIN="${SCRIPT_DIR}/../claude-follow-tui"

if [[ ! -x "$TUI_BIN" ]]; then
    echo "Error: TUI binary not found. Run 'make build' first."
    exit 1
fi

echo "📤 Sending simulated Claude edits to TUI..."
echo ""

# Simulate Edit operation
echo "1️⃣  Simulating Edit to main.go..."
echo '{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/Users/ztaylor/project/main.go",
    "old_string": "func main() {\n\tfmt.Println(\"Hello\")\n}",
    "new_string": "func main() {\n\tfmt.Println(\"Hello, World!\")\n\tfmt.Println(\"Welcome to the app\")\n}"
  }
}' | "$TUI_BIN" send

sleep 1

# Simulate Write operation
echo "2️⃣  Simulating Write to config.yaml..."
echo '{
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/Users/ztaylor/project/config.yaml",
    "content": "server:\n  port: 8080\n  host: localhost\n\ndatabase:\n  driver: postgres\n  name: myapp"
  }
}' | "$TUI_BIN" send

sleep 1

# Simulate another Edit
echo "3️⃣  Simulating Edit to handler.go..."
echo '{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/Users/ztaylor/project/internal/handler.go",
    "old_string": "return nil",
    "new_string": "return json.NewEncoder(w).Encode(response)"
  }
}' | "$TUI_BIN" send

sleep 1

# Simulate MultiEdit
echo "4️⃣  Simulating Edit to utils.go..."
echo '{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/Users/ztaylor/project/pkg/utils.go",
    "old_string": "// TODO: implement",
    "new_string": "if err != nil {\n\treturn fmt.Errorf(\"failed to process: %w\", err)\n}"
  }
}' | "$TUI_BIN" send

echo ""
echo "✅ Demo complete! Check the TUI to see the changes."
echo ""
echo "TIP: Use j/k to navigate, Tab to switch panes, Ctrl+G to open in nvim"
