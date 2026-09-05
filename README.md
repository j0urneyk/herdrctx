# herdrctx

[한국어](README.ko.md)

`herdrctx` is a terminal UI for managing local [Herdr](https://herdr.dev/) sessions. Search by name or directory, attach to a session, or create, stop, and delete sessions from one keyboard-driven list. The list refreshes every 3 seconds and returns when you detach from Herdr.

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

Select a session with `↑` / `↓` and press `enter` to attach. To create one, press `n`, enter a name such as `work`, and press `enter` to create it in the current directory and attach immediately. Detach from Herdr to return to the list.

### Keyboard shortcuts

| Key | Action |
| --- | --- |
| `↑` / `k`, `↓` / `j` | Move up or down |
| `enter` / `a` | Attach to the selected session |
| `/` | Search sessions |
| `n` | Create a session in the current directory and attach |
| `N` | Choose a directory, create a session, and attach |
| `s` | Stop the selected session, after confirmation |
| `d` | Delete the selected session, after confirmation |
| `r` | Refresh the list |
| `?` | Toggle help |
| `q` / `ctrl+c` | Quit |

Close warning and error dialogs with `enter`, `esc`, or `q`. `ctrl+c` quits even while a dialog is open.

### Finding and creating sessions

Press `/` and type part of a session name to filter the list. While searching, `tab` switches between names and directory paths. Press `enter` to keep the filter and close the search field, or `esc` to clear it.

When creating a session with `N`, you must enter a directory. Missing directories are created for you. Use `↑` / `↓` to move between fields or browse directory suggestions, and `tab` to accept a suggestion. `esc` closes the suggestions first; press it again to cancel the form.

New session names must be 1–64 characters, start with an ASCII letter or number, and contain only ASCII letters, numbers, `-`, `_`, or `.`. The name `help` is reserved. Existing sessions remain visible even if their names do not meet the creation rules.

### Stopping and deleting sessions

**Stopping a session can end the shells, servers, and other processes running inside it.** Both stopping and deleting ask for confirmation: `y` / `enter` confirms, and `n` / `esc` cancels.

Deleting removes the saved session state. Stop a running session before deleting it. The default session cannot be deleted.

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
```

CI covers Ubuntu 24.04 and macOS 15 on both supported architectures. See the [testing guide](docs/testing.md) to choose checks for your change, including tests with real Herdr sessions, and the [release guide](docs/releases.md) for publishing and release builds.
