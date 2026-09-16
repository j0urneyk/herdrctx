# herdrctx

[한국어](README.ko.md)

`herdrctx` is a terminal UI for managing local or SSH-hosted [Herdr](https://herdr.dev/) sessions. Search and filter sessions, keep favorites at the top, and inspect full session details. Attach, create, stop, and delete sessions from one keyboard-driven list. The list refreshes every 3 seconds and returns when you detach from Herdr.

## Install

Install [Herdr](https://herdr.dev/) 0.6.5 or newer first, with `herdr` on your `PATH`. `herdrctx` supports macOS and Linux on x86_64 and arm64 and requires an interactive terminal.

Install with Homebrew:

```sh
brew install j0urneyk/tap/herdrctx
```

Or install from source with Go 1.26.3:

```sh
go install github.com/j0urneyk/herdrctx/cmd/herdrctx@latest
```

Make sure Go's binary directory (`go env GOBIN`, or `$(go env GOPATH)/bin` when unset) is on your `PATH`.

### Herdr plugin

With Herdr 0.7.0 or newer, you can install the released binary through its [plugin manager](https://herdr.dev/docs/cli-reference/#plugins). This requires `git`, `curl`, `tar`, and either `sha256sum` or `shasum`; Go is not required.

```sh
herdr plugin install j0urneyk/herdrctx
export PATH="$HOME/.local/bin:$PATH"
```

The plugin installs the standalone CLI; it does not add a pane or action inside Herdr. No running Herdr server is needed for installation. The installer verifies the archive's SHA-256 against the release checksums. Add the PATH setting to your shell configuration to keep it across terminals; the installer does not edit shell settings. See [plugin installation details](#managing-plugin-installations) for custom paths, versions, and removal.

## Usage

Run this **from a terminal outside Herdr**:

```sh
herdrctx
```

Select a running session with `↑` / `↓` and press `enter` to attach. To create one, press `n`, enter a name such as `work`, and press `enter` to create it in the current directory and attach immediately. Detach from Herdr to return to the list.

### Keyboard shortcuts

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Move up or down |
| `enter` / `a` | Attach to the selected session (running only by default) |
| `/` | Search sessions |
| `f` | Cycle all, running, and stopped sessions |
| `o` | Switch between name order and running first |
| `p` | Toggle the selected session's favorite status |
| `i` | Show full session details |
| `H` | Open host selection and saved-host management |
| `n` | Create/reuse a session and attach; ask for a host in the combined view |
| `N` | Choose a local directory and attach; unavailable for remote hosts |
| `s` | Stop the selected session, after confirmation |
| `d` | Delete the selected session, after confirmation |
| `r` | Refresh the list |
| `?` | Toggle help |
| `q` / `ctrl+c` | Quit |

Close warning and error dialogs with `enter`, `esc`, or `q`. `ctrl+c` quits even while a dialog is open.

### Finding and creating sessions

Press `/` and type part of a session name to filter the list. While searching, `tab` switches between names and directory paths. Press `enter` to keep the filter and close the search field, or `esc` to clear it.

When creating a local session with `N`, you must enter a directory. Missing directories are created for you. Use `↑` / `↓` to move between fields or browse directory suggestions, and `tab` to accept a suggestion. `esc` closes the suggestions first; press it again to cancel the form.

New session names must be 1–64 characters, start with an ASCII letter or number, and contain only ASCII letters, numbers, `-`, `_`, or `.`. The name `help` is reserved. Existing sessions remain visible even if their names do not meet the creation rules.

### Filters, favorites, and details

The list starts with all sessions in name order. Use `f` to cycle through all, running, and stopped sessions; `o` switches between name order and running first. The current conditions and matching count appear above the list. Status filters combine with search. `esc` in search clears only the search text. Filters and sorting survive refreshes and reset when the app restarts.

Press `p` to mark the selected session with `⭐` and keep it above other matching sessions. The chosen sort applies within each group; favorites still have to match the current search and status filter. Favorites are scoped to the local host or exact SSH target and saved by exact session name. They survive deletion: a new session with the same name inherits the favorite. The default session still has its separate `*` marker.

Press `i` to see full session values. Use `↑` / `↓` or `PgUp` / `PgDn` to scroll, and `enter`, `esc`, or `q` to close. Details follow refreshes and indicate when a session disappears or a refresh fails. **Directory** and directory search refer to Herdr's session-state storage path, not the project's working directory.

### Saved preferences

Favorites are stored in `herdrctx/preferences.json` under the platform's user configuration directory:

| Platform | Default path |
| --- | --- |
| macOS | `~/Library/Application Support/herdrctx/preferences.json` |
| Linux | `$XDG_CONFIG_HOME/herdrctx/preferences.json`, or `~/.config/herdrctx/preferences.json` when unset |

The file is created on the first saved change. A failed save leaves the previous preferences intact and opens an error dialog. Writes use an atomic replacement and an application lock; if another instance is saving, retry the action. Each save reloads the file and changes only the selected favorite, preserving other instances' changes. Changes are not watched continuously.

If preferences cannot be read at startup, herdrctx shows a warning and disables saved preferences while allowing ordinary session management. It preserves the original file. Fix it and restart to re-enable saving. To remove favorites for absent sessions or edit JSON manually, close herdrctx first. The versioned format and implementation details are documented in [UI behavior](docs/ui.md#saved-preferences).

### Stopping and deleting sessions

**Stopping a session can end the shells, servers, and other processes running inside it.** Both stopping and deleting ask for confirmation: `y` / `enter` confirms, and `n` / `esc` cancels.

Deleting removes the saved session state. Stop a running session before deleting it. The default session cannot be deleted.

### Attaching to stopped sessions

`enter` / `a` only attaches to running sessions by default. Selecting a stopped session shows a warning without invoking Herdr. To allow starting and attaching to stopped sessions, run `herdrctx --allow-stopped-attach` or set `HERDRCTX_ALLOW_STOPPED_ATTACH=1`. Use `--allow-stopped-attach=false` to override that environment setting. The footer shows `start and attach` for a stopped session when enabled; no additional confirmation is shown.

This setting applies to session-list attachment. `n` and `N` still create or reuse sessions, including restarting an existing stopped session with the same name. The guard uses the latest list status; a running session that stops between refresh and attach can still be restarted by Herdr.

## Remote sessions

Use Herdr 0.8.2 or newer on both machines and have OpenSSH (`ssh`) available locally. Prepare ordinary SSH access and ensure the remote `herdr` command is available to non-interactive SSH:

```sh
ssh workbox 'herdr --version'
herdr --remote workbox
herdrctx --remote workbox
```

Complete any initial authentication or Herdr setup in the foreground, then detach before starting herdrctx. You can also pass `user@host` or `ssh://user@host:2222`; use an SSH URL for IPv6. `--herdr-bin` selects the **local** Herdr binary. Management commands require `herdr` on the remote PATH even if native Herdr attach can discover an installation elsewhere.

`--remote <target>` initially shows that host's sessions, with the target in the header and stop/delete confirmations. Search, filters, sorting, favorites, details, and the existing stopped/nested attach policies apply. `n` creates or reuses a session in the remote default directory and attaches immediately. `N` shows an unsupported-operation notice in remote mode. Directory and socket values belong to the remote host. Running without `--remote` or `--all-hosts` starts locally. Use `H` to select another host or a combined view during the run.

Background SSH never answers authentication prompts or installs or restarts Herdr. Refresh failures keep the last known list. Attaching, stopping, or deleting from that list first rechecks the target, waiting for any active query to finish; `Esc` cancels this check. A lost response to stop/delete is shown as **Remote result unknown**: the operation may already have completed. herdrctx does not repeat it automatically; reconnect and check the current state before trying again.

The first remote favorite save upgrades preferences to version 2 while preserving local favorites. Different SSH target spellings remain separate, even when they point to the same host. Earlier herdrctx versions cannot read version 2 and will disable preferences without overwriting them. New-version instances preserve each other's local and remote changes.

### Multiple hosts

Press `H` to open Hosts, then select a host with Up/Down and Enter. `A` shows all saved hosts plus Local. In this menu, `n` adds a label and SSH destination, `e` edits, `d` removes saved metadata after confirmation, and `i` imports Herdr machine profiles. Tab switches editor fields; Esc closes the current layer. Removing a host preserves its sessions and favorites. Host navigation shows a Host column; state directories remain available in details and search.

Start directly in the combined view with `herdrctx --all-hosts`. Use `--hosts-file /path/to/hosts.json` to override the host catalog beside your preferences file. `--all-hosts` and `--remote` are mutually exclusive. Ordinary local and single-remote launches do not connect to other saved hosts until you select them. In the combined view, `n` and `N` ask for a creation host first.

Hosts are stored separately from favorites in `hosts.json`, in the same configuration directory shown [above](#saved-preferences). You can change a host's label without changing its favorites. Changing its SSH destination creates a different identity; favorites are not moved automatically. Local and the original `--remote` target remain available for the current run even if their saved entry is removed.

If the host file is invalid, `H` displays the error and disables host edits; `--all-hosts` fails startup. Fix the file and restart. External edits are not watched; each save reloads the file to preserve other hosts. Close herdrctx before manually editing JSON. See the [host settings format](docs/ui.md#host-settings) for a complete example.

Each host refreshes independently, with at most four list queries overall. Partial failures keep that host's last-known list and do not hide successful hosts. `Partial results` and `?` beside a session status indicate data that needs revalidation. Host selection also filters the combined list; existing search/status filters still apply.

Machine import requires **local Herdr 0.9.0+** and previews the target and designated session before registering the host. Disabled profiles are rejected. Select a profile with Enter; a changed or conflicting record asks for `y` before replacement, and Esc cancels it. Import registers the whole host; the designated session is retained as metadata, not as a restriction on the session list. Import copies metadata, does not attach or edit Herdr's catalog, and does not automatically follow later profile changes. The combined view includes registered hosts in its background refreshes.

The tested mixed 0.8.2/0.9.0 pairs require a remote binary matching the native client's version before creating a remote session. Complete any installation prompt directly in Herdr; herdrctx never approves it in background commands.

## Options

To change the refresh interval or use a specific Herdr binary:

```sh
herdrctx --interval 5s
herdrctx --herdr-bin /opt/homebrew/bin/herdr
```

The minimum interval is `500ms`. You can also set the binary path with `HERDRCTX_HERDR_BIN`; the command-line flag takes precedence. Run `herdrctx --help` for all options, including directory completion settings.

If you run `herdrctx` inside Herdr, attaching and creating sessions are blocked by default. To opt in, enable [`experimental.allow_nested`](https://herdr.dev/docs/config-reference/#experimental) in Herdr, then either run `herdrctx --allow-nested` or set `HERDRCTX_ALLOW_NESTED=1`.

## Managing plugin installations

The installer uses the version in the selected checkout's [`herdr-plugin.toml`](herdr-plugin.toml) and writes to `~/.local/bin/herdrctx`. Run the install command again to replace the binary with that version. To choose another directory:

```sh
HERDRCTX_INSTALL_DIR=/your/bin herdr plugin install j0urneyk/herdrctx
```

Add that directory to your `PATH`. If you also installed through Homebrew or Go, use `command -v herdrctx` to check which binary your shell runs.

To select a revision, use `herdr plugin install j0urneyk/herdrctx --ref <tag-or-commit>`. It must contain the manifest and installer, and the binary release named in its manifest must be published. See [plugin versions and publication](docs/releases.md#plugin-versions-and-publication) for version availability and older revisions.

`herdr plugin uninstall herdrctx` removes the managed checkout and registration, but leaves the binary. For the default installation path, remove it with `rm "$HOME/.local/bin/herdrctx"`; if you set `HERDRCTX_INSTALL_DIR`, remove `herdrctx` from that directory instead.

## Development

Use Go 1.26.3 and `golangci-lint`. If you use asdf, run `asdf install` in the repository to set up Go.

```sh
git clone https://github.com/j0urneyk/herdrctx.git
cd herdrctx
go run ./cmd/herdrctx
```

To check your changes and build:

```sh
make test
make vet
make lint
make build
make test-integration
```

Integration tests are Go tests behind the `integration` build tag. `make test-integration` builds the app and runs the local suites, downloading checksum-pinned Herdr test binaries on first use. To run one suite, use `make test-integration INTEGRATION_ARGS='-run ^TestNavigation$'`.

To test remote sessions without an external server, run `make test-integration-remote` with a local Docker engine. See [remote SSH tests](docs/testing.md#remote-ssh-tests) for requirements and isolation.

CI covers Ubuntu 24.04 and macOS 15 on both supported architectures. See the [testing guide](docs/testing.md) to choose checks for your change, including tests with real Herdr sessions, and the [release guide](docs/releases.md) for publishing and release builds.

User-visible changes are recorded in the [changelog](CHANGELOG.md).
