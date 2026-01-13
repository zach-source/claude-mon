# claude-mon Daemon

The claude-mon daemon provides persistent storage and querying capabilities for all Claude Code activity across multiple sessions.

## Architecture

```
┌─────────────────┐      ┌──────────────────┐
│  Claude Hook    │─────►│                  │
│ (PostToolUse)   │      │   claude-mon     │
└─────────────────┘      │     Daemon       │
                          │                  │
┌─────────────────┐      │  ┌────────────┐  │
│  Another Hook   │─────►│  │   SQLite   │  │
│  (2nd Session)  │      │  │   Database │  │
└─────────────────┘      │  └────────────┘  │
                          │                  │
┌─────────────────┐      │  ┌────────────┐  │
│  claude-mon     │◄─────┘  │   Query    │  │
│  query cli     │           └────────────┘  │
└─────────────────┘                      └──────────────────┘
```

## Features

- **Multi-session tracking**: Track activity from multiple Claude Code sessions simultaneously
- **Persistent storage**: All edits, prompts, and sessions stored in SQLite database
- **Query interface**: Query activity by file, time, or type
- **Unix socket IPC**: Fast communication via Unix domain sockets
- **WAL mode**: Write-Ahead Logging for concurrent access

## Database Schema

### Tables

- **sessions**: Tracks Claude Code workspaces with branches and commits
- **edits**: Records all file edits with line numbers and timestamps
- **prompts**: Stores prompt templates with version history
- **prompt_versions**: Version history for prompts
- **hooks**: Raw hook events for debugging

### Views

- **recent_activity**: Pre-computed view of recent edits across sessions

## Usage

### Starting the Daemon

```bash
# Start in foreground (for testing)
claude-mon daemon start

# Or run in background
claude-mon daemon start &
```

The daemon creates two Unix sockets:
- `/tmp/claude-mon-daemon.sock` - Data ingestion from hooks
- `/tmp/claude-mon-query.sock` - Query interface for CLI

### Checking Status

```bash
claude-mon daemon status
# Output: Daemon: running (or not running)
```

### Stopping the Daemon

```bash
claude-mon daemon stop
```

### Querying the Database

#### Recent Activity

```bash
# Show last 50 edits
claude-mon query recent

# Show last 100 edits
claude-mon query recent 100
```

#### File History

```bash
# Show edits for specific file
claude-mon query file /path/to/file.go

# Show last 20 edits for file
claude-mon query file /path/to/file.go 20
```

#### Prompts

```bash
# List all prompts
claude-mon query prompts

# Search prompts by name
claude-mon query prompts "test"

# Limit results
claude-mon query prompts "test" 10
```

#### Sessions

```bash
# List all active sessions
claude-mon query sessions

# Limit results
claude-mon query sessions 20
```

## Integration with Claude Code

### Hook Setup

Add the daemon hook to your Claude Code hooks:

**`.claude/hooks/PostToolUse`:**
```bash
#!/bin/bash
# PostToolUse hook for claude-mon daemon

SCRIPT_DIR="/path/to/claude-mon/scripts/hooks"

"${SCRIPT_DIR}/claude-mon-daemon-hook.sh" edit "Edit" "$TOOL_INPUT"
```

### Environment Variables

- `CLAUDE_MON_DAEMON_SOCKET`: Path to daemon socket (default: `/tmp/claude-mon-daemon.sock`)
- `WORKSPACE_PATH`: Workspace directory path
- `WORKSPACE_NAME`: Project name (default: basename of path)

### Hook Payload Format

The daemon accepts JSON payloads with the following structure:

**Edit Event:**
```json
{
  "type": "edit",
  "workspace": "/path/to/workspace",
  "workspace_name": "my-project",
  "branch": "main",
  "commit_sha": "abc123",
  "tool_name": "Edit",
  "file_path": "/path/to/file.go",
  "old_string": "old code",
  "new_string": "new code",
  "line_num": 42,
  "line_count": 5
}
```

**Prompt Event:**
```json
{
  "type": "prompt",
  "workspace": "/path/to/workspace",
  "workspace_name": "my-project",
  "branch": "main",
  "commit_sha": "abc123",
  "prompt_name": "Code Review",
  "prompt_description": "Review code for best practices",
  "new_string": "Prompt content here...",
  "prompt_tags": ["review", "quality"]
}
```

## Database Location

The SQLite database is stored at:
```
~/.claude-mon/claude-mon.db
```

WAL files (for concurrent access):
```
~/.claude-mon/claude-mon.db-wal
~/.claude-mon/claude-mon.db-shm
```

## Service Integration

### macOS (launchd)

Create `~/Library/LaunchAgents/com.zachsource.claude-mon.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.zachsource.claude-mon</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/claude-mon</string>
        <string>daemon</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/claude-mon-daemon.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/claude-mon-daemon-error.log</string>
</dict>
</plist>
```

Load the service:
```bash
launchctl load ~/Library/LaunchAgents/com.zachsource.claude-mon.plist
launchctl start com.zachsource.claude-mon
```

### Linux (systemd)

Create `/etc/systemd/user/claude-mon.service`:

```ini
[Unit]
Description=Claude Mon Daemon
After=network.target

[Service]
ExecStart=/usr/local/bin/claude-mon daemon start
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

Enable and start:
```bash
systemctl --user daemon-reload
systemctl --user enable claude-mon
systemctl --user start claude-mon
```

## Performance

- **WAL mode**: Enables concurrent reads/writes
- **Indexed queries**: Fast lookups on file_path, timestamp, session_id
- **Connection pooling**: Daemon handles multiple simultaneous connections
- **Async writes**: Non-blocking hook execution

## Troubleshooting

### Daemon won't start

```bash
# Check if socket already exists
ls -l /tmp/claude-mon-daemon.sock

# Remove stale socket
rm -f /tmp/claude-mon-daemon.sock
```

### Database locked

```bash
# Check for WAL files
ls -l ~/.claude-mon/

# Close all connections and restart daemon
claude-mon daemon stop
claude-mon daemon start
```

### Queries return no results

```bash
# Check daemon is running
claude-mon daemon status

# Verify database has data
sqlite3 ~/.claude-mon/claude-mon.db "SELECT COUNT(*) FROM edits;"
```

## Advanced Usage

### Direct SQL Access

```bash
sqlite3 ~/.claude-mon/claude-mon.db

# Complex queries
SELECT s.workspace_name, COUNT(e.id) as edit_count
FROM sessions s
LEFT JOIN edits e ON s.id = e.session_id
GROUP BY s.id
ORDER BY edit_count DESC
LIMIT 10;
```

### Export Data

```bash
# Export edits to JSON
sqlite3 ~/.claude-mon/claude-mon.db <<EOF
.mode json
.output edits.json
SELECT * FROM edits;
EOF
```

### Backup Database

```bash
# Simple backup
cp ~/.claude-mon/claude-mon.db ~/.claude-mon/backup-$(date +%Y%m%d).db

# Or use SQLite backup command
sqlite3 ~/.claude-mon/claude-mon.db ".backup ~/.claude-mon/backup.db"
```

## Examples

### Track activity across multiple projects

```bash
# Start daemon once
claude-mon daemon start &

# Work in project A
cd ~/project-a
# Claude makes edits...

# Work in project B (simultaneously)
cd ~/project-b
# Claude makes edits...

# Query all activity
claude-mon query recent
```

### Find all edits to a specific file today

```bash
# Get file history
claude-mon query file main.go

# Or use SQL for date filtering
sqlite3 ~/.claude-mon/claude-mon.db <<EOF
SELECT * FROM edits
WHERE file_path = '%main.go'
  AND date(timestamp) = date('now');
EOF
```

### Compare activity between branches

```bash
# View all sessions
claude-mon query sessions

# Count edits per branch
sqlite3 ~/.claude-mon/claude-mon.db <<EOF
SELECT s.branch, COUNT(*) as edits
FROM sessions s
JOIN edits e ON s.id = e.session_id
GROUP BY s.branch;
EOF
```

## API Reference

### Hook Script Functions

**`edit <tool_name> <tool_input_json>`**
- Records a file edit to the database
- Auto-detects file path, line numbers from tool input
- Returns: exit code 0 (success), 1 (error)

**`prompt <name> <description> <content> <tags>`**
- Records a prompt to the database
- Creates version history automatically
- Returns: exit code 0 (success), 1 (error)

### Query Types

**`recent [limit]`**
- Get recent edits across all sessions
- Default limit: 50
- Sort: timestamp DESC

**`file <path> [limit]`**
- Get edits for specific file
- Default limit: 50
- Sort: timestamp DESC

**`prompts [name_pattern] [limit]`**
- Search prompts by name (optional)
- Default limit: 50
- Sort: updated_at DESC

**`sessions [limit]`**
- List all active sessions
- Default limit: 50
- Sort: last_activity DESC
