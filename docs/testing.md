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

Session state comes from `herdr session list --json`. All UI and shell input travels through the PTY. Shell commands use bracketed paste, and the test waits for the complete command to reach the shell before pressing Enter. UI output confirms handoff and return; `herdr pane read` checks fresh shell markers and preserved output after reattach. Reading pane text handles Herdr's incremental screen rendering without reconstructing a terminal screen. Checks wait for observed conditions with deadlines, without automatic retries or full-screen snapshots.

Every run uses a new directory under `/tmp`. Child processes inherit the real `HOME`; Herdr storage is isolated with XDG config/data/state/runtime paths, `HERDR_CONFIG_PATH`, and a temporary directory. A test shell wrapper starts `/bin/sh` without login arguments or an `ENV` startup file. The test verifies that session and socket paths remain inside that directory. Success and failure both close the TUI, stop and delete the test's session if it remains, check shell termination, and remove temporary files. Host Herdr environment variables and credential environment variables are not inherited.

To check the failure path, this command deliberately exits with an error after the first shell marker:

```sh
python3 scripts/test-herdr-integration.py --fail-after-attach --artifacts dist/integration-failure
```

Verify that `failure.txt` reports the injected failure and `cleanup.json` confirms cleanup. A timeout or unexpected error is a test failure, even when cleanup succeeds.

## Plugin installation

The native CI matrix also runs these checks:

```sh
python3 scripts/test-plugin-install.py
bash scripts/install-test-plugin-herdr.sh
python3 scripts/test-plugin-herdr.py --herdr bin/herdr-plugin-ci --start-server
```

The installer tests use local tar/checksum fixtures and temporary command shims for `curl` and `uname`. They cover all four asset names, pinned versions, reinstall and version replacement, invalid manifests, download/checksum/extraction failures, preserved existing binaries, unsupported platforms, directory permissions, and PATH guidance. Each native runner packages its built `bin/herdrctx` into a fixture, installs it, and checks `--version` and `--help`. This does not depend on unpublished release assets. Tests inherit `HOME` and isolate installation with `HERDRCTX_INSTALL_DIR`.

The second installer downloads a digest-pinned Herdr 0.7.0 binary to `bin/herdr-plugin-ci`. The registration test uses fresh XDG directories, verifies session/socket paths, links the build-only manifest, checks that `link` did not run the installer, exercises the CLI and both nested-context signals through a PTY, and uninstalls the plugin. Both create shortcuts and attach must open warning dialogs. The binary must remain after uninstallation. Herdr 0.7.0 requires a server for local `link`; `--start-server` starts and stops only this isolated server. Newer Herdr versions that support offline linking can run the check without this option. Logs go to `dist/integration/plugin-*`.

After the plugin files are pushed, verify the **public GitHub install** separately:

```sh
python3 scripts/test-plugin-herdr.py --herdr bin/herdr-plugin-ci --source-ref <published-tag-or-commit>
```

This mode starts without a server, calls `herdr plugin install j0urneyk/herdrctx --ref <ref> --yes`, checks the installed version against the selected manifest, and verifies that uninstall removes the managed checkout while retaining the binary. It runs the published build script, so review that revision first. The original `v0.0.2` tag has no plugin files and cannot be used for this check.

For a downloaded release binary, pass `--binary /absolute/path/to/herdrctx` to the local registration and lifecycle checks. Fixture, local-link, and public-install results cover different steps; only a public-install result proves GitHub distribution works.

## Results

Open the [CI runs](https://github.com/j0urneyk/herdrctx/actions/workflows/ci.yml) and select the run for the commit you are checking. Confirm all four platform jobs, including the real Herdr test, and the format/lint job succeeded. Any validation failure blocks release publishing and the Homebrew update.

Each test writes to a new `run-*` directory under `dist/integration` by default, keeping results from different local runs separate. CI uploads failed runs as `herdr-integration-<os>-<arch>` artifacts, retained for seven days. They contain the raw `pty.log`, session JSON, CLI output, failure details, cleanup result, and Herdr/OS/architecture metadata. Herdr's own logs are copied before session cleanup. Installation failures leave the host and installer logs even when the PTY test cannot start.

The [initial native CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33596300473) passed on all four runners at `bcd2033`. The [full lifecycle CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33598213007) passed all four platforms and formatting/lint at `1263d87`, including installation and the real Herdr lifecycle. Failed Linux runs also verified diagnostic artifact uploads and cleanup. These remote results are separate from local macOS arm64 checks.
