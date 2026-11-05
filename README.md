# Aeolus

In Greek mythology, **Aeolus** is the keeper of the winds - he holds them in a cave and releases or restrains them at will.

<img src='images/aeolus.png' width='800' alt='aeolus' />

*Clouds don't move themselves.*

Aeolus is a TUI dashboard for managing [Claude Code](https://claude.ai/code), [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), and [Gemini CLI](https://geminicli.com/docs/get-started/installation/) sessions running across tmux panes. It shows provider identity and live output, and lets you approve or deny Claude permission requests without switching panes.

## Features

- Lists tmux panes running Claude Code, Codex CLI, or Gemini CLI (other panes are hidden)
- Shows Claude session status: idle, working, waiting for permission, or waiting for an answer
- Approve / deny Claude requests with a single keypress
- Shows Codex and Gemini activity as unknown; use `enter` to inspect and respond in the pane
- Auto-refreshes every 2 seconds by default
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
| `y` | Approve Claude permission request / answer yes |
| `a` | Approve Claude permission request for the session |
| `n` | Deny Claude permission request / answer no |
| `x` | Kill selected pane after confirmation |
| `r` | Force refresh |
| `/` | Edit a captured-output search |
| `i` | Compose a prompt for the selected session |
| `ctrl+n` / `ctrl+p` | Next / previous matching line |
| `esc` | Cancel query editing or clear the search |
| `q` | Quit |

## Configuration

Aeolus reads `$XDG_CONFIG_HOME/aeolus/config.json`, or `~/.config/aeolus/config.json` when `XDG_CONFIG_HOME` is unset. A missing default file uses defaults. Set `XDG_CONFIG_HOME` to an absolute path when overriding it.

```json
{"version": 1, "refresh_interval": "3s"}
```

Use `aeolus --config /path/to/config.json` to select another file. An explicitly selected file must exist. The version field is required and must be `1`; `refresh_interval` is optional and defaults to `2s`. Its allowed range is `100ms` through `1m`. Invalid files, duplicate or unknown fields, and unsupported versions produce an error before the TUI starts. Settings are read at startup. This first configuration slice supports the polling interval only.

## Prompt composer

Press `i` to draft a prompt for the selected pane. `enter` adds a newline, `ctrl+s` sends the draft, and `esc` discards it. Arrow keys, home/end, backspace, and delete edit Unicode text. Session shortcuts become ordinary text while composing. Delivery runs asynchronously; repeat sends and edits are blocked until it finishes.

The draft stays bound to the original pane and provider across refreshes. If that target disappears or changes provider, sending is blocked. Errors retain the draft. A failure after pasting may leave text in the target, so inspect that pane before retrying. The composer sends explicit user text to any detected provider, including sessions with unknown status; it does not infer provider-specific approval actions.

Text is pasted through a uniquely named tmux buffer and followed by a separate Enter only after paste succeeds. Words such as `Enter` and `Escape` remain literal. Newlines and tabs are allowed; other terminal control characters are rejected by the transport. Bracketed paste is requested when supported by the receiving application; the target application controls how multiline input and the final Enter behave. The temporary buffer is removed after delivery attempts.

## Output search

Press `/`, type a literal query, and press `enter` to keep it. Search ignores case and terminal color escapes, includes the full captured output (currently 50 history lines plus the visible pane), and counts matching lines. The active line has a `>` marker. `ctrl+n` and `ctrl+p` wrap through matches without changing the selected session.

While editing, navigation and action keys become query text; `esc` restores the previous query and `ctrl+u` clears the input. Arrow keys, home/end, backspace, and delete edit Unicode text. While a query is kept, normal session shortcuts remain available. `esc` clears it and restores the regular preview and permission summary. Search uses an uncolored capture view and stays scoped to the selected pane across refreshes; changing or removing that pane clears it. Search reads the latest capture only, not unlimited tmux history.

## Requirements

- tmux
- go v1.26.4 or higher
- One or more `claude`, `codex`, or `gemini` sessions running inside tmux panes

## Provider detection

Detection uses executable names and known npm entry points in the pane process tree. It does not treat agent names in prompts, command arguments, or captured output as evidence of an agent. Custom wrapper names and process paths containing spaces may not be detected; use a standard CLI entry point. Codex and Gemini do not inherit Claude output parsing or approval keys. A detected agent with unreadable output remains listed with unknown activity.

## License

[MIT](LICENSE)
