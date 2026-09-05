# Testing

Use this guide to choose checks, run them from the repository root, and interpret their results. [AGENTS.md](../AGENTS.md) defines the required verification policy; [UI behavior](ui.md) defines the interaction rules to preserve. Publishing and artifact checks belong in the [release guide](releases.md).

## Choosing checks

Select every row that applies to the change. Update tests when behavior changes.

| Changed area | Required local verification |
| --- | --- |
| Go source, dependencies, or build configuration | Run the four [baseline Go checks](#baseline-go-checks). |
| Session creation, attach/detach, stop, or delete | Run the baseline checks and the [real Herdr lifecycle test](#real-herdr-lifecycle). |
| Plugin installer or manifest | Build the current binary and run the [installer fixture tests](#installer-fixtures). For registration changes, also run the [local registration test](#local-registration-and-nested-guards). |
| Nested attach/create guards, even without plugin file changes | Run the baseline checks, lifecycle test, and local registration test. The latter exercises both environment signals and both create shortcuts. |
| Release packaging or workflow | Run the baseline checks, review affected CI checks, and follow the [release guide](releases.md). Packaging changes also require a [snapshot build](releases.md#local-snapshot-builds). |
| Documentation only | Review content against implementation, local file links and heading anchors, and consistency with both READMEs. The Go suite is unnecessary unless code or build configuration also changed. |

The plugin tests cover three different boundaries: local download fixtures, local Herdr registration, and public GitHub distribution. The first two can run before new release assets exist. Only the [public installation check](#public-github-installation) tests a published revision's distribution path.

## Baseline Go checks

Use the Go version in [go.mod](../go.mod), kept aligned with [.tool-versions](../.tool-versions), and the golangci-lint version pinned in [CI](../.github/workflows/ci.yml) (currently `2.12.0`). The [Makefile](../Makefile) exposes the same four checks as `make test`, `make vet`, `make lint`, and `make build`:

```sh
go test ./...
go vet ./...
golangci-lint run
go build -o bin/herdrctx ./cmd/herdrctx
```

The unit tests use fake Herdr commands and temporary files; no installed Herdr is required. UI regression tests cover state transitions and rendered text, while Herdr package tests cover CLI arguments, validation, output limits, and command failures.

CI also checks formatting with `golangci-lint fmt` followed by `git diff --exit-code`. To fix formatting locally, `make fmt` uses the formatters in [.golangci.yml](../.golangci.yml). It edits source files, so inspect the resulting diff and preserve unrelated changes.

## Real Herdr lifecycle

[test-herdr-integration.py](../scripts/test-herdr-integration.py) drives the TUI and a real shell through a Unix pseudo-terminal (PTY). It requires Python `3.9+` and **exactly Herdr `0.6.5`**, the CLI's minimum supported version. The download helper also needs Bash, curl, shasum, and standard Unix tools. No third-party Python packages are needed.

```sh
bash scripts/install-test-herdr.sh
CGO_ENABLED=0 go build -o bin/herdrctx ./cmd/herdrctx
python3 scripts/test-herdr-integration.py
```

[install-test-herdr.sh](../scripts/install-test-herdr.sh) downloads the native asset from the official Herdr `v0.6.5` release, verifies its pinned SHA-256 and `--version`, and writes `bin/herdr-ci`. It leaves the normal Herdr installation alone. Download, checksum, version, and execution failures are failures of this check. To use an existing `0.6.5` binary, skip the installer and pass `--herdr /absolute/path/to/herdr` to the Python test. `--binary /absolute/path/to/herdrctx` selects a different native TUI binary, including an extracted release binary.

The test uses `TERM=xterm-256color`, a `160×40` PTY, and a `500ms` refresh interval:

1. Press `n` to create a unique session and attach immediately.
2. Open a shell workspace with Herdr's `ctrl+b`, then `shift+n`; a new `0.6.5` session has no workspace yet.
3. Print a unique shell marker, then detach with `ctrl+b`, then `q`.
4. Reattach from the filtered TUI list, verify preserved output, print another marker, and confirm the shell PID is unchanged.
5. Detach, stop with confirmation, verify the shell exited, then delete with confirmation. Check that neither action runs before confirmation.
6. Quit the TUI normally.

All UI and shell input travels through the PTY. Shell commands use bracketed paste; the test waits for the complete command before sending Enter. It observes session state through `herdr session list --json` and shell output through `herdr pane read`. Conditions have deadlines, with no automatic test retries. This verifies terminal handoff and session persistence, but does not reconstruct a full screen or cover the `N` form, every dialog, arbitrary terminal sizes, or all terminal applications.

### Isolation and cleanup

Each lifecycle run creates a unique directory under `/tmp` and a unique session name. It keeps the real `HOME`, but supplies fresh XDG config/data/state/runtime paths, `HERDR_CONFIG_PATH`, and `TMPDIR`. A minimal child environment excludes inherited Herdr signals and credential variables. A shell wrapper starts `/bin/sh` without login arguments or an inherited `ENV` startup file. This is storage and environment isolation, not a separate user account or filesystem sandbox.

The test checks that reported session and socket paths stay under its temporary root. On success or failure, cleanup closes the TUI, stops and deletes its test session if necessary, checks shell termination, and removes the temporary root. Preserve these boundaries when changing the test; never substitute an ordinary user session as its target. Cleanup errors are failures too, not evidence of a clean run.

To exercise failure reporting and cleanup deliberately:

```sh
python3 scripts/test-herdr-integration.py --fail-after-attach --artifacts dist/integration-failure
```

Expect a nonzero exit. In the printed `run-*` artifact directory, confirm that `failure.txt` names the injected failure and `cleanup.json` confirms session removal and shell exit. If `cleanup-error.txt` exists, investigate it. A timeout or unrelated error does not count as the expected injected failure, even when cleanup succeeds.

## Plugin installation

Use Python `3.9+` on a supported macOS or Linux host and build the current native `bin/herdrctx` before running plugin checks. The download helper needs Bash, curl, and shasum; fixture installation needs tar and either sha256sum or shasum, plus standard Unix tools. Public GitHub installation also needs git and curl, as listed in the [README](../README.md#herdr-plugin).

Fixture tests isolate `HERDRCTX_INSTALL_DIR` and temporary downloads. Registration and public-install tests use fresh XDG paths, config, runtime storage, and an installation directory under a temporary root, while retaining the real `HOME` and host `PATH`. They also verify that reported session and socket paths remain inside that root.

### Installer fixtures

```sh
python3 scripts/test-plugin-install.py
```

[test-plugin-install.py](../scripts/test-plugin-install.py) replaces `curl` and `uname` with temporary command shims and uses local tar/checksum fixtures. It covers all four asset names, pinned versions, reinstall and version replacement, invalid manifests, download/checksum/extraction failures, preservation of existing binaries, unsupported platforms, destination permissions, and PATH guidance.

Its native-binary case packages the built `bin/herdrctx`, installs it into the temporary destination, and compares `--version` and `--help` behavior. Other platform cases use shell fixtures; they do not execute foreign-architecture release binaries. A local development binary may report `herdrctx dev`: these tests do not establish release-version stamping or the existence of public assets. They leave no persistent `dist/integration` artifact bundle; use their test output when diagnosing failures.

### Local registration and nested guards

```sh
bash scripts/install-test-plugin-herdr.sh
python3 scripts/test-plugin-herdr.py --herdr bin/herdr-plugin-ci --start-server
```

[install-test-plugin-herdr.sh](../scripts/install-test-plugin-herdr.sh) downloads and verifies a digest-pinned Herdr `0.7.0` at `bin/herdr-plugin-ci`. This version satisfies [herdr-plugin.toml](../herdr-plugin.toml); standalone CLI compatibility is tested separately with `0.6.5`.

[test-plugin-herdr.py](../scripts/test-plugin-herdr.py) links the local manifest, verifies that linking did not run its installer, and copies the built binary into the isolated installation directory. It checks the build-only registration, CLI version/help, and warning dialogs for attach, `n`, and `N` under each of `HERDR_ENV=1` and `HERDR_SOCKET_PATH`. It then uninstalls, checking that the registration is gone while the local checkout and installed binary remain.

Herdr `0.7.0` needs a running server for local `plugin link`; `--start-server` starts and stops only an isolated test server. With a Herdr version that supports offline linking, this option can be omitted; report that version separately from CI's pinned `0.7.0`. Pass `--binary /absolute/path/to/herdrctx` to test another native binary. Logs and a successful `result.json` go to a new `dist/integration/plugin-*` directory. Unlike the lifecycle test, this script does not emit `cleanup.json` or `failure.txt`; inspect the traceback and command/server/PTY logs on failure.

### Public GitHub installation

Run this only after the selected revision and the binary release named in its manifest are public. Review that revision first: the test executes its published build script and uses Herdr's noninteractive `--yes` option.

```sh
python3 scripts/test-plugin-herdr.py --herdr bin/herdr-plugin-ci --source-ref <published-tag-or-commit>
```

Replace the placeholder with the exact reviewed revision. This mode starts without a server and runs `herdr plugin install j0urneyk/herdrctx --ref <ref> --yes` inside isolated storage. It verifies that installation does not start a server, checks the installed version against the selected manifest, runs the registration and nested-guard checks, and verifies that uninstall removes the managed checkout while preserving the binary. `--source-ref` and `--start-server` are mutually exclusive.

The script still resolves `--binary` (default `bin/herdrctx`) at startup, so that file must exist even though public mode tests the downloaded binary. Build it before running this check. Results use the same `plugin-*` artifact format as local registration. A passing fixture or local-link test cannot substitute for this public-install result. See [plugin versions and publication](releases.md#plugin-versions-and-publication) for eligible revisions and the early `v0.0.2` exception.

## Native CI checks

The shared [ci.yml](../.github/workflows/ci.yml) runs on pull requests, pushes to `main`, and calls from the release workflow.

| Platform | Runner | Go target |
| --- | --- | --- |
| Linux x86_64 | `ubuntu-24.04` | `linux/amd64` |
| Linux arm64 | `ubuntu-24.04-arm` | `linux/arm64` |
| macOS x86_64 | `macos-15-intel` | `darwin/amd64` |
| macOS arm64 | `macos-15` | `darwin/arm64` |

Each native job verifies the Go host and target architecture, runs tests and vet, builds with `CGO_ENABLED=0`, and checks successful `--version` and `--help` execution and output. The unversioned CI build must print `herdrctx dev`. Each job also runs installer fixtures, the real `0.6.5` lifecycle test, and `0.7.0` local registration/nested-guard checks. Public GitHub installation is a separate post-publication check. A platform failure leaves the other matrix jobs running; a separate Linux x86_64 job checks formatting and lint.

These results cover the listed runners and test scenarios. They do not establish compatibility with every Linux distribution, older macOS versions, or all Herdr versions above the minimum. Local checks establish only what ran locally; they do not prove remote CI passed.

## Results and failure diagnosis

For the exact commit or tag being evaluated, inspect the [CI](https://github.com/j0urneyk/herdrctx/actions/workflows/ci.yml) or [Release](https://github.com/j0urneyk/herdrctx/actions/workflows/release.yml) run. Confirm all four platform jobs and the format/lint job succeeded. Release validation failure or cancellation blocks publishing; see the [release failure guidance](releases.md#handling-a-failed-release) for failures after validation.

Lifecycle runs write separate `run-*` directories under `dist/integration` by default. They contain PTY output, session JSON, CLI output, environment metadata, and cleanup results. On failure they also save a traceback and copy Herdr diagnostic files before cleanup. Binary/path setup failures can occur before this collection starts, so keep the command's stderr as well.

CI uploads `dist/integration/` on job failure as `herdr-integration-<os>-<arch>`, retained for seven days. The bundle may contain lifecycle `run-*` and registration `plugin-*` directories, depending on how far the job got. CI captures `host.txt` and `install.log` around the `0.6.5` installer; `0.7.0` download errors remain in the workflow step log. An early failure may leave no uploadable files. Preserve needed evidence before a snapshot build, whose `--clean` removes `dist/`.

Start with the failed command or traceback, then examine CLI/PTY output and any cleanup result. Fix failures caused by the change and rerun affected checks. Report the commit, platform, binary/Herdr versions, commands, results, and any check that could not run. Do not infer cleanup success from a missing log or hide an unexplained failure behind a passing retry.

### Historical evidence

The original documentation recorded the [initial native CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33596300473) at `bcd2033` and the [full lifecycle CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33598213007) at `1263d87` as passing all four platforms, with formatting/lint included in the latter. These links preserve the provenance of the initial validation work; they do not validate the current commit or later plugin checks. Diagnostic artifacts from those runs may have expired.
