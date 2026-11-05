# TUI expansion roadmap

Aeolus will expand its existing tmux session dashboard through independently reviewed changes. All user interaction remains in the TUI. Warp provides a feature reference, not a source-code dependency. Full cloud service and native editor parity is outside this local roadmap.

Track implementation and review progress in [the parent issue](https://github.com/sfate/aeolus/issues/15).

## Delivery order

- [Session backend abstraction](https://github.com/sfate/aeolus/issues/4): Separate tmux operations and monitoring dependencies from the UI. Preserve existing Claude detection and approval behavior with injectable dependencies and regression tests.
- [Multiple agent providers](https://github.com/sfate/aeolus/issues/5): Add separate Claude, Codex, and Gemini detection adapters. Expose only supported actions and test provider identification and unknown states.
- [Session lifecycle](https://github.com/sfate/aeolus/issues/6): Launch sessions with a selected project directory and agent; rename, stop, and navigate sessions. Validate inputs and retain confirmation for destructive actions.
- [Multiline prompt composer](https://github.com/sfate/aeolus/issues/7): Compose multiline prompts and free-text answers for one selected session, with cancellation and literal text delivery tests.
- [Session and output search](https://github.com/sfate/aeolus/issues/8): Filter sessions by provider, project, and status; search captured output without changing the underlying session selection.
- [Persistent configuration](https://github.com/sfate/aeolus/issues/9): Configure shortcuts, refresh interval, providers, and projects; persist dashboard preferences with versioned validation and actionable errors.
- [Shell sessions and command blocks](https://github.com/sfate/aeolus/issues/10): Add explicit shell integration for command boundaries, output, exit status, and duration. Do not infer command completion from captured text.
- [Command history and workflows](https://github.com/sfate/aeolus/issues/11): Persist searchable command history and parameterized local workflows. Require explicit execution and avoid persisting secrets by default.
- [Git review and worktrees](https://github.com/sfate/aeolus/issues/12): Inspect changed files and diffs through the TUI, and launch isolated agent sessions in worktrees. Preserve user changes.
- [Context and agent integrations](https://github.com/sfate/aeolus/issues/13): Attach local files and reusable prompts; configure integrations supported by each agent provider without inventing unsupported provider APIs.
- [TUI usability](https://github.com/sfate/aeolus/issues/14): Add a command palette, contextual help, themes, narrow-terminal layouts, and attention indicators. Verify keyboard-only workflows.

## Worktree launch slice

Issue #36 adds explicit creation of a new branch and isolated checkout followed by an agent launch. Creation failures retain artifacts; launch failures retry launch only. Read-only Git review remains available independently.

## Review contract

Each implementation PR links its issue, describes user-visible behavior, records automated and manual verification, and remains open for review. Split oversized issues before implementation. Dependent changes must identify their base PR and remain independently understandable.

## Architecture constraints

Keep terminal transport, agent-specific interpretation, and Bubble Tea presentation separate. Unknown provider states must not expose guessed approval actions. Use explicit shell integration for command metadata. Preserve user worktrees and session contents. Do not copy AGPL code into the MIT repository without an explicit licensing decision.
