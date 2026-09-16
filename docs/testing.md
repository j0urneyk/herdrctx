# Testing

Run checks from the repository root with the Go version in [go.mod](../go.mod). [AGENTS.md](../AGENTS.md) defines the required verification policy; [UI behavior](ui.md) defines the interactions to preserve. Publishing and artifact verification belong in the [release guide](releases.md).

All test suites are Go tests. Fast tests live beside the application packages. Tests that launch the built TUI, real Herdr, or the installer live in [integration](../integration) behind the `integration` build tag. The production [installer](../scripts/install.sh) remains a shell script and is tested as an external command.

## Choosing checks

Select every applicable row. Update tests when behavior changes.

| Changed area | Required local verification |
| --- | --- |
| Go source, dependencies, or build configuration | Run the four [baseline Go checks](#baseline-go-checks). |
| Session navigation, favorites, details, or preferences | Baseline checks and the [navigation PTY test](#navigation-through-a-pty). |
| Session creation, attach/detach, stop, or delete | Baseline checks and the [real Herdr lifecycle test](#real-herdr-lifecycle). |
| Plugin installer or manifest | Baseline checks and [installer fixtures](#installer-fixtures); registration changes also need [local registration](#local-registration-and-nested-guards). |
| Host catalogs, host switching, combined lists, or machine import | Baseline checks, `TestHostsNavigation` through a PTY, and `make test-integration-remote`. |
| Remote CLI, UI, or SSH harness | Applicable checks above plus `make test-integration-remote`. |
| Nested attach/create guards | Baseline checks, lifecycle, and local registration. The latter checks both environment signals, attach, and both creation shortcuts. |
| Integration harness or CI | Baseline checks and the complete `make test-integration` suite. |
| Release packaging or workflow | Baseline checks, applicable integration tests, and the [release guide](releases.md). Packaging changes also require a [snapshot build](releases.md#local-snapshot-builds). |
| Documentation only | Review implementation consistency, both READMEs, and local links/anchors. Go checks are unnecessary unless code or build configuration also changed. |

`make test-integration` builds the app and runs all local integration suites. Public GitHub installation and release-tag validation require explicit flags and are skipped in an ordinary local run.

## Baseline Go checks

Keep [.tool-versions](../.tool-versions) aligned with `go.mod`, and use the golangci-lint version pinned in [CI](../.github/workflows/ci.yml), currently `2.12.0`.

```sh
make test
make vet
make lint
make build
```

`make test` runs `go test ./...` without the integration tag, so it does not launch a real Herdr server or download test binaries. `make vet` includes the integration tag, and the linter configuration includes it too: the integration source receives vet and lint checks even when its tests are not running. `make build` writes `bin/herdrctx`.

Package tests use fake commands and temporary files. They cover UI state transitions, CLI arguments and validation, failure handling, and preference persistence. [Navigation tests](../internal/ui/navigation_test.go) cover filtering, sorting, favorites, selection, and detail scrolling. [Preferences tests](../internal/preferences/store_test.go) cover invalid-file preservation, atomic-write cleanup, lock contention, and updates from multiple instances.

`make fmt` applies gofmt, gofumpt, and goimports through [.golangci.yml](../.golangci.yml). It edits files; preserve unrelated work when inspecting the resulting diff. CI checks formatting with `golangci-lint fmt` followed by `git diff --exit-code`.

## Running integration tests

```sh
make test-integration
```

This is equivalent to building the app and running:

```sh
go test -tags=integration ./integration -count=1 -timeout=10m -v
```

The tests require a native macOS or Linux host on x86_64 or arm64. The PTY harness uses [creack/pty](https://pkg.go.dev/github.com/creack/pty) as a test dependency; it is not linked into the shipped app. The installer tests exercise the actual shell installer, so its standard Unix tool requirements still apply. Python is not required.

The [download helper](../integration/download_test.go) fetches missing Herdr binaries from official GitHub release assets and checks their pinned SHA-256 and version before installing them into the repository's `bin` directory. Existing cached binaries are checked too. It uses Herdr `0.6.5` at `bin/herdr-ci` for standalone compatibility and `0.7.0` at `bin/herdr-plugin-ci` for plugin registration. The normal Herdr installation is untouched. A download, digest, version, or execution failure fails the test rather than skipping it. Once dependencies, these two Herdr binaries, and the pinned herdrctx v0.0.4 archive for mixed-version preferences tests are cached, the local suites do not require network access.

Use Go's `-run` option to select a suite, through `INTEGRATION_ARGS`:

```sh
make test-integration INTEGRATION_ARGS='-run ^TestNavigation$'
make test-integration INTEGRATION_ARGS='-run ^TestInstaller'
make test-integration INTEGRATION_ARGS='-run ^TestLifecycle$'
make test-integration INTEGRATION_ARGS='-run ^TestPluginRegistration$'
```

Custom test flags accept repository-relative or absolute paths:

| Flag | Purpose |
| --- | --- |
| `-integration.binary` | Test an existing native herdrctx binary instead of `bin/herdrctx`, including an extracted release binary. |
| `-integration.herdr` | Use an existing Herdr binary for the lifecycle test. It must report exactly `0.6.5`. |
| `-integration.plugin-herdr` | Use an existing Herdr binary for plugin tests. Report its version separately from the pinned `0.7.0` check. |
| `-integration.artifacts` | Change the artifact directory from `dist/integration`. |
| `-integration.fail-after-attach` | Deliberately fail the lifecycle test to exercise cleanup and diagnostics. |
| `-integration.source-ref` | Enable the separate public-install test for a reviewed, published Git ref. |
| `-integration.release-tag` | Enable comparison of a release tag with the plugin manifest version. |

An explicit Herdr path is version-checked and does not use the download cache's pinned digest check. Use the defaults when verifying the same binaries as CI. To test an existing release binary without rebuilding the local app, use the direct `go test` command with `-integration.binary=/absolute/path/to/herdrctx`.

## Navigation through a PTY

[Navigation integration tests](../integration/navigation_test.go) launch the built TUI in a real Unix pseudo-terminal and send keyboard bytes. The [Go fake CLI](../integration/fixtures_test.go) serves controlled JSON session state and records foreground invocations; no Herdr server runs in this suite.

The six scenarios cover:

1. Status filters combined with search, name/running-first sorting, and selected targets checked through details. Session action keys cannot escape a detail overlay.
2. Favorite persistence across restart, a failed save while another process holds the preference lock, and retry after releasing it.
3. Detail refreshes, disappearance, failure/recovery, long-path scrolling, and terminal resizing.
4. The unbound `b` key leaves navigation available, and ordinary attachment invokes the foreground CLI exactly once with the selected name and working directory.
5. Invalid startup preferences remain intact, ordinary session creation stays accessible, and preference actions are blocked.
6. Stopped attachment is blocked by default, enabled by flag or environment, and blocked by an explicit false flag overriding the environment. Exact invocation counts are checked.

Each test supplies an isolated HOME, XDG directories, preferences, session fixtures, and working directory. Temporary command wrappers invoke the Go test executable in fixture mode; no separate fixture runtime is needed. Tests observe state changes rather than retrying failed scenarios. Output may arrive in complete frames or small updates, and short-lived status messages may never be rendered before a refresh. Handoff checks therefore look for the restored list footer after the foreground command's output.

The PTY harness inspects terminal output without reconstructing every screen cell. Keep model/rendering unit tests for layout assertions. Emitting an emoji sequence does not establish how each terminal/font will display it.

### Remote navigation without Docker

`TestRemoteNavigation` uses a fake SSH executable and the real herdrctx PTY. It runs in the ordinary integration suite without Docker or network access. Its query fixture takes a process lock and records any overlapping invocation. A controlled response gate checks waiting behind background refreshes, fresh revalidation after old responses, cancellation both while waiting and during an active check, and confirmation-time refresh failures. It also verifies modal input ownership, remote `N`, stopped attach opt-in, and both nested-environment signals.

`TestMixedVersionPreferences` runs the published **herdrctx v0.0.4 binary** beside the current app against one isolated preferences file. Archive SHA-256 values are pinned for all four supported platforms. On first use the test downloads the archive into `bin`; it extracts only the binary into the scenario's temporary directory. It checks an already-running old app's save after v2 migration, an old app's startup against v2, preservation of local favorites, and continued saves/restart by the current app. This test needs network access only until the verified legacy archive is cached.

## Real Herdr lifecycle

[The lifecycle test](../integration/lifecycle_test.go) uses Herdr `0.6.5`, a `160×40` PTY, `TERM=xterm-256color`, and a `500ms` refresh interval:

1. Press `n` to create a unique session and attach immediately.
2. Open a shell workspace with Herdr's `ctrl+b`, then `shift+n`; a new `0.6.5` session starts without a workspace.
3. Print a unique shell marker, then detach with `ctrl+b`, then `q`.
4. Reattach from the filtered list, verify preserved output, print another marker, and confirm that the shell PID is unchanged.
5. Detach and stop with confirmation. Verify the shell exits and Enter leaves the stopped session stopped while showing the opt-in warning.
6. Delete with confirmation and quit normally. Neither stop nor delete may execute before confirmation.

Shell commands use bracketed paste, and the test waits for the complete command before sending Enter. It observes Herdr state through `session list --json` and `pane read`. The shell marker is split in the typed command, preventing its echo from falsely satisfying the output assertion. This tests terminal handoff and live shell persistence; it does not cover every terminal application or every Herdr version above the minimum.

### Isolation and cleanup

Each scenario creates a short path under `/tmp`, resolving the host's symlinks before checking containment. Keep it short: Herdr's nested session/socket paths must fit the platform's Unix-domain socket limit. Both HOME and XDG settings are isolated, including Go's macOS preference location under `HOME/Library/Application Support`.

Child environments exclude inherited Herdr signals and credentials. Lifecycle shells run `/bin/sh` without login startup files or inherited `ENV`. Every reported session directory and socket path must stay under the scenario root. This isolates storage and process configuration, not operating-system user privileges.

Go cleanup handlers close PTY clients, stop/delete only the isolated lifecycle session, check shell termination, and remove temporary roots. The plugin suite also stops and verifies its isolated server. Handled interrupts cancel bounded waits so cleanup can run. Cleanup errors fail the test. Inspect the recorded cleanup result; missing output is not proof of successful cleanup.

To test the failure path deliberately:

```sh
make test-integration INTEGRATION_ARGS='-run ^TestLifecycle$ -integration.fail-after-attach -integration.artifacts=dist/integration-failure'
```

Expect a nonzero exit with `Injected failure after attach`. In the printed `lifecycle-*` directory, `cleanup.json` must report session removal and shell exit, while `result.json` reports failure. A timeout or unrelated failure is not the intended injected failure. `cleanup-error.json` indicates an additional cleanup failure.

## Plugin installation

### Installer fixtures

[Installer tests](../integration/installer_test.go) run `scripts/install.sh` with Go-backed `curl` and `uname` fixtures and locally generated tar/checksum assets. They cover all four asset names, pinned versions, reinstall and replacement, malformed manifests, download/checksum/extraction failures, preservation of existing binaries, unsupported platforms, destination permissions, and PATH guidance.

The native case packages the current binary, installs it in a temporary destination, and compares its version/help behavior. Other platform cases use small shell executables inside fixture archives; they check asset selection rather than executing foreign-architecture binaries. The permission-denial case requires an unprivileged user and is skipped when run as root. The four native CI runners execute it as unprivileged users.

These tests do not prove release-version stamping or the existence of public release assets. Every installer case now retains command logs and result metadata alongside the other integration artifacts.

### Local registration and nested guards

[Plugin tests](../integration/plugin_test.go) start an isolated Herdr `0.7.0` server, link the local manifest, verify that linking did not run the installer, and copy the built app into the isolated installation directory. They check the build-only registration, CLI version/help, and attach/`n`/`N` warnings under both `HERDR_ENV=1` and `HERDR_SOCKET_PATH`.

Uninstall must remove registration while preserving the external binary and the local checkout. The server is stopped during cleanup, with its stopped state recorded in `cleanup.json`. The test always manages its own server for local linking; a custom Herdr version is still isolated and reported in the artifacts.

### Public GitHub installation

Run this only after the selected revision and its manifest's binary release are public. Review the revision first: this test executes its published build command through Herdr's noninteractive `--yes` option.

```sh
make test-integration INTEGRATION_ARGS='-run ^TestPublicPlugin$ -integration.source-ref=<published-tag-or-commit>'
```

Replace the placeholder with the reviewed ref. The public path installs `j0urneyk/herdrctx` in an isolated root without starting the local-link server. It verifies that installation does not start a server, compares the installed version with the registered manifest, checks nested guards, and uninstalls. Registration and the managed checkout must disappear, while the external installed binary remains.

A local-link or installer-fixture pass cannot replace this post-publication check. See [plugin versions and publication](releases.md#plugin-versions-and-publication) for version eligibility and the early `v0.0.2` exception.

## Remote SSH tests

A separate local Docker engine provides disposable Linux SSH hosts; no external server or user SSH configuration is required. Docker/OrbStack on macOS and a local Unix-socket Docker engine on Linux are supported test setups. The host needs OpenSSH (`ssh` and `ssh-keygen`). A stopped or unavailable engine fails the explicitly selected suite. Ordinary `make test-integration` skips the remote tests before touching Docker.

```sh
make test-integration-remote
```

This builds the host herdrctx binary and two images from [integration/remote/Dockerfile](../integration/remote/Dockerfile), then runs `TestRemote*` with `-integration.remote`. The Debian base is digest-pinned. Linux Herdr assets for 0.8.2 and 0.9.0 are verified against embedded SHA-256 values; the Go harness separately verifies downloaded native clients and the container binaries. Dependencies require network access on first setup. Images and verified binaries are retained for reuse.

The native Herdr contract runs against matching 0.8.2 and 0.9.0 clients/servers. The herdrctx lifecycle runs with both matching versions; the fault tests use 0.8.2, the remote minimum. They verify create/detach/reattach, shell PID and output persistence, favorites and details, stop confirmation/cancellation, stopped attach protection, default-session deletion blocked before any SSH delete invocation, deletion, authentication/host-key failures, and missing remote PATH binaries. A loopback TCP proxy disconnects SSH without stopping Herdr; another fixture completes a stop but withholds its response to verify unknown-outcome reporting without replay.

Each run creates a nonprivileged container user, fresh user/host SSH keys, a strict dedicated known_hosts file, random loopback ports, and isolated local/remote Herdr state. A Go test helper named `ssh` forwards to `/usr/bin/ssh` with an explicit test config, replacing any generated `-F` argument. This wrapper is test-only. The fixture disables Herdr's managed SSH config and OpenSSH agents/control-socket reuse, checks effective SSH settings, and records both management and native attach invocations. The app itself uses normal user OpenSSH configuration. User homes, credentials, source checkouts, and engine sockets are not mounted in test containers.

Containers, proxy connections, keys, and temporary roots are removed by cleanup handlers; cleanup failures fail the suite. Image tags `herdrctx-remote-test:0.8.2` and `herdrctx-remote-test:0.9.0` remain as test caches. Failure artifacts under `dist/integration/remote-*` contain commands, PTY/SSH logs, hashes, current session JSON, and `cleanup.json`. Snapshot collection uses an allowlist and root-scoped reads, excludes symlinks/private-key content, and scans remote artifacts for generated private keys. `TestSnapshotExcludesKeysAndSymlinks` also exercises renamed key copies and preserved safe diagnostics without Docker.

To select a scenario or deliberately verify cleanup after failure:

```sh
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteLifecycle$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteLifecycle$ -integration.fail-after-attach'
```

The second command must fail with `Injected failure after remote attach`, retain diagnostics without keys, and record container removal in `cleanup.json`. It is a failure-path check, not a passing suite.

### Host navigation and expanded remote checks

`TestHostsNavigation` runs without Docker using isolated fake SSH/Herdr commands. It covers same-name host selection and handoff, explicit creation destination, cancelled stop, machine conflict confirmation, and host registration. Host state/scheduling tests also check identity-preserving selection, the four-query limit, fair scheduling, stale state after cancellation, and generation rejection. Regression cases also cover queued action checks, delete preflight waiting for capacity, cancelled import conflicts, invalid-catalog visibility, and cursor-following host/profile menus. Hosts settings tests check locking, atomic updates, conflicts, and invalid-file preservation.

The opt-in remote suite additionally runs `TestRemoteMultipleHosts` with two independent 0.9.0 Docker SSH hosts, same-name sessions, correct stop routing, partial connection failure, and real machine import. `TestRemoteMixedVersions` separates local client, original remote PATH binary, and actual running-server versions. Both 0.8.2/0.9.0 directions cover management without installation, default-release and explicit-file installation refusal, installation acceptance, an already-running target, upload failure, and Ctrl+C cancellation. `TestRemoteMachineVersions` verifies the local machine-import version boundary with the opposite remote PATH version and checks that import preserves the source catalog bytes. See [version installation checks](#version-installation-checks) for setup and interpretation. `TestRemoteDelayedTransport` adds per-write delay and a transport stall, checks bounded query failure, and restores connectivity without restarting Herdr. `TestRemoteJumpHost` checks management and native SSH through a separate Docker bastion with strict host keys and separate identities.

Run the fake-host UI without Docker, or select the real host scenarios:

```sh
make test-integration INTEGRATION_ARGS='-run ^TestHostsNavigation$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteMultipleHosts$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteMixedVersions$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteDelayedTransport$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteJumpHost$'
```

For the two-host cleanup failure path, run:

```sh
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteMultipleHosts$ -integration.fail-after-attach'
```

It must fail with `Injected failure with two remote hosts`, record both container removals, and retain no private keys. For SIGTERM verification, signal only the process created for this test after its `both hosts ready for cancellation` marker; preserve each scenario's cleanup and result files. Do not signal unrelated Herdr or Docker processes. New host/fake state diagnostics are included in the existing bounded snapshot allowlist.

The September 15 implementation validation used macOS arm64 clients and Linux arm64 containers. The remaining native platforms, macOS SSH server, real WAN/MFA and enterprise policy environments require separate evidence. A host sshd executable alone does not establish an isolated macOS user/VM test environment. Do not enable system Remote Login or use personal SSH credentials to fill that gap. GitHub CI must be checked against the exact pushed SHA after publication is authorized.

### Version installation checks

`TestRemoteMixedVersions` uses `HERDR_REMOTE_BINARY` with a pinned, SHA-256-verified Linux release asset, cached separately when the client platform differs. Herdr performs its real foreground SSH installation into the disposable container; the test does not replace the installer. This tests explicit-file transfer, not Herdr's latest-manifest download service. The override forces installation on each launch in the tested versions, so the test restarts herdrctx without it before checking ordinary reattachment and shell PID/output persistence.

The original PATH binary and remote config remain unchanged. The newly installed binary's version/hash and the running server's `/proc/<pid>/exe` path/hash are checked separately. Management through the original PATH must see and stop/delete the same session. A separate original-version session survives every case. For an already-running target, 0.9.0 explicitly replaces the 0.8.2 server after confirmation; 0.8.2 instead returns a restart-required error for the 0.9.0 server and preserves its shell until an explicit stop. This error is expected compatibility evidence, not a successful attach.

Upload failure/cancellation uses a test-only remote `tee` wrapper: it writes the first chunk, records a readiness marker, then fails or stalls while an older binary already occupies the installation destination. Both paths must preserve that binary's version/hash. Ctrl+C must return to the app's error dialog. `interrupted-install.txt` records temporary files and remote processes before teardown; neither killing SSH nor returning to the app proves remote cleanup. Container removal disposes of those remnants and is checked by the fixture. Private-key scanning remains active on all paths.

Run the two version suites, or the intentional cleanup failure, with:

```sh
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteMixedVersions$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteMachineVersions$'
make test-integration-remote INTEGRATION_ARGS='-run ^TestRemoteMixedVersions$/0.9.0-client-0.8.2-server/accept$ -integration.fail-after-attach'
```

The third command must fail with `Injected failure after mixed-version installation` and record successful container removal. To test SIGTERM cleanup, build the integration test executable, start only that acceptance scenario, and signal its exact process after `installed version ready for cancellation`. Preserve its result and cleanup files; do not signal unrelated processes. Version scenarios include Git HEAD/working-tree status, native client and original remote versions/hashes, installed/running binary evidence, and before/after session snapshots. A source build has its own binary hash even while the working tree is uncommitted.

These checks require network only when official fixed assets or Docker images are missing from the cache; Herdr may independently fetch optional agent-detection data. Ordinary `make test-integration` skips remote scenarios before accessing Docker. This evidence covers the executed platforms and fixed releases only. CI results require the exact pushed SHA; local success does not establish CI or other-platform success.

## Release-tag validation

The release workflow uses a Go test to compare its tag with the quoted top-level version in `herdr-plugin.toml`:

```sh
go test -tags=integration ./integration -run '^TestReleaseVersion$' -count=1 -integration.release-tag=v0.0.3
```

Use the intended tag, not the example value, for release work. Full manifest registration is validated by the plugin suite; this test enforces exact `v<manifest-version>` alignment. It does not publish anything.

## Native CI checks

The shared [ci.yml](../.github/workflows/ci.yml) runs for pull requests, pushes to `main`, and release-workflow calls.

| Platform | Runner | Go target |
| --- | --- | --- |
| Linux x86_64 | `ubuntu-24.04` | `linux/amd64` |
| Linux arm64 | `ubuntu-24.04-arm` | `linux/arm64` |
| macOS x86_64 | `macos-15-intel` | `darwin/amd64` |
| macOS arm64 | `macos-15` | `darwin/arm64` |

Each native job verifies the host/target architecture, runs package tests and vet, and invokes `CGO_ENABLED=0 make test-integration`. It also checks the built binary's version/help output; an unversioned build must report `herdrctx dev`. A separate Linux job runs formatting and lint, including the integration source. Another Linux x86_64 job runs the local Docker SSH suite. The platform jobs do not cancel each other on failure. Public installation is a separate explicit check, and tag validation runs before publishing.

Local results establish only what ran locally. Cross-compiling tests checks portability of the source, not runtime behavior on the other platforms. These checks do not establish compatibility with every Linux distribution, macOS version, terminal, or newer Herdr release.

## Results and failure diagnosis

Each scenario prints an artifact directory under `dist/integration`, including `navigation-*`, `hosts-navigation-*`, `remote-navigation-*`, `remote-<version>-*`, `lifecycle-*`, `plugin-*`, and `installer-*`. `result.json` records the test, Go/platform information, tested binary hash when available, attempted checks, and pass/fail status. PTY and CLI logs are saved during execution. Failures copy allowlisted regular diagnostic files from the isolated root into `snapshot` before removal, bounded to 16 MiB per file and 64 MiB total. Symlinks and files containing private-key markers are excluded. Lifecycle and local plugin scenarios additionally record cleanup outcomes.

CI uploads this directory on failure as `herdr-integration-<os>-<arch>` for seven days. Setup or compilation errors may precede artifact creation, so preserve the Go test output too. Before a snapshot build, save any evidence still needed: GoReleaser's `--clean` removes `dist`.

Inspect the exact [CI](https://github.com/j0urneyk/herdrctx/actions/workflows/ci.yml) or [release](https://github.com/j0urneyk/herdrctx/actions/workflows/release.yml) run when evaluating a commit/tag. Fix failures caused by the change and rerun affected checks. Report versions, platform, results, and anything that could not run. Do not infer cleanup success or remote CI success from a local pass.

### Historical evidence

The original documentation recorded the [initial native CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33596300473) at `bcd2033` and the [full lifecycle CI run](https://github.com/j0urneyk/herdrctx/actions/runs/33598213007) at `1263d87` as passing all four platforms. These preserve the provenance of earlier validation; they do not validate the current Go integration harness or later changes. Their artifacts may have expired.
