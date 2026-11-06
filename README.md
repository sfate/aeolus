# Aeolus

In Greek mythology, **Aeolus** is the keeper of the winds - he holds them in a cave and releases or restrains them at will.

<img src='images/aeolus.png' width='800' alt='aeolus' />

*Clouds don't move themselves.*

Aeolus is a TUI dashboard for managing [Claude Code](https://claude.ai/code), [Codex CLI](https://learn.chatgpt.com/docs/codex/cli), [Gemini CLI](https://geminicli.com/docs/get-started/installation/), and explicitly managed shell sessions across tmux panes. It shows provider identity and live output, and lets you approve or deny Claude permission requests without switching panes.

## Features

- Lists tmux panes running Claude Code, Codex CLI, or Gemini CLI, plus shells launched through Aeolus (other panes are hidden)
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
| `W` | Create a new Git worktree and launch an agent |
| `x` | Kill selected pane after confirmation |
| `r` | Force refresh |
| `f` | Filter sessions by provider and status |
| `/` | Edit a captured-output search |
| `i` | Compose a prompt for the selected session |
| `ctrl+n` / `ctrl+p` | Next / previous matching line |
| `esc` | Cancel query editing or clear the search |
| `!` | Acknowledge the selected session's unread attention |
| `q` | Quit |
| `?` | Open contextual keyboard help |
| `ctrl+k` | Search dashboard actions in the command palette |

## Keyboard help

Press `?` from the dashboard to open scrollable keyboard help. The first section shows the selected provider and status, with approval keys only when supported by the current Claude request. It updates as sessions refresh. Other sections cover dashboard navigation, composer and attachment editing, session forms, filters, search, and Git review.

Use arrows, j/k, page up/down, or Home/End to scroll and Escape to return to the dashboard. Session actions are disabled while help is open; Ctrl+C quits. In text inputs, `?` stays ordinary text. Close other modals before opening help.

Approval and yes/no response keys run asynchronously. While a response is being delivered, repeated response keys are disabled. The request remains visible until delivery succeeds; failures retain it and show an error. A transport failure can occur after some keys reached the pane, so inspect the pane before retrying. Completion stays bound to the original pane and does not clear a newer request discovered during refresh.

## Command palette

Press `ctrl+k` from the dashboard to search available actions. Type a case-insensitive substring, select with up/down or ctrl+p/ctrl+n, and press Enter to execute. Escape cancels. The palette uses the same handlers as dashboard shortcuts, so choosing Kill pane opens the existing confirmation before termination. Launch, rename, and composer actions appear only when the backend supports them; approval actions require a supported request from the original selected provider. If that pane disappears or changes provider, its actions become unavailable until you close and reopen the palette. Background refresh does not retarget a pending selection to another session.

Edit the query with left/right, Home/End or ctrl+a/ctrl+e, Backspace/Delete, and ctrl+u to clear. Ordinary letters, including dashboard shortcuts, stay query text. Open the palette after closing other modals; Ctrl+C quits from the palette.

## Attention indicators

Observed Claude transitions into permission or question states, and from working to idle, add an unread `!` marker to the session. The header counts unread sessions, including those hidden by filters. Select a session and press `!` to acknowledge its latest event. Visiting a pane, responding, or later activity does not acknowledge it automatically. Composer and text-entry forms keep `!` as ordinary input.

Initial snapshots and newly discovered panes establish a baseline without alerts. Removed panes lose their badges; disappearance is not interpreted as task completion. Unknown activity and unsupported providers do not produce inferred completion events. Idle means the existing detector observed an idle state, not that a task succeeded. Indicators are local to the running dashboard and are not persisted.

## Terminal layout

At 80 columns and wider, sessions and captured output appear side by side. Narrower terminals stack a short session list above the output; navigation keeps the selected session visible. Composer and filter forms use the available width. The supported minimum is 24 columns by 12 rows; smaller terminals show a resize message. Labels and clipped lines use terminal cell widths, preserving Unicode characters and ANSI color sequences. Long composer drafts wrap visually, while path and search inputs keep the insertion cursor visible. Resizing preserves session selection and adjusts scrollable viewports.

## Session lifecycle

Press `c` to open the launch form. Use tab or shift+tab to choose Claude, Codex, or Gemini, edit the project directory, and press enter. The directory must exist and the selected agent must be on PATH. Absolute paths, relative paths, and `~/` paths are supported. Launch creates a uniquely named detached tmux session and selects it in the dashboard. It works outside tmux and can start a missing tmux server. Agent authentication and setup remain in the agent itself; open its pane to interact.

Press `R` to rename the window containing the selected pane. This affects all panes sharing that window. Names must contain 1 to 80 characters and cannot contain control characters or `|`. Both forms support cursor movement, home/end, backspace/delete, and ctrl+u. Esc cancels before submission; submitted operations stay locked until completion. Errors retain the form input for correction. Existing enter-to-pane navigation and confirmed pane termination remain available outside the forms.

## Managed shell sessions

Press `c`, use tab or shift+tab to select **Shell (zsh)**, choose a directory, and press enter. Aeolus launches an interactive zsh in its own tmux session and marks that pane explicitly. Unrelated shell panes remain hidden. The shell appears in the session list and has a Shell filter choice. Launch requires `zsh` on PATH; your normal zsh startup files and shell history settings still apply. Aeolus does not install hooks, modify startup files, or save command history in this slice.

Navigation, rename, confirmed termination, output search, and the composer work with managed shells. In a shell composer, `ctrl+s` sends text followed by Enter and can execute commands; review the command before sending. Approval shortcuts are unavailable for shells. If a recognized agent starts inside a managed pane, its provider and existing agent behavior take priority. Shell activity remains unknown until explicit shell integration is available; command boundaries are never guessed from terminal text.

## Git review

Press `g` to load changed paths and aggregate staged/unstaged diffs for the selected pane's current directory. The view stays bound to that pane and directory across dashboard refreshes. Use arrow keys, j/k, page up/down, home/end, or the mouse wheel to scroll; `r` reloads the snapshot and `esc` closes it. Other dashboard actions are disabled in the review. A missing pane directory or non-Git directory shows an error rather than reviewing the dashboard's own repository.

Review does not stage, edit, commit, fetch, or create worktrees. Git commands have a shared ten-second timeout and bounded output. Each diff is limited to 1 MiB; changed-path lists over 256 KiB produce an error. Large diffs display a truncation notice. Untracked paths are listed without reading their contents, binary changes use Git's summary, and submodule changes are omitted. External diff drivers, text conversion, clean filters, filesystem-monitor commands, and optional index writes are disabled. This produces a raw read-only snapshot; concurrent edits can change files between Git commands.

## Isolated agent worktrees

Press `W` with a session selected to create an isolated Git worktree from that pane's repository HEAD. Enter an absolute destination whose parent exists and a new branch name. Use tab or shift+tab to move between destination, branch, and agent; left/right changes the agent. Enter creates the checkout and launches the chosen agent. Esc cancels before submission. Submitted operations block duplicate actions until completion.

Existing paths and branches are rejected. Staged, unstaged, and untracked source changes remain in the original checkout. Creation disables repository hooks, external filters, filesystem monitors, and network/lazy fetch, and uses a shared 30-second timeout. It rejects destinations inside Git metadata. The source pane, provider, and directory must still match when submitting and before launch.

If launching fails after checkout succeeds, the form retains the worktree and branch; Enter retries launch only. Partial creation failures retain artifacts for inspection and cannot be retried from the form. If tmux reports an uncertain launch outcome, retries are blocked until you inspect its sessions and reopen the form. No worktree or branch is automatically removed. Review retained paths in the form before closing it.

## Configuration

Aeolus reads `$XDG_CONFIG_HOME/aeolus/config.json`, or `~/.config/aeolus/config.json` when `XDG_CONFIG_HOME` is unset. A missing default file uses defaults. Set `XDG_CONFIG_HOME` to an absolute path when overriding it.

```json
{
  "version": 1,
  "refresh_interval": "3s",
  "theme": "dark",
  "default_provider": "codex",
  "projects": [
    {"name": "Application", "directory": "/absolute/path/to/application", "provider": "claude"}
  ]
}
```

Use `aeolus --config /path/to/config.json` to select another file. An explicitly selected file must exist. The version field is required and must be `1`; `refresh_interval` is optional and defaults to `2s`. Its allowed range is `100ms` through `1m`. Invalid files, duplicate or unknown fields, and unsupported versions produce an error before the TUI starts. Settings are read at startup. `default_provider` is optional and defaults to `claude`. It sets the initial agent when opening the launch form with `c`. Supported providers are `claude`, `codex`, and `gemini`. `projects` is an optional array of launch presets; each entry requires a unique, nonblank `name` (up to 80 characters), an absolute `directory`, and a supported `provider`. Names and paths cannot contain control characters. Relative paths and `~` are not expanded in configuration. Preset directories need not exist when loading settings; launch validates them. In the launch form, press `ctrl+p` to cycle project presets, then submit explicitly with Enter. Selecting a preset never starts an agent. Manual directory and provider edits remain available and mark the selection as Manual. Keybindings and saved dashboard preferences remain planned.

## Themes

Set the optional `theme` setting to `dark`, `light`, or `monochrome` and restart Aeolus. Missing theme settings retain the current dark palette. The light palette uses darker text and pale highlights for light terminal backgrounds. Dark and light themes preserve colors supplied by captured session output. Monochrome removes ANSI styling from the entire dashboard display, including captured output, while retaining status text, icons, selection markers, and borders. It does not change the underlying session output or sent text. Unsupported names, nulls, and non-string values are configuration errors.

## Prompt composer

Press `i` to draft a prompt for the selected pane. `enter` adds a newline, `ctrl+s` sends the draft, and `esc` discards it. Arrow keys, home/end, backspace, and delete edit Unicode text. Session shortcuts become ordinary text while composing. Delivery runs asynchronously; repeat sends and edits are blocked until it finishes.

The draft stays bound to the original pane and provider across refreshes. If that target disappears or changes provider, sending is blocked. Errors retain the draft. A failure after pasting may leave text in the target, so inspect that pane before retrying. The composer sends explicit user text to any detected provider, including sessions with unknown status; it does not infer provider-specific approval actions.

Text is pasted through a uniquely named tmux buffer and followed by a separate Enter only after paste succeeds. Words such as `Enter` and `Escape` remain literal. Newlines and tabs are allowed; other terminal control characters are rejected by the transport. Bracketed paste is requested when supported by the receiving application; the target application controls how multiline input and the final Enter behave. The temporary buffer is removed after delivery attempts.

## Reusable prompts

Within the composer, press `ctrl+p` to open the local prompt library. Select a prompt with `j`/`k` or arrow keys and press `enter` to insert its text at the draft cursor for review. Use page up/down to inspect the selected prompt. Nothing is sent automatically. Press `esc` to return to the draft.

Press `s` in the library to save the current draft under a name, then `enter` to save. Names are unique and duplicates are rejected without overwriting existing prompts. Save errors retain the draft and naming input. While a save is pending, cancellation and repeat actions are blocked until its result arrives. `ctrl+r` reloads the library.

The versioned JSON library lives at `$XDG_DATA_HOME/aeolus/prompts.json`, falling back to `~/.local/share/aeolus/prompts.json`. New directories use mode 0700 and library files use mode 0600. Writes use a lock, reload the current library, and replace it atomically. Busy libraries report an error rather than losing another instance's changes. Corrupt or unsupported files are never silently overwritten. The library allows up to 128 prompts, each with an 80-character name and 64 KiB of valid UTF-8 text; total JSON is limited to 1 MiB. Unknown or duplicate JSON fields, missing/null prompt arrays, duplicate names, and unsupported control characters are rejected. Provider integration configuration remains separate.

## Local tmux layouts

From a selected shell session, open `ctrl+k` and choose Browse tmux layouts. Type to search, press Enter to preview, or `N` to create a versioned JSON layout and `ctrl+s` to save it locally. Press `l` in a preview and then `y` to confirm creation. Aeolus adds every created pane to the dashboard and reports partial failures without removing panes. See [the local layout format](docs/layouts.md) for schema and limits.

## Local notebooks

From a selected shell session, press `ctrl+k` and choose Browse local notebooks. Type to fuzzy search, press Enter to open, or `N` to create a notebook. Write Markdown notes and put runnable commands in fenced `sh` blocks. Press `ctrl+s` to save the notebook locally; saving never runs a command.

To run a saved command block, select it in the notebook and press `r` to review the exact command, then `y` to submit it to the original shell. Escape or `n` cancels. Aeolus checks that the same shell is still available before submission. Notebook files are versioned, validated, and stored privately at `$XDG_DATA_HOME/aeolus/notebooks.json` or `~/.local/share/aeolus/notebooks.json`. Saving stores the text you entered, so leave secrets out of notebook documents.

## Local command workflows

In an explicit shell session's composer, press `ctrl+w` to open saved workflows. Write a template such as `rg {{pattern}} {{path}}` in the draft, open workflows, press `s`, and enter a unique name to save it. Saving retains the draft. Escape back to the draft, then Escape again to discard it before opening a fresh composer for reuse. Names and templates are saved locally; avoid secrets in saved text.

Select a workflow with arrows or j/k and Enter. Enter each parameter value, using Enter to advance or Tab/Shift+Tab to revisit values. The final Enter inserts the rendered command at the draft cursor for review; templates without parameters insert immediately. Nothing sends automatically. Review the draft and use the normal `ctrl+s` execution step only when ready. Values entered in the parameter form are not persisted.

Placeholders must be standalone unquoted command words, such as `{{path}}`; quoted or concatenated placeholders are rejected. Values are shell-quoted as literals. Use Escape to return from parameter/name entry to the list, then Escape to return to the draft. Ctrl+R reloads and PageUp/PageDown scroll previews. Errors preserve input, duplicate names are rejected, and changed targets block insertion and saving. See [the workflow contract](docs/workflows.md) for storage and syntax limits. Workflows are available only in explicit shell composers.

## Send a command to selected shells

From a selected managed shell, open `ctrl+k` and choose Send command to shells. Select visible managed shell panes with Space; the current pane starts selected. Compose one command, review the exact text and every selected pane, session, window, and path, then press `y` to submit it. Press `n` to cancel or Escape to return to editing. Aeolus verifies every target against live tmux metadata before sending to any pane, then reports each pane's result. It does not retry or roll back sends. This is a one-shot command send; it does not enable ongoing synchronized input.

## Attach file context

Within the composer, press `ctrl+o`, enter a local file path, and press `enter` to attach it. Relative paths resolve against the original target pane's reported directory; if that directory is unavailable, enter an absolute path. No shell expansion is performed, so use the full home directory path instead of `~`. Press `esc` to return to the draft without attaching.

Attachments must be regular UTF-8 text files no larger than 64 KiB. Symlinks to regular files are supported. Invalid UTF-8, nonregular files, oversized input, and terminal controls other than tabs and newlines are rejected. CR line endings are normalized. A successful read inserts the quoted file path and literal contents into the draft at the cursor for review. Nothing is sent automatically. Read errors and cancellation retain the draft; sending is disabled while the attachment input is open.

## Output search

Press `/`, type a literal query, and press `enter` to keep it. Search ignores case and terminal color escapes, includes the full captured output (currently 50 history lines plus the visible pane), and counts matching lines. The active line has a `>` marker. `ctrl+n` and `ctrl+p` wrap through matches without changing the selected session.

While editing, navigation and action keys become query text; `esc` restores the previous query and `ctrl+u` clears the input. Arrow keys, home/end, backspace, and delete edit Unicode text. While a query is kept, normal session shortcuts remain available. `esc` clears it and restores the regular preview and permission summary. Search uses an uncolored capture view and stays scoped to the selected pane across refreshes; changing or removing that pane clears it. Search reads the latest capture only, not unlimited tmux history.

## Requirements

- tmux (pane metadata framing and working-directory capture tested with tmux 3.5a)
- go v1.26.4 or higher
- A `claude`, `codex`, or `gemini` session inside tmux, or `zsh` on PATH to launch a managed shell

## Provider detection

Detection uses executable names and known npm entry points in the pane process tree. It does not treat agent names in prompts, command arguments, or captured output as evidence of an agent. Custom wrapper names and process paths containing spaces may not be detected; use a standard CLI entry point. Codex activity is reported as Working only when its native terminal status row is visible; other Codex states remain Unknown. Gemini does not inherit Claude output parsing or approval keys. No Codex or Gemini approval actions are inferred. A detected agent with unreadable output remains listed with unknown activity.

## License

[MIT](LICENSE)

## Session filters

Press `f` to open session filters. Press `p` to cycle providers and `s` to cycle statuses, then `enter` to apply. Press `c` to clear both choices or `esc` to discard draft changes. The header shows active filters and visible/total session counts. Filtering keeps the selected pane when it remains visible and retains hidden sessions so clearing filters restores them. An empty result does not send actions to hidden panes. Output search remains scoped to the visible selected pane; typing `f` while editing a search query inserts text instead of opening filters. Press `tab` in the filter modal to edit a case-sensitive working-directory substring, then `tab` again to return to provider/status controls. While editing the directory, ordinary letters (including `p`, `s`, and `c`) enter text; `ctrl+u` clears it. Enter applies all choices and Escape discards the draft. An empty substring matches all directories; sessions with unknown directories do not match a nonempty substring. These paths are the pane working directory reported by tmux, not a Git repository root. The header shows the selected pane directory with control characters quoted.

Launching a session clears active provider/status filters so the new session is visible and selected.

## Saved dashboard filters

Press `S` from the dashboard to save the applied provider, status, and working-directory filters for the next startup. Saving is explicit; editing filters does not write anything. Clear the filters with `f`, `c`, Enter, then press `S` to restore unfiltered startup behavior. Session selection and composer contents are not saved. Read and write errors appear in the TUI and do not prevent session access.

Preferences use `$XDG_STATE_HOME/aeolus/dashboard.json`, falling back to `~/.local/state/aeolus/dashboard.json`. Overrides must be absolute. The bounded, versioned JSON file has strict validation. Saves use a private temporary file and atomic replacement under an exclusive lock; concurrent saves report busy. New directories use 0700 and the saved file uses 0600. Incomplete or unsupported files are reported without applying partial filters.

## Dashboard shortcuts

Version 1 configuration accepts an optional `keybindings` object mapping dashboard action names to shortcuts. For example: `{"version":1,"keybindings":{"launch":"L","refresh":"ctrl+r"}}`. Unspecified actions retain defaults. Remapped defaults stop triggering their former action. Text-entry and modal controls, arrows, Enter, and Ctrl+C stay fixed; a configured dashboard shortcut is ordinary text inside input fields. The footer shows configured shortcuts.

Supported action names are `launch`, `rename`, `git_review`, `compose`, `filters`, `search`, `refresh`, `quit`, `kill`, `approve`, `deny`, `approve_session`, `next`, and `previous`. Keys may be one printable nonspace Unicode character, excluding reserved `?`, `W`, `S`, `H`, and `!`, or one of `ctrl+b`, `ctrl+d`, `ctrl+f`, `ctrl+g`, `ctrl+j`, `ctrl+l`, `ctrl+n`, `ctrl+r`, `ctrl+t`, `ctrl+w`, `ctrl+x`, `ctrl+y`, or `ctrl+z`. Duplicate actions, unsupported shortcuts, unknown actions, null values, and conflicts with effective defaults are rejected at startup. To exchange two defaults, configure both actions together.

## Provider launch arguments

Version 1 configuration accepts optional `provider_args` arrays for `claude`, `codex`, and `gemini`. For example: `{"version":1,"provider_args":{"claude":["--mcp-config","/absolute/path/to/mcp.json"],"codex":["--profile","work"]}}`. Aeolus appends these arguments when launching that provider, including launches from project presets. Existing panes are unaffected.

Arguments use direct process argv, preserving spaces and shell-looking text as literals. No shell interpolation or home/environment expansion occurs. Each provider accepts at most 64 arguments, 4096 bytes per argument, and 16 KiB total; controls, nulls, duplicate providers, and unsupported provider names are rejected. Provider CLIs own the meaning and availability of their flags, profiles, MCP servers, and credentials. Review arguments according to the installed provider's documentation; Aeolus does not implement or infer provider integration APIs. Store credentials in the provider's own configuration rather than these argument arrays.

## Shell command history

Inside a managed shell composer, press `ctrl+r` to browse explicitly saved commands. Type to search commands and directories without case sensitivity, use arrows or ctrl+n/ctrl+p to select, and press Enter to insert the command at the draft cursor for review. Nothing executes until you explicitly press Ctrl+S in the composer. Escape returns to the unchanged draft. Changed or disappeared shell targets block insertion, and read errors retain the draft.

History stores command, directory, exit status, duration, and a save timestamp; output is excluded. Commands are never saved automatically. The private version 1 file uses `$XDG_DATA_HOME/aeolus/history.json` or `~/.local/share/aeolus/history.json`, with strict validation and locked atomic writes. It retains at most 128 entries and 1 MiB, evicting oldest entries on explicit saves. The captured-block browser provides the explicit save action in a dependent change. Saving a command can retain sensitive arguments; review the selected command before saving.
