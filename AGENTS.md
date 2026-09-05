# AGENTS.md

## Project overview

`herdrctx` is a Go terminal UI for managing local Herdr sessions. It lists sessions, refreshes them automatically, and lets users attach, stop, delete, and create sessions from one keyboard-driven screen.

Keep the project name, command, module, documentation, and release artifacts aligned with `herdrctx`.

## Technology constraints

- Use the Go version specified in `go.mod`; keep `.tool-versions` aligned.
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
- Keep user-visible behavior and keyboard shortcuts aligned in `README.md` and `README.ko.md`.
- Keep user-facing text natural English and UI state transitions explicit in the Bubble Tea model.
- Ask for confirmation before stopping or deleting sessions.
- Block deletion of the default session before invoking Herdr.
- Require running sessions to be stopped before deletion; do not stop-and-delete in one step.
- The `n` flow creates a session in the current/default directory and attaches immediately.
- The `N` flow creates a session in a user-selected directory, attaches immediately, and must reject an empty directory.
- Nested popovers must receive key input before their parent modal; `Esc` closes one layer at a time.
- Show one-shot action warnings and failures in alert dialogs, list/refresh failures in the status line, and form validation errors near the relevant input.
- While a dialog is open, background UI must not receive key input.

For UI changes, follow [UI behavior](docs/ui.md) for alert cases, dismissal keys, and rendering.

## Nested Herdr policy

Detect nested Herdr contexts with Herdr environment variables such as `HERDR_ENV=1` and `HERDR_SOCKET_PATH`.

Attach and create actions MUST be blocked by default when `herdrctx` is already running inside Herdr. Allow nested launches only when the user explicitly opts in with `--allow-nested` or `HERDRCTX_ALLOW_NESTED=1`.

## Testing and verification

Use the [testing guide](docs/testing.md#choosing-checks) to select checks for the changed behavior. Go source, dependency, or build configuration changes require the four baseline Go checks; installer and session-control changes also need their relevant tests. Update tests when behavior changes.

Within the requested scope, continue editing, run the applicable local checks, fix failures caused by the change, and rerun affected checks without asking for approval at each step. The documented unit tests use a fake Herdr command; local integration tests isolate their sessions and installation paths. Preserve that isolation.

Documentation-only changes need a content and local-link review, not the Go suite. Report what was verified and any checks that could not run.

## Release constraints

Release builds must stay limited to the Herdr-supported target matrix:

- `darwin_amd64`
- `darwin_arm64`
- `linux_amd64`
- `linux_arm64`

Keep GoReleaser output names and documentation consistent with these targets.

For release work, follow the [release guide](docs/releases.md), including manifest/tag alignment and published artifact verification.

## Git and workspace safety

- MUST NOT commit unless the user explicitly asks for a commit.
- MUST NOT stage, unstage, reset, checkout, stash, clean, or otherwise alter the git index or unrelated working-tree changes unless explicitly asked.
- Treat uncommitted changes as user-owned, even when they are adjacent to files you need to edit.
- Before broad edits, inspect the working tree and avoid overwriting unrelated work.
- If a requested change conflicts with existing uncommitted changes, stop and ask for guidance.
