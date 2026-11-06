# Local tmux layouts

Aeolus stores local layouts as version 1 JSON at `$XDG_DATA_HOME/aeolus/layouts.json`, falling back to `~/.local/share/aeolus/layouts.json`. The store and lock files use mode 0600 and updates are atomic. Existing layouts are never overwritten by a name collision.

Open the command palette from a selected shell session and choose Browse tmux layouts. Type to fuzzy search, press Enter to preview, or `N` to create a layout. The editor accepts one layout JSON object and saves it with Ctrl+S. From a saved layout preview, press `l` and then `y` to confirm creation. Aeolus creates a detached tmux session and adds each created pane to the dashboard. It reports partial failures and keeps panes that were already created.

A layout has a display name, an absolute project directory, and one or more named windows. Each window contains one or more panes. Pane types are `shell` and `agent`; agent panes require one of `claude`, `codex`, or `gemini`. Every pane after the first declares a `horizontal` or `vertical` split relative to the previous layout. Optional `size_percent` is an integer from 1 to 99.

```json
{
  "name": "Review",
  "directory": "/home/me/project",
  "windows": [
    {
      "name": "Code",
      "panes": [
        {"type": "agent", "provider": "codex"},
        {"type": "shell", "split": "vertical", "size_percent": 40}
      ]
    },
    {"name": "Notes", "panes": [{"type": "shell"}]}
  ]
}
```

Names are unique within the store or window, and limited to 80 characters. A layout may contain at most 16 windows and each window at most 8 panes. The JSON store is limited to 64 KiB. Unknown fields, duplicate JSON keys, unsupported versions, invalid providers, relative directories, and invalid split declarations are rejected without replacing saved data.
