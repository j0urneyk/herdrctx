# herdrctx

[한국어](README.ko.md)

`herdrctx` brings your local [Herdr](https://herdr.dev/) sessions into one terminal view. Pick a session to attach, or create, stop, and delete sessions without leaving the keyboard. The list refreshes every 3 seconds.

## Install

You'll need Herdr 0.6.5 or newer available as `herdr` on your `PATH`. `herdrctx` supports macOS and Linux on x86_64 and arm64.

CI covers Ubuntu 24.04 and macOS 15 on both architectures. See the [testing guide](docs/testing.md) for the checks and their scope.

Install with Homebrew:

```sh
brew install j0urneyk/tap/herdrctx
```

Or install from source with Go 1.26.3:

```sh
go install github.com/j0urneyk/herdrctx/cmd/herdrctx@latest
```

### Herdr plugin

With Herdr 0.7.0 or newer, you can install the released binary through its [plugin manager](https://herdr.dev/docs/plugins/). This requires `git`, `curl`, `tar`, and either `sha256sum` or `shasum`; Go is not required.

```sh
herdr plugin install j0urneyk/herdrctx
export PATH="$HOME/.local/bin:$PATH"
herdrctx
```

Run `herdrctx` **from a terminal outside Herdr**. The plugin provides installation and discovery; it does not add an in-session pane or action. No running Herdr server is needed for installation. The CLI itself still supports Herdr 0.6.5 or newer.

The installer checks SHA-256 and installs the version in the selected checkout's `herdr-plugin.toml` to `~/.local/bin/herdrctx`. Use `HERDRCTX_INSTALL_DIR=/your/bin herdr plugin install j0urneyk/herdrctx` for another directory, and add it to your shell's PATH. Check `command -v herdrctx` if you also installed it through Homebrew or Go. The installer does not edit shell settings.

Run the install command again to replace the binary with the selected manifest version. To select a revision, use `herdr plugin install j0urneyk/herdrctx --ref <tag-or-commit>`. That revision must contain the manifest and installer, and its binary release must be published. The existing `v0.0.2` tag predates the plugin files and cannot be used as a plugin `--ref`, even though the initial manifest installs that binary version.

`herdr plugin uninstall herdrctx` removes the managed checkout and registration. The installed binary remains; remove `~/.local/bin/herdrctx` yourself with `rm "$HOME/.local/bin/herdrctx"`, or remove `herdrctx` from the directory you chose with `HERDRCTX_INSTALL_DIR`.

## Usage

```sh
herdrctx
```

Select a session and press `enter` to attach. When you detach from Herdr, you'll return to the session list. Creating a session attaches immediately, too.

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

New session names must be 1–64 characters, start with an ASCII letter or number, and contain only ASCII letters, numbers, `-`, `_`, or `.`. For example, `my-project`, `work_1`, and `v1.2` are valid. The name `help` is reserved. Existing sessions remain visible even if their names do not meet the creation rules.

### Stopping and deleting sessions

**Stopping a session can end the shells, servers, and other processes running inside it.** Both stopping and deleting ask for confirmation.

Deleting removes the saved session state. Stop a running session before deleting it. The default session cannot be deleted.

## Options

To change the refresh interval or use a specific Herdr binary:

```sh
herdrctx --interval 5s
herdrctx --herdr-bin /opt/homebrew/bin/herdr
```

The minimum interval is `500ms`. You can also set the binary path with `HERDRCTX_HERDR_BIN`; the command-line flag takes precedence. Run `herdrctx --help` for all options, including directory completion settings.

If you run `herdrctx` inside Herdr, attaching and creating sessions are blocked by default. For nested sessions, enable `experimental.allow_nested` in Herdr and run `herdrctx --allow-nested`, or set `HERDRCTX_ALLOW_NESTED=1`.

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

See the [testing guide](docs/testing.md) for CI checks and their scope, and the [release guide](docs/releases.md) for publishing and release builds.
