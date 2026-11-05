# Explicit shell command blocks

The `internal/shellblocks` package is the protocol engine for issue #58, a child of #10. It does not enable shell recording, write files, install startup hooks, or execute commands by itself. The TUI capture flow and runtime collector are separate work.

## Stream contract

Zsh `preexec` and `precmd` hooks emit invisible OSC records into terminal output. A tmux `pipe-pane -O` output tap receives those records and the intervening terminal bytes in stream order while commands retain their terminal stdin and stdout. This avoids coordinating separately captured output with independently delivered events. The generated script preserves existing hook arrays and is idempotent per token.

Records use `ESC ] 777;aeolus;v1;TOKEN;TYPE;ID;TIME;FIELDS BEL`. TOKEN is a random 128-bit lowercase hexadecimal value allocated per capture session. TYPE S starts a command and has base64-encoded UTF-8 command text and directory fields. TYPE E finishes the same positive sequence ID and has an exit status from 0 through 255. TIME is Unix time in fractional seconds. The token prevents accidental collision, not attacks by a command running as the same user. Commands can inspect shell state and forge events; this protocol is not a security boundary.

The parser accepts only explicit valid records. It never infers completion from prompts, captured text, elapsed time, or process names. Unknown, malformed, oversized, mismatched, or replayed records are ordinary terminal bytes within an active block. A new valid start retains the previous unfinished command as incomplete. EOF also retains incomplete commands without guessing an exit status. An existing hook that prevents later hooks from running, an exited shell, or redirected shell output may leave a command incomplete.

## Bounds and timing

The in-memory parser keeps at most 32 blocks, at most 64 KiB of output per block, 16 KiB of command text, 4 KiB of directory text, and 32 KiB per protocol frame. Large output is retained up to the cap and explicitly marked truncated. Oversized command metadata does not create a block. `Snapshot` returns deep copies.

Start and finish wall times come from the explicit hook records. Duration uses the collector's monotonic clock between receiving those records, clamped at zero; stream scheduling and backpressure can affect observed timing. Commands keep their tty semantics. Output is raw terminal bytes between hooks, including terminal escape sequences and possibly concurrent background output or hook output; it is not exclusive process stdout. Consumers must sanitize control sequences before rendering.

## Privacy and future integration

The engine has no filesystem persistence. A future capture flow must require explicit TUI opt-in, explain that command text and terminal output can contain secrets, use bounded private runtime storage, and clean up its capture lifecycle. Persistent history must require an explicit user save, rather than automatically retaining captured commands or output.

References: [zsh hook functions](https://zsh.sourceforge.io/Doc/Release/Functions.html#Hook-Functions), [tmux pipe-pane](https://man.openbsd.org/tmux#pipe-pane). Real-tmux tests verify raw marker ordering, command status, existing hooks, and terminal stdin/stdout.
