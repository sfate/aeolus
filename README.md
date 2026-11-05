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
- Refreshes run one at a time; requests received during a refresh queue one follow-up
- Colored pane preview with live terminal output
- Search the selected session capture without switching panes
- Launch agents in a chosen directory and rename their tmux windows
- Review staged and unstaged Git changes for the selected agent directory

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
| `c` | Launch a new agent session |
| `R` | Rename the selected pane's window |
| `g` | Open read-only Git review for the selected pane directory |
| `x` | Kill selected pane after confirmation |
| `r` | Force refresh |
| `f` | Filter sessions by provider and status |
| `/` | Edit a captured-output search |
| `i` | Compose a prompt for the selected session |
| `ctrl+n` / `ctrl+p` | Next / previous matching line |
| `esc` | Cancel query editing or clear the search |
| `q` | Quit |

Approval and yes/no response keys run asynchronously. While a response is being delivered, repeated response keys are disabled. The request remains visible until delivery succeeds; failures retain it and show an error. A transport failure can occur after some keys reached the pane, so inspect the pane before retrying. Completion stays bound to the original pane and does not clear a newer request discovered during refresh.

## Session lifecycle

Press `c` to open the launch form. Use tab or shift+tab to choose Claude, Codex, or Gemini, edit the project directory, and press enter. The directory must exist and the selected agent must be on PATH. Absolute paths, relative paths, and `~/` paths are supported. Launch creates a uniquely named detached tmux session and selects it in the dashboard. It works outside tmux and can start a missing tmux server. Agent authentication and setup remain in the agent itself; open its pane to interact.

Press `R` to rename the window containing the selected pane. This affects all panes sharing that window. Names must contain 1 to 80 characters and cannot contain control characters or `|`. Both forms support cursor movement, home/end, backspace/delete, and ctrl+u. Esc cancels before submission; submitted operations stay locked until completion. Errors retain the form input for correction. Existing enter-to-pane navigation and confirmed pane termination remain available outside the forms.

## Git review

Press `g` to load changed paths and aggregate staged/unstaged diffs for the selected pane's current directory. The view stays bound to that pane and directory across dashboard refreshes. Use arrow keys, j/k, page up/down, home/end, or the mouse wheel to scroll; `r` reloads the snapshot and `esc` closes it. Other dashboard actions are disabled in the review. A missing pane directory or non-Git directory shows an error rather than reviewing the dashboard's own repository.

Review does not stage, edit, commit, fetch, or create worktrees. Git commands have a shared ten-second timeout and bounded output. Each diff is limited to 1 MiB; changed-path lists over 256 KiB produce an error. Large diffs display a truncation notice. Untracked paths are listed without reading their contents, binary changes use Git's summary, and submodule changes are omitted. External diff drivers, text conversion, clean filters, filesystem-monitor commands, and optional index writes are disabled. This produces a raw read-only snapshot; concurrent edits can change files between Git commands.

## Configuration

Aeolus reads `$XDG_CONFIG_HOME/aeolus/config.json`, or `~/.config/aeolus/config.json` when `XDG_CONFIG_HOME` is unset. A missing default file uses defaults. Set `XDG_CONFIG_HOME` to an absolute path when overriding it.

```json
{
  "version": 1,
  "refresh_interval": "3s",
  "default_provider": "codex",
  "projects": [
    {"name": "Application", "directory": "/absolute/path/to/application", "provider": "claude"}
  ]
}
```

Use `aeolus --config /path/to/config.json` to select another file. An explicitly selected file must exist. The version field is required and must be `1`; `refresh_interval` is optional and defaults to `2s`. Its allowed range is `100ms` through `1m`. Invalid files, duplicate or unknown fields, and unsupported versions produce an error before the TUI starts. Settings are read at startup. `default_provider` is optional and defaults to `claude`. It sets the initial agent when opening the launch form with `c`. Supported providers are `claude`, `codex`, and `gemini`. `projects` is an optional array of launch presets; each entry requires a unique, nonblank `name` (up to 80 characters), an absolute `directory`, and a supported `provider`. Names and paths cannot contain control characters. Relative paths and `~` are not expanded in configuration. Preset directories need not exist when loading settings; launch validates them. In the launch form, press `ctrl+p` to cycle project presets, then submit explicitly with Enter. Selecting a preset never starts an agent. Manual directory and provider edits remain available and mark the selection as Manual. Keybindings and saved dashboard preferences remain planned.

## Prompt composer

Press `i` to draft a prompt for the selected pane. `enter` adds a newline, `ctrl+s` sends the draft, and `esc` discards it. Arrow keys, home/end, backspace, and delete edit Unicode text. Session shortcuts become ordinary text while composing. Delivery runs asynchronously; repeat sends and edits are blocked until it finishes.

The draft stays bound to the original pane and provider across refreshes. If that target disappears or changes provider, sending is blocked. Errors retain the draft. A failure after pasting may leave text in the target, so inspect that pane before retrying. The composer sends explicit user text to any detected provider, including sessions with unknown status; it does not infer provider-specific approval actions.

Text is pasted through a uniquely named tmux buffer and followed by a separate Enter only after paste succeeds. Words such as `Enter` and `Escape` remain literal. Newlines and tabs are allowed; other terminal control characters are rejected by the transport. Bracketed paste is requested when supported by the receiving application; the target application controls how multiline input and the final Enter behave. The temporary buffer is removed after delivery attempts.

## Attach file context

Within the composer, press `ctrl+o`, enter a local file path, and press `enter` to attach it. Relative paths resolve against the original target pane's reported directory; if that directory is unavailable, enter an absolute path. No shell expansion is performed, so use the full home directory path instead of `~`. Press `esc` to return to the draft without attaching.

Attachments must be regular UTF-8 text files no larger than 64 KiB. Symlinks to regular files are supported. Invalid UTF-8, nonregular files, oversized input, and terminal controls other than tabs and newlines are rejected. CR line endings are normalized. A successful read inserts the quoted file path and literal contents into the draft at the cursor for review. Nothing is sent automatically. Read errors and cancellation retain the draft; sending is disabled while the attachment input is open.

## Output search

Press `/`, type a literal query, and press `enter` to keep it. Search ignores case and terminal color escapes, includes the full captured output (currently 50 history lines plus the visible pane), and counts matching lines. The active line has a `>` marker. `ctrl+n` and `ctrl+p` wrap through matches without changing the selected session.

While editing, navigation and action keys become query text; `esc` restores the previous query and `ctrl+u` clears the input. Arrow keys, home/end, backspace, and delete edit Unicode text. While a query is kept, normal session shortcuts remain available. `esc` clears it and restores the regular preview and permission summary. Search uses an uncolored capture view and stays scoped to the selected pane across refreshes; changing or removing that pane clears it. Search reads the latest capture only, not unlimited tmux history.

## Requirements

- tmux (pane metadata framing and working-directory capture tested with tmux 3.5a)
- go v1.26.4 or higher
- One or more `claude`, `codex`, or `gemini` sessions running inside tmux panes

## Provider detection

Detection uses executable names and known npm entry points in the pane process tree. It does not treat agent names in prompts, command arguments, or captured output as evidence of an agent. Custom wrapper names and process paths containing spaces may not be detected; use a standard CLI entry point. Codex and Gemini do not inherit Claude output parsing or approval keys. A detected agent with unreadable output remains listed with unknown activity.

## License

[MIT](LICENSE)

## Session filters

Press `f` to open session filters. Press `p` to cycle providers and `s` to cycle statuses, then `enter` to apply. Press `c` to clear both choices or `esc` to discard draft changes. The header shows active filters and visible/total session counts. Filtering keeps the selected pane when it remains visible and retains hidden sessions so clearing filters restores them. An empty result does not send actions to hidden panes. Output search remains scoped to the visible selected pane; typing `f` while editing a search query inserts text instead of opening filters. Press `tab` in the filter modal to edit a case-sensitive working-directory substring, then `tab` again to return to provider/status controls. While editing the directory, ordinary letters (including `p`, `s`, and `c`) enter text; `ctrl+u` clears it. Enter applies all choices and Escape discards the draft. An empty substring matches all directories; sessions with unknown directories do not match a nonempty substring. These paths are the pane working directory reported by tmux, not a Git repository root. The header shows the selected pane directory with control characters quoted.

Launching a session clears active provider/status filters so the new session is visible and selected.
