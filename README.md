# claude-follow-tui

A TUI application for watching Claude Code's file edits in real-time and managing prompts.

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

### UI Features
- **Two-pane layout**: List on left, content preview on right
- **Toast notifications**: Floating feedback for all actions
- **Mode switching**: Toggle between History and Prompts views

## Installation

```bash
# Clone the repo
git clone https://github.com/ztaylor/claude-follow-tui
cd claude-follow-tui

# Build
go build -o claude-follow-tui ./cmd/claude-follow-tui

# Or use make
make build
make install
```

## Usage

```bash
# Basic usage
claude-follow-tui

# With debug logging
claude-follow-tui -debug

# With persistent history
claude-follow-tui -persist

# Custom socket path
claude-follow-tui -socket /path/to/socket
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

## Storage Structure

```
~/.claude/prompts/                    # Global prompts
  ├── code-review.prompt.md
  ├── code-review.v1.prompt.md        # Version backup
  └── refactor-helper.prompt.md

.claude/prompts/                      # Project-local prompts
  ├── project-context.prompt.md
  └── test-generator.prompt.md
```

## Integration with Claude Code

Add to your Claude Code hooks (e.g., `follow-mode-notify.sh`):

```bash
#!/bin/bash
SOCKET_PATH="/tmp/claude-follow-${WORKSPACE_ID}.sock"

# Send to TUI if socket exists
if [[ -S "$SOCKET_PATH" ]]; then
    echo "$TOOL_INPUT" | nc -U "$SOCKET_PATH" &
fi
```

## Architecture

```
Claude PostToolUse hook
        │
        ▼
follow-mode-notify.sh
        │
        └──► Unix socket ──► claude-follow-tui (TUI)
                                    │
                                    ├── History View
                                    │   └── Diff with syntax highlighting
                                    │
                                    └── Prompts View
                                        ├── Prompt list (global + project)
                                        ├── Version management
                                        └── Claude CLI refinement
```

## Requirements

- Go 1.24+
- nvim (for editor integration)
- Claude CLI (optional, for prompt refinement)

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-debug` | `false` | Enable debug logging to `/tmp/claude-follow-tui.log` |
| `-persist` | `false` | Save history to disk |
| `-socket` | auto | Custom Unix socket path |

## License

MIT
