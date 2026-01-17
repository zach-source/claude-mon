# claude-mon (clmon)

A TUI application and daemon for watching Claude Code's file edits in real-time, managing prompts, and querying edit history.

## Features

### Real-time Edit Tracking
- **Live updates**: Watch Claude's edits as they happen via Unix socket
- **Word-level diffs**: See exactly what changed with inline highlighting
- **Syntax highlighting**: Code displayed with proper syntax colors
- **History navigation**: Browse through previous changes
- **Persistent history**: Optionally save history across sessions
- **Editor integration**: Jump to exact line in nvim

### Prompt Manager
- **Prompt storage**: Store prompts as `.prompt.md` files with YAML frontmatter
- **Dual locations**: Global (`~/.claude/prompts/`) and per-project (`.claude/prompts/`)
- **Template variables**: Use `{{file}}`, `{{project}}`, `{{plan}}`, etc. in prompts
- **Auto-versioning**: Automatic backup created before every edit
- **Version management**: View, restore, or delete version backups
- **Claude refinement**: Use Claude CLI to improve prompts with diff review
- **Multiple injection methods**: Send prompts via tmux, OSC52, or clipboard

### Working Context
- **Project-specific context**: Each project has its own isolated working context
- **Kubernetes integration**: Set context, namespace, and kubeconfig path
- **AWS profiles**: Store profile and region for quick reference
- **Git awareness**: Auto-detects branch and repository
- **Environment variables**: Store project-specific env vars
- **Custom values**: Add arbitrary key-value pairs
- **Stale warnings**: Alerts when context is older than 24 hours
- **Automatic injection**: Inject context into Claude prompts via hooks
- **TUI management**: Full UI for viewing, editing, and managing context

### Task Automation
- **Ralph Loop**: Monitor and control iterative Claude loops with promise tracking
- **And-Then Queue**: Sequential task queue that auto-advances when tasks complete
- **Queue management**: Cancel, skip, or monitor task progress in real-time
- **State persistence**: YAML-based state files for reliable resumption

### UI Features
- **Two-pane layout**: List on left, content preview on right
- **Toast notifications**: Floating feedback for all actions
- **Mode switching**: Toggle between History, Prompts, Ralph, Plan, and Context views
- **Auto-refresh**: Ralph page auto-refreshes every 5 seconds to track loop progress
- **Status indicators**: Real-time daemon and socket connection status in status bar

### Daemon & Data Management
- **Background daemon**: Tracks all edits from any Claude session
- **Persistent storage**: SQLite database with WAL mode for reliability
- **Query interface**: Search edits by file, session, or recency
- **Heartbeat status**: Real-time connection and workspace activity tracking
- **Automated cleanup**: Configurable data retention and vacuum
- **Backup system**: Periodic compressed backups
- **Workspace filtering**: Track or ignore specific paths
- **Comprehensive configuration**: TOML-based config with env var overrides

## Installation

```bash
# Clone the repo
git clone https://github.com/ztaylor/claude-mon
cd claude-mon

# Build
go build -o claude-mon ./cmd/claude-mon

# Or use make
make build
make install
```

## Usage

### TUI Mode

```bash
# Basic usage (both commands work the same)
claude-mon
clmon

# With debug logging
claude-mon -debug
clmon -debug

# With persistent history
claude-mon -persist

# Custom socket path
claude-mon -socket /path/to/socket
```

### Daemon Mode

The daemon runs in the background, tracking all Claude edits to a persistent database:

```bash
# Start the daemon
claude-mon daemon start

# Stop the daemon
claude-mon daemon stop

# Check daemon status
claude-mon daemon status

# Start with custom config
claude-mon daemon start --config /path/to/config.toml
```

### Querying Edit History

Query the daemon for edit history:

```bash
# Show recent activity
claude-mon query recent

# Show recent activity with limit
claude-mon query recent 100

# Show edits for a specific file
claude-mon query file /path/to/file.go

# List all prompts
claude-mon query prompts

# List all sessions
claude-mon query sessions
```

### Configuration

Generate a default configuration file:

```bash
# Write to default location (~/.config/claude-mon/daemon.toml)
claude-mon write-config

# Write to custom path
claude-mon write-config /path/to/config.toml
```

**Configuration priority:** CLI flags > Config file > Environment variables > Defaults

**Environment variable overrides:**

```bash
export CLAUDE_MON_DATA_DIR=/custom/path
claude-mon daemon start
```

## Keybindings

### Global
| Key | Action |
|-----|--------|
| `o` | Toggle between History and Prompts mode |
| `Tab` | Switch between left and right panes |
| `q` / `Ctrl+C` | Quit |
| `?` | Show help |

### History Mode
| Key | Action |
|-----|--------|
| `j` / `↓` | Next change |
| `k` / `↑` | Previous change |
| `h` / `←` | Scroll diff left |
| `l` / `→` | Scroll diff right |
| `Ctrl+G` | Open file in nvim at exact line |
| `Ctrl+O` | Open file in nvim |
| `c` | Clear history |

### Prompts Mode
| Key | Action |
|-----|--------|
| `j` / `↓` | Next prompt |
| `k` / `↑` | Previous prompt |
| `n` | Create new prompt |
| `e` | Edit prompt (auto-creates version backup) |
| `r` | Refine prompt with Claude CLI |
| `v` | Create version backup manually |
| `V` | View version history |
| `Enter` | Inject prompt (using current method) |
| `y` | Copy prompt to clipboard |
| `i` | Cycle injection method (tmux/OSC52/clipboard) |
| `Ctrl+D` | Delete prompt |

### Ralph Mode
| Key | Action |
|-----|--------|
| `r` | Manual refresh |
| `C` | Cancel Ralph loop |
| `Q` | Cancel And-Then queue |
| `s` | Skip current And-Then task |
| `R` | Open Ralph chat |
| **Auto-refresh** | State refreshes every 5 seconds automatically |

**Display Features:**
- Ralph Loop: Shows iteration progress (e.g., "3/10"), promise, and elapsed time
- And-Then Queue: Shows task progress (e.g., "2/5"), current task, and "done when" criteria
- State path: Shows which state file is active

### Context Mode
| Key | Action |
|-----|--------|
| `k` | Set Kubernetes context (context [namespace] [--kubeconfig path]) |
| `a` | Set AWS profile (profile [region]) |
| `g` | Set Git context ([branch] [repo], auto-detects if empty) |
| `e` | Set environment variable (KEY=VALUE [KEY2=VALUE2...]) |
| `c` | Set custom value (KEY=VALUE [KEY2=VALUE2...]) |
| `C` | Clear all context or specific section |
| `r` | Reload context from disk |
| `l` | List all project contexts in right pane |
| `Enter` | Save edited value |
| `Esc` | Cancel editing |

### Version View Mode
| Key | Action |
|-----|--------|
| `j` / `↓` | Next version |
| `k` / `↑` | Previous version |
| `Enter` | Restore selected version |
| `Ctrl+D` | Delete version backup |
| `Esc` | Exit version view |

## Prompt File Format

Prompts are stored as Markdown files with YAML frontmatter:

```markdown
---
name: Code Review Helper
description: Reviews code for best practices
version: 1
created: 2026-01-11
updated: 2026-01-11
tags: [review, quality]
---

You are a code review assistant. Analyze the following code for:
- Security vulnerabilities
- Performance issues
- Code style violations

Be concise and actionable in your feedback.
```

## Template Variables

Prompts support template variables that are expanded when injected:

| Variable | Description |
|----------|-------------|
| `{{file}}` | Full path of currently selected file |
| `{{file_name}}` | Name of currently selected file |
| `{{project}}` | Current project/directory name |
| `{{cwd}}` | Current working directory |
| `{{plan}}` | Content of current plan file |
| `{{plan_name}}` | Name of plan file |

### Example

```markdown
---
name: Review Current File
---

Review the file {{file_name}} in project {{project}}.

Focus on:
- Code quality
- Potential bugs
- Performance issues

Current plan context:
{{plan}}
```

## Configuration

The daemon uses a comprehensive TOML configuration file at `~/.config/claude-mon/daemon.toml`.

### Configuration Sections

```toml
[directory]
data_dir = "~/.claude-mon"              # Base directory for data

[database]
path = "claude-mon.db"                  # Database filename
max_db_size_mb = 500                    # Trigger cleanup when exceeded
wal_checkpoint_pages = 1000             # WAL checkpoint threshold

[sockets]
daemon_socket = "/tmp/claude-mon-daemon.sock"
query_socket = "/tmp/claude-mon-query.sock"
buffer_size = 8192                       # Socket buffer size

[query]
default_limit = 50                       # Default query result limit
max_limit = 1000                         # Maximum allowed limit
timeout_seconds = 30                     # Query timeout

[retention]
retention_days = 90                      # Auto-delete records older than N days
max_edits_per_session = 10000           # Cap per session
cleanup_interval_hours = 24             # How often to cleanup
auto_vacuum = true                       # Reclaim disk space

[backup]
enabled = true
path = "backups"                         # Relative to data_dir
interval_hours = 24                      # Backup interval
retention_days = 30                      # Keep backups for N days
format = "sqlite"                        # "sqlite" or "export"

[workspaces]
tracked = []                             # Empty = track all
ignored = ["/tmp", "/var/tmp"]           # Blacklist

[hooks]
timeout_seconds = 30                     # Socket read timeout
retry_attempts = 3                       # Retry on failure
async_mode = false                       # Fire-and-forget mode

[logging]
path = "claude-mon.log"                  # Relative to data_dir
level = "info"                           # debug, info, warn, error
max_size_mb = 100                        # Rotation threshold
max_backups = 3                          # Old logs to keep
compress = true                          # Gzip rotation

[performance]
max_connections = 50
pool_size = 10
cache_enabled = true
cache_ttl_seconds = 300
```

### Generating Default Config

```bash
claude-mon write-config
```

This creates `~/.config/claude-mon/daemon.toml` with all default values and comments.

## Storage Structure

```
~/.claude/prompts/                    # Global prompts
  ├── code-review.prompt.md
  ├── code-review.v1.prompt.md        # Version backup
  └── refactor-helper.prompt.md

.claude/prompts/                      # Project-local prompts
  ├── project-context.prompt.md
  └── test-generator.prompt.md

~/.claude/contexts/                   # Working context (per-project)
  ├── claude-mon-a1b2c3d4e5f6.json   # Context for claude-mon project
  └── myproject-123456789012.json    # Context for myproject
```

## Integration with Claude Code

Add to your Claude Code hooks (e.g., `claude-mon-notify.sh`):

```bash
#!/bin/bash
SOCKET_PATH="/tmp/claude-mon-${WORKSPACE_ID}.sock"

# Send to TUI if socket exists
if [[ -S "$SOCKET_PATH" ]]; then
    echo "$TOOL_INPUT" | nc -U "$SOCKET_PATH" &
fi
```

### Context Injection Hook

To automatically inject working context into your Claude prompts, add a `UserPromptSubmit` hook:

```bash
#!/bin/bash
# ~/.claude/hooks/inject-context.sh
# Context injection hook for Claude Code

# Ensure inject-context is installed
if ! command -v inject-context &> /dev/null; then
    echo "Warning: inject-context not found in PATH" >&2
    # Continue without injection
    echo '{"continue": true}'
    exit 0
fi

# Pass stdin to inject-context and output result
inject-context
```

Then in your Claude settings (`~/.config/claude/settings.json`):

```json
{
  "hooks": {
    "UserPromptSubmit": "~/.claude/hooks/inject-context.sh"
  }
}
```

This will automatically inject your project's working context as a `<working-context>` block at the start of each conversation, unless context is already present in the prompt.

**Context Block Format:**
```
<working-context>
  Kubernetes: orbstack / default
  AWS Profile: dev (us-west-2)
  Git: main @ my-repo
  Env: ENV=dev, DEBUG=true
  Updated: 2h ago
</working-context>
```

## Architecture

```
Claude PostToolUse hook
        │
        ▼
claude-mon-notify.sh
        │
        ├──► Unix socket ──► claude-mon daemon ──► SQLite Database
        │                      │                      │
        │                      ├── Cleanup Manager    │
        │                      ├── Backup Manager     │
        │                      ├── Query Interface    │
        │                      └── Status/Heartbeat   │
        │                           └── Workspace activity tracking
        │
        └──► Unix socket ──► claude-mon (TUI)
                                    │
                                    ├── History View
                                    │   └── Diff with syntax highlighting
                                    │
                                    ├── Prompts View
                                    │   ├── Prompt list (global + project)
                                    │   └── Version management
                                    │
                                    ├── Ralph View
                                    │   ├── Ralph Loop status monitoring
                                    │   └── And-Then Queue management
                                    │
                                    ├── Plan View
                                    │   └── Plan generation
                                    │
                                    ├── Context View
                                    │   ├── Project context display
                                    │   └── Kubernetes/AWS/Git/Env management
                                    │
                                    └── Status Bar
                                        ├── D● Daemon connection indicator
                                        └── S● Socket connection indicator
```

**Data Flow:**

1. **Edit Tracking:** Claude hooks → Unix socket → Daemon → SQLite
2. **Cleanup:** Background goroutine → Delete old records → Vacuum database
3. **Backup:** Background goroutine → Copy database → Gzip compression
4. **Querying:** CLI query → Unix socket → Daemon → SQL query → Results
5. **TUI Display:** TUI connects to socket → Real-time updates

## Requirements

- Go 1.24+
- nvim (for editor integration)
- Claude CLI (optional, for prompt refinement)

## Flags

### TUI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--theme, -t` | `dark` | Color theme (dark, light, dracula, monokai, gruvbox, nord, catppuccin) |
| `--list-themes` | - | List available themes |
| `--persist, -p` | `false` | Save history to `.claude-mon-history.json` |
| `--debug, -d` | `false` | Enable debug logging |
| `--config` | `~/.config/claude-mon/daemon.toml` | Path to daemon config file |

### Daemon Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Path to custom configuration file |

### Query Flags

| Flag | Description |
|------|-------------|
| `recent [limit]` | Show recent edits (default limit from config) |
| `file <path> [limit]` | Show edits for specific file |
| `prompts [name] [limit]` | List all prompts or filter by name |
| `sessions [limit]` | List all sessions |

## Recent Enhancements

- ✅ **And-Then Queue** sequential task automation with auto-advance
- ✅ **Daemon heartbeat** real-time connection and workspace activity tracking
- ✅ **Status bar indicators** showing daemon (D●) and socket (S●) connection state
- ✅ **Working context management** with per-project context storage
- ✅ **Context injection hook** for automatic prompt enhancement
- ✅ **Five-tab layout**: History, Prompts, Ralph, Plan, and Context modes
- ✅ **Ralph Loop integration** with auto-refresh every 5 seconds
- ✅ **Comprehensive configuration system** with TOML support
- ✅ **Automated data retention** with configurable cleanup policies
- ✅ **Backup system** with periodic snapshots and compression
- ✅ **Workspace filtering** to track or ignore specific paths
- ✅ **Query interface** for searching edit history
- ✅ **Environment variable overrides** for all settings
- ✅ **100% E2E test coverage** of configuration system

## License

MIT
