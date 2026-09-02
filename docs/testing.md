# Testing

Use Go 1.26.3 from `go.mod`. CI uses golangci-lint 2.12.0 for formatting and lint.

```sh
go test ./...
go vet ./...
golangci-lint run
go build -o bin/herdrctx ./cmd/herdrctx
```

The unit tests use a fake Herdr command and do not require Herdr to be installed.

## Native CI checks

The shared `.github/workflows/ci.yml` workflow runs on pull requests, pushes to `main`, and calls from the release workflow.

| Platform | Runner | Go target |
| --- | --- | --- |
| Linux x86_64 | `ubuntu-24.04` | `linux/amd64` |
| Linux arm64 | `ubuntu-24.04-arm` | `linux/arm64` |
| macOS x86_64 | `macos-15-intel` | `darwin/amd64` |
| macOS arm64 | `macos-15` | `darwin/arm64` |

Each job checks the Go host and target architecture, runs tests and vet, and builds with `CGO_ENABLED=0` to match the release configuration. It then executes `--version` and `--help`, checking both exit status and output. A failure on one platform leaves the other jobs running. A separate Linux x86_64 job checks formatting and lint.

These checks cover the listed OS versions. They do not establish compatibility with every Linux distribution, older macOS releases, or every terminal application.

## Real Herdr lifecycle

The same four jobs run a PTY integration test with Herdr 0.6.5, the minimum supported version. It needs Python 3.9 or newer, Bash, curl, shasum, and standard Unix tools. These are available on the selected runners; no Python packages or Go dependencies are added.

From the repository root:

```sh
bash scripts/install-test-herdr.sh
CGO_ENABLED=0 go build -o bin/herdrctx ./cmd/herdrctx
python3 scripts/test-herdr-integration.py
```

The installer downloads the native binary from the [official v0.6.5 release](https://github.com/herdrdev/herdr/releases/tag/v0.6.5), checks its pinned SHA-256 digest, and verifies `herdr --version`. Download, checksum, version, and execution failures fail the job. It installs to `bin/herdr-ci`, leaving your normal Herdr installation alone. To use an existing 0.6.5 binary, pass `--herdr /absolute/path/to/herdr` to the test.

The test drives `herdrctx` in a 160×40 PTY with `TERM=xterm-256color`:

1. Press `n` to create a uniquely named session and attach immediately.
2. Open a workspace with Herdr's `ctrl+b`, then `shift+n` shortcut. A new Herdr 0.6.5 session starts without workspaces.
3. Print a unique marker through the real shell, then detach with `ctrl+b`, then `q`.
4. Use the returned session list to reattach, check the old marker and a new one, and verify that the shell PID is unchanged.
5. Detach again, stop with confirmation, verify the shell has exited, and delete with confirmation. Check the session still exists before each confirmation.
6. Quit `herdrctx` normally.

Session state comes from `herdr session list --json`. All UI and shell input travels through the PTY. UI output confirms handoff and return; `herdr pane read` checks fresh shell markers and preserved output after reattach. Reading pane text handles Herdr's incremental screen rendering without reconstructing a terminal screen. Checks wait for observed conditions with deadlines, without automatic retries or full-screen snapshots.

Every run uses a new directory under `/tmp`. Only child processes receive the isolated `HOME`, XDG path, `HERDR_CONFIG_PATH`, shell, and temporary directory. The test verifies that session and socket paths remain inside that directory. Success and failure both close the TUI, stop and delete the test's session if it remains, check shell termination, and remove temporary files. Host Herdr environment variables, credentials, shell startup files, and existing sessions are not inherited.

To check the failure path, this command deliberately exits with an error after the first shell marker:

```sh
python3 scripts/test-herdr-integration.py --fail-after-attach --artifacts dist/integration-failure
```

Verify that `failure.txt` reports the injected failure and `cleanup.json` confirms cleanup. A timeout or unexpected error is a test failure, even when cleanup succeeds.

## Results

Open the [CI runs](https://github.com/j0urneyk/herdrctx/actions/workflows/ci.yml) and select the run for the commit you are checking. Confirm all four platform jobs, including the real Herdr test, and the format/lint job succeeded. Any validation failure blocks release publishing and the Homebrew update.

Each test writes to a new `run-*` directory under `dist/integration` by default, keeping results from different local runs separate. CI uploads failed runs as `herdr-integration-<os>-<arch>` artifacts, retained for seven days. They contain the raw `pty.log`, session JSON, CLI output, failure details, cleanup result, and Herdr/OS/architecture metadata. Herdr's own logs are copied before session cleanup. Installation failures leave the host and installer logs even when the PTY test cannot start.

The [initial native CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33596300473) passed on all four runners at `bcd2033`. That run predates the real Herdr test. Local results alone do not establish that the remote lifecycle test passed.
