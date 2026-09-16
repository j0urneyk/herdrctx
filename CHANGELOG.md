# Changelog

User-visible changes are recorded here. Entries under Unreleased have not been published. See the [release guide](docs/releases.md) for versioning and publication checks.

## Unreleased

### Added

- Explicit host switching with `H`, combined local/SSH lists with `--all-hosts`, and a separate editable host catalog selected with `--hosts-file`. Per-host refresh state keeps partial results available with up to four concurrent list queries. Combined creation asks for its destination.
- Read-only Herdr 0.9.0+ machine import with a profile preview, duplicate/conflict handling, and preserved designated-session metadata.
- Manage one SSH host's sessions with `--remote`: list, search, filter, favorite, inspect, attach, create, stop, and delete. Remote mode requires Herdr 0.8.2+ on both hosts; `n` uses the remote default directory and `N` reports that remote directory selection is unsupported.
- Host-scoped remote favorites with a version 2 preferences format that preserves existing local favorites. Earlier herdrctx versions disable preferences when reading this format without overwriting it.
- Remote stale-list revalidation and explicit unknown-outcome alerts when a stop/delete response is lost; mutation commands are never automatically replayed.
- Opt-in local Docker/OpenSSH integration tests and a Linux SSH CI job, including connection loss, process persistence, response loss, and credential-safe failure diagnostics.

### Changed

- Clarify tested Herdr 0.8.2/0.9.0 installation and restart behavior, including original PATH management after native installation and the running-server compatibility limits.

### Fixed

- Queued host refreshes and deletion preflight respect the shared query limit before authorizing session actions.
- Cancelling a machine-import conflict clears its replacement target, so a later import cannot overwrite the cancelled host. Long host/profile menus keep the selected row visible.

- Remote action checks wait for active list queries to finish, including cancellation and foreground-return refreshes, so SSH queries do not overlap or authorize actions from old responses.
- Cancelling remote revalidation keeps the list marked as needing a fresh check, including when the preceding background refresh failed.

## v0.0.4 — 2026-09-09

### Added

- Session status filters (`f`) and name or running-first sorting (`o`) that combine with name/directory search and preserve the selected session across refreshes.
- Persistent favorites (`p`), marked with ⭐ and grouped above other matching sessions. The marker requests emoji presentation; its appearance depends on the terminal and font. Favorites follow exact session names and survive deletion and recreation of a session with the same name.
- Scrollable session details (`i`) with full session-state and socket paths, plus notices for refresh failures or sessions that disappear.
- Local preferences in the platform's user configuration directory. Saves preserve favorites changed by another app instance and leave existing preferences intact on failure. Invalid startup preferences show a warning while ordinary session management remains available.

### Fixed

- Expanded keyboard help keeps all shortcuts visible at 60 columns and reserves space above it for the session list and search input.
- The refresh spinner continues updating while session details are open.

### Changed

- Integration tests now run entirely in Go via `make test-integration`, including navigation, lifecycle, installer, plugin registration, and opt-in release checks. Test binaries are downloaded and checksum-verified by the Go harness; Python is no longer required.

- Session-list attachment (`enter` / `a`) now blocks stopped sessions by default. Enable `--allow-stopped-attach` or `HERDRCTX_ALLOW_STOPPED_ATTACH=1` to start and attach; the footer reflects the selected session and policy. Creation retains its existing behavior.

- The session list defaults to name order. Search, status filters, and sort order survive refreshes and terminal handoff, then reset when herdrctx restarts.
