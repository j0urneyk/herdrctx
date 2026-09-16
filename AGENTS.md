# AGENTS.md

## Project overview

`herdrctx` is a Go terminal UI for managing local Herdr sessions and explicitly selected SSH hosts, including combined lists. It lists and filters sessions, refreshes them automatically, and lets users attach, stop, delete, and create sessions from one keyboard-driven screen. Favorites and full session details support navigation.

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

These launch commands describe local mode. With `--remote <target>`, run management commands through non-interactive OpenSSH on that target and attach/create using the local `herdr --remote <target> --session <name>`. Remote mode requires Herdr 0.8.2 or newer on both hosts and `herdr` on the remote non-interactive PATH. Use H for explicit host selection and --all-hosts for an explicit combined view. Keep hosts.json separate from preferences and never connect to saved hosts merely because the catalog exists. Preserve host identity in selection, actions, and preferences; do not implement Herdr's server protocol.

Background remote commands must not answer prompts or install/restart servers. A lost mutation response is an unknown outcome, not proof of cancellation; never replay it automatically. Revalidate stale remote lists before list-based actions. Keep stop/delete confirmation and deletion preflight on the same host and session.

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
- In remote mode, `n` uses the remote default directory and attaches immediately. Remote `N` shows an unsupported-operation alert and must not inspect or create local directories.
- Nested popovers must receive key input before their parent modal; `Esc` closes one layer at a time.
- Show one-shot action warnings and failures in alert dialogs, list/refresh failures in the status line, and form validation errors near the relevant input.
- While a dialog is open, background UI must not receive key input.

For UI changes, follow [UI behavior](docs/ui.md) for alert cases, dismissal keys, and rendering.

## Nested Herdr policy

Detect nested Herdr contexts with Herdr environment variables such as `HERDR_ENV=1` and `HERDR_SOCKET_PATH`.

Attach and create actions MUST be blocked by default when `herdrctx` is already running inside Herdr. Allow nested launches only when the user explicitly opts in with `--allow-nested` or `HERDRCTX_ALLOW_NESTED=1`.

## Testing and verification

Use the [testing guide](docs/testing.md#choosing-checks) to select checks for the changed behavior. Go source, dependency, or build configuration changes require the four baseline Go checks; installer and session-control changes also need their relevant tests. Update tests when behavior changes.

Within the requested scope, continue editing, run the applicable local checks, fix failures caused by the change, and rerun affected checks without asking for approval at each step. The documented unit tests use a fake Herdr command. Go integration tests live under `integration` with the `integration` build tag; `make test-integration` builds and runs them with isolated sessions and installation paths. Preserve that isolation.

Remote integration changes also require `make test-integration-remote`, which uses dedicated local Docker SSH hosts. The ordinary integration suite must not require Docker. Isolate both management SSH and native Herdr SSH configuration, credentials, and storage. Diagnostics must exclude private keys, including on intentional failure paths.

Documentation-only changes need a content and local-link review, not the Go suite. Report what was verified and any checks that could not run.

## Release constraints

Release builds must stay limited to the Herdr-supported target matrix:

- `darwin_amd64`
- `darwin_arm64`
- `linux_amd64`
- `linux_arm64`

Keep GoReleaser output names and documentation consistent with these targets.

Record user-visible changes in [CHANGELOG.md](CHANGELOG.md) under `Unreleased`. For release work, follow the [release guide](docs/releases.md), including manifest/tag alignment, changelog/release-note reconciliation, and published artifact verification.

## Git and workspace safety

- MUST NOT commit unless the user explicitly asks for a commit.
- MUST NOT stage, unstage, reset, checkout, stash, clean, or otherwise alter the git index or unrelated working-tree changes unless explicitly asked.
- Treat uncommitted changes as user-owned, even when they are adjacent to files you need to edit.
- Before broad edits, inspect the working tree and avoid overwriting unrelated work.
- If a requested change conflicts with existing uncommitted changes, stop and ask for guidance.
