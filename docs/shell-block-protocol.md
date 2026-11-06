# Explicit shell command blocks

The `internal/shellblocks` package provides the protocol engine for issue #58 and the opt-in runtime backend for issue #68, both children of #10. The parser and hook generator have no side effects. The runtime backend starts recording only when its captured-shell launch API is explicitly called; ordinary managed shell launches remain unchanged. TUI opt-in controls and block browsing are separate work.

## Stream contract

Zsh `preexec` and `precmd` hooks emit invisible OSC records into terminal output. A tmux `pipe-pane -O` output tap receives those records and the intervening terminal bytes in stream order while commands retain their terminal stdin and stdout. This avoids coordinating separately captured output with independently delivered events. The generated script preserves existing hook arrays and is idempotent per token.

Records use `ESC ] 777;aeolus;v1;TOKEN;TYPE;ID;TIME;FIELDS BEL`. TOKEN is a random 128-bit lowercase hexadecimal value allocated per capture session. TYPE S starts a command and has base64-encoded UTF-8 command text and directory fields. TYPE E finishes the same positive sequence ID and has an exit status from 0 through 255. TIME is Unix time in fractional seconds. The token prevents accidental collision, not attacks by a command running as the same user. Commands can inspect shell state and forge events; this protocol is not a security boundary.

The parser accepts only explicit valid records. It never infers completion from prompts, captured text, elapsed time, or process names. Unknown, malformed, oversized, mismatched, or replayed records are ordinary terminal bytes within an active block. A new valid start retains the previous unfinished command as incomplete. EOF also retains incomplete commands without guessing an exit status. An existing hook that prevents later hooks from running, an exited shell, or redirected shell output may leave a command incomplete.

## Bounds and timing

The in-memory parser keeps at most 32 blocks, at most 64 KiB of output per block, 16 KiB of command text, 4 KiB of directory text, and 32 KiB per protocol frame. Large output is retained up to the cap and explicitly marked truncated. Oversized command metadata does not create a block. `Snapshot` returns deep copies.

Start and finish wall times come from the explicit hook records. Duration uses the collector's monotonic clock between receiving those records, clamped at zero; stream scheduling and backpressure can affect observed timing. Commands keep their tty semantics. Output is raw terminal bytes between hooks, including terminal escape sequences and possibly concurrent background output or hook output; it is not exclusive process stdout. Consumers must sanitize control sequences before rendering.

## Privacy and future integration

The parser has no filesystem persistence. Capture initiation must require explicit TUI opt-in and explain that command text and terminal output can contain secrets. Persistent history must require an explicit user save, rather than automatically retaining captured commands or output.

## Runtime backend

`tmux.CapturedShellLifecycle.CreateCapturedShellSession` starts a new dedicated zsh only. It never attaches a recorder to an existing pane or replaces an existing output pipe. `Pane.CaptureID` is an opaque 32-character lowercase hexadecimal identifier. `shellblocks.ReadCapture` accepts this identifier, never an arbitrary path, and reads only the fixed snapshot in the matching private runtime directory under the process temporary directory.

Capture directories use mode 0700; bootstrap files and snapshots use mode 0600. The hidden helper validates IDs, ownership and permissions, locks against duplicate collectors, and atomically publishes bounded snapshots at most four times per second. A snapshot is limited to 8 MiB including JSON encoding; block and output limits still apply. Reads reject symlink snapshots, nonregular files, unsupported schemas and oversized content. No persistent history is written. Raw bytes remain bounded data; a TUI consumer must sanitize them before display.

The bootstrap sources the user's normal `.zshenv` and `.zshrc`, including a `ZDOTDIR` change made by `.zshenv`, and then installs the generated hooks. It does not edit the user's startup files. Launch verifies collector and bootstrap-hook readiness before reporting capture success. Capture requires an absolute `ZDOTDIR` when that variable is set. Global or user startup files that disable later startup processing or replace the shell with `exec` can prevent startup readiness and produce a setup error. Subsequent hook overrides can interrupt integration; an empty capture is not evidence that no commands ran.

The collector removes its runtime directory when the pane output stream closes. Closing only the dashboard leaves the detached shell and its collector running. Killing the shell closes the stream and removes successful-capture runtime files. Process termination, a machine crash, or partial setup failure can retain a private runtime directory. If tmux might have started a child, setup errors deliberately retain bootstrap files and report the exact directory instead of removing a live shell's `ZDOTDIR`. Inspect and close that shell before manually removing the reported directory. No automatic cleanup removes a potentially live shell's bootstrap files.

References: [zsh hook functions](https://zsh.sourceforge.io/Doc/Release/Functions.html#Hook-Functions), [tmux pipe-pane](https://man.openbsd.org/tmux#pipe-pane). Real-tmux tests verify raw marker ordering, command status, existing hooks, and terminal stdin/stdout.
