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
- Search the selected session capture without switching panes

## Usage

```
make build   # build ./aeolus binary
make run     # go run .
```

| Key | Action |
|-----|--------|
| `j` / `↓` | Next session |
| `k` / `↑` | Previous session |
| `enter` | Switch tmux client to selected pane |
| `y` | Approve permission request |
| `a` | Approve permission request for the session |
| `n` | Deny permission request |
| `x` | Kill selected pane after confirmation |
| `r` | Force refresh |
| `/` | Edit a captured-output search |
| `ctrl+n` / `ctrl+p` | Next / previous matching line |
| `esc` | Cancel query editing or clear the search |
| `q` | Quit |

## Output search

Press `/`, type a literal query, and press `enter` to keep it. Search ignores case and terminal color escapes, includes the full captured output (currently 50 history lines plus the visible pane), and counts matching lines. The active line has a `>` marker. `ctrl+n` and `ctrl+p` wrap through matches without changing the selected session.

While editing, navigation and action keys become query text; `esc` restores the previous query and `ctrl+u` clears the input. Arrow keys, home/end, backspace, and delete edit Unicode text. While a query is kept, normal session shortcuts remain available. `esc` clears it and restores the regular preview and permission summary. Search uses an uncolored capture view and stays scoped to the selected pane across refreshes; changing or removing that pane clears it. Search reads the latest capture only, not unlimited tmux history.

## Requirements

- tmux
- go v1.26.4 or higher
- One or more `claude` sessions running inside tmux panes

## License

[MIT](LICENSE)
