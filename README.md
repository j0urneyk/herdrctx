# herdrctx

`herdrctx` is a small terminal UI for managing [Herdr](https://herdr.dev/) sessions without copying names out of `herdr session list`.

It shows your local Herdr sessions, refreshes the list automatically, and lets you attach, stop, or delete sessions from one keyboard-driven screen.

## Requirements

- Go 1.26.3 for development
- `herdr` 0.6.5 or newer available on your `PATH`
- An interactive terminal

Herdr currently supports Linux and macOS. This project follows the same release target matrix.

## Install with Homebrew

```sh
brew install j0urneyk/tap/herdrctx
```

`herdrctx` still requires `herdr` 0.6.5 or newer on your `PATH`.

## Install from source

```sh
go install github.com/j0urneyk/herdrctx/cmd/herdrctx@latest
```

For local development:

```sh
git clone https://github.com/j0urneyk/herdrctx.git
cd herdrctx
asdf install
go run ./cmd/herdrctx
```

## Usage

```sh
herdrctx
```

Flags:

```sh
herdrctx --interval 5s
herdrctx --herdr-bin /opt/homebrew/bin/herdr
herdrctx --version
herdrctx --allow-nested
herdrctx --complete-hidden always
herdrctx --complete-count 12
```

Environment variables:

- `HERDRCTX_HERDR_BIN`: path to the Herdr binary
- `HERDRCTX_ALLOW_NESTED`: set to `1`, `true`, `yes`, or `on` to allow nested Herdr launches
- `HERDRCTX_COMPLETE_HIDDEN`: hidden directory completion mode, one of `auto`, `always`, or `never`
- `HERDRCTX_COMPLETE_COUNT`: number of visible directory completion rows
- `HERDR_BIN`: fallback path to the Herdr binary

Flags take precedence over environment variables.

At startup, `herdrctx` checks `herdr --version` and exits with a clear error if the configured Herdr binary is older than 0.6.5.

## Keybindings

| Key | Action |
| --- | --- |
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `enter` / `a` | Attach to the selected session |
| `n` | Create a session in the current directory and attach |
| `N` | Create a session in a chosen directory and attach |
| `s` | Stop the selected session, after confirmation |
| `d` | Delete the selected session, after confirmation |
| `r` | Refresh now |
| `?` | Toggle help |
| `enter` / `esc` / `q` | Close a warning or error dialog |
| `q` / `ctrl+c` | Quit |

The session list refreshes every 3 seconds by default. Custom refresh intervals must be at least 500ms.

Creating a session always follows Herdr's normal flow: after you submit the form, `herdrctx` hands the terminal to `herdr --session <name>`. You remain in Herdr until you detach from that session. Use `n` for the current directory, or `N` when you want to choose a different start directory. The `N` flow requires a non-empty directory path and creates missing directories automatically.

In the `N` flow, use `↑` / `↓` to move between fields. Directory candidates appear under the start directory field while you type; when the dropdown is open, `↑` / `↓` move through matches, `tab` accepts the selected candidate, and `Esc` closes the dropdown before cancelling the form. Hidden directories use `auto` mode by default, which shows hidden candidates only when the typed path segment starts with `.`. The dropdown shows 8 rows by default; use `--complete-count` or `HERDRCTX_COMPLETE_COUNT` to change that.

If `herdrctx` is running inside a Herdr-managed pane, attach and create actions are blocked by default and shown as a warning dialog. Herdr disables nested launches by default, and blocking them in `herdrctx` keeps Herdr's nested-launch warning from taking over your terminal. If you intentionally enabled `[experimental] allow_nested = true` in Herdr, pass `--allow-nested` or set `HERDRCTX_ALLOW_NESTED=1`.

## Safety notes

Stopping a Herdr session can terminate panes, shells, agents, servers, tests, and other running processes in that session. `herdrctx` asks for confirmation before stopping a session.

Deleting a session removes its saved session state directory. `herdrctx` asks for confirmation and blocks deletion of the default session before invoking Herdr.

Running sessions must be stopped before deletion. The tool does not stop and delete in one step.

## Development

```sh
make test
make vet
make lint
make build
```

The repository uses:

- Bubble Tea and Bubbles for the TUI
- `golangci-lint` for linting
- GoReleaser for release builds

If your shell exports an old `GOROOT`, unset it before running Go commands. asdf should select Go 1.26.3 from `.tool-versions`.

## Releases

Releases are built with GoReleaser from version tags.

```sh
git tag -a v0.0.1 -m "v0.0.1"
git push origin v0.0.1
```

The release workflow publishes binaries for the same platform families Herdr supports:

- Linux x86_64
- Linux aarch64
- macOS x86_64
- macOS aarch64

See [docs/releases.md](docs/releases.md) for the release automation details.
