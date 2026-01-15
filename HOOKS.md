# Claude Code Hooks Setup

This guide explains how to install the hooks that send Claude's edits to claude-mon for real-time tracking and persistent history.

## Prerequisites

- `jq` - JSON processor (required for daemon communication)
- `nc` (netcat) - For Unix socket communication
- `sha256sum` or equivalent - For workspace hashing

```bash
# macOS
brew install jq netcat

# Ubuntu/Debian
sudo apt install jq netcat-openbsd
```

## Quick Install

### Option 1: Copy to your project

Copy the hook to your project's `.claude/hooks/` directory:

```bash
# From the claude-mon repo
cp .claude/hooks/PostToolUse /path/to/your/project/.claude/hooks/
chmod +x /path/to/your/project/.claude/hooks/PostToolUse
```

### Option 2: Symlink (recommended for multiple projects)

Create a symlink from your project to a central location:

```bash
# Install hook to a central location
mkdir -p ~/.local/share/claude-mon/hooks
cp .claude/hooks/PostToolUse ~/.local/share/claude-mon/hooks/
chmod +x ~/.local/share/claude-mon/hooks/PostToolUse

# In each project, symlink to it
mkdir -p /path/to/your/project/.claude/hooks
ln -s ~/.local/share/claude-mon/hooks/PostToolUse /path/to/your/project/.claude/hooks/PostToolUse
```

### Option 3: Global hook (all projects)

Install as a global Claude Code hook:

```bash
mkdir -p ~/.claude/hooks
cp .claude/hooks/PostToolUse ~/.claude/hooks/
chmod +x ~/.claude/hooks/PostToolUse
```

## What the Hook Does

The `PostToolUse` hook runs after every Claude tool call (Edit, Write, etc.) and:

1. **Sends to TUI** - Raw tool input to the TUI socket for real-time display
2. **Sends to Daemon** - Formatted payload to the daemon for persistent storage

### Data Captured

| Field | Source | Description |
|-------|--------|-------------|
| `workspace` | `$PWD` | Current working directory |
| `workspace_name` | `basename $PWD` | Project name |
| `branch` | `git branch` | Current git branch |
| `commit_sha` | `git rev-parse HEAD` | Current commit |
| `tool_name` | `$TOOL_NAME` | Edit, Write, etc. |
| `file_path` | Tool input | File being modified |
| `old_string` | Tool input | Original content (max 10KB) |
| `new_string` | Tool input | New content (max 10KB) |

## Verifying Installation

### 1. Check the hook is executable

```bash
ls -la .claude/hooks/PostToolUse
# Should show: -rwxr-xr-x ... PostToolUse
```

### 2. Start the daemon

```bash
claude-mon daemon start
```

### 3. Verify daemon is listening

```bash
ls -la /tmp/claude-mon-daemon.sock
# Should show: srwxr-xr-x ... claude-mon-daemon.sock
```

### 4. Test the hook manually

```bash
export TOOL_NAME="Edit"
export TOOL_INPUT='{"file_path": "/tmp/test.go", "old_string": "old", "new_string": "new"}'
.claude/hooks/PostToolUse

# Check database
sqlite3 ~/.claude-mon/claude-mon.db "SELECT * FROM edits ORDER BY id DESC LIMIT 1;"
```

### 5. Make an edit with Claude

Ask Claude to make a small edit, then verify:

```bash
sqlite3 ~/.claude-mon/claude-mon.db "SELECT timestamp, tool_name, file_path FROM edits ORDER BY id DESC LIMIT 5;"
```

## Troubleshooting

### Hook not running

Verify Claude Code is configured to use project hooks:

```bash
# Check Claude settings
cat ~/.config/claude/settings.json
```

Should include hooks enabled or not explicitly disabled.

### Daemon not receiving data

1. Check daemon is running: `claude-mon daemon status`
2. Check socket exists: `ls -la /tmp/claude-mon-daemon.sock`
3. Test socket manually:
   ```bash
   echo '{"type":"edit","workspace":"/tmp/test","workspace_name":"test","tool_name":"Edit","file_path":"/tmp/test.go","old_string":"a","new_string":"b","line_num":0,"line_count":1}' | nc -U /tmp/claude-mon-daemon.sock
   ```

### jq not found

The hook silently skips daemon communication if `jq` is not installed:

```bash
which jq || echo "jq not installed!"
```

### TUI not receiving updates

The TUI socket is workspace-specific. Check the socket exists:

```bash
# Get the expected socket path
HASH=$(echo -n "$(pwd)" | sha256sum | cut -c1-12)
ls -la "/tmp/claude-mon-${USER}-${HASH}.sock"
```

## Socket Paths

| Socket | Purpose | Path |
|--------|---------|------|
| Daemon | Persistent storage | `/tmp/claude-mon-daemon.sock` |
| TUI | Real-time display | `/tmp/claude-mon-${USER}-${HASH}.sock` |

The TUI socket is unique per workspace (hashed from the directory path).

## Content Limits

To prevent huge payloads:
- `old_string`: Truncated to 10KB
- `new_string`: Truncated to 10KB

For larger edits, only the first 10KB is stored. The full file diff can still be viewed in the TUI by reading the actual file.

## Multiple Projects

Each project can have its own hook, or share a global hook. The workspace path is automatically detected, so the same hook works across all projects.

## Uninstalling

```bash
# Remove project hook
rm .claude/hooks/PostToolUse

# Remove global hook
rm ~/.claude/hooks/PostToolUse

# Stop daemon
claude-mon daemon stop
```
