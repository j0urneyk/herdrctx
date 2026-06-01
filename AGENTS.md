# AGENTS.md

## Project overview

`herdrctx` is a Go terminal UI for managing local Herdr sessions. It lists sessions, refreshes them automatically, and lets users attach, stop, delete, and create sessions from one keyboard-driven screen.

Keep the project name, command, module, documentation, and release artifacts aligned with `herdrctx`.

## Technology constraints

- Use Go `1.26.3` for development.
- Use Bubble Tea, Bubbles, and Lip Gloss for the TUI.
- Use `golangci-lint` for linting and GoReleaser for releases.
- Keep `herdr` as an external CLI dependency; do not vendor or reimplement Herdr internals.

## Herdr integration rules

Use the Herdr CLI as the integration boundary:

- List sessions with `herdr session list --json`.
- Attach with `herdr session attach`.
- Stop with `herdr session stop --json`.
- Delete with `herdr session delete --json`.
- Create and attach with `herdr --session <name>`.

Preserve Herdr's normal control flow. Creating a session must immediately hand the terminal to Herdr and `herdrctx` should resume only after the user detaches from Herdr.

## TUI and UX rules

- Default refresh interval is `3s`.
- Keep keyboard-first behavior documented in `README.md`.
- Ask for confirmation before stopping or deleting sessions.
- Block deletion of the default session before invoking Herdr.
- Require running sessions to be stopped before deletion; do not stop-and-delete in one step.
- The `n` flow creates a session in the current/default directory and attaches immediately.
- The `N` flow creates a session in a user-selected directory, attaches immediately, and must reject an empty directory.
- Nested popovers must receive key input before their parent modal; `Esc` closes one layer at a time.

## Dialog and status policy

Use alert dialogs for one-shot, user-triggered warnings and errors that explain why the requested action did not run or failed.

Show dialogs for:

- Nested Herdr attach/create blocks.
- No selected session for an action.
- Default-session delete attempts.
- Running-session delete attempts.
- Attach, stop, or delete command failures.

Dialog behavior:

- Dialogs are alerts, not confirmations.
- Close dialogs with `enter`, `esc`, or `q`.
- Keep `ctrl+c` as app quit.
- While a dialog is open, background table/form/confirmation UI must not receive key input.
- Prefer overlay rendering so the session list remains visible behind the dialog.

Keep repeated or polling-related failures in the status line, not dialogs:

- Automatic refresh/list failures.
- Manual refresh failures that share the list code path.
- Form validation errors that should stay near the relevant input.

## Nested Herdr policy

Detect nested Herdr contexts with Herdr environment variables such as `HERDR_ENV=1` and `HERDR_SOCKET_PATH`.

Attach and create actions MUST be blocked by default when `herdrctx` is already running inside Herdr. Allow nested launches only when the user explicitly opts in with `--allow-nested` or `HERDRCTX_ALLOW_NESTED=1`.

## Testing and verification

Use these exact commands before reporting implementation work as complete:

```sh
go test ./...
go vet ./...
golangci-lint run
go build -o bin/herdrctx ./cmd/herdrctx
```

Documentation-only changes do not require the full Go validation suite unless code or build configuration changed.

## Release constraints

Release builds must stay limited to the Herdr-supported target matrix:

- `darwin_amd64`
- `darwin_arm64`
- `linux_amd64`
- `linux_arm64`

Keep GoReleaser output names and documentation consistent with these targets.

## Coding style

- Prefer small, focused Go functions with clear names.
- Keep UI state transitions explicit in the Bubble Tea model.
- Keep user-facing text natural English.
- Update tests when behavior changes.
- Avoid stale implementation history in docs; document current rules and behavior.

## Git and workspace safety

- MUST NOT commit unless the user explicitly asks for a commit.
- MUST NOT stage, unstage, reset, checkout, stash, clean, or otherwise alter the git index or unrelated working-tree changes unless explicitly asked.
- Treat uncommitted changes as user-owned, even when they are adjacent to files you need to edit.
- Before broad edits, inspect the working tree and avoid overwriting unrelated work.
- If a requested change conflicts with existing uncommitted changes, stop and ask for guidance.
