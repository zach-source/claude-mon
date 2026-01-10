# claude-follow-tui

A TUI application that displays Claude Code's file edits in real-time, similar to lazygit.

![Screenshot placeholder]

## Features

- **Real-time updates**: Watch Claude's edits as they happen
- **Diff view**: See exactly what changed with syntax highlighting
- **History navigation**: Cycle through previous changes
- **Editor integration**: Press `Ctrl+G` to open file in nvim at the exact line
- **Workspace-aware**: Uses same workspace detection as claude-follow-hook.nvim

## Installation

```bash
# Clone the repo
git clone https://github.com/ztaylor/claude-follow-tui
cd claude-follow-tui

# Build and install
make install

# Or just build
make build
./bin/claude-follow-tui
```

## Usage

1. Run the TUI in a tmux pane or terminal:
   ```bash
   claude-follow-tui
   ```

2. In another terminal, use Claude Code in the same directory

3. Watch edits appear in real-time

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Next item in history |
| `k` / `↑` | Previous item |
| `Tab` | Switch between panes |
| `Ctrl+G` | Open file in nvim at line |
| `Ctrl+O` | Open file in nvim |
| `c` | Clear history |
| `q` | Quit |
| `?` | Show help |

## Integration with Claude Code

Add to your `follow-mode-notify.sh` hook:

```bash
# Send to TUI if running
if command -v claude-follow-tui >/dev/null 2>&1; then
    cat "$TEMP_JSON" | claude-follow-tui send &
fi
```

## Architecture

```
Claude PostToolUse hook
        │
        ▼
follow-mode-notify.sh
        │
        └──► claude-follow-tui send
                    │
                    ▼
            Unix socket
                    │
                    ▼
            claude-follow-tui (TUI)
```

## Requirements

- Go 1.24+
- nvim (for editor integration)

## License

MIT
