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
- Auto-refreshes every 2 seconds by default
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
| `enter` | Switch tmux client to selected pane |
| `y` | Approve permission request |
| `a` | Approve permission request for the session |
| `n` | Deny permission request |
| `x` | Kill selected pane after confirmation |
| `r` | Force refresh |
| `q` | Quit |

## Configuration

Aeolus reads `$XDG_CONFIG_HOME/aeolus/config.json`, or `~/.config/aeolus/config.json` when `XDG_CONFIG_HOME` is unset. A missing default file uses defaults. Set `XDG_CONFIG_HOME` to an absolute path when overriding it.

```json
{"version": 1, "refresh_interval": "3s"}
```

Use `aeolus --config /path/to/config.json` to select another file. An explicitly selected file must exist. The version field is required and must be `1`; `refresh_interval` is optional and defaults to `2s`. Its allowed range is `100ms` through `1m`. Invalid files, duplicate or unknown fields, and unsupported versions produce an error before the TUI starts. Settings are read at startup. This first configuration slice supports the polling interval only.

## Requirements

- tmux
- go v1.26.4 or higher
- One or more `claude` sessions running inside tmux panes

## License

[MIT](LICENSE)
