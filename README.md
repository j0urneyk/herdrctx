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
