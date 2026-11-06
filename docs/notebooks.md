# Local notebooks

Aeolus stores notebooks as version 1 JSON under `$XDG_DATA_HOME/aeolus/notebooks.json`, falling back to `~/.local/share/aeolus/notebooks.json`. The data home must be absolute. Store files and locks use mode 0600, writes are bounded and atomic, and corrupt or unsupported files are reported instead of overwritten.

Open the command palette from a selected shell session and choose Browse local notebooks. The list supports fuzzy search over titles and notebook content. Press Enter to open a notebook or `N` to create one. The editor accepts Markdown notes and runnable shell blocks written as fenced `sh`, `bash`, or `shell` sections. Press Ctrl+S to save; a title collision is rejected without changing the existing notebook.

Markdown headings and bullets receive terminal formatting. Shell blocks remain distinct from notes. Opening or selecting a block never runs it. Press `r` to review the exact saved command and `y` to submit it to the shell; `n` or Escape cancels. The selected pane, provider, and session kind are checked again at confirmation. The command is sent only through the existing TUI shell transport.

Documents are limited to 128 per store, 64 blocks per document, 64 KiB of source text, and 1 MiB of JSON storage. Titles are unique and limited to 80 characters. The store validates UTF-8, terminal controls, schema versions, duplicate JSON fields, and block types. Shell blocks are stored only when the user explicitly saves the notebook. Run confirmation does not write command data or entered values back to the notebook.
