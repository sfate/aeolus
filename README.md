# Aeolus

In Greek mythology, **Aeolus** is the keeper of the winds — he holds them in a cave and releases or restrains them at will.

<img src='images/aeolus.png' width='800' alt='aeolus' />

*Clouds don't move themselves.*

Aeolus is a terminal dashboard for managing multiple [Claude Code](https://claude.ai/code) sessions running across tmux panes.
It watches all your sessions, shows what each one is doing, and lets you approve or deny permission requests without switching panes.

## Features

- Lists all tmux panes running Claude Code (others are hidden)
- Shows session status: idle, working, or waiting for permission
- Approve / deny permission requests with a single keypress
- Auto-refreshes every 2 seconds
- Colored pane preview with live terminal output

## Usage

```
make build   # build ./aeolus binary
make run     # go run .
```

| Key | Action |
|-----|--------|
| `j` / `↓` | Next session |
| `k` / `↑` | Previous session |
| `y` | Approve permission request |
| `n` | Deny permission request |
| `r` | Force refresh |
| `q` | Quit |

## Requirements

- tmux
- go v1.26.3 or higher
- One or more `claude` sessions running inside tmux panes
