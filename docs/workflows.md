# Local workflow backend

This backend slice of [#11](https://github.com/sfate/aeolus/issues/11) stores named command templates and renders parameters without executing commands. The dependent TUI slice will provide saving, selection, parameter entry, and insertion into an explicit shell session's composer for review. It must never send automatically.

## Template contract

`workflows.Entry` contains `Name` and `Template`. `Parameters(template)` validates interpolation contexts and returns unique names in first-occurrence order. `Render(template, values)` requires exactly those names and replaces each occurrence with a POSIX single-quoted literal, including correct apostrophe escaping. Empty parameter values are allowed. Parameter values remain in memory and are never passed to the store.

Use standalone unquoted placeholders such as `rg {{pattern}} {{path}}`. Names are ASCII identifiers beginning with a letter or underscore, followed by letters, digits, or underscores, with a maximum of 64 characters. There may be at most 16 unique parameters and 256 placeholder occurrences. Names and values are case-sensitive. Double braces are reserved for placeholders.

Validation substitutes collision-free sentinels and parses the template as POSIX shell with [mvdan's syntax parser](https://pkg.go.dev/mvdan.cc/sh/v3/syntax). Each sentinel must be an entire unquoted literal command word. Quoted or concatenated placeholders, assignments, redirections, heredocs, comments, command substitutions, and parameter expansions are rejected as interpolation locations. Parameter-free templates remain literal and do not require POSIX parsing. The validator does not execute or rewrite fixed commands.

Templates and rendered commands are limited to 64 KiB. They must be UTF-8 and may contain tabs and newlines, but other terminal controls are rejected. Quoting prevents parameter values from adding syntax to the surrounding shell command. A trusted template can still deliberately execute its arguments, for example through `sh -c` or `eval`; the TUI must require review and explicit execution.

## Storage contract

`DefaultPath()` resolves `$XDG_DATA_HOME/aeolus/workflows.json`, or `~/.local/share/aeolus/workflows.json`. An explicitly set data home must be absolute. `Load(path)` treats a missing library as empty and otherwise reads a regular UTF-8 file no larger than 1 MiB. Version 1 JSON requires `version` and a nonnull `workflows` array, with each entry containing exactly `name` and `template` strings. Unknown, duplicate, null, missing, and unsupported fields or versions produce errors. Libraries may contain up to 128 entries with unique names of 1-80 characters, without surrounding whitespace or controls.

`Save(path, name, template)` adds a unique name to an absolute library path. It takes a nonblocking exclusive lock, reloads the current library, validates all entries, and atomically replaces the file through a synced temporary file. Corrupt libraries and duplicate names are not overwritten. Newly created directories use 0700 and files use 0600. Existing parent directory permissions are retained. Lock contention reports an error so callers can retry explicitly. The lock file remains in place to keep cooperating processes on the same lock inode.
